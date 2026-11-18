package platform

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/asn1"
	"encoding/binary"
	"encoding/json"
	"errors"
	"math/big"
	"sort"
	"testing"
	"time"
)

// cborEncode writes the subset decodeCBOR reads (test only).
func cborEncode(v any) []byte {
	head := func(major byte, n int) []byte {
		switch {
		case n < 24:
			return []byte{major<<5 | byte(n)}
		case n < 256:
			return []byte{major<<5 | 24, byte(n)}
		default:
			return []byte{major<<5 | 25, byte(n >> 8), byte(n)}
		}
	}
	switch x := v.(type) {
	case int:
		return head(0, x)
	case []byte:
		return append(head(2, len(x)), x...)
	case string:
		return append(head(3, len(x)), x...)
	case []any:
		out := head(4, len(x))
		for _, item := range x {
			out = append(out, cborEncode(item)...)
		}
		return out
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := head(5, len(x))
		for _, k := range keys {
			out = append(out, cborEncode(k)...)
			out = append(out, cborEncode(x[k])...)
		}
		return out
	}
	panic("unsupported")
}

type fakeApple struct {
	rootKey, caKey *ecdsa.PrivateKey
	root, ca       *x509.Certificate
}

func newFakeApple(t *testing.T) *fakeApple {
	t.Helper()
	f := &fakeApple{}
	f.rootKey, _ = ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	f.caKey, _ = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	rootTemplate := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "TEST App Attestation Root"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, err := x509.CreateCertificate(rand.Reader, rootTemplate, rootTemplate, &f.rootKey.PublicKey, f.rootKey)
	if err != nil {
		t.Fatal(err)
	}
	f.root, _ = x509.ParseCertificate(der)
	caTemplate := &x509.Certificate{SerialNumber: big.NewInt(2), Subject: pkix.Name{CommonName: "TEST App Attestation CA 1"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign}
	der, err = x509.CreateCertificate(rand.Reader, caTemplate, f.root, &f.caKey.PublicKey, f.rootKey)
	if err != nil {
		t.Fatal(err)
	}
	f.ca, _ = x509.ParseCertificate(der)
	saved := appAttestRoots
	pool := x509.NewCertPool()
	pool.AddCert(f.root)
	appAttestRoots = pool
	t.Cleanup(func() { appAttestRoots = saved })
	return f
}

// attest returns an attestation object for a fresh App Attest key.
func (f *fakeApple) attest(t *testing.T, appID string, aaguid, clientDataHash []byte) ([]byte, []byte, *ecdsa.PrivateKey) {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	point, _ := key.PublicKey.Bytes()
	keyID := sha256.Sum256(point)
	rp := sha256.Sum256([]byte(appID))
	authData := append(rp[:], 0x40, 0, 0, 0, 0)
	authData = append(authData, aaguid...)
	authData = append(authData, 0, 32)
	authData = append(authData, keyID[:]...)
	nonce := sha256.Sum256(append(append([]byte{}, authData...), clientDataHash...))
	ext, err := asn1.Marshal(struct {
		Nonce []byte `asn1:"explicit,tag:1"`
	}{nonce[:]})
	if err != nil {
		t.Fatal(err)
	}
	leafTemplate := &x509.Certificate{SerialNumber: big.NewInt(3), Subject: pkix.Name{CommonName: "TEST credential"}, NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), ExtraExtensions: []pkix.Extension{{Id: appAttestNonceOID, Value: ext}}}
	der, err := x509.CreateCertificate(rand.Reader, leafTemplate, f.ca, &key.PublicKey, f.caKey)
	if err != nil {
		t.Fatal(err)
	}
	object := map[string]any{"fmt": "apple-appattest", "authData": authData, "attStmt": map[string]any{"x5c": []any{der, f.ca.Raw}, "receipt": []byte("receipt")}}
	return cborEncode(object), keyID[:], key
}

func assertion(t *testing.T, key *ecdsa.PrivateKey, appID string, counter uint32, clientDataHash []byte) []byte {
	t.Helper()
	rp := sha256.Sum256([]byte(appID))
	authData := append(rp[:], 0x40, 0, 0, 0, 0)
	binary.BigEndian.PutUint32(authData[33:], counter)
	nonce := sha256.Sum256(append(append([]byte{}, authData...), clientDataHash...))
	digest := sha256.Sum256(nonce[:])
	sig, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	return cborEncode(map[string]any{"signature": sig, "authenticatorData": authData})
}

func TestCBORRejectsMalformed(t *testing.T) {
	for _, b := range [][]byte{{}, {0x5f}, {0x59, 0xff}, {0xa1, 0x01, 0x01}, {0xa2, 0x61, 'a', 0x01, 0x61, 'a', 0x02}, {0x01, 0x02}, {0xf9, 0, 0}} {
		if _, err := decodeCBOR(b); err == nil {
			t.Errorf("accepted % x", b)
		}
	}
}

func TestVerifyAppAttestRejectsWrongBindings(t *testing.T) {
	f := newFakeApple(t)
	const appID = "ABCDE12345.com.test.ios"
	security := AppSecurity{IOSAppIDs: []string{appID}}
	cdh := sha256.Sum256([]byte("client"))
	now := time.Now()
	object, keyID, _ := f.attest(t, appID, aaguidProduction, cdh[:])
	if _, proof, err := verifyAppAttest(object, keyID, cdh[:], security, now); err != nil || proof.Level != "app_attest" || proof.Environment != "production" || proof.AppIdentity != appID {
		t.Fatal(proof, err)
	}
	other := sha256.Sum256([]byte("other"))
	if _, _, err := verifyAppAttest(object, keyID, other[:], security, now); err == nil {
		t.Fatal("accepted another challenge")
	}
	if _, _, err := verifyAppAttest(object, other[:], cdh[:], security, now); err == nil {
		t.Fatal("accepted another key ID")
	}
	if _, _, err := verifyAppAttest(object, keyID, cdh[:], AppSecurity{IOSAppIDs: []string{"ABCDE12345.com.other"}}, now); !errors.Is(err, errAttestApp) {
		t.Fatal("accepted another app", err)
	}
	dev, devID, _ := f.attest(t, appID, aaguidDevelopment, cdh[:])
	if _, _, err := verifyAppAttest(dev, devID, cdh[:], security, now); !errors.Is(err, errAttestDevelopment) {
		t.Fatal("accepted development environment", err)
	}
	security.AllowAppAttestDevelopment = true
	if _, proof, err := verifyAppAttest(dev, devID, cdh[:], security, now); err != nil || proof.Environment != "development" {
		t.Fatal(proof, err)
	}
	appAttestRoots = x509.NewCertPool()
	if _, _, err := verifyAppAttest(object, keyID, cdh[:], security, now); err == nil {
		t.Fatal("accepted an untrusted root")
	}
}

func TestIOSEnrollmentAssertionsAndPackage(t *testing.T) {
	f := newFakeApple(t)
	const appID = "ABCDE12345.com.test.ios"
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := App{ID: "app", Package: "com.test.app", KeyHash: digest("secret")}
	store.put("apps", app.ID, app)
	store.put("security", app.ID, AppSecurity{RequireSignedReports: true, MaxEnrollmentHours: 24, IOSAppIDs: []string{appID}, AllowAppAttestDevelopment: true})
	server := &Server{Store: store, AdminToken: "admin"}
	h := server.Handler(t.TempDir())
	challenge := func(eventID, keyID string) Challenge {
		c := request(t, h, "POST", "/v1/challenges", "secret", map[string]any{"appId": "app", "eventId": eventID, "accountId": "a", "type": "login", "keyId": keyID}, 201)
		raw, _ := json.Marshal(c)
		var out Challenge
		json.Unmarshal(raw, &out)
		return out
	}
	enclave, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	spki, _ := x509.MarshalPKIXPublicKey(&enclave.PublicKey)
	c := challenge("init", "")
	object, attestID, attestKey := f.attest(t, appID, aaguidDevelopment, appAttestClientData(c.Nonce, spki))
	enroll, _ := json.Marshal(map[string]any{"platform": "ios", "publicKey": spki, "appAttest": map[string]any{"keyId": attestID, "attestation": object}})
	enrolled := binaryRequest(t, h, makeFrame(t, 1, c, enclave, enroll), 201)
	if enrolled["proofLevel"] != "app_attest" {
		t.Fatal(enrolled)
	}
	keyID := enrolled["keyId"].(string)
	signals := map[string]Signal{"platform": {Status: "available", Value: "ios"}}
	report := func(c Challenge, pkg string, counter uint32, want int) map[string]any {
		payload, _ := json.Marshal(reportInput{AppID: app.ID, Package: pkg, Signals: signals, AppAttest: assertion(t, attestKey, appID, counter, assertionClientData(c.Nonce, keyID))})
		frame := makeFrame(t, 2, c, enclave, payload)
		out := binaryRequest(t, h, frame, want)
		if want == 201 {
			binaryRequest(t, h, frame, 200) // an identical retry keeps the counter
		}
		return out
	}
	if r := report(c, "com.test.ios", 1, 201); r["proofLevel"] != "app_attest" {
		var bad Report
		store.get("reports", r["reportId"].(string), &bad)
		t.Fatal(r, bad.Proof)
	}
	report(challenge("other-package", keyID), "com.other", 2, 403)
	if r := report(challenge("replayed", keyID), "com.test.ios", 1, 201); r["proofLevel"] != "signature_only" {
		t.Fatal("counter replay accepted", r)
	}
	r := report(challenge("next", keyID), "com.test.ios", 2, 201)
	var saved Report
	store.get("reports", r["reportId"].(string), &saved)
	if saved.Proof.Level != "app_attest" || saved.Package != "com.test.ios" {
		t.Fatal(saved.Proof, saved.Package)
	}

	// Without App Attest the key is only a signing key; apps requiring hardware refuse it.
	c = challenge("simulator", "")
	other, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	otherSPKI, _ := x509.MarshalPKIXPublicKey(&other.PublicKey)
	plain, _ := json.Marshal(map[string]any{"platform": "ios", "publicKey": otherSPKI, "attestationError": "DCError.featureUnsupported"})
	if r := binaryRequest(t, h, makeFrame(t, 1, c, other, plain), 201); r["proofLevel"] != "signature_only" {
		t.Fatal(r)
	}
	store.put("security", app.ID, AppSecurity{RequireSignedReports: true, RequireHardware: true, MaxEnrollmentHours: 24, IOSAppIDs: []string{appID}})
	c = challenge("simulator-2", "")
	binaryRequest(t, h, makeFrame(t, 1, c, other, plain), 403)
	// The development key no longer passes once development is not accepted.
	request(t, h, "POST", "/v1/challenges", "secret", map[string]any{"appId": "app", "eventId": "after-policy", "accountId": "a", "type": "login", "keyId": keyID}, 403)
}

func TestIOSIntegrityRules(t *testing.T) {
	av := func(v any) Signal { return Signal{Status: "available", Value: v} }
	codes := func(signals map[string]Signal) map[string]int {
		signals["platform"] = av("ios")
		out := map[string]int{}
		for _, hit := range environmentHits(Report{Signals: signals}) {
			out[hit.Code] = hit.Points
		}
		return out
	}
	clean := codes(map[string]Signal{
		"jailbreak":        av(map[string]any{"nativePaths": []any{}, "foundationPaths": []any{}, "sandboxEscape": false}),
		"injection":        av(map[string]any{"hookImages": []any{}, "dyldInsert": false, "fridaPort": false}),
		"iosAccessibility": av(map[string]any{"assistiveTouch": true, "switchControl": false}),
		"peripherals":      av(map[string]any{"keyboard": false, "mice": 0.0}),
		"application":      av(map[string]any{"installer": "provisioned", "cryptid": 0.0}),
	})
	if len(clean) != 0 {
		t.Fatal("clean iPhone scored", clean)
	}
	bad := codes(map[string]Signal{
		"jailbreak":        av(map[string]any{"nativePaths": []any{"/var/jb"}, "foundationPaths": []any{}, "sandboxEscape": false}),
		"injection":        av(map[string]any{"hookImages": []any{"/usr/lib/frida-agent.dylib"}, "dyldInsert": false, "fridaPort": true}),
		"iosAccessibility": av(map[string]any{"switchControl": true}),
		"peripherals":      av(map[string]any{"keyboard": false, "mice": 1.0}),
		"application":      av(map[string]any{"installer": "appstore", "cryptid": 0.0}),
	})
	for _, code := range []string{"ios_jailbreak", "ios_layer_mismatch", "ios_injected_library", "frida_default_port", "ios_switch_control", "ios_hid_input", "ios_binary_decrypted"} {
		if bad[code] == 0 {
			t.Error("missing", code, bad)
		}
		if ruleNames[code] == "" {
			t.Error("no name for", code)
		}
	}
}
