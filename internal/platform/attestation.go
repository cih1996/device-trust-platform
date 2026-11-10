package platform

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/sha256"
	"crypto/x509"
	"encoding/asn1"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

var attestationOID = asn1.ObjectIdentifier{1, 3, 6, 1, 4, 1, 11129, 2, 1, 17}

type AttestationVerifier struct {
	Roots          *x509.CertPool
	RevocationFile string
}

func LoadAttestationVerifier(rootsFile, revocationsFile string) (*AttestationVerifier, error) {
	b, err := os.ReadFile(rootsFile)
	if err != nil {
		return nil, err
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(b) {
		return nil, errors.New("no trusted attestation roots in PEM bundle")
	}
	return &AttestationVerifier{roots, revocationsFile}, nil
}

type hardwareProof struct {
	Level      string    `json:"level"`
	BootLocked bool      `json:"bootLocked"`
	BootState  int       `json:"bootState"`
	OSPatch    int       `json:"osPatch"`
	VerifiedAt time.Time `json:"verifiedAt"`
	RootSHA256 string    `json:"rootSha256"`
}
type keyDescription struct {
	Version     int
	Security    asn1.Enumerated
	KeyVersion  int
	KeySecurity asn1.Enumerated
	Challenge   []byte
	UniqueID    []byte
	Software    asn1.RawValue
	Hardware    asn1.RawValue
}
type rootOfTrust struct {
	Key    []byte
	Locked bool
	State  asn1.Enumerated
	Hash   []byte `asn1:"optional"`
}
type packageInfo struct {
	Name    []byte
	Version int64
}
type applicationIdentity struct {
	Packages []packageInfo `asn1:"set"`
	Digests  [][]byte      `asn1:"set"`
}

func unmarshalDER(data []byte, v any) error {
	rest, err := asn1.Unmarshal(data, v)
	if err != nil {
		return err
	}
	if len(rest) != 0 {
		return errors.New("trailing ASN.1 data")
	}
	return nil
}
func authorizationFields(raw asn1.RawValue) (map[int][]byte, error) {
	if raw.Tag != asn1.TagSequence || raw.Class != 0 {
		return nil, errors.New("invalid authorization sequence")
	}
	result := map[int][]byte{}
	data := raw.Bytes
	for len(data) > 0 {
		var field asn1.RawValue
		rest, err := asn1.Unmarshal(data, &field)
		if err != nil {
			return nil, err
		}
		if field.Class != 2 || !field.IsCompound {
			return nil, errors.New("invalid explicit authorization tag")
		}
		if _, exists := result[field.Tag]; exists {
			return nil, errors.New("duplicate authorization tag")
		}
		result[field.Tag] = field.Bytes
		data = rest
	}
	return result, nil
}
func parseCertificateChain(chain []string) ([]*x509.Certificate, error) {
	if len(chain) == 0 || len(chain) > 8 {
		return nil, errors.New("certificate chain must contain 1 to 8 certificates")
	}
	certs := make([]*x509.Certificate, len(chain))
	for i, value := range chain {
		der, err := base64.StdEncoding.DecodeString(value)
		if err != nil {
			return nil, err
		}
		certs[i], err = x509.ParseCertificate(der)
		if err != nil {
			return nil, err
		}
	}
	key, ok := certs[0].PublicKey.(*ecdsa.PublicKey)
	if !ok || key.Curve != elliptic.P256() {
		return nil, errors.New("signing key must be ECDSA P-256")
	}
	return certs, nil
}
func certificateKeyID(cert *x509.Certificate) string {
	sum := sha256.Sum256(cert.RawSubjectPublicKeyInfo)
	return hex.EncodeToString(sum[:])
}
func (v *AttestationVerifier) verify(chain []string, nonce []byte, app App, security AppSecurity) (hardwareProof, error) {
	var proof hardwareProof
	if v == nil {
		return proof, errors.New("attestation roots and revocations are not configured")
	}
	certs, err := parseCertificateChain(chain)
	if err != nil {
		return proof, err
	}
	intermediates := x509.NewCertPool()
	for _, cert := range certs[1:] {
		intermediates.AddCert(cert)
	}
	now := time.Now().UTC()
	verified, err := certs[0].Verify(x509.VerifyOptions{Roots: v.Roots, Intermediates: intermediates, CurrentTime: now, KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageAny}})
	if err != nil {
		return proof, err
	}
	// Only accept a signing leaf directly carrying the hardware-issued extension.
	// A parent extension means an attacker could have appended an untrusted signing leaf.
	trustedChain := verified[0]
	var extension []byte
	for i := len(trustedChain) - 1; i >= 0; i-- {
		for _, ext := range trustedChain[i].Extensions {
			if ext.Id.Equal(attestationOID) {
				if i != 0 {
					return proof, errors.New("attestation extension belongs to a parent key; delegated leaf unsupported")
				}
				if extension != nil {
					return proof, errors.New("duplicate attestation extension")
				}
				extension = ext.Value
			}
		}
	}
	if extension == nil {
		return proof, errors.New("key attestation extension missing")
	}
	info, err := os.Stat(v.RevocationFile)
	if err != nil {
		return proof, err
	}
	age := now.Sub(info.ModTime())
	if age < -5*time.Minute || age > 24*time.Hour {
		return proof, errors.New("attestation revocation snapshot must be refreshed within 24 hours")
	}
	revocations, err := os.ReadFile(v.RevocationFile)
	if err != nil {
		return proof, err
	}
	var status struct {
		Entries map[string]struct {
			Status string `json:"status"`
			Reason string `json:"reason"`
		} `json:"entries"`
	}
	if err = json.Unmarshal(revocations, &status); err != nil {
		return proof, err
	}
	if status.Entries == nil {
		return proof, errors.New("revocation snapshot has no entries object")
	}
	for _, cert := range trustedChain {
		if entry, found := status.Entries[strings.ToLower(cert.SerialNumber.Text(16))]; found {
			return proof, fmt.Errorf("attestation certificate %s: %s %s", cert.SerialNumber.Text(16), entry.Status, entry.Reason)
		}
	}
	var desc keyDescription
	if err = unmarshalDER(extension, &desc); err != nil {
		return proof, err
	}
	if !bytes.Equal(desc.Challenge, nonce) {
		return proof, errors.New("attestation challenge mismatch")
	}
	if (desc.Security != 1 && desc.Security != 2) || (desc.KeySecurity != 1 && desc.KeySecurity != 2) {
		return proof, errors.New("attestation key is not TEE or StrongBox backed")
	}
	software, err := authorizationFields(desc.Software)
	if err != nil {
		return proof, err
	}
	hardware, err := authorizationFields(desc.Hardware)
	if err != nil {
		return proof, err
	}
	var boot rootOfTrust
	if err = unmarshalDER(hardware[704], &boot); err != nil {
		return proof, err
	}
	if !boot.Locked || boot.State != 0 {
		return proof, errors.New("verified boot and locked bootloader required")
	}
	var origin int
	if err = unmarshalDER(hardware[702], &origin); err != nil {
		return proof, err
	}
	if origin != 0 {
		return proof, errors.New("hardware-generated signing key required")
	}
	var purposes []int
	if err = unmarshalSet(hardware[1], &purposes); err != nil {
		return proof, err
	}
	if !containsInt(purposes, 2) {
		return proof, errors.New("key is not authorized for signing")
	}
	var digests []int
	if err = unmarshalSet(hardware[5], &digests); err != nil {
		return proof, err
	}
	if !containsInt(digests, 4) {
		return proof, errors.New("SHA-256 key authorization required")
	}
	var identityBytes []byte
	if err = unmarshalDER(software[709], &identityBytes); err != nil {
		return proof, err
	}
	var identity applicationIdentity
	if err = unmarshalDER(identityBytes, &identity); err != nil {
		return proof, err
	}
	if len(identity.Packages) != 1 || string(identity.Packages[0].Name) != app.Package {
		return proof, errors.New("attested application package mismatch")
	}
	if len(identity.Digests) == 0 || len(security.SigningDigests) == 0 {
		return proof, errors.New("application signing certificate allowlist required")
	}
	for _, digest := range identity.Digests {
		allowed := false
		for _, expected := range security.SigningDigests {
			if hex.EncodeToString(digest) == expected {
				allowed = true
			}
		}
		if !allowed {
			return proof, errors.New("attested application signing certificate mismatch")
		}
	}
	patch := 0
	if value := hardware[706]; len(value) > 0 {
		if err = unmarshalDER(value, &patch); err != nil {
			return proof, err
		}
	}
	if patch < security.MinimumOSPatch {
		return proof, errors.New("attested OS patch is older than application requirement")
	}
	rootHash := sha256.Sum256(trustedChain[len(trustedChain)-1].Raw)
	proof = hardwareProof{Level: "tee", BootLocked: true, BootState: 0, OSPatch: patch, VerifiedAt: now, RootSHA256: hex.EncodeToString(rootHash[:])}
	if desc.KeySecurity == 2 {
		proof.Level = "strongbox"
	}
	return proof, nil
}
func containsInt(values []int, value int) bool {
	for _, x := range values {
		if x == value {
			return true
		}
	}
	return false
}
func unmarshalSet(data []byte, v any) error {
	rest, err := asn1.UnmarshalWithParams(data, v, "set")
	if err != nil {
		return err
	}
	if len(rest) > 0 {
		return errors.New("trailing authorization set data")
	}
	return nil
}

// Used by the trust sync CLI. Roots are never accepted from an Android request.
func RootsJSONToPEM(data []byte) ([]byte, error) {
	var roots []string
	if err := json.Unmarshal(data, &roots); err != nil {
		return nil, err
	}
	if len(roots) == 0 {
		return nil, errors.New("root list empty")
	}
	var out []byte
	for _, root := range roots {
		block, rest := pem.Decode([]byte(root))
		if block == nil || len(bytes.TrimSpace(rest)) > 0 {
			return nil, errors.New("invalid root PEM")
		}
		if _, err := x509.ParseCertificate(block.Bytes); err != nil {
			return nil, err
		}
		out = append(out, []byte(root+"\n")...)
	}
	return out, nil
}
