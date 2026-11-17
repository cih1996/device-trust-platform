package platform

import (
	"testing"
	"time"
)

func available(v any) Signal { return Signal{Status: "available", Value: v} }

func hitCodes(r Report) map[string]int {
	codes := map[string]int{}
	toolingSignals(r, func(points int, code string) { codes[code] = points })
	return codes
}

func TestToolingSignals(t *testing.T) {
	clean := Report{Signals: map[string]Signal{
		"riskApps":      available(map[string]any{"installed": []any{}}),
		"accessibility": available(map[string]any{"enabled": true, "services": []any{map[string]any{"package": "com.google.android.marvin.talkback", "system": true, "gestures": true}}, "configured": []any{"com.google.android.marvin.talkback"}}),
		"automation":    available(map[string]any{"monkey": false, "touches": map[string]any{"downs": 5.0, "synthetic": 0.0}}),
		"javaHooks":     available(map[string]any{"classes": []any{}, "frames": []any{}}),
		"integrityNative": available(map[string]any{"injectedCode": []any{}, "ports": map[string]any{
			"loopback": []any{5037.0}, "frida": []any{}, "exposed": []any{}}}),
	}}
	if codes := hitCodes(clean); len(codes) != 0 {
		t.Fatal(codes)
	}
	risky := Report{Signals: map[string]Signal{
		"riskApps":      available(map[string]any{"installed": []any{"org.lsposed.manager", "com.topjohnwu.magisk", "me.weishu.kernelsu", "org.autojs.autojs"}}),
		"accessibility": available(map[string]any{"enabled": true, "services": []any{}, "configured": []any{"org.autojs.autojs"}}),
		"automation":    available(map[string]any{"monkey": true, "touches": map[string]any{"downs": 1.0, "synthetic": 1.0}}),
		"javaHooks":     available(map[string]any{"classes": []any{"de.robv.android.xposed.XposedBridge"}, "frames": []any{}}),
		"integrityNative": available(map[string]any{"injectedCode": []any{"memfd_code"}, "ports": map[string]any{
			"loopback": []any{27042.0, 23946.0, 8080.0}, "frida": []any{}, "exposed": []any{8080.0}}}),
	}}
	want := map[string]int{"risk_app_hook": 35, "risk_app_root": 30, "risk_app_automation": 25, "accessibility_automation": 20, "monkey_test": 30,
		"injected_touches": 30, "java_hook_framework": 40, "injected_code": 35, "frida_default_port": 30, "debug_server_port": 30, "exposed_listening_ports": 15}
	codes := hitCodes(risky)
	for code, points := range want {
		if codes[code] != points {
			t.Fatalf("%s: got %d in %v", code, codes[code], codes)
		}
	}
	if len(codes) != len(want) {
		t.Fatal(codes)
	}
	risky.Signals["integrityNative"] = available(map[string]any{"ports": map[string]any{"loopback": []any{31337.0}, "frida": []any{31337.0}}})
	risky.Signals["accessibility"] = available(map[string]any{"enabled": true, "services": []any{}, "configured": []any{}})
	if codes := hitCodes(risky); codes["frida_server"] != 50 || codes["frida_default_port"] != 0 || codes["hidden_accessibility_client"] != 25 {
		t.Fatal(codes)
	}
}

type checkLog map[string]int

func (c checkLog) add(code, state, dimension, detail, source string, points int) { c[code] = points }

func TestVendorPackages(t *testing.T) {
	packages := func(names ...string) Report {
		list := []any{}
		for _, n := range names {
			list = append(list, map[string]any{"package": n, "status": "visible", "system": true})
		}
		return Report{Signals: map[string]Signal{"systemPackages": available(list)}}
	}
	log := checkLog{}
	vendorPackageChecks(packages("com.miui.home", "com.xiaomi.market"), hardwareObservation{Brand: "Redmi"}, true, log.add)
	if log["vendor_packages"] != 0 || len(log) != 1 {
		t.Fatal(log)
	}
	log = checkLog{}
	vendorPackageChecks(packages("com.miui.home"), hardwareObservation{Brand: "OPPO"}, true, log.add)
	if log["vendor_packages_missing"] != 25 || log["foreign_vendor_packages"] != 25 {
		t.Fatal(log)
	}
	log = checkLog{}
	vendorPackageChecks(packages(), hardwareObservation{Brand: "OPPO"}, false, log.add)
	vendorPackageChecks(packages(), hardwareObservation{Brand: "google"}, true, log.add)
	if len(log) != 0 {
		t.Fatal(log)
	}
}

func TestEgressAndCellChecks(t *testing.T) {
	r := Report{Signals: map[string]Signal{"simOperator": available("46001"), "operator": available("46001")}}
	log := checkLog{}
	egressChecks(r, &IPRegion{Country: "中国", Province: "广东省", ISP: "阿里", CountryCode: "CN"}, log.add)
	egressChecks(r, &IPRegion{Country: "中国", Province: "香港特别行政区", CountryCode: "CN"}, log.add)
	if log["datacenter_egress"] != 30 || log["ip_outside_sim_country"] != 20 {
		t.Fatal(log)
	}
	age := 1000.0
	log = checkLog{}
	cellChecks(r, []cellObservation{{MCC: "460", MNC: "11", Registered: true, Age: &age}}, log.add)
	if log["cell_operator"] != 0 || len(log) != 1 {
		t.Fatal(log) // Telecom cell serving a Unicom SIM: shared network.
	}
	cellChecks(r, []cellObservation{{MCC: "460", MNC: "00", Registered: true, Age: &age}}, log.add)
	if log["cell_operator_mismatch"] != 25 {
		t.Fatal(log)
	}
}

func TestLocationJump(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Store: store}
	fix := func(id string, lat, lon float64, at time.Time) Report {
		age, accuracy := 1000.0, 20.0
		return Report{ID: id, AppID: "a", DeviceID: "d", ReceivedAt: at, Resources: &ResourceAssessment{},
			Signals: map[string]Signal{"location": available(map[string]any{"latitude": lat, "longitude": lon, "ageMillis": age, "accuracyMeters": accuracy, "coordinateSystem": "WGS84"})}}
	}
	now := time.Now()
	store.put("reports", "old", fix("old", 39.9, 116.4, now.Add(-time.Hour))) // Beijing
	shanghai := fix("new", 31.2, 121.5, now)
	if err := s.locationJump(&shanghai); err != nil || len(shanghai.Resources.Checks) != 1 || shanghai.Resources.Checks[0].Code != "location_jump" {
		t.Fatal(err, shanghai.Resources.Checks)
	}
	later := fix("later", 31.2, 121.5, now.Add(4*time.Hour)) // 1070 km in 5 hours
	if err := s.locationJump(&later); err != nil || len(later.Resources.Checks) != 0 {
		t.Fatal(err, later.Resources.Checks)
	}
}
