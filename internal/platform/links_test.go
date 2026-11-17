package platform

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAttestationIssues(t *testing.T) {
	nonce, signer := randomBytes(32), randomBytes(32)
	file := filepath.Join(t.TempDir(), "status.json")
	os.WriteFile(file, []byte(`{"entries":{}}`), 0600)
	app := App{Package: "com.test.app"}
	chain, roots := fakeAttestedChain(t, nonce, app.Package, signer, true, 0)
	verifier := &AttestationVerifier{Roots: roots, RevocationFile: file}
	if proof, err := verifier.verify(chain, nonce, app, AppSecurity{}); err != nil || proof.Level != "tee" {
		t.Fatal("package attestation without signer allowlist", proof, err)
	}
	cases := []struct {
		name   string
		config AppSecurity
		app    App
		status string
		issue  string
	}{
		{"signer", AppSecurity{SigningDigests: []string{hex.EncodeToString(randomBytes(32))}}, app, `{"entries":{}}`, "app_mismatch"},
		{"package", AppSecurity{}, App{Package: "com.repackaged"}, `{"entries":{}}`, "app_mismatch"},
		{"revoked", AppSecurity{}, app, `{"entries":{"2":{"status":"REVOKED","reason":"KEY_COMPROMISE"}}}`, "revoked"},
		{"hardware requires allowlist", AppSecurity{RequireHardware: true}, app, `{"entries":{}}`, "unverified"},
	}
	for _, c := range cases {
		os.WriteFile(file, []byte(c.status), 0600)
		_, err := verifier.verify(chain, nonce, c.app, c.config)
		if err == nil || attestationIssue(err) != c.issue {
			t.Fatalf("%s: %v -> %s", c.name, err, attestationIssue(err))
		}
	}
	os.WriteFile(file, []byte(`{"entries":{}}`), 0600)
	unlocked, pool := fakeAttestedChain(t, nonce, app.Package, signer, false, 2)
	verifier.Roots = pool
	if _, err := verifier.verify(unlocked, nonce, app, AppSecurity{}); attestationIssue(err) != "boot_unlocked" {
		t.Fatal(err)
	}
}

type linkHarness struct {
	t *testing.T
	h http.Handler
}

// upload enrolls a new key for appID and sends one signed report; it returns the
// upload response and the account the challenge was bound to.
func (x linkHarness) upload(appID, androidID string, boot int64, signers []string) map[string]any {
	t := x.t
	context := BusinessContext{RequestHash: HashBusinessRequest("POST", "/login", "u1", []byte(appID))}
	raw, _ := json.Marshal(request(t, x.h, "POST", "/v1/challenges", "secret-"+appID, map[string]any{"appId": appID, "eventId": "login-" + androidID + appID, "accountId": "u1", "type": "login", "context": context}, 201))
	var challenge Challenge
	json.Unmarshal(raw, &challenge)
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	enrollment, _ := json.Marshal(map[string]any{"certificateChain": []string{selfSigned(t, key)}})
	binaryRequest(t, x.h, makeFrame(t, 1, challenge, key, enrollment), 201)
	signals := map[string]Signal{
		"androidId":        {Status: "available", Value: androidID},
		"buildFingerprint": {Status: "available", Value: "brand/device:15/AP3A/1:user/release-keys"},
		"application":      {Status: "available", Value: map[string]any{"signers": signers}},
		"osInfo":           {Status: "available", Value: map[string]any{"bootEpochMillis": boot}},
		"resourceJava":     {Status: "available", Value: map[string]any{"memoryBytes": 5826281472}},
		"storage":          {Status: "available", Value: map[string]any{"totalBytes": 115560099840}},
		"display":          {Status: "available", Value: map[string]any{"widthPixels": 1080, "heightPixels": 2244}},
	}
	payload, _ := json.Marshal(reportInput{AppID: appID, Package: "com.test." + appID, InstallationID: "x", Signals: signals})
	result := binaryRequest(t, x.h, makeFrame(t, 2, challenge, key, payload), 201)
	result["eventId"] = challenge.EventID
	return result
}

func TestCrossAppLinksAndVerdicts(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	signer := hex.EncodeToString(randomBytes(32))
	for _, id := range []string{"a", "b", "c"} {
		store.put("apps", id, App{ID: id, Name: "App " + id, Package: "com.test." + id, KeyHash: digest("secret-" + id)})
		store.put("security", id, AppSecurity{RequireSignedReports: true, RequireRequestBinding: true, MaxEnrollmentHours: 24, SigningDigests: []string{signer}})
	}
	server := &Server{Store: store, AdminToken: "admin"}
	x := linkHarness{t: t, h: server.Handler(t.TempDir())}
	boot := time.Now().Add(-time.Hour).UnixMilli()

	first := x.upload("a", "aid-1", boot, []string{signer})
	// Same signer, same ANDROID_ID, booted at another time: linked by android_id.
	second := x.upload("b", "aid-1", boot-int64(time.Hour/time.Millisecond), []string{signer})
	// Different signer (so a different ANDROID_ID) but the same boot instant.
	third := x.upload("c", "aid-other", boot+800, []string{"ff"})
	// Same hardware rebooted elsewhere with another ANDROID_ID: a separate device.
	fourth := x.upload("c", "aid-4", boot+int64(10*time.Second/time.Millisecond), []string{signer})

	links, _ := server.deviceLinks()
	method := map[string]string{}
	for _, l := range links {
		method[l.AppID+l.LinkID] = l.Method
	}
	var saved [4]Report
	for i, r := range []map[string]any{first, second, third, fourth} {
		store.get("reports", r["reportId"].(string), &saved[i])
	}
	if saved[0].LinkID == "" || saved[1].LinkID != saved[0].LinkID || saved[2].LinkID != saved[0].LinkID || saved[3].LinkID == saved[0].LinkID {
		t.Fatalf("links: %s %s %s %s", saved[0].LinkID, saved[1].LinkID, saved[2].LinkID, saved[3].LinkID)
	}
	if method["b"+saved[0].LinkID] != "android_id" || method["c"+saved[0].LinkID] != "boot_session" {
		t.Fatal(method)
	}

	verdict := map[string]any{"appId": "a", "reportId": first["reportId"], "eventId": first["eventId"], "accountId": "other"}
	request(t, x.h, "POST", "/v1/verdicts", "secret-a", verdict, 403)
	verdict["accountId"] = "u1"
	request(t, x.h, "POST", "/v1/verdicts", "secret-b", verdict, 401)
	got := request(t, x.h, "POST", "/v1/verdicts", "secret-a", verdict, 200)
	linked := got["linked"].(map[string]any)
	appInfo := got["app"].(map[string]any)
	if got["deviceId"] != first["deviceId"] || got["keyStatus"] != "active" || linked["devices"] != float64(2) || linked["apps"] != float64(2) || appInfo["signer"] != "matched" || appInfo["attestation"] != "none" {
		t.Fatal(got)
	}
	// The third app reported a foreign signer: the verdict and the score say so.
	thirdVerdict := request(t, x.h, "POST", "/v1/devices/query", "secret-c", map[string]any{"appId": "c", "deviceId": third["deviceId"]}, 200)
	if thirdVerdict["app"].(map[string]any)["signer"] != "mismatch" || thirdVerdict["riskScore"].(float64) < 40 {
		t.Fatal(thirdVerdict)
	}
	request(t, x.h, "POST", "/v1/devices/query", "secret-a", map[string]any{"appId": "a", "deviceId": third["deviceId"]}, 404)

	var stored Report
	store.get("reports", first["reportId"].(string), &stored)
	stored.ReceivedAt = time.Now().Add(-time.Hour)
	store.put("reports", stored.ID, stored)
	request(t, x.h, "POST", "/v1/verdicts", "secret-a", verdict, 410)
}

func TestEnrollmentWithKeystoreAttestationFailure(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	signer := hex.EncodeToString(randomBytes(32))
	store.put("apps", "soft", App{ID: "soft", Package: "com.test.soft", KeyHash: digest("secret-soft")})
	store.put("security", "soft", AppSecurity{RequireSignedReports: true, MaxEnrollmentHours: 24})
	store.put("apps", "hard", App{ID: "hard", Package: "com.test.hard", KeyHash: digest("secret-hard")})
	store.put("security", "hard", AppSecurity{RequireSignedReports: true, RequireHardware: true, MaxEnrollmentHours: 24, SigningDigests: []string{signer}})
	server := &Server{Store: store, AdminToken: "admin"}
	h := server.Handler(t.TempDir())
	enroll := func(appID string, status int) {
		raw, _ := json.Marshal(request(t, h, "POST", "/v1/challenges", "secret-"+appID, map[string]any{"appId": appID, "eventId": "e-" + appID, "accountId": "u1", "type": "login"}, 201))
		var challenge Challenge
		json.Unmarshal(raw, &challenge)
		if challenge.HardwareRequired != (appID == "hard") {
			t.Fatal("hardwareRequired", challenge.HardwareRequired)
		}
		key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		body, _ := json.Marshal(map[string]any{"certificateChain": []string{selfSigned(t, key)}, "attestationError": "ProviderException: Failed to generate key pair <- KeyStoreException: -10003"})
		binaryRequest(t, h, makeFrame(t, 1, challenge, key, body), status)
		if status != 201 {
			return
		}
		var accepted enrollment
		if err := store.get("enrollments", challenge.ID, &accepted); err != nil || accepted.Proof.Level != "signature_only" || accepted.Proof.Issue != "keystore_failed" || accepted.Proof.Error == "" {
			t.Fatal(accepted, err)
		}
	}
	enroll("soft", 201)
	enroll("hard", 403)
}
