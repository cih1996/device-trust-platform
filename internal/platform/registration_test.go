package platform

import (
	"encoding/hex"
	"testing"
	"time"
)

func identityReport(aid, drm string, storage float64) Report {
	signals := map[string]Signal{
		"model":       {Status: "available", Value: "PDEM10"},
		"storage":     {Status: "available", Value: map[string]any{"totalBytes": storage}},
		"display":     {Status: "available", Value: map[string]any{"widthPixels": 1080.0, "heightPixels": 2400.0}},
		"application": {Status: "available", Value: map[string]any{"signers": []any{"aa"}}},
	}
	if aid != "" {
		signals["androidId"] = Signal{Status: "available", Value: aid}
	}
	if drm != "" {
		signals["drmId"] = Signal{Status: "available", Value: drm}
	}
	return Report{Signals: signals}
}

func TestStableDeviceID(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Store: store}
	codes := func(issues []identityIssue) map[string]bool {
		out := map[string]bool{}
		for _, i := range issues {
			out[i.Code] = true
		}
		return out
	}
	first, _, err := s.registerDevice("a", "k1", identityReport("aid-1", "drm-1", 128e9))
	if err != nil || first.Method != "new" || first.InstallationID() != "key:k1" {
		t.Fatal(first, err)
	}
	// Reinstall: new key, same ANDROID_ID.
	second, _, _ := s.registerDevice("a", "k2", identityReport("aid-1", "drm-1", 128e9))
	if second.DeviceID != first.DeviceID || second.InstallationID() != "key:k1" || second.Method != "android_id" {
		t.Fatal(second)
	}
	// Host turned ANDROID_ID off; Widevine still matches.
	third, issues, _ := s.registerDevice("a", "k3", identityReport("", "drm-1", 128e9))
	if third.DeviceID != first.DeviceID || third.Method != "drm" || !codes(issues)["frequent_reinstall"] {
		t.Fatal(third, issues)
	}
	// An ID-reset tool changed ANDROID_ID; Widevine keeps the device and flags it.
	reset, issues, _ := s.registerDevice("a", "k4", identityReport("aid-new", "drm-1", 128e9))
	if reset.DeviceID != first.DeviceID || !codes(issues)["device_identifier_changed"] {
		t.Fatal(reset, issues)
	}
	// Another phone (different storage) carrying a copied ANDROID_ID.
	clone, issues, _ := s.registerDevice("a", "k5", identityReport("aid-1", "", 256e9))
	if clone.DeviceID == first.DeviceID || clone.Method != "new" || !codes(issues)["device_identity_clone"] {
		t.Fatal(clone, issues)
	}
	// Same identifiers in another app: device IDs stay per app.
	other, _, _ := s.registerDevice("b", "k6", identityReport("aid-1", "drm-1", 128e9))
	if other.DeviceID == first.DeviceID || other.Method != "new" {
		t.Fatal(other)
	}
	keys, err := s.deviceKeys("a", first.DeviceID)
	if err != nil || len(keys) != 4 || keys[0].KeyID != "k1" {
		t.Fatal(keys, err)
	}
	// Repeating a registered key returns it unchanged.
	again, issues, _ := s.registerDevice("a", "k2", identityReport("aid-1", "drm-1", 128e9))
	if again != second || len(issues) != 0 {
		t.Fatal(again, issues)
	}
}

func TestReinstallKeepsProfile(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	signer := hex.EncodeToString(randomBytes(32))
	store.put("apps", "a", App{ID: "a", Name: "A", Package: "com.test.a", KeyHash: digest("secret-a")})
	store.put("security", "a", AppSecurity{RequireSignedReports: true, RequireRequestBinding: true, MaxEnrollmentHours: 24, SigningDigests: []string{signer}})
	server := &Server{Store: store, AdminToken: "admin"}
	x := linkHarness{t: t, h: server.Handler(t.TempDir())}
	boot := time.Now().Add(-time.Hour).UnixMilli()
	one := x.upload("a", "aid-1", boot, []string{signer})
	two := x.upload("a", "aid-1", boot, []string{signer}) // new key, as after reinstall
	if one["deviceId"] != two["deviceId"] || one["keyId"] == two["keyId"] {
		t.Fatal(one, two)
	}
	reports, events, _ := server.deviceData(nil)
	if p := profiles(reports, events, "a"); len(p) != 1 || p[0].ReportCount != 2 {
		t.Fatalf("%d profiles", len(p))
	}
}
