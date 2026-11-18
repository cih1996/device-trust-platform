package platform

import (
	"testing"
	"time"
)

func TestInsights(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	admin := "test-admin-token-of-at-least-24-chars"
	h := (&Server{Store: store, AdminToken: admin}).Handler(t.TempDir())
	app := request(t, h, "POST", "/api/apps", admin, map[string]string{"name": "one", "package": "com.example.one"}, 201)["app"].(map[string]any)["id"].(string)
	av := func(v any) Signal { return Signal{Status: "available", Value: v} }
	now := time.Now().UTC()
	for i, r := range []Report{
		{ID: "r1", InstallationID: "a", LinkID: "L1", Signals: map[string]Signal{
			"manufacturer": av("Xiaomi"), "transport": av("wifi"), "simOperator": av("46001"), "httpProxy": av(true),
			"battery": av(map[string]any{"level": 100.0, "scale": 100.0, "plugged": 2.0}), "osInfo": av(map[string]any{"release": "14", "uptimeMillis": 10 * 86400e3}),
			"memory": av(map[string]any{"totalBytes": 7.6 * (1 << 30)})}},
		// The same handset after a reinstall.
		{ID: "r2", InstallationID: "b", LinkID: "L1", Signals: map[string]Signal{"manufacturer": av("Xiaomi"), "vpn": av(true), "simOperator": av("")}},
		{ID: "r3", InstallationID: "c", ReceivedAt: now.Add(-40 * 24 * time.Hour)},
	} {
		r.AppID = app
		if r.ReceivedAt.IsZero() {
			r.ReceivedAt = now.Add(-time.Duration(i) * time.Minute)
		}
		if err = store.put("reports", r.ID, r); err != nil {
			t.Fatal(err)
		}
	}
	out := request(t, h, "GET", "/api/insights?range=7d&appId="+app, admin, nil, 200)
	rows := out["rows"].([]any)
	if len(rows) != 2 {
		t.Fatalf("stale device counted: %d rows", len(rows))
	}
	// Rows are ordered by risk; put r1's device first.
	if rows[0].(map[string]any)["id"] != digest(app+":a") {
		rows[0], rows[1] = rows[1], rows[0]
	}
	dims := map[string]int{}
	for i, d := range out["dims"].([]any) {
		dims[d.(map[string]any)["key"].(string)] = i
	}
	value := func(row int, dim string) string {
		d := out["dims"].([]any)[dims[dim]].(map[string]any)
		return d["values"].([]any)[int(rows[row].(map[string]any)["d"].([]any)[dims[dim]].(float64))].(string)
	}
	want := map[string][2]string{
		"brand": {"xiaomi", "xiaomi"}, "network": {"WiFi", "VPN"}, "sim": {"中国联通", "无 SIM 卡"}, "proxy": {"设置了 HTTP 代理", "未知"},
		"battery": {"充电中 · 满电", "未知"}, "uptime": {"7 ~ 30 天", "未知"}, "memory": {"8 GB", "未知"}, "android": {"Android 14", "未知"},
	}
	for dim, w := range want {
		for i := range 2 {
			if got := value(i, dim); got != w[i] {
				t.Errorf("%s row %d: %q want %q", dim, i, got, w[i])
			}
		}
	}
	if rows[0].(map[string]any)["handset"] != rows[1].(map[string]any)["handset"] {
		t.Error("reinstall counted as another handset")
	}
	if heat := out["activity"].([]any); len(heat) != 7 || len(heat[0].([]any)) != 24 {
		t.Fatal(heat)
	}
}
