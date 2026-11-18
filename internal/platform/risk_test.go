package platform

import (
	"testing"
	"time"
)

func hit(e Event, code string) bool {
	for _, h := range e.Hits {
		if h.Code == code {
			return true
		}
	}
	return false
}
func TestRegistrationDeviceClusterAndSharedNetwork(t *testing.T) {
	now := time.Now().UTC()
	current := Event{AppID: "one", AccountID: "c", InstallationID: "device", Type: "register", ReceivedAt: now, ClientIP: "203.0.113.1"}
	history := []Event{{AppID: "one", AccountID: "a", InstallationID: "device", Type: "register", ReceivedAt: now.Add(-time.Minute)}, {AppID: "one", AccountID: "b", InstallationID: "device", Type: "register", ReceivedAt: now.Add(-2 * time.Minute)}}
	evaluateBusiness(&current, Report{}, history, nil, DefaultPolicy())
	if current.Action != "verify" || !hit(current, "registration_device_cluster") || current.Hits[0].Observed != 3 {
		t.Fatal(current)
	}
	// Distinct users on a shared NAT are not sufficient for a verification recommendation.
	history = nil
	for i := 0; i < 7; i++ {
		history = append(history, Event{AppID: "one", AccountID: string(rune('d' + i)), InstallationID: string(rune('x' + i)), Type: "register", ClientIP: current.ClientIP, ReceivedAt: now.Add(-time.Minute)})
	}
	evaluateBusiness(&current, Report{}, history, nil, DefaultPolicy())
	if current.Score != 15 || current.Action != "observe" || !hit(current, "registration_ip_cluster") {
		t.Fatal(current)
	}
	for i := range history {
		history[i].AppID = "other"
	}
	evaluateBusiness(&current, Report{}, history, nil, DefaultPolicy())
	if current.Score != 0 {
		t.Fatal(current)
	}
}
func TestCampaignAssociationsAndBoundaries(t *testing.T) {
	now := time.Now().UTC()
	current := Event{AppID: "one", AccountID: "c", InstallationID: "device", Type: "reward", ReceivedAt: now, Context: BusinessContext{CampaignID: "summer", RecipientID: "recipient"}}
	history := []Event{}
	for _, account := range []string{"a", "b"} {
		history = append(history, Event{AppID: "one", AccountID: account, InstallationID: "device", Type: "coupon", ReceivedAt: now.Add(-time.Hour), Context: current.Context})
	}
	evaluateBusiness(&current, Report{}, history, nil, DefaultPolicy())
	if current.Action != "hold" || current.Score != 85 || current.Mode != "shadow" {
		t.Fatal(current)
	}
	for i := range history {
		history[i].Context.CampaignID = "another"
	}
	evaluateBusiness(&current, Report{}, history, nil, DefaultPolicy())
	if hit(current, "campaign_device_accounts") || hit(current, "campaign_shared_recipient") {
		t.Fatal(current)
	}
	current.Type = "register"
	current.Context = BusinessContext{}
	for i := range history {
		history[i].Type = "register"
		history[i].ReceivedAt = now.Add(-10 * time.Minute)
	}
	evaluateBusiness(&current, Report{}, history, nil, DefaultPolicy())
	if hit(current, "registration_device_cluster") {
		t.Fatal(current)
	}
}
func TestPaymentFailuresUseLatestProviderTimeAndPolicy(t *testing.T) {
	now := time.Now().UTC()
	current := Event{AppID: "one", AccountID: "a", InstallationID: "device", Type: "payment", ReceivedAt: now, Context: BusinessContext{PaymentMethodID: "method-c", OrderID: "order-new"}}
	history := []Event{}
	outcomes := []Outcome{}
	for i := 0; i < 4; i++ {
		id := string(rune('a' + i))
		method := "method-a"
		if i == 1 {
			method = "method-b"
		}
		history = append(history, Event{AppID: "one", BusinessID: id, AccountID: "a", InstallationID: "device", Type: "payment", ReceivedAt: now.Add(-5 * time.Minute), Context: BusinessContext{PaymentMethodID: method}})
		if i < 3 {
			outcomes = append(outcomes, Outcome{AppID: "one", EventID: id, Status: "failed", OccurredAt: now.Add(-4 * time.Minute), ReceivedAt: now.Add(-3 * time.Minute)})
		}
	}
	evaluateBusiness(&current, Report{}, history, outcomes, DefaultPolicy())
	if current.Action != "hold" || !hit(current, "payment_failure_history") || current.Score != 85 {
		t.Fatal(current)
	}
	// A success happened after a decline, despite arriving before a delayed decline webhook.
	outcomes = append(outcomes, Outcome{AppID: "one", EventID: "a", Status: "succeeded", OccurredAt: now.Add(-2 * time.Minute), ReceivedAt: now.Add(-2 * time.Minute)})
	outcomes = append(outcomes, Outcome{AppID: "one", EventID: "a", Status: "failed", OccurredAt: now.Add(-3 * time.Minute), ReceivedAt: now.Add(-time.Minute)})
	evaluateBusiness(&current, Report{}, history, outcomes, DefaultPolicy())
	if hit(current, "payment_failure_history") || current.Action != "verify" || current.Score != 50 {
		t.Fatal(current)
	}
	p := DefaultPolicy()
	p.PaymentMethods = 10
	p.ReviewScore = 60
	p.HoldScore = 90
	p.Version = "custom"
	evaluateBusiness(&current, Report{}, history, outcomes, p)
	if current.Score != 25 || current.Action != "observe" || current.PolicySnapshot.Version != "custom" {
		t.Fatal(current)
	}
}
func TestMissingPaymentContextAndFailedClientCollector(t *testing.T) {
	current := Event{AppID: "one", AccountID: "a", InstallationID: "device", Type: "payment", ReceivedAt: time.Now().UTC()}
	report := Report{Signals: map[string]Signal{"rootBeerSuBinary": {Status: "error", Value: true}, "rootBeerNativeSu": {Status: "error", Value: true}}}
	evaluateBusiness(&current, report, nil, nil, DefaultPolicy())
	if current.Score != 0 || len(current.MissingFields) != 3 || hit(current, "root_indicators") {
		t.Fatal(current)
	}
}

func TestOverlappingDeviceRulesDoNotCountAsIndependentEvidence(t *testing.T) {
	now := time.Now().UTC()
	current := Event{AppID: "one", AccountID: "d", InstallationID: "device", Type: "reward", ReceivedAt: now, Context: BusinessContext{CampaignID: "activity"}}
	history := []Event{}
	for _, account := range []string{"a", "b", "c"} {
		history = append(history, Event{AppID: "one", AccountID: account, InstallationID: "device", Type: "reward", ReceivedAt: now.Add(-time.Minute), Context: current.Context})
	}
	evaluateBusiness(&current, Report{}, history, nil, DefaultPolicy())
	if current.Score != 80 || current.Action != "verify" {
		t.Fatal(current)
	}
}

func TestChargebackHistoryIsScopedAndNotAConfirmedFraudLabel(t *testing.T) {
	now := time.Now().UTC()
	current := Event{AppID: "one", AccountID: "a", InstallationID: "device", Type: "payment", ReceivedAt: now}
	history := []Event{{AppID: "one", BusinessID: "prior", AccountID: "a", Type: "payment", ReceivedAt: now.Add(-48 * time.Hour)}}
	outcomes := []Outcome{{AppID: "one", EventID: "prior", Status: "chargeback", OccurredAt: now.Add(-time.Hour)}}
	evaluateBusiness(&current, Report{}, history, outcomes, DefaultPolicy())
	if current.Score != 30 || current.Action != "verify" || !hit(current, "payment_chargeback_history") {
		t.Fatal(current)
	}
	outcomes[0].AppID = "other"
	evaluateBusiness(&current, Report{}, history, outcomes, DefaultPolicy())
	if current.Score != 0 {
		t.Fatal(current)
	}
}

func TestPaymentToolAssociationAcrossDifferentDevices(t *testing.T) {
	now := time.Now().UTC()
	current := Event{AppID: "one", AccountID: "c", InstallationID: "device-c", Type: "payment", ReceivedAt: now, Context: BusinessContext{PaymentMethodID: "shared-token"}}
	history := []Event{{AppID: "one", AccountID: "a", InstallationID: "device-a", Type: "payment", Context: current.Context, ReceivedAt: now.Add(-time.Hour)}, {AppID: "one", AccountID: "b", InstallationID: "device-b", Type: "payment", Context: current.Context, ReceivedAt: now.Add(-time.Hour)}}
	evaluateBusiness(&current, Report{}, history, nil, DefaultPolicy())
	if current.Score != 40 || current.Action != "verify" || !hit(current, "payment_shared_method") {
		t.Fatal(current)
	}
}

func TestRuntimeIntegritySignals(t *testing.T) {
	r := Report{Signals: map[string]Signal{
		"integrityNative": {Status: "available", Value: map[string]any{"tracerPid": float64(4121), "hooks": []any{"frida"}, "hookThreads": []any{"gum-js-loop"}, "emulator": []any{"ro.kernel.qemu"}}},
		"resourceNative":  {Status: "available", Value: map[string]any{"machine": "x86_64"}},
		"sensors":         {Status: "available", Value: []any{map[string]any{"type": float64(5)}}},
		"appDebuggable":   {Status: "available", Value: true},
		"httpProxy":       {Status: "available", Value: map[string]any{"host": "192.168.1.9", "port": float64(8888)}},
		"appSandbox":      {Status: "available", Value: map[string]any{"userId": float64(0), "ownDataDir": false}},
		"adbEnabled":      {Status: "available", Value: true},
	}}
	got := map[string]int{}
	runtimeIntegrity(r, func(points int, code string) { got[code] = points })
	want := map[string]int{"native_debugger": 25, "hook_framework": 40, "emulator_traits": 35, "x86_runtime": 15, "missing_motion_sensor": 15, "app_debuggable": 30, "http_proxy": 10, "virtual_container": 30, "adb_enabled": 5}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for code, points := range want {
		if got[code] != points {
			t.Fatalf("%s: got %d want %d (%v)", code, got[code], points, got)
		}
	}

	clean := Report{Signals: map[string]Signal{
		"integrityNative": {Status: "available", Value: map[string]any{"tracerPid": float64(0), "hooks": []any{}, "hookThreads": []any{}, "emulator": []any{}}},
		"sensors":         {Status: "available", Value: []any{map[string]any{"type": float64(1)}}},
		"httpProxy":       {Status: "available", Value: false},
		"appSandbox":      {Status: "available", Value: map[string]any{"userId": float64(999), "ownDataDir": true}},
		// Unreadable signals are not evidence either way.
		"appDebuggable": {Status: "error", Reason: "x"},
	}}
	got = map[string]int{}
	runtimeIntegrity(clean, func(points int, code string) { got[code] = points })
	if len(got) != 1 || got["secondary_user"] != 5 {
		t.Fatalf("clean device should only note the secondary user: %v", got)
	}
}

func TestEnvironmentConsistency(t *testing.T) {
	av := func(v any) Signal { return Signal{Status: "available", Value: v} }
	spoofed := Report{Signals: map[string]Signal{
		"integrityNative": av(map[string]any{
			"bootState":      map[string]any{"ro.boot.verifiedbootstate": "green", "ro.vendor.boot.verifiedbootstate": "orange"},
			"rootProperties": []any{"persist.zygisk.enabled"},
			"rootMounts":     []any{"/data/adb"},
			"selinuxContext": "u:r:magisk:s0",
			"interfaces":     []any{map[string]any{"name": "eth0", "family": "ipv4", "scope": "private"}, map[string]any{"name": "rmnet_data0", "family": "ipv6", "scope": "linklocal"}},
		}),
		"osInfo":    av(map[string]any{"display": "AP3A dev-keys", "tags": "release-keys"}),
		"transport": av("cellular"),
	}}
	got := map[string]int{}
	environmentConsistency(spoofed, func(points int, code string) { got[code] = points })
	want := map[string]int{"boot_state_spoofed": 40, "root_properties": 35, "root_mounts": 40, "selinux_domain": 30, "build_tags_mismatch": 25, "network_interface_mismatch": 30}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for code, points := range want {
		if got[code] != points {
			t.Fatalf("%s: got %d want %d", code, got[code], points)
		}
	}

	clean := Report{Signals: map[string]Signal{
		"integrityNative": av(map[string]any{
			"bootState":      map[string]any{"ro.boot.verifiedbootstate": "green", "ro.boot.flash.locked": "1"},
			"selinuxContext": "u:r:untrusted_app_32:s0:c512,c768",
			"interfaces":     []any{map[string]any{"name": "rmnet_data0", "family": "ipv4", "scope": "cgnat"}},
		}),
		"osInfo":          av(map[string]any{"display": "AP3A release-keys", "tags": "release-keys"}),
		"transport":       av("cellular"),
		"activeInterface": av("rmnet_data0"),
	}}
	environmentConsistency(clean, func(points int, code string) { t.Fatalf("clean device hit %s", code) })

	unlocked := Report{Signals: map[string]Signal{"integrityNative": av(map[string]any{"bootState": map[string]any{"ro.boot.verifiedbootstate": "orange"}})}}
	got = map[string]int{}
	environmentConsistency(unlocked, func(points int, code string) { got[code] = points })
	if len(got) != 1 || got["bootloader_unlocked"] != 20 {
		t.Fatal(got)
	}
}

func TestCarrierChecks(t *testing.T) {
	av := func(v any) Signal { return Signal{Status: "available", Value: v} }
	run := func(sim, network, ipISP string, roaming bool) map[string]int {
		got := map[string]int{}
		r := Report{Signals: map[string]Signal{"simOperator": av(sim), "operator": av(network), "networkRoaming": av(roaming), "transport": av("cellular")}}
		carrierChecks(r, ipISP, "中国", func(code, state, _, _, _ string, points int) { got[code+"/"+state] = points })
		return got
	}
	if got := run("46003", "46000", "电信", false); got["sim_network_mismatch/conflict"] != 25 || got["cellular_ip_carrier/matched"] != 0 {
		t.Fatal(got)
	}
	// Telecom and Unicom share their radio network.
	if got := run("46011", "46001", "电信", false); got["sim_network/matched"] != 0 || len(got) != 2 {
		t.Fatal(got)
	}
	if got := run("46000", "46000", "联通", false); got["cellular_ip_carrier_mismatch/conflict"] != 20 {
		t.Fatal(got)
	}
	if got := run("46003", "46000", "电信", true); got["sim_network/unknown"] != 0 || len(got) != 2 {
		t.Fatal(got)
	}
	// A US eSIM used in China reports no roaming; that is not a contradiction.
	if got := run("310410", "46000", "移动", false); got["sim_network/unknown"] != 0 || len(got) != 2 {
		t.Fatal(got)
	}
}

func TestLegacyKSUMountMarkerIgnored(t *testing.T) {
	native := map[string]any{"rootMounts": []any{"ksu"}}
	if got := rootMounts(native); len(got) != 0 {
		t.Fatal(got)
	}
	native["rootMounts"] = []any{"ksu", "overlay KSU /system/bin"}
	if got := rootMounts(native); len(got) != 1 {
		t.Fatal(got)
	}
}

func TestRuleLabels(t *testing.T) {
	hits := labelHits([]RuleHit{{Code: "debugger"}, {Code: "good_standing"}, {Code: "unknown_code"}})
	if hits[0].Label != "正在调试" || hits[1].Label != "良好使用恢复" || hits[2].Label != "" {
		t.Fatal(hits)
	}
}
