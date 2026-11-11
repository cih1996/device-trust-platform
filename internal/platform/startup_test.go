package platform

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"encoding/pem"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestStartupRegistersBeforeLoginAndNeverCreatesBusinessEffect(t *testing.T) {
	root := t.TempDir()
	store, err := Open(filepath.Join(root, "platform"))
	if err != nil {
		t.Fatal(err)
	}
	app := App{ID: "startup-app", Package: "com.device.trust.sample", KeyHash: digest("backend-secret")}
	store.put("apps", app.ID, app)
	store.put("security", app.ID, AppSecurity{RequireSignedReports: true, RequireRequestBinding: true, MaxEnrollmentHours: 24})
	h := (&Server{Store: store, AdminToken: "admin"}).Handler(t.TempDir())
	tls := httptest.NewTLSServer(h)
	defer tls.Close()
	ca := filepath.Join(root, "ca.pem")
	os.WriteFile(ca, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: tls.Certificate().Raw}), 0600)
	business, err := NewDemoBusiness(DemoConfig{PlatformURL: tls.URL, AppID: app.ID, AppKey: "backend-secret", Password: "demo-test-password-16", CAFile: ca, Data: filepath.Join(root, "business")})
	if err != nil {
		t.Fatal(err)
	}
	b := business.Handler()
	input := map[string]string{"appId": app.ID, "installationId": "test-installation-guid-0001", "keyId": ""}
	first := request(t, b, "POST", "/device/prepare", "", input, 201)
	raw, _ := json.Marshal(first["challenge"])
	var initial Challenge
	json.Unmarshal(raw, &initial)
	if initial.Type != "device_init" || initial.HardwareRequired || initial.Context.RequestHash == "" {
		t.Fatal(initial)
	}
	if _, ok := first["challenge"].(map[string]any)["hardwareRequired"]; !ok {
		t.Fatal("bootstrap dropped policy field")
	}
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	payload, _ := json.Marshal(map[string]any{"certificateChain": []string{selfSigned(t, key)}})
	enrollment := binaryRequest(t, h, makeFrame(t, 1, initial, key, payload), 201)
	payload, _ = json.Marshal(reportInput{AppID: app.ID, Package: app.Package, Signals: map[string]Signal{"model": {Status: "available", Value: "Startup test device"}}})
	registered := binaryRequest(t, h, makeFrame(t, 2, initial, key, payload), 201)
	if registered["deviceId"] == "" {
		t.Fatal("device ID not returned at startup")
	}
	events, _ := records[Event](store, "events")
	if len(events) != 0 {
		t.Fatal("startup polluted business events")
	}
	listing := request(t, h, "GET", "/api/devices?appId="+app.ID+"&q="+registered["deviceId"].(string), "admin", nil, 200)
	items := listing["items"].([]any)
	if len(items) != 1 {
		t.Fatal(listing)
	}
	profile := items[0].(map[string]any)
	if profile["accountCount"].(float64) != 0 || profile["eventCount"].(float64) != 0 {
		t.Fatal(profile)
	}
	detail := request(t, h, "GET", "/api/devices/"+profile["id"].(string), "admin", nil, 200)
	if len(detail["reports"].([]any)) != 1 || len(detail["accounts"].([]any)) != 0 {
		t.Fatal(detail)
	}
	input["keyId"] = enrollment["keyId"].(string)
	second := request(t, b, "POST", "/device/prepare", "", input, 201)
	raw, _ = json.Marshal(second["challenge"])
	var refresh Challenge
	json.Unmarshal(raw, &refresh)
	if refresh.Type != "device_refresh" {
		t.Fatal(refresh)
	}
	repeated := binaryRequest(t, h, makeFrame(t, 2, refresh, key, payload), 201)
	if registered["deviceId"] != repeated["deviceId"] {
		t.Fatal("device changed on next startup")
	}
	request(t, h, "POST", "/v1/events", "backend-secret", map[string]any{"appId": app.ID, "eventId": "reward-forged", "reportId": registered["reportId"], "accountId": "real-user", "type": "reward", "context": initial.Context}, 403)
	request(t, h, "GET", "/api/devices", "", nil, 401)
	input["appId"] = "different-app"
	request(t, b, "POST", "/device/prepare", "", input, 403)
}
