package platform

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func request(t *testing.T, h http.Handler, method, path, key string, body any, want int) map[string]any {
	t.Helper()
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(method, path, bytes.NewReader(b))
	r.Header.Set("Authorization", "Bearer "+key)
	r.Header.Set("X-Forwarded-For", "203.0.113.99")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != want {
		t.Fatalf("%s %s: got %d want %d: %s", method, path, w.Code, want, w.Body.String())
	}
	var result map[string]any
	if err = json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}
func TestApplicationReportEventLifecycle(t *testing.T) {
	root := t.TempDir()
	store, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	admin := "test-admin-token-of-at-least-24-chars"
	h := (&Server{store, admin}).Handler(t.TempDir())
	request(t, h, "GET", "/api/overview", "", nil, 401)
	create := func(name string) (string, string) {
		a := request(t, h, "POST", "/api/apps", admin, map[string]string{"name": name, "package": "com.example." + name}, 201)
		return a["app"].(map[string]any)["id"].(string), a["key"].(string)
	}
	id, key := create("one")
	other, otherKey := create("two")
	report := request(t, h, "POST", "/v1/reports", "", map[string]any{"appId": id, "installationId": "install-1", "package": "com.example.one", "signals": map[string]any{"debugger": map[string]any{"status": "available", "value": false}}}, 201)
	rid := report["reportId"].(string)
	var saved Report
	if err = store.get("reports", rid, &saved); err != nil {
		t.Fatal(err)
	}
	if saved.ObservedIP != "192.0.2.1" {
		t.Fatalf("untrusted forwarded IP used: %s", saved.ObservedIP)
	}
	event := map[string]any{"eventId": "cross-app", "appId": other, "reportId": rid, "accountId": "u1", "type": "login"}
	request(t, h, "POST", "/v1/events", otherKey, event, 403)
	event["appId"] = id
	request(t, h, "POST", "/v1/events", "wrong", event, 401)
	for _, account := range []string{"u1", "u2", "u3", "u4"} {
		event["eventId"] = "login-" + account
		event["accountId"] = account
		e := request(t, h, "POST", "/v1/events", key, event, 201)
		if account == "u4" && e["decision"] != "review" {
			t.Fatalf("expected multi-account review: %v", e)
		}
	}
	newKey := request(t, h, "POST", "/api/apps/"+id+"/rotate-key", admin, map[string]any{}, 200)["key"].(string)
	request(t, h, "POST", "/v1/events", key, event, 401)
	event["eventId"] = "after-rotation"
	request(t, h, "POST", "/v1/events", newKey, event, 201)
	// Reopen storage to verify persistence and separation, not just in-memory behavior.
	reopened, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	h = (&Server{reopened, admin}).Handler(t.TempDir())
	summary := request(t, h, "GET", "/api/overview?appId="+id, admin, nil, 200)
	if summary["stats"].(map[string]any)["events"] != float64(5) {
		t.Fatal(summary)
	}
	summary = request(t, h, "GET", "/api/overview?appId="+other, admin, nil, 200)
	if summary["stats"].(map[string]any)["events"] != float64(0) {
		t.Fatal(summary)
	}
	// Corrupt records surface their decoding failure rather than silently disappearing.
	if err = os.WriteFile(filepath.Join(root, "events", "corrupt.json"), []byte("{"), 0600); err != nil {
		t.Fatal(err)
	}
	request(t, h, "GET", "/api/overview", admin, nil, 500)
}
func TestConcurrentReportsAndMissingSignals(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := App{ID: "app", Package: "example"}
	if err = store.put("apps", a.ID, a); err != nil {
		t.Fatal(err)
	}
	h := (&Server{store, "admin"}).Handler(t.TempDir())
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			body := `{"appId":"app","package":"example","installationId":"device","signals":{"vpn":{"status":"unavailable"}}}`
			r := httptest.NewRequest("POST", "/v1/reports", bytes.NewBufferString(body))
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != 201 {
				t.Errorf("%d: %s", w.Code, w.Body.String())
			}
		}()
	}
	wg.Wait()
	reports, err := records[Report](store, "reports")
	if err != nil {
		t.Fatal(err)
	}
	if len(reports) != 24 {
		t.Fatal(len(reports))
	}
	var e Event
	evaluate(&e, Report{}, nil)
	if e.Score != 0 || e.Decision != "observe" {
		t.Fatal(e)
	}
}
func TestInputAndReportFreshness(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	a := App{ID: "app", Package: "example", KeyHash: digest("secret")}
	store.put("apps", a.ID, a)
	h := (&Server{store, "admin"}).Handler(t.TempDir())
	request(t, h, "POST", "/v1/reports", "", map[string]any{"appId": "app", "package": "example", "installationId": "id", "signals": map[string]Signal{"wifi": {Status: "made_up"}}}, 400)
	p := Report{ID: "old", AppID: "app", InstallationID: "id"}
	store.put("reports", p.ID, p)
	request(t, h, "POST", "/v1/events", "secret", map[string]string{"eventId": "expired", "appId": "app", "reportId": "old", "accountId": "u", "type": "login"}, 400)
}
