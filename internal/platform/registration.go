package platform

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// Stable device IDs. Every Keystore key gets a registration; a new key (after
// a reinstall or data wipe) is matched to a device the app already knows and
// inherits its device ID and installation, so profiles, links and business
// rules continue on the same device. Matching, strongest first:
//   - android_id: ANDROID_ID under the app's signers (survives reinstall)
//   - drm: digest of the Widevine device unique ID (survives reinstall on most devices)
//   - boot_session: same hardware build and boot instant as a recent report
// A match is refused when the hardware profile differs: two phones sharing an
// identifier means one of them copied it.

type DeviceRegistration struct {
	DeviceID  string    `json:"deviceId"`
	AppID     string    `json:"appId"`
	KeyID     string    `json:"keyId"`
	CreatedAt time.Time `json:"createdAt"`
	// Installation is the report installation ID shared by all keys of the
	// device; registrations made before stable IDs use "key:" + KeyID.
	Installation string `json:"installationId,omitempty"`
	Method       string `json:"method,omitempty"`
}

func (d DeviceRegistration) InstallationID() string {
	if d.Installation != "" {
		return d.Installation
	}
	return "key:" + d.KeyID
}

// deviceIdentity is the last identifier set seen for a device ID.
type deviceIdentity struct {
	DeviceID     string    `json:"deviceId"`
	Installation string    `json:"installationId"`
	AndroidID    string    `json:"androidId,omitempty"`
	DRM          string    `json:"drm,omitempty"`
	Hardware     string    `json:"hardware,omitempty"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// identityIssue is a risk finding from matching, scored with the report.
type identityIssue struct {
	Code, Detail string
	Points       int
}

const (
	reinstallWindow = 24 * time.Hour
	reinstallLimit  = 3
)

func drmKey(r Report) string {
	if id := stringSignal(r, "drmId"); id != "" {
		return digest("drm|" + strings.ToLower(id))
	}
	return ""
}

// hardwareProfile is what an identifier copy cannot bring along: model,
// data partition size and screen. OS updates leave it unchanged.
func hardwareProfile(r Report) string {
	model := stringSignal(r, "model")
	storage, _ := signalMap(r, "storage")["totalBytes"].(float64)
	display := signalMap(r, "display")
	width, _ := display["widthPixels"].(float64)
	height, _ := display["heightPixels"].(float64)
	if model == "" || storage <= 0 || width <= 0 {
		return ""
	}
	return fmt.Sprintf("%s|%.0f|%.0fx%.0f", model, storage, min(width, height), max(width, height))
}

func (s *Server) registration(appID, keyID string) (DeviceRegistration, error) {
	var current DeviceRegistration
	err := s.Store.get("registrations", appID+":"+keyID, &current)
	return current, err
}

// registerDevice returns the registration of the report's key, creating it
// on first use by matching the report to a known device of the app.
func (s *Server) registerDevice(appID, keyID string, r Report) (DeviceRegistration, []identityIssue, error) {
	current, err := s.registration(appID, keyID)
	if err == nil {
		// Keep the identifiers current, so devices registered before stable
		// IDs become matchable on their next report.
		return current, nil, s.rememberIdentity(appID, current, r)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return current, nil, err
	}
	match, method, issues, err := s.matchDevice(appID, r)
	if err != nil {
		return current, nil, err
	}
	current = DeviceRegistration{DeviceID: token(), AppID: appID, KeyID: keyID, CreatedAt: time.Now().UTC(), Installation: "key:" + keyID, Method: "new"}
	if match != nil {
		current.DeviceID, current.Installation, current.Method = match.DeviceID, match.Installation, method
	}
	if err = s.Store.create("registrations", appID+":"+keyID, current); errors.Is(err, os.ErrExist) {
		current, err = s.registration(appID, keyID)
		return current, nil, err
	}
	if err != nil {
		return current, nil, err
	}
	if match != nil {
		recent, err := s.recentKeys(appID, current.DeviceID)
		if err != nil {
			return current, nil, err
		}
		if recent >= reinstallLimit {
			issues = append(issues, identityIssue{"frequent_reinstall", fmt.Sprintf("24 小时内重新登记 %d 次", recent), 20})
		}
	}
	return current, issues, s.rememberIdentity(appID, current, r)
}

func (s *Server) matchDevice(appID string, r Report) (*deviceIdentity, string, []identityIssue, error) {
	hardware, aid, drm := hardwareProfile(r), androidIDKey(r), drmKey(r)
	var issues []identityIssue
	for _, c := range []struct{ method, key string }{{"android_id", aid}, {"drm", drm}} {
		if c.key == "" {
			continue
		}
		var indexed deviceIdentity
		err := s.Store.get("identity-keys", appID+"|"+c.method+"|"+c.key, &indexed)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, "", nil, err
		}
		var known deviceIdentity
		if err = s.Store.get("device-identities", appID+":"+indexed.DeviceID, &known); err != nil {
			return nil, "", nil, err
		}
		if known.Hardware != "" && hardware != "" && known.Hardware != hardware {
			issues = append(issues, identityIssue{"device_identity_clone", "设备标识与另一台硬件不同的设备相同（" + c.method + "）", 30})
			continue
		}
		// One identifier matched; another one present on both sides changed.
		if aid != "" && known.AndroidID != "" && aid != known.AndroidID {
			issues = append(issues, identityIssue{"device_identifier_changed", "同一设备的 ANDROID_ID 已改变", 20})
		}
		if drm != "" && known.DRM != "" && drm != known.DRM {
			issues = append(issues, identityIssue{"device_identifier_changed", "同一设备的 DRM 设备 ID 已改变", 20})
		}
		return &known, c.method, issues, nil
	}
	if len(issues) > 0 {
		return nil, "", issues, nil
	}
	found, err := s.bootSessionDevice(appID, r)
	if err != nil || found == nil {
		return nil, "", issues, err
	}
	var known deviceIdentity
	if err = s.Store.get("device-identities", appID+":"+found.DeviceID, &known); err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, "", nil, err
		}
		// A device registered before stable IDs: its own key installation.
		reg, err := s.registration(appID, found.Proof.KeyID)
		if err != nil {
			return nil, "", nil, err
		}
		known = deviceIdentity{DeviceID: reg.DeviceID, Installation: reg.InstallationID()}
	}
	return &known, "boot_session", issues, nil
}

// bootSessionDevice is a recent signed report of the app from a device with
// the same hardware build and boot instant.
func (s *Server) bootSessionDevice(appID string, r Report) (*Report, error) {
	hardware, boot, ok := bootSession(r)
	if !ok {
		return nil, nil
	}
	reports, err := records[Report](s.Store, "reports")
	if err != nil {
		return nil, err
	}
	for i := range reports {
		other := &reports[i]
		if other.AppID != appID || other.DeviceID == "" || other.Proof == nil || time.Since(other.ReceivedAt) > bootLinkWindow {
			continue
		}
		if h, b, ok := bootSession(*other); ok && h == hardware && b.Sub(boot).Abs() <= bootLinkTolerance {
			return other, nil
		}
	}
	return nil, nil
}

func (s *Server) rememberIdentity(appID string, reg DeviceRegistration, r Report) error {
	// A report that lacks an identifier (host turned it off) keeps the known value.
	var identity deviceIdentity
	if err := s.Store.get("device-identities", appID+":"+reg.DeviceID, &identity); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	identity.DeviceID, identity.Installation, identity.UpdatedAt = reg.DeviceID, reg.InstallationID(), time.Now().UTC()
	for _, f := range []struct {
		field *string
		value string
	}{{&identity.AndroidID, androidIDKey(r)}, {&identity.DRM, drmKey(r)}, {&identity.Hardware, hardwareProfile(r)}} {
		if f.value != "" {
			*f.field = f.value
		}
	}
	if err := s.Store.put("device-identities", appID+":"+reg.DeviceID, identity); err != nil {
		return err
	}
	for method, key := range map[string]string{"android_id": androidIDKey(r), "drm": drmKey(r)} {
		if key == "" {
			continue
		}
		// The first device that claimed an identifier keeps it.
		if err := s.Store.create("identity-keys", appID+"|"+method+"|"+key, deviceIdentity{DeviceID: reg.DeviceID}); err != nil && !errors.Is(err, os.ErrExist) {
			return err
		}
	}
	return nil
}

// recentKeys counts keys registered to the device in the reinstall window.
func (s *Server) recentKeys(appID, deviceID string) (int, error) {
	regs, err := records[DeviceRegistration](s.Store, "registrations")
	if err != nil {
		return 0, err
	}
	n := 0
	for _, reg := range regs {
		if reg.AppID == appID && reg.DeviceID == deviceID && time.Since(reg.CreatedAt) <= reinstallWindow {
			n++
		}
	}
	return n, nil
}

// deviceKeys lists the keys registered to a device, oldest first.
func (s *Server) deviceKeys(appID, deviceID string) ([]DeviceRegistration, error) {
	regs, err := records[DeviceRegistration](s.Store, "registrations")
	if err != nil {
		return nil, err
	}
	out := []DeviceRegistration{}
	for _, reg := range regs {
		if reg.AppID == appID && reg.DeviceID == deviceID {
			out = append(out, reg)
		}
	}
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].CreatedAt.Before(out[j-1].CreatedAt); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out, nil
}
