package platform

import (
	"fmt"
	"testing"
)

func TestConsoleAnalyticsEventsAndDeviceRisk(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	admin := "test-admin-token-of-at-least-24-chars"
	h := (&Server{Store: store, AdminToken: admin}).Handler(t.TempDir())
	a := request(t, h, "POST", "/api/apps", admin, map[string]string{"name": "one", "package": "com.example.one"}, 201)
	id, key := a["app"].(map[string]any)["id"].(string), a["key"].(string)
	request(t, h, "PUT", "/api/apps/"+id+"/security", admin, AppSecurity{MaxEnrollmentHours: 24}, 200)

	report := func(install string, emulator bool) string {
		signals := map[string]any{"emulatorHint": map[string]any{"status": "available", "value": emulator}, "debugger": map[string]any{"status": "available", "value": emulator}}
		return request(t, h, "POST", "/v1/reports", "", map[string]any{"appId": id, "installationId": install, "package": "com.example.one", "signals": signals}, 201)["reportId"].(string)
	}
	clean, risky := report("clean", false), report("risky", true)
	request(t, h, "POST", "/v1/events", key, map[string]any{"eventId": "c1", "appId": id, "reportId": clean, "accountId": "u0", "type": "login", "clientIp": "198.51.100.1"}, 201)
	for i := range 4 {
		request(t, h, "POST", "/v1/events", key, map[string]any{"eventId": fmt.Sprint("r", i), "appId": id, "reportId": risky, "accountId": fmt.Sprint("u", i+1), "type": "login", "clientIp": "198.51.100.2", "context": map[string]any{"pageDurationMs": 4000}}, 201)
	}

	devices := request(t, h, "GET", "/api/devices?appId="+id+"&sort=risk", admin, nil, 200)
	items := devices["items"].([]any)
	top := items[0].(map[string]any)
	if top["installationId"] != "risky" || top["envScore"].(float64) != 35 || top["riskScore"].(float64) < 70 || top["riskLevel"] != "high" {
		t.Fatalf("risky device not ranked first with env and business score: %v", top)
	}
	if len(top["factors"].([]any)) < 3 {
		t.Fatalf("expected environment and business factors: %v", top["factors"])
	}
	if low := request(t, h, "GET", "/api/devices?appId="+id+"&level=low", admin, nil, 200); low["total"].(float64) != 1 {
		t.Fatalf("level filter: %v", low)
	}
	request(t, h, "GET", "/api/devices?sort=nope", admin, nil, 400)

	detail := request(t, h, "GET", "/api/devices/"+top["id"].(string), admin, nil, 200)
	if len(detail["trend"].([]any)) != 30 || len(detail["events"].([]any)) != 4 || len(detail["ips"].([]any)) == 0 {
		t.Fatalf("device detail missing trend, events or ips: %v", detail)
	}
	var trendEvents float64
	for _, d := range detail["trend"].([]any) {
		trendEvents += d.(map[string]any)["events"].(float64)
	}
	if trendEvents != 4 {
		t.Fatalf("trend should hold today's 4 events, got %v", trendEvents)
	}

	stats := request(t, h, "GET", "/api/analytics?appId="+id+"&range=7d", admin, nil, 200)
	summary := stats["summary"].(map[string]any)
	if summary["events"].(float64) != 5 || summary["devices"].(float64) != 2 || summary["accounts"].(float64) != 5 || summary["dwellSamples"].(float64) != 4 {
		t.Fatalf("summary: %v", summary)
	}
	if len(stats["series"].([]any)) != 7 || stats["deviceLevels"].(map[string]any)["high"].(float64) != 1 {
		t.Fatalf("series or device levels: %v", stats)
	}
	for _, reason := range stats["reasons"].([]any) {
		if r := reason.(map[string]any); r["key"] == "emulator_hint" && r["count"].(float64) != 4 {
			t.Fatalf("rule counted more than once per event: %v", r)
		}
	}
	request(t, h, "GET", "/api/analytics?range=1y", admin, nil, 400)

	events := request(t, h, "GET", "/api/events?appId="+id+"&level=high&limit=2", admin, nil, 200)
	if events["total"].(float64) == 0 || len(events["items"].([]any)) > 2 || events["counts"].(map[string]any)["low"].(float64) != 1 {
		t.Fatalf("event paging: %v", events)
	}
	first := events["items"].([]any)[0].(map[string]any)
	if first["profileId"] != top["id"] || first["status"] != "observed" {
		t.Fatalf("event row: %v", first)
	}
	one := request(t, h, "GET", "/api/events/"+first["id"].(string), admin, nil, 200)
	if one["id"] != first["id"] {
		t.Fatalf("event detail: %v", one)
	}
	request(t, h, "GET", "/api/events/missing", admin, nil, 404)
	if byDevice := request(t, h, "GET", "/api/events?profileId="+top["id"].(string), admin, nil, 200); byDevice["total"].(float64) != 4 {
		t.Fatalf("device filter: %v", byDevice)
	}
}
