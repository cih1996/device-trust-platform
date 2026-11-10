package platform

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func makeFrame(t *testing.T, kind byte, ch Challenge, key *ecdsa.PrivateKey, payload []byte) []byte {
	t.Helper()
	ephemeral, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	public, err := x509.ParsePKIXPublicKey(ch.ServerPublicKey)
	if err != nil {
		t.Fatal(err)
	}
	server, err := public.(*ecdsa.PublicKey).ECDH()
	if err != nil {
		t.Fatal(err)
	}
	secret, err := ephemeral.ECDH(server)
	if err != nil {
		t.Fatal(err)
	}
	aesKey, err := hkdf.Key(sha256.New, secret, ch.Nonce, frameKeyInfo, 32)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := aes.NewCipher(aesKey)
	gcm, _ := cipher.NewGCM(block)
	pub, _ := x509.MarshalPKIXPublicKey(&key.PublicKey)
	keyID := sha256.Sum256(pub)
	id, _ := hex.DecodeString(ch.ID)
	header := make([]byte, 142)
	copy(header, []byte("DTP4"))
	header[4] = kind
	copy(header[5:29], id)
	copy(header[29:61], keyID[:])
	copy(header[61:126], ephemeral.PublicKey().Bytes())
	rand.Read(header[126:138])
	binary.BigEndian.PutUint32(header[138:142], uint32(len(payload)+16))
	encrypted := gcm.Seal(nil, header[126:138], payload, header)
	signed := append(header, encrypted...)
	hash := sha256.Sum256(signed)
	signature, err := ecdsa.SignASN1(rand.Reader, key, hash[:])
	if err != nil {
		t.Fatal(err)
	}
	out := append(signed, byte(len(signature)>>8), byte(len(signature)))
	return append(out, signature...)
}
func binaryRequest(t *testing.T, h http.Handler, frame []byte, want int) map[string]any {
	t.Helper()
	r := httptest.NewRequest("POST", "/v1/secure", bytes.NewReader(frame))
	r.Header.Set("Content-Type", "application/octet-stream")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != want {
		t.Fatalf("binary status %d want %d: %s", w.Code, want, w.Body.String())
	}
	var result map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}
func selfSigned(t *testing.T, key *ecdsa.PrivateKey) string {
	t.Helper()
	template := &x509.Certificate{SerialNumber: big.NewInt(7), Subject: pkix.Name{CommonName: "TEST ONLY"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(der)
}

func TestSecureEnrollmentBindingReplayAndEnforcement(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := App{ID: "app", Package: "com.test.app", KeyHash: digest("secret")}
	store.put("apps", app.ID, app)
	store.put("security", app.ID, AppSecurity{RequireSignedReports: true, MaxEnrollmentHours: 24})
	server := &Server{Store: store, AdminToken: "admin"}
	h := server.Handler(t.TempDir())
	request(t, h, "POST", "/v1/reports", "", map[string]any{"appId": "app", "package": app.Package, "installationId": "fake", "signals": map[string]Signal{"debugger": {Status: "available", Value: false}}}, 403)
	context := BusinessContext{CampaignID: "campaign", RecipientID: "recipient", RequestHash: HashBusinessRequest("POST", "/claim", "a", []byte(`{"campaign":"campaign","recipient":"recipient"}`))}
	c := request(t, h, "POST", "/v1/challenges", "secret", map[string]any{"appId": "app", "eventId": "reward-1", "accountId": "a", "type": "reward", "context": context}, 201)
	raw, _ := json.Marshal(c)
	var challenge Challenge
	json.Unmarshal(raw, &challenge)
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	certificate := selfSigned(t, key)
	enrollmentPayload, _ := json.Marshal(map[string]any{"certificateChain": []string{certificate}})
	enrolled := binaryRequest(t, h, makeFrame(t, 1, challenge, key, enrollmentPayload), 201)
	if enrolled["proofLevel"] != "signature_only" {
		t.Fatal(enrolled)
	}
	var signatureKey deviceKey
	store.get("device-keys", app.ID+":"+enrolled["keyId"].(string), &signatureKey)
	signatureKey.EnrolledAt = time.Now().Add(-365 * 24 * time.Hour)
	store.put("device-keys", app.ID+":"+signatureKey.ID, signatureKey)
	if _, _, err := server.activeKey(app, signatureKey.ID, AppSecurity{MaxEnrollmentHours: 24}); err != nil {
		t.Fatal("ordinary signing identity unexpectedly rotated", err)
	}
	signals := map[string]Signal{"debugger": {Status: "available", Value: true}, "emulatorHint": {Status: "available", Value: true}}
	payload, _ := json.Marshal(reportInput{AppID: app.ID, Package: app.Package, InstallationID: "spoofed", Signals: signals})
	frame := makeFrame(t, 2, challenge, key, payload)
	uploaded := binaryRequest(t, h, frame, 201)
	retried := binaryRequest(t, h, frame, 200)
	if uploaded["deviceId"] == "" || uploaded["deviceId"] != retried["deviceId"] || uploaded["deviceId"] == enrolled["keyId"] {
		t.Fatal("device registration identity unstable", uploaded, retried)
	}
	tampered := append([]byte{}, frame...)
	tampered[142] ^= 1
	binaryRequest(t, h, tampered, 403)
	changedPayload, _ := json.Marshal(reportInput{AppID: app.ID, Package: app.Package, InstallationID: "changed", Signals: signals})
	binaryRequest(t, h, makeFrame(t, 2, challenge, key, changedPayload), 409)
	var saved Report
	store.get("reports", uploaded["reportId"].(string), &saved)
	if !strings.HasPrefix(saved.InstallationID, "key:") || saved.InstallationID == "spoofed" {
		t.Fatal(saved)
	}
	p := DefaultPolicy()
	p.Mode = "enforce"
	store.put("policies", app.ID, p)
	event := map[string]any{"eventId": "reward-1", "appId": "app", "reportId": saved.ID, "accountId": "different", "type": "reward", "context": context}
	request(t, h, "POST", "/v1/events", "secret", event, 403)
	event["accountId"] = "a"
	evaluated := request(t, h, "POST", "/v1/events", "secret", event, 201)
	if evaluated["mode"] != "enforce" || evaluated["action"] != "verify" {
		t.Fatal(evaluated)
	}
	claim := map[string]string{"appId": "app", "eventId": "reward-1", "operationId": "operation-1", "requestHash": context.RequestHash}
	request(t, h, "POST", "/v1/decisions/claim", "secret", claim, 403)
	resolution := map[string]string{"resolutionId": "operator-1", "status": "approved", "note": "test approval"}
	request(t, h, "POST", "/api/events/"+evaluated["id"].(string)+"/resolve", "admin", resolution, 201)
	claim["requestHash"] = HashBusinessRequest("POST", "/claim", "a", []byte(`{"campaign":"campaign","recipient":"different"}`))
	request(t, h, "POST", "/v1/decisions/claim", "secret", claim, 403)
	claim["requestHash"] = context.RequestHash
	permit := request(t, h, "POST", "/v1/decisions/claim", "secret", claim, 201)
	request(t, h, "POST", "/v1/decisions/claim", "secret", claim, 200)
	claim["operationId"] = "operation-2"
	request(t, h, "POST", "/v1/decisions/claim", "secret", claim, 409)
	receipt := map[string]string{"appId": "app", "eventId": "reward-1", "operationId": "operation-1", "token": "forged", "receiptId": "receipt-1", "status": "succeeded"}
	request(t, h, "POST", "/v1/executions", "secret", receipt, 403)
	receipt["token"] = permit["token"].(string)
	request(t, h, "POST", "/v1/executions", "secret", receipt, 201)
	request(t, h, "POST", "/v1/executions", "secret", receipt, 200)
	request(t, h, "POST", "/api/device-keys/revoke", "admin", map[string]string{"appId": "app", "keyId": enrolled["keyId"].(string), "reason": "test compromise"}, 201)
	binaryRequest(t, h, frame, 403)
	stored := storedChallenge{Challenge: challenge}
	store.get("challenges", challenge.ID, &stored)
	stored.ExpiresAt = time.Now().Add(-time.Second)
	store.put("challenges", challenge.ID, stored)
	binaryRequest(t, h, frame, 410)
}

func explicit(t *testing.T, tag int, v any, params string) []byte {
	t.Helper()
	var b []byte
	var err error
	if params != "" {
		b, err = asn1.MarshalWithParams(v, params)
	} else {
		b, err = asn1.Marshal(v)
	}
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := asn1.Marshal(asn1.RawValue{Class: 2, Tag: tag, IsCompound: true, Bytes: b})
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
func fakeAttestedChain(t *testing.T, nonce []byte, appPackage string, signerDigest []byte, locked bool, state asn1.Enumerated) ([]string, *x509.CertPool) {
	t.Helper()
	rootKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	leafKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	now := time.Now()
	root := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "UNIT TEST ROOT NEVER USE IN PRODUCTION"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	rootDER, err := x509.CreateCertificate(rand.Reader, root, root, &rootKey.PublicKey, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	rootCert, _ := x509.ParseCertificate(rootDER)
	pool := x509.NewCertPool()
	pool.AddCert(rootCert)
	identity, _ := asn1.Marshal(applicationIdentity{Packages: []packageInfo{{[]byte(appPackage), 1}}, Digests: [][]byte{signerDigest}})
	software := asn1.RawValue{Tag: 16, IsCompound: true, Bytes: explicit(t, 709, identity, "")}
	hardwareBytes := append(explicit(t, 704, rootOfTrust{[]byte{1}, locked, state, []byte{2}}, ""), explicit(t, 702, 0, "")...)
	hardwareBytes = append(hardwareBytes, explicit(t, 1, []int{2}, "set")...)
	hardwareBytes = append(hardwareBytes, explicit(t, 5, []int{4}, "set")...)
	hardwareBytes = append(hardwareBytes, explicit(t, 706, 202611, "")...)
	extension, err := asn1.Marshal(keyDescription{Version: 3, Security: 1, KeyVersion: 4, KeySecurity: 1, Challenge: nonce, Software: software, Hardware: asn1.RawValue{Tag: 16, IsCompound: true, Bytes: hardwareBytes}})
	if err != nil {
		t.Fatal(err)
	}
	leaf := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "TEST leaf"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour), KeyUsage: x509.KeyUsageDigitalSignature, ExtraExtensions: []pkix.Extension{{Id: attestationOID, Value: extension}}}
	leafDER, err := x509.CreateCertificate(rand.Reader, leaf, rootCert, &leafKey.PublicKey, rootKey)
	if err != nil {
		t.Fatal(err)
	}
	return []string{base64.StdEncoding.EncodeToString(leafDER), base64.StdEncoding.EncodeToString(rootDER)}, pool
}
func TestHardwareVerifierRejectsFalseIdentityAndRevocation(t *testing.T) {
	nonce := randomBytes(32)
	signer := randomBytes(32)
	chain, roots := fakeAttestedChain(t, nonce, "com.test.app", signer, true, 0)
	file := filepath.Join(t.TempDir(), "status.json")
	os.WriteFile(file, []byte(`{"entries":{}}`), 0600)
	verifier := &AttestationVerifier{Roots: roots, RevocationFile: file}
	app := App{Package: "com.test.app"}
	config := AppSecurity{SigningDigests: []string{hex.EncodeToString(signer)}, MinimumOSPatch: 202610}
	proof, err := verifier.verify(chain, nonce, app, config)
	if err != nil || proof.Level != "tee" {
		t.Fatalf("synthetic verification: %+v %v", proof, err)
	}
	if _, err = verifier.verify(chain, randomBytes(32), app, config); err == nil {
		t.Fatal("wrong nonce accepted")
	}
	app.Package = "changed"
	if _, err = verifier.verify(chain, nonce, app, config); err == nil {
		t.Fatal("wrong package accepted")
	}
	app.Package = "com.test.app"
	config.SigningDigests = []string{strings.Repeat("0", 64)}
	if _, err = verifier.verify(chain, nonce, app, config); err == nil {
		t.Fatal("wrong APK signer accepted")
	}
	config.SigningDigests = []string{hex.EncodeToString(signer)}
	os.WriteFile(file, []byte(`{"entries":{"2":{"status":"REVOKED","reason":"test"}}}`), 0600)
	if _, err = verifier.verify(chain, nonce, app, config); err == nil {
		t.Fatal("revoked certificate accepted")
	}
	os.WriteFile(file, []byte(`{"entries":{}}`), 0600)
	old := time.Now().Add(-25 * time.Hour)
	os.Chtimes(file, old, old)
	if _, err = verifier.verify(chain, nonce, app, config); err == nil {
		t.Fatal("stale revocations accepted")
	}
	os.Chtimes(file, time.Now(), time.Now())
	unlocked, pool := fakeAttestedChain(t, nonce, app.Package, signer, false, 2)
	verifier.Roots = pool
	if _, err = verifier.verify(unlocked, nonce, app, config); err == nil {
		t.Fatal("unlocked boot accepted")
	}
}
