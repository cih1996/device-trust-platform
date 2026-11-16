package platform

import (
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

// Cross-app device linking. Each app holds its own Keystore key, so device IDs
// never coincide across apps. A link ID groups the profiles that the server
// believes run on one handset:
//   - android_id: Android 8+ scopes ANDROID_ID to the signing key and user, so
//     apps signed with the same certificate read the same value.
//   - boot_session: identical hardware build and the same boot instant (to
//     bootLinkTolerance), which only one running handset has.
// Both inputs are client observations; a link is evidence, not identity proof.

type DeviceLink struct {
	LinkID         string    `json:"linkId"`
	Method         string    `json:"method"`
	AppID          string    `json:"appId"`
	InstallationID string    `json:"installationId"`
	LinkedAt       time.Time `json:"linkedAt"`
}

const (
	bootLinkTolerance = 2 * time.Second
	bootLinkWindow    = 7 * 24 * time.Hour
)

// signerSignal compares the self-reported APK signers with the app allowlist.
func signerSignal(signals map[string]Signal, allowed []string) Signal {
	application, _ := signals["application"].Value.(map[string]any)
	signers, _ := application["signers"].([]any)
	if signals["application"].Status != "available" || len(signers) == 0 {
		return Signal{Status: "unavailable", Reason: "no signer reported"}
	}
	for _, raw := range signers {
		signer, _ := raw.(string)
		matched := false
		for _, expected := range allowed {
			matched = matched || strings.EqualFold(signer, expected)
		}
		if !matched {
			return Signal{Status: "available", Value: false}
		}
	}
	return Signal{Status: "available", Value: true}
}

// androidIDKey is the developer-scoped key: ANDROID_ID under the APK signers.
func androidIDKey(r Report) string {
	id := stringSignal(r, "androidId")
	application := signalMap(r, "application")
	signers, _ := application["signers"].([]any)
	if id == "" || len(signers) == 0 {
		return ""
	}
	names := []string{}
	for _, raw := range signers {
		if signer, ok := raw.(string); ok {
			names = append(names, strings.ToLower(signer))
		}
	}
	sort.Strings(names)
	return digest("android_id|" + strings.Join(names, ",") + "|" + id)
}

// bootSession returns the hardware build key and the boot instant of a report.
func bootSession(r Report) (string, time.Time, bool) {
	boot, _ := signalMap(r, "osInfo")["bootEpochMillis"].(float64)
	fingerprint := stringSignal(r, "buildFingerprint")
	java := signalMap(r, "resourceJava")
	memory, _ := java["memoryBytes"].(float64)
	storage, _ := signalMap(r, "storage")["totalBytes"].(float64)
	display := signalMap(r, "display")
	width, _ := display["widthPixels"].(float64)
	height, _ := display["heightPixels"].(float64)
	if boot <= 0 || fingerprint == "" || memory <= 0 || storage <= 0 || width <= 0 {
		return "", time.Time{}, false
	}
	hardware := fmt.Sprintf("%s|%.0f|%.0f|%.0fx%.0f", fingerprint, memory, storage, min(width, height), max(width, height))
	return hardware, time.UnixMilli(int64(boot)), true
}

// linkDevice returns the link ID for the profile of r, creating or joining one.
func (s *Server) linkDevice(r Report) (string, error) {
	profile := r.AppID + ":" + r.InstallationID
	var current DeviceLink
	err := s.Store.get("links", profile, &current)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	key := androidIDKey(r)
	if err == nil {
		return current.LinkID, s.indexLinkKey(key, current.LinkID)
	}
	current = DeviceLink{AppID: r.AppID, InstallationID: r.InstallationID, LinkedAt: time.Now().UTC()}
	if key != "" {
		var indexed DeviceLink
		if err = s.Store.get("link-keys", key, &indexed); err == nil {
			current.LinkID, current.Method = indexed.LinkID, "android_id"
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}
	if current.LinkID == "" {
		if current.LinkID, err = s.bootSessionLink(r); err != nil {
			return "", err
		}
		current.Method = "boot_session"
	}
	if current.LinkID == "" {
		current.LinkID, current.Method = token(), "new"
	}
	if err = s.Store.create("links", profile, current); errors.Is(err, os.ErrExist) {
		err = s.Store.get("links", profile, &current)
	}
	if err != nil {
		return "", err
	}
	return current.LinkID, s.indexLinkKey(key, current.LinkID)
}

func (s *Server) indexLinkKey(key, linkID string) error {
	if key == "" {
		return nil
	}
	err := s.Store.create("link-keys", key, DeviceLink{LinkID: linkID, LinkedAt: time.Now().UTC()})
	if errors.Is(err, os.ErrExist) {
		return nil // The first link that claimed this key keeps it.
	}
	return err
}

// bootSessionLink finds a recent linked report from another profile with the
// same hardware and boot instant.
func (s *Server) bootSessionLink(r Report) (string, error) {
	hardware, boot, ok := bootSession(r)
	if !ok {
		return "", nil
	}
	reports, err := records[Report](s.Store, "reports")
	if err != nil {
		return "", err
	}
	for _, other := range reports {
		if other.LinkID == "" || (other.AppID == r.AppID && other.InstallationID == r.InstallationID) || r.ReceivedAt.Sub(other.ReceivedAt) > bootLinkWindow {
			continue
		}
		h, b, ok := bootSession(other)
		if ok && h == hardware && b.Sub(boot).Abs() <= bootLinkTolerance {
			return other.LinkID, nil
		}
	}
	return "", nil
}

// deviceLinks maps profile keys (app:installation) to their link record.
func (s *Server) deviceLinks() (map[string]DeviceLink, error) {
	items, err := records[DeviceLink](s.Store, "links")
	if err != nil {
		return nil, err
	}
	result := map[string]DeviceLink{}
	for _, item := range items {
		result[item.AppID+":"+item.InstallationID] = item
	}
	return result, nil
}

// LinkedDevice is one other profile sharing a link ID.
type LinkedDevice struct {
	ID        string    `json:"id"`
	AppID     string    `json:"appId"`
	AppName   string    `json:"appName,omitempty"`
	DeviceID  string    `json:"deviceId,omitempty"`
	Method    string    `json:"method"`
	RiskScore int       `json:"riskScore"`
	RiskLevel string    `json:"riskLevel"`
	LastSeen  time.Time `json:"lastSeen"`
}

// linkedProfiles lists the profiles sharing linkID, except the one named self.
func linkedProfiles(all []Profile, links map[string]DeviceLink, apps map[string]string, linkID, self string) []LinkedDevice {
	result := []LinkedDevice{}
	if linkID == "" {
		return result
	}
	for _, p := range all {
		link, ok := links[p.AppID+":"+p.InstallationID]
		if !ok || link.LinkID != linkID || p.ID == self {
			continue
		}
		result = append(result, LinkedDevice{ID: p.ID, AppID: p.AppID, AppName: apps[p.AppID], DeviceID: p.DeviceID, Method: link.Method, RiskScore: p.RiskScore, RiskLevel: p.RiskLevel, LastSeen: p.LastSeen})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].LastSeen.After(result[j].LastSeen) })
	return result
}
