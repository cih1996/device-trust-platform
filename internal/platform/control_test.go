package platform

import (
	"testing"
	"time"
)

func TestBusinessContextPolicyOutcomesAndFeedback(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = store.put("apps", "app", App{ID: "app", KeyHash: digest("secret")}); err != nil {
		t.Fatal(err)
	}
	if err = store.put("reports", "report", Report{ID: "report", AppID: "app", InstallationID: "device", ReceivedAt: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	h := (&Server{store, "admin"}).Handler(t.TempDir())
	body := map[string]any{"eventId": "p1", "appId": "app", "reportId": "report", "accountId": "u", "type": "payment", "clientIp": "::ffff:203.0.113.1", "context": map[string]any{"orderId": "order1", "paymentMethodId": "provider-token", "amountMinor": 100, "currency": "CNY"}}
	body["context"].(map[string]any)["accountCreatedAt"] = time.Now().Add(-time.Hour).UTC().Format(time.RFC3339)
	first := request(t, h, "POST", "/v1/events", "secret", body, 201)
	if first["clientIp"] != "203.0.113.1" || first["mode"] != "shadow" {
		t.Fatal(first)
	}
	p := DefaultPolicy()
	p.ReviewScore = 50
	p.HoldScore = 80
	changed := request(t, h, "PUT", "/api/apps/app/policy", "admin", p, 200)
	retry := request(t, h, "POST", "/v1/events", "secret", body, 200)
	if retry["policy"].(map[string]any)["version"] == changed["version"] {
		t.Fatal("retry changed original policy")
	}
	body["context"].(map[string]any)["amountMinor"] = 200
	request(t, h, "POST", "/v1/events", "secret", body, 409)
	now := time.Now().UTC()
	outcome := map[string]any{"outcomeId": "provider-webhook-1", "appId": "app", "eventId": "p1", "status": "failed", "occurredAt": now}
	request(t, h, "POST", "/v1/outcomes", "wrong", outcome, 401)
	request(t, h, "POST", "/v1/outcomes", "secret", outcome, 201)
	request(t, h, "POST", "/v1/outcomes", "secret", outcome, 200)
	if err = store.put("apps", "another", App{ID: "another", KeyHash: digest("other-key")}); err != nil {
		t.Fatal(err)
	}
	outcome["appId"] = "another"
	request(t, h, "POST", "/v1/outcomes", "other-key", outcome, 404)
	outcome["appId"] = "app"
	outcome["status"] = "succeeded"
	request(t, h, "POST", "/v1/outcomes", "secret", outcome, 409)
	id := first["id"].(string)
	request(t, h, "POST", "/api/events/"+id+"/reviews", "admin", map[string]string{"label": "fraud", "note": "confirmed by operator"}, 201)
	request(t, h, "POST", "/api/events/"+id+"/reviews", "admin", map[string]string{"label": "legitimate", "note": "appeal accepted"}, 201)
	summary := request(t, h, "GET", "/api/overview", "admin", nil, 200)
	if summary["feedback"].(map[string]any)[id].(map[string]any)["label"] != "legitimate" {
		t.Fatal(summary)
	}
	reviews, err := records[Review](store, "reviews")
	if err != nil || len(reviews) != 2 {
		t.Fatalf("review audit: %v %v", reviews, err)
	}
	if summary["scenes"].(map[string]any)["payment"] != float64(1) {
		t.Fatal(summary)
	}
}
