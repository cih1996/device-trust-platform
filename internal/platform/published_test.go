package platform

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"testing"
)

func publishedProxy(target *url.URL) *httputil.ReverseProxy {
	p := httputil.NewSingleHostReverseProxy(target)
	director := p.Director
	p.Director = func(r *http.Request) { director(r); r.Host = target.Host }
	return p
}

// Opt-in deployment acceptance; writes a clearly marked demo event only.
func TestPublishedDeviceTrust(t *testing.T) {
	file := os.Getenv("DTP_PUBLISHED_CREDENTIALS")
	if file == "" {
		t.Skip("isolated published demo acceptance is opt-in")
	}
	raw, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	var credentials struct {
		AdminToken string `json:"adminToken"`
		Password   string `json:"demoPassword"`
	}
	if err = json.Unmarshal(raw, &credentials); err != nil {
		t.Fatal(err)
	}
	api, _ := url.Parse("https://devicetrust-sdk-api.lingzidev.com")
	console, _ := url.Parse("https://devicetrust-console.lingzidev.com")
	business, _ := url.Parse("https://devicetrust-sdk-api.lingzidev.com/demo")
	p := publishedProxy(api)
	c := publishedProxy(console)
	b := publishedProxy(business)
	login := request(t, b, "POST", "/login", "", map[string]string{"accountId": "deployment-acceptance", "password": credentials.Password}, 200)
	session := login["token"].(string)
	appID := login["appId"].(string)
	prepared := request(t, b, "POST", "/prepare", session, map[string]string{"type": "reward", "keyId": ""}, 201)
	raw, _ = json.Marshal(prepared["challenge"])
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
	payload, _ = json.Marshal(reportInput{AppID: appID, Package: "com.device.trust.sample", Signals: map[string]Signal{"debugger": {Status: "available", Value: true}, "emulatorHint": {Status: "available", Value: true}, "rootBeerSuBinary": {Status: "available", Value: true}, "rootBeerNativeSu": {Status: "available", Value: true}}})
	frame := makeFrame(t, 2, ch, key, payload)
	report := binaryRequest(t, p, frame, 201)
	binaryRequest(t, p, frame, 200)
	tampered := append([]byte(nil), frame...)
	tampered[len(tampered)-1] ^= 1
	binaryRequest(t, p, tampered, 403)
	completion := map[string]string{"eventId": prepared["eventId"].(string), "reportId": report["reportId"].(string)}
	waiting := request(t, b, "POST", "/complete", session, completion, 200)
	if waiting["state"] != "verify" && waiting["state"] != "hold" {
		t.Fatal(waiting)
	}
	id := digest(appID + ":" + completion["eventId"])
	overview := request(t, c, "GET", "/api/overview?appId="+appID, credentials.AdminToken, nil, 200)
	if _, exists := overview["executions"].(map[string]any)[id]; exists {
		t.Fatal("effect before approval")
	}
	request(t, c, "POST", "/api/events/"+id+"/resolve", credentials.AdminToken, map[string]string{"resolutionId": token(), "status": "approved", "note": "上线验收 · 合成风险事件"}, 201)
	for i := 0; i < 2; i++ {
		done := request(t, b, "POST", "/complete", session, completion, 200)
		if done["state"] != "completed" {
			t.Fatal(done)
		}
	}
	overview = request(t, c, "GET", "/api/overview?appId="+appID, credentials.AdminToken, nil, 200)
	receipt, ok := overview["executions"].(map[string]any)[id].(map[string]any)
	if !ok || receipt["status"] != "succeeded" {
		t.Fatal("missing successful receipt")
	}
	t.Log("Public TLS, encrypted enrollment/report, tamper rejection, replay deduplication, approval and execution passed")
}
