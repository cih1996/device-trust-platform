package platform

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestConcurrentEventRetriesPublishOneDecision(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range []App{{ID: "one", KeyHash: digest("secret")}, {ID: "two", KeyHash: digest("secret")}} {
		if err = store.put("apps", a.ID, a); err != nil {
			t.Fatal(err)
		}
		if err = store.put("reports", a.ID, Report{ID: a.ID, AppID: a.ID, InstallationID: "device", ReceivedAt: time.Now().UTC()}); err != nil {
			t.Fatal(err)
		}
	}
	h := (&Server{store, "admin"}).Handler(t.TempDir())
	body := map[string]string{"eventId": "order-1001", "appId": "one", "reportId": "one", "accountId": "u1", "type": "reward", "clientIp": "203.0.113.1"}
	data, _ := json.Marshal(body)
	results := make([]map[string]any, 32)
	codes := make([]int, 32)
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r := httptest.NewRequest("POST", "/v1/events", bytes.NewReader(data))
			r.Header.Set("Authorization", "Bearer secret")
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			codes[i] = w.Code
			if err := json.Unmarshal(w.Body.Bytes(), &results[i]); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	created := 0
	for i, result := range results {
		if codes[i] == 201 {
			created++
		} else if codes[i] != 200 {
			t.Fatalf("status %d: %v", codes[i], result)
		}
		if result["id"] != results[0]["id"] || result["receivedAt"] != results[0]["receivedAt"] || result["score"] != results[0]["score"] {
			t.Fatalf("retries changed decision: %v", results)
		}
	}
	if created != 1 {
		t.Fatalf("created %d records", created)
	}
	events, err := records[Event](store, "events")
	if err != nil || len(events) != 1 {
		t.Fatalf("events: %v, error: %v", events, err)
	}
	body["accountId"] = "different"
	request(t, h, "POST", "/v1/events", "secret", body, 409)
	body["accountId"] = "u1"
	// Retries retain the original decision even after the source report has expired.
	if err = store.put("reports", "one", Report{ID: "one", AppID: "one", ReceivedAt: time.Now().Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(store.Root)
	if err != nil {
		t.Fatal(err)
	}
	h = (&Server{reopened, "admin"}).Handler(t.TempDir())
	request(t, h, "POST", "/v1/events", "secret", body, 200)
	body["appId"] = "two"
	body["reportId"] = "two"
	request(t, h, "POST", "/v1/events", "secret", body, 201)
}
