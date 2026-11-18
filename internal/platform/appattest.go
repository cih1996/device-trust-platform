package platform

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
)

// iOS devices enroll a Secure Enclave P-256 signing key, which signs DTP4
// frames exactly like an Android Keystore key. App Attest binds that key to a
// genuine Apple device running the app identified by Team ID and bundle ID:
// the attestation's client data covers the challenge nonce and the signing
// key, and every report carries an assertion with a growing counter.
// https://developer.apple.com/documentation/devicecheck/validating-apps-that-connect-to-your-server

// appleAppAttestRootPEM is Apple App Attestation Root CA, SHA-256
// 1CB9823BA28BA6AD2D33A006941DE2AE4F513EF1D4E831B9F7E0FA7B6242C932.
const appleAppAttestRootPEM = `-----BEGIN CERTIFICATE-----
MIICITCCAaegAwIBAgIQC/O+DvHN0uD7jG5yH2IXmDAKBggqhkjOPQQDAzBSMSYw
JAYDVQQDDB1BcHBsZSBBcHAgQXR0ZXN0YXRpb24gUm9vdCBDQTETMBEGA1UECgwK
QXBwbGUgSW5jLjETMBEGA1UECAwKQ2FsaWZvcm5pYTAeFw0yMDAzMTgxODMyNTNa
Fw00NTAzMTUwMDAwMDBaMFIxJjAkBgNVBAMMHUFwcGxlIEFwcCBBdHRlc3RhdGlv
biBSb290IENBMRMwEQYDVQQKDApBcHBsZSBJbmMuMRMwEQYDVQQIDApDYWxpZm9y
bmlhMHYwEAYHKoZIzj0CAQYFK4EEACIDYgAERTHhmLW07ATaFQIEVwTtT4dyctdh
NbJhFs/Ii2FdCgAHGbpphY3+d8qjuDngIN3WVhQUBHAoMeQ/cLiP1sOUtgjqK9au
Yen1mMEvRq9Sk3Jm5X8U62H+xTD3FE9TgS41o0IwQDAPBgNVHRMBAf8EBTADAQH/
MB0GA1UdDgQWBBSskRBTM72+aEH/pwyp5frq5eWKoTAOBgNVHQ8BAf8EBAMCAQYw
CgYIKoZIzj0EAwMDaAAwZQIwQgFGnByvsiVbpTKwSga0kP0e8EeDS4+sQmTvb7vn
53O5+FRXgeLhpJ06ysC5PrOyAjEAp5U4xDgEgllF7En3VcE3iexZZtKeYnpqtijV
oyFraWVIyd/dganmrduC1bmTBGwD
-----END CERTIFICATE-----`

// appAttestRoots is replaced in tests.
var appAttestRoots = func() *x509.CertPool {
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM([]byte(appleAppAttestRootPEM)) {
		panic("invalid Apple App Attestation root")
	}
	return pool
}()

var appAttestNonceOID = asn1.ObjectIdentifier{1, 2, 840, 113635, 100, 8, 2}

var (
	aaguidProduction  = append([]byte("appattest"), make([]byte, 7)...)
	aaguidDevelopment = []byte("appattestdevelop")
	iosAppIDPattern   = regexp.MustCompile(`^[A-Z0-9]{10}\.[A-Za-z0-9][A-Za-z0-9.-]*$`)
)

var (
	errAttestDevelopment = errors.New("App Attest development environment not accepted")
	errAssertionCounter  = errors.New("App Attest assertion counter did not increase")
)

const (
	appAttestLabel = "device-trust/appattest/v1"
	assertionLabel = "device-trust/assertion/v1"
)

// appAttestKey is the attested App Attest key of an enrolled iOS device.
type appAttestKey struct {
	KeyID       []byte `json:"keyId"`
	PublicKey   []byte `json:"publicKey"` // uncompressed P-256 point
	AppIdentity string `json:"appIdentity"`
	Receipt     []byte `json:"receipt,omitempty"`
}

// appAttestCounter is the last assertion counter accepted for a key; the
// challenge lets an identical retry of the same frame through.
type appAttestCounter struct {
	Counter     uint32 `json:"counter"`
	ChallengeID string `json:"challengeId"`
}

func appAttestClientData(nonce, signingKey []byte) []byte {
	h := sha256.New()
	h.Write([]byte(appAttestLabel))
	h.Write(nonce)
	h.Write(signingKey)
	return h.Sum(nil)
}

func assertionClientData(nonce []byte, keyID string) []byte {
	id, _ := hex.DecodeString(keyID)
	h := sha256.New()
	h.Write([]byte(assertionLabel))
	h.Write(nonce)
	h.Write(id)
	return h.Sum(nil)
}

// verifyAppAttest checks an attestation object against Apple's root and the
// app identities configured for the app.
func verifyAppAttest(attestation, keyID, clientDataHash []byte, security AppSecurity, now time.Time) (appAttestKey, hardwareProof, error) {
	var key appAttestKey
	var proof hardwareProof
	raw, err := decodeCBOR(attestation)
	if err != nil {
		return key, proof, err
	}
	object, ok := raw.(map[string]any)
	if !ok || object["fmt"] != "apple-appattest" {
		return key, proof, errors.New("not an apple-appattest object")
	}
	statement, ok := object["attStmt"].(map[string]any)
	if !ok {
		return key, proof, errors.New("attestation statement missing")
	}
	authData, err := cborBytes(object, "authData")
	if err != nil {
		return key, proof, err
	}
	x5c, _ := statement["x5c"].([]any)
	if len(x5c) < 2 {
		return key, proof, errors.New("attestation certificate chain missing")
	}
	var certs []*x509.Certificate
	for _, item := range x5c {
		der, ok := item.([]byte)
		if !ok {
			return key, proof, errors.New("invalid attestation certificate")
		}
		cert, err := x509.ParseCertificate(der)
		if err != nil {
			return key, proof, err
		}
		certs = append(certs, cert)
	}
	intermediates := x509.NewCertPool()
	for _, cert := range certs[1:] {
		intermediates.AddCert(cert)
	}
	verified, err := certs[0].Verify(x509.VerifyOptions{Roots: appAttestRoots, Intermediates: intermediates, CurrentTime: now, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}})
	if err != nil {
		return key, proof, err
	}
	nonce := sha256.Sum256(append(append([]byte{}, authData...), clientDataHash...))
	certNonce, err := appAttestCertNonce(certs[0])
	if err != nil {
		return key, proof, err
	}
	if !bytes.Equal(certNonce, nonce[:]) {
		return key, proof, errors.New("attestation nonce mismatch")
	}
	public, ok := certs[0].PublicKey.(*ecdsa.PublicKey)
	if !ok || public.Curve != elliptic.P256() {
		return key, proof, errors.New("attested key is not P-256")
	}
	point, err := public.Bytes()
	if err != nil {
		return key, proof, err
	}
	pointHash := sha256.Sum256(point)
	if !bytes.Equal(pointHash[:], keyID) {
		return key, proof, errors.New("attested key identifier mismatch")
	}
	if len(authData) < 55 {
		return key, proof, errors.New("authenticator data too short")
	}
	identity := matchAppIdentity(authData[:32], security.IOSAppIDs)
	if identity == "" {
		return key, proof, fmt.Errorf("%w: App ID not configured", errAttestApp)
	}
	if binary.BigEndian.Uint32(authData[33:37]) != 0 {
		return key, proof, errors.New("attestation counter must be zero")
	}
	environment := ""
	switch aaguid := authData[37:53]; {
	case bytes.Equal(aaguid, aaguidProduction):
		environment = "production"
	case bytes.Equal(aaguid, aaguidDevelopment):
		environment = "development"
	default:
		return key, proof, errors.New("unknown App Attest environment")
	}
	idLength := int(binary.BigEndian.Uint16(authData[53:55]))
	if len(authData) < 55+idLength || !bytes.Equal(authData[55:55+idLength], keyID) {
		return key, proof, errors.New("attested credential identifier mismatch")
	}
	if environment == "development" && !security.AllowAppAttestDevelopment {
		return key, proof, errAttestDevelopment
	}
	receipt, _ := statement["receipt"].([]byte)
	rootHash := sha256.Sum256(verified[0][len(verified[0])-1].Raw)
	key = appAttestKey{KeyID: keyID, PublicKey: point, AppIdentity: identity, Receipt: receipt}
	proof = hardwareProof{Level: "app_attest", BootLocked: true, VerifiedAt: now, RootSHA256: hex.EncodeToString(rootHash[:]), AppIdentity: identity, Environment: environment}
	return key, proof, nil
}

// appAttestCertNonce reads the nonce extension: SEQUENCE { [1] OCTET STRING }.
func appAttestCertNonce(cert *x509.Certificate) ([]byte, error) {
	for _, ext := range cert.Extensions {
		if !ext.Id.Equal(appAttestNonceOID) {
			continue
		}
		var seq asn1.RawValue
		if rest, err := asn1.Unmarshal(ext.Value, &seq); err != nil || len(rest) != 0 || seq.Tag != asn1.TagSequence {
			return nil, errors.New("invalid nonce extension")
		}
		var tagged asn1.RawValue
		if rest, err := asn1.Unmarshal(seq.Bytes, &tagged); err != nil || len(rest) != 0 || tagged.Class != asn1.ClassContextSpecific || tagged.Tag != 1 {
			return nil, errors.New("invalid nonce extension")
		}
		var nonce []byte
		if rest, err := asn1.Unmarshal(tagged.Bytes, &nonce); err != nil || len(rest) != 0 {
			return nil, errors.New("invalid nonce extension")
		}
		return nonce, nil
	}
	return nil, errors.New("attestation nonce extension missing")
}

func matchAppIdentity(rpIDHash []byte, ids []string) string {
	for _, id := range ids {
		sum := sha256.Sum256([]byte(id))
		if bytes.Equal(sum[:], rpIDHash) {
			return id
		}
	}
	return ""
}

// verifyAssertion checks a report's assertion and returns its counter.
func verifyAssertion(key appAttestKey, assertion, clientDataHash []byte) (uint32, error) {
	raw, err := decodeCBOR(assertion)
	if err != nil {
		return 0, err
	}
	object, ok := raw.(map[string]any)
	if !ok {
		return 0, errors.New("invalid assertion")
	}
	signature, err := cborBytes(object, "signature")
	if err != nil {
		return 0, err
	}
	authData, err := cborBytes(object, "authenticatorData")
	if err != nil {
		return 0, err
	}
	if len(authData) < 37 {
		return 0, errors.New("assertion authenticator data too short")
	}
	if rp := sha256.Sum256([]byte(key.AppIdentity)); !bytes.Equal(authData[:32], rp[:]) {
		return 0, fmt.Errorf("%w: assertion App ID", errAttestApp)
	}
	public, err := ecdsa.ParseUncompressedPublicKey(elliptic.P256(), key.PublicKey)
	if err != nil {
		return 0, err
	}
	nonce := sha256.Sum256(append(append([]byte{}, authData...), clientDataHash...))
	digest := sha256.Sum256(nonce[:])
	if !ecdsa.VerifyASN1(public, digest[:], signature) {
		return 0, errors.New("invalid assertion signature")
	}
	return binary.BigEndian.Uint32(authData[33:37]), nil
}

// checkAssertion verifies a report assertion and advances the key's counter.
// The first error is the assertion's fault; the second is a storage failure.
func (s *Server) checkAssertion(key deviceKey, challenge storedChallenge, assertion []byte) (error, error) {
	if len(assertion) == 0 {
		return errors.New("App Attest assertion missing"), nil
	}
	counter, err := verifyAssertion(*key.AppAttest, assertion, assertionClientData(challenge.Nonce, key.ID))
	if err != nil {
		return err, nil
	}
	id := key.AppID + ":" + key.ID
	var last appAttestCounter
	if err = s.Store.get("appattest-counters", id, &last); err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if counter == last.Counter && challenge.ID == last.ChallengeID {
		return nil, nil
	}
	if counter <= last.Counter {
		return errAssertionCounter, nil
	}
	return nil, s.Store.put("appattest-counters", id, appAttestCounter{counter, challenge.ID})
}

// assertionIssue names the scoring category of an assertion failure.
func assertionIssue(err error) string {
	switch {
	case errors.Is(err, errAssertionCounter):
		return "counter_replay"
	case errors.Is(err, errAttestApp):
		return "app_mismatch"
	}
	return "assertion_invalid"
}

// normalizeIOSAppIDs validates "TEAMID.bundle.id" entries.
func normalizeIOSAppIDs(ids []string) ([]string, error) {
	out := []string{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id == "" {
			continue
		}
		if !iosAppIDPattern.MatchString(id) {
			return nil, errors.New("iOS App ID 需为 团队ID.包名，例如 ABCDE12345.com.example.app")
		}
		out = append(out, id)
	}
	return out, nil
}

// iosBundleAllowed reports whether a bundle ID belongs to a configured App ID.
func iosBundleAllowed(security AppSecurity, bundle string) bool {
	for _, id := range security.IOSAppIDs {
		if len(id) > 11 && id[11:] == bundle {
			return true
		}
	}
	return false
}

// iosSigningKey parses an enrolled Secure Enclave key (SubjectPublicKeyInfo).
func iosSigningKey(spki []byte) (*ecdsa.PublicKey, error) {
	parsed, err := x509.ParsePKIXPublicKey(spki)
	if err != nil {
		return nil, err
	}
	public, ok := parsed.(*ecdsa.PublicKey)
	if !ok || public.Curve != elliptic.P256() {
		return nil, errors.New("signing key is not P-256")
	}
	return public, nil
}
