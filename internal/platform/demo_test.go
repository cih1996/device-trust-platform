package platform

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
)

type failReceiptOnce struct {
	base   http.RoundTripper
	failed atomic.Bool
}

func (f *failReceiptOnce) RoundTrip(r *http.Request) (*http.Response, error) {
	if strings.HasSuffix(r.URL.Path, "/v1/executions") && f.failed.CompareAndSwap(false, true) {
		return &http.Response{StatusCode: 503, Body: io.NopCloser(strings.NewReader(`{"error":"injected receipt outage"}`)), Header: make(http.Header), Request: r}, nil
	}
	return f.base.RoundTrip(r)
}

func TestDemoBlocksThenApprovesAndRecoversReceiptWithoutDuplicateBusinessEffect(t *testing.T) {
	root := t.TempDir()
	store, err := Open(filepath.Join(root, "platform"))
	if err != nil {
		t.Fatal(err)
	}
	app := App{ID: "demo-app", Package: "com.device.trust.sample", KeyHash: digest("application-secret")}
	store.put("apps", app.ID, app)
	store.put("security", app.ID, AppSecurity{RequireSignedReports: true, MaxEnrollmentHours: 24})
	policy := DefaultPolicy()
	policy.Mode = "enforce"
	policy.ReviewScore = 10
	store.put("policies", app.ID, policy)
	platform := &Server{Store: store, AdminToken: "admin"}
	platformHandler := platform.Handler(t.TempDir())
	server := httptest.NewTLSServer(platformHandler)
	defer server.Close()
	caFile := filepath.Join(root, "test-ca.pem")
	os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600)
	business, err := NewDemoBusiness(DemoConfig{PlatformURL: server.URL, AppID: app.ID, AppKey: "application-secret", Password: "test-password-at-least-sixteen", CAFile: caFile, Data: filepath.Join(root, "business")})
	if err != nil {
		t.Fatal(err)
	}
	h := business.Handler()
	login := request(t, h, "POST", "/login", "", map[string]string{"accountId": "user-one", "password": business.Config.Password}, 200)
	session := login["token"].(string)
	prepare := request(t, h, "POST", "/prepare", session, map[string]string{"type": "reward", "keyId": ""}, 201)
	raw, _ := json.Marshal(prepare["challenge"])
	var ch Challenge
	json.Unmarshal(raw, &ch)
	if strings.Contains(string(raw), "serverPrivateKey") {
		t.Fatal("challenge exposed server private key")
	}
	signer, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	chain := []string{selfSigned(t, signer)}
	payload, _ := json.Marshal(map[string]any{"certificateChain": chain})
	binaryRequest(t, platformHandler, makeFrame(t, 1, ch, signer, payload), 201)
	payload, _ = json.Marshal(reportInput{AppID: app.ID, Package: app.Package, Signals: map[string]Signal{"debugger": {Status: "available", Value: true}}})
	report := binaryRequest(t, platformHandler, makeFrame(t, 2, ch, signer, payload), 201)
	completion := map[string]string{"eventId": prepare["eventId"].(string), "reportId": report["reportId"].(string)}
	waiting := request(t, h, "POST", "/complete", session, completion, 200)
	if waiting["state"] != "verify" {
		t.Fatal(waiting)
	}
	operations, err := records[demoOperation](business.Store, "operations")
	if err != nil || len(operations) != 0 {
		t.Fatalf("blocked business action executed: %v %v", operations, err)
	}
	events, _ := records[Event](store, "events")
	if len(events) != 1 {
		t.Fatal(events)
	}
	request(t, platformHandler, "POST", "/api/events/"+events[0].ID+"/resolve", "admin", map[string]string{"resolutionId": "approval", "status": "approved"}, 201)
	failure := &failReceiptOnce{base: business.Client.Transport}
	business.Client.Transport = failure
	request(t, h, "POST", "/complete", session, completion, 502)
	operations, err = records[demoOperation](business.Store, "operations")
	if err != nil || len(operations) != 1 {
		t.Fatal(operations, err)
	}
	completed := request(t, h, "POST", "/complete", session, completion, 200)
	if completed["state"] != "completed" {
		t.Fatal(completed)
	}
	request(t, h, "POST", "/complete", session, completion, 200)
	operations, _ = records[demoOperation](business.Store, "operations")
	receipts, _ := records[Execution](store, "executions")
	if len(operations) != 1 || len(receipts) != 1 {
		t.Fatalf("duplicate effect: %v %v", operations, receipts)
	}
	other := request(t, h, "POST", "/login", "", map[string]string{"accountId": "another-user", "password": business.Config.Password}, 200)
	request(t, h, "POST", "/complete", other["token"].(string), completion, 403)
}
