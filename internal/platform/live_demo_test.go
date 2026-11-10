package platform

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Opt in against an isolated local delivery. This writes a test event and ledger effect.
func TestLiveDemoDelivery(t *testing.T) {
	root := os.Getenv("DTP_LIVE_DEMO_DATA")
	if root == "" {
		t.Skip("set DTP_LIVE_DEMO_DATA to an isolated running demo data directory")
	}
	raw, err := os.ReadFile(filepath.Join(root, "demo.json"))
	if err != nil {
		t.Fatal(err)
	}
	var config DemoConfig
	if err = json.Unmarshal(raw, &config); err != nil {
		t.Fatal(err)
	}
	business, err := NewDemoBusiness(config)
	if err != nil {
		t.Fatal(err)
	}
	platformURL, _ := url.Parse(config.PlatformURL)
	p := httputil.NewSingleHostReverseProxy(platformURL)
	p.Transport = business.Client.Transport
	businessURL, _ := url.Parse("https://localhost:9444")
	b := httputil.NewSingleHostReverseProxy(businessURL)
	b.Transport = business.Client.Transport
	login := request(t, b, "POST", "/login", "", map[string]string{"accountId": "delivery-check", "password": config.Password}, 200)
	session := login["token"].(string)
	prepare := request(t, b, "POST", "/prepare", session, map[string]string{"type": "reward", "keyId": ""}, 201)
	raw, _ = json.Marshal(prepare["challenge"])
	var ch Challenge
	if err = json.Unmarshal(raw, &ch); err != nil {
		t.Fatal(err)
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]any{"certificateChain": []string{selfSigned(t, key)}})
	binaryRequest(t, p, makeFrame(t, 1, ch, key, payload), 201)
	signals := map[string]Signal{}
	for _, name := range []string{"debugger", "emulatorHint", "rootBeerSuBinary", "rootBeerMagiskBinary", "rootBeerNativeSu"} {
		signals[name] = Signal{Status: "available", Value: true}
	}
	payload, _ = json.Marshal(reportInput{AppID: config.AppID, Package: "com.device.trust.sample", Signals: signals})
	frame := makeFrame(t, 2, ch, key, payload)
	report := binaryRequest(t, p, frame, 201)
	binaryRequest(t, p, frame, 200)
	complete := map[string]string{"eventId": prepare["eventId"].(string), "reportId": report["reportId"].(string)}
	waiting := request(t, b, "POST", "/complete", session, complete, 200)
	if waiting["state"] != "verify" && waiting["state"] != "hold" {
		t.Fatal(waiting)
	}
	operations, err := records[demoOperation](business.Store, "operations")
	if err != nil {
		t.Fatal(err)
	}
	for _, op := range operations {
		if op.EventID == complete["eventId"] {
			t.Fatal("blocked operation executed")
		}
	}
	admin, err := os.ReadFile(filepath.Join(root, "admin-token"))
	if err != nil {
		t.Fatal(err)
	}
	internalID := digest(config.AppID + ":" + complete["eventId"])
	request(t, p, "POST", "/api/events/"+internalID+"/resolve", strings.TrimSpace(string(admin)), map[string]string{"resolutionId": token(), "status": "approved", "note": "交付联调验证"}, 201)
	for i := 0; i < 2; i++ {
		done := request(t, b, "POST", "/complete", session, complete, 200)
		if done["state"] != "completed" {
			t.Fatal(done)
		}
	}
	operations, err = records[demoOperation](business.Store, "operations")
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, op := range operations {
		if op.EventID == complete["eventId"] {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected one effect, got %d", count)
	}
	t.Log("Live TLS enrollment, binary replay deduplication, blocking, approval, execution and retry passed")
}
