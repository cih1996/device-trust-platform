package platform

import (
	"bytes"
	"crypto/ecdh"
	"crypto/ecdsa"
	"crypto/rand"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"reflect"
	"strings"
	"time"
)

type AppSecurity struct {
	RequireRequestBinding bool     `json:"requireRequestBinding"`
	RequireSignedReports  bool     `json:"requireSignedReports"`
	RequireHardware       bool     `json:"requireHardware"`
	SigningDigests        []string `json:"signingDigests"`
	MinimumOSPatch        int      `json:"minimumOsPatch"`
	MaxEnrollmentHours    int      `json:"maxEnrollmentHours"`
}

var errEnrollmentExpired = errors.New("device enrollment expired; enroll a new key")
var errHardwareUpgrade = errors.New("hardware-backed attestation required")

func (s *Server) security(appID string) (AppSecurity, error) {
	result := AppSecurity{MaxEnrollmentHours: 24, SigningDigests: []string{}}
	err := s.Store.get("security", appID, &result)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	return result, err
}
func (s *Server) saveSecurity(w http.ResponseWriter, r *http.Request, appID string) {
	if _, ok := s.app(appID, w); !ok {
		return
	}
	var config AppSecurity
	if err := decode(w, r, &config); err != nil {
		fail(w, 400, err)
		return
	}
	if !config.RequireSignedReports {
		policy, err := s.policy(appID)
		if err != nil {
			fail(w, 500, err)
			return
		}
		if policy.Mode == "enforce" {
			fail(w, 400, errors.New("执行模式需保持签名上报"))
			return
		}
	}
	if config.MaxEnrollmentHours < 1 || config.MaxEnrollmentHours > 168 {
		fail(w, 400, errors.New("设备密钥有效期需为 1 至 168 小时"))
		return
	}
	for i, value := range config.SigningDigests {
		value = strings.ToLower(strings.ReplaceAll(value, ":", ""))
		b, err := hex.DecodeString(value)
		if err != nil || len(b) != 32 {
			fail(w, 400, errors.New("应用证书摘要需为 SHA-256"))
			return
		}
		config.SigningDigests[i] = value
	}
	if config.RequireHardware && (!config.RequireSignedReports || len(config.SigningDigests) == 0) {
		fail(w, 400, errors.New("硬件证明需同时启用签名上报并设置应用证书"))
		return
	}
	if err := s.Store.put("security", appID, config); err != nil {
		fail(w, 500, err)
		return
	}
	reply(w, 200, config)
}

type Challenge struct {
	HardwareRequired bool            `json:"hardwareRequired"`
	ID               string          `json:"id"`
	AppID            string          `json:"appId"`
	EventID          string          `json:"eventId"`
	AccountID        string          `json:"accountId"`
	Type             string          `json:"type"`
	Context          BusinessContext `json:"context"`
	ClientIP         string          `json:"clientIp"`
	KeyID            string          `json:"keyId"`
	Nonce            []byte          `json:"nonce"`
	ServerPublicKey  []byte          `json:"serverPublicKey"`
	ExpiresAt        time.Time       `json:"expiresAt"`
}
type storedChallenge struct {
	Challenge
	ServerPrivateKey []byte `json:"serverPrivateKey"`
}
type deviceKey struct {
	ID                  string        `json:"id"`
	AppID               string        `json:"appId"`
	Chain               []string      `json:"chain"`
	Nonce               []byte        `json:"nonce"`
	EnrollmentChallenge string        `json:"enrollmentChallenge"`
	EnrolledAt          time.Time     `json:"enrolledAt"`
	Proof               hardwareProof `json:"proof"`
}
type enrollment struct {
	KeyID       string        `json:"keyId"`
	PayloadHash string        `json:"payloadHash"`
	Proof       hardwareProof `json:"proof"`
}
type ReportProof struct {
	KeyID       string    `json:"keyId"`
	Level       string    `json:"level"`
	ChallengeID string    `json:"challengeId"`
	EventID     string    `json:"eventId"`
	PayloadHash string    `json:"payloadHash"`
	VerifiedAt  time.Time `json:"verifiedAt"`
	EnrolledAt  time.Time `json:"enrolledAt"`
}

func (s *Server) challenge(w http.ResponseWriter, r *http.Request) {
	var input struct {
		AppID     string          `json:"appId"`
		EventID   string          `json:"eventId"`
		AccountID string          `json:"accountId"`
		Type      string          `json:"type"`
		Context   BusinessContext `json:"context"`
		ClientIP  string          `json:"clientIp"`
		KeyID     string          `json:"keyId"`
	}
	if err := decode(w, r, &input); err != nil {
		fail(w, 400, err)
		return
	}
	app, ok := s.app(input.AppID, w)
	if !ok {
		return
	}
	if !same(digest(bearer(r)), app.KeyHash) {
		fail(w, 401, errors.New("invalid application key"))
		return
	}
	if input.EventID == "" || input.AccountID == "" || input.Type == "" {
		fail(w, 400, errors.New("eventId, accountId and type required"))
		return
	}
	if input.ClientIP != "" {
		ip := net.ParseIP(input.ClientIP)
		if ip == nil {
			fail(w, 400, errors.New("invalid clientIp"))
			return
		}
		input.ClientIP = ip.String()
	}
	security, err := s.security(app.ID)
	if err != nil {
		fail(w, 500, err)
		return
	}
	if (security.RequireRequestBinding || input.Context.RequestHash != "") && !validRequestHash(input.Context.RequestHash) {
		fail(w, 400, errRequestBinding)
		return
	}
	if input.KeyID != "" {
		if _, _, err = s.activeKey(app, input.KeyID, security); err != nil {
			if errors.Is(err, errEnrollmentExpired) || errors.Is(err, errHardwareUpgrade) {
				input.KeyID = ""
			} else {
				fail(w, 403, err)
				return
			}
		}
	}
	private, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		fail(w, 500, err)
		return
	}
	public, err := x509.MarshalPKIXPublicKey(private.PublicKey())
	if err != nil {
		fail(w, 500, err)
		return
	}
	challenge := Challenge{ID: token(), AppID: app.ID, EventID: input.EventID, AccountID: input.AccountID, Type: input.Type, Context: input.Context, ClientIP: input.ClientIP, KeyID: input.KeyID, Nonce: randomBytes(32), ServerPublicKey: public, ExpiresAt: time.Now().UTC().Add(3 * time.Minute)}
	challenge.HardwareRequired = security.RequireHardware
	if err = s.Store.create("challenges", challenge.ID, storedChallenge{challenge, private.Bytes()}); err != nil {
		fail(w, 500, err)
		return
	}
	reply(w, 201, challenge)
}
func (s *Server) activeKey(app App, id string, security AppSecurity) (deviceKey, hardwareProof, error) {
	var key deviceKey
	var revoked json.RawMessage
	if err := s.Store.get("revoked-keys", app.ID+":"+id, &revoked); err == nil {
		return key, hardwareProof{}, errors.New("device key revoked")
	} else if !errors.Is(err, os.ErrNotExist) {
		return key, hardwareProof{}, err
	}
	err := s.Store.get("device-keys", app.ID+":"+id, &key)
	if err != nil {
		return key, hardwareProof{}, err
	}
	if key.AppID != app.ID || key.ID != id {
		return key, hardwareProof{}, errors.New("device key application mismatch")
	}
	var accepted enrollment
	if err = s.Store.get("enrollments", key.EnrollmentChallenge, &accepted); err != nil {
		return key, hardwareProof{}, err
	}
	if accepted.KeyID != id {
		return key, hardwareProof{}, errors.New("device enrollment was not accepted")
	}
	if (security.RequireHardware || key.Proof.Level == "tee" || key.Proof.Level == "strongbox") && time.Since(key.EnrolledAt) > time.Duration(security.MaxEnrollmentHours)*time.Hour {
		return key, hardwareProof{}, errEnrollmentExpired
	}
	proof := key.Proof
	if proof.Level == "tee" || proof.Level == "strongbox" {
		proof, err = s.Trust.verify(key.Chain, key.Nonce, app, security)
		if err != nil {
			return key, proof, err
		}
	}
	if security.RequireHardware && proof.Level != "tee" && proof.Level != "strongbox" {
		return key, proof, errHardwareUpgrade
	}
	return key, proof, nil
}

func (s *Server) revokeKey(w http.ResponseWriter, r *http.Request) {
	var input struct {
		AppID  string `json:"appId"`
		KeyID  string `json:"keyId"`
		Reason string `json:"reason"`
	}
	if err := decode(w, r, &input); err != nil {
		fail(w, 400, err)
		return
	}
	if _, ok := s.app(input.AppID, w); !ok {
		return
	}
	var key deviceKey
	if err := s.Store.get("device-keys", input.AppID+":"+input.KeyID, &key); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fail(w, 404, errors.New("device key not found"))
		} else {
			fail(w, 500, err)
		}
		return
	}
	record := map[string]any{"appId": input.AppID, "keyId": input.KeyID, "reason": input.Reason, "revokedAt": time.Now().UTC()}
	if err := s.Store.create("revoked-keys", input.AppID+":"+input.KeyID, record); errors.Is(err, os.ErrExist) {
		var old json.RawMessage
		if err = s.Store.get("revoked-keys", input.AppID+":"+input.KeyID, &old); err != nil {
			fail(w, 500, err)
			return
		}
		reply(w, 200, old)
	} else if err != nil {
		fail(w, 500, err)
	} else {
		reply(w, 201, record)
	}
}
func (s *Server) secure(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Content-Type") != "application/octet-stream" {
		fail(w, 415, errors.New("binary content-type required"))
		return
	}
	data, err := io.ReadAll(http.MaxBytesReader(w, r.Body, frameLimit))
	if err != nil {
		fail(w, 400, err)
		return
	}
	frame, err := parseFrame(data)
	if err != nil {
		fail(w, 400, err)
		return
	}
	var challenge storedChallenge
	if err = s.Store.get("challenges", frame.ChallengeID, &challenge); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fail(w, 404, errors.New("challenge not found"))
		} else {
			fail(w, 500, err)
		}
		return
	}
	if time.Now().After(challenge.ExpiresAt) {
		fail(w, 410, errors.New("challenge expired"))
		return
	}
	app, ok := s.app(challenge.AppID, w)
	if !ok {
		return
	}
	security, err := s.security(app.ID)
	if err != nil {
		fail(w, 500, err)
		return
	}
	if frame.Kind == 1 {
		s.enroll(w, frame, challenge, app, security)
		return
	}
	key, proof, err := s.activeKey(app, frame.KeyID, security)
	if err != nil {
		fail(w, 403, err)
		return
	}
	if challenge.KeyID != "" && challenge.KeyID != key.ID {
		fail(w, 403, errors.New("challenge device key mismatch"))
		return
	}
	if challenge.KeyID == "" && key.EnrollmentChallenge != challenge.ID {
		fail(w, 403, errors.New("challenge is bound to another enrollment"))
		return
	}
	certs, err := parseCertificateChain(key.Chain)
	if err != nil {
		fail(w, 500, err)
		return
	}
	if err = frame.verify(certs[0].PublicKey.(*ecdsa.PublicKey)); err != nil {
		fail(w, 403, err)
		return
	}
	payload, err := frame.decrypt(challenge.ServerPrivateKey, challenge.Nonce)
	if err != nil {
		fail(w, 403, err)
		return
	}
	var input reportInput
	if err = decodePayload(payload, &input); err != nil {
		fail(w, 400, err)
		return
	}
	if input.AppID != app.ID || input.Package != app.Package || len(input.Signals) == 0 {
		fail(w, 403, errors.New("report application or package mismatch"))
		return
	}
	if err = validateSignals(input.Signals); err != nil {
		fail(w, 400, err)
		return
	}
	input.Signals["expectedPackage"] = Signal{Status: "available", Value: true}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		fail(w, 500, err)
		return
	}
	report := Report{ID: digest("secure:" + challenge.ID), AppID: app.ID, InstallationID: "key:" + key.ID, Package: app.Package, Signals: input.Signals, ObservedIP: ip, ReceivedAt: time.Now().UTC(), Proof: &ReportProof{KeyID: key.ID, Level: proof.Level, ChallengeID: challenge.ID, EventID: challenge.EventID, PayloadHash: digest(string(payload)), VerifiedAt: time.Now().UTC(), EnrolledAt: key.Proof.VerifiedAt}}
	deviceID, err := s.registerDevice(app.ID, key.ID)
	if err != nil {
		fail(w, 500, err)
		return
	}
	report.DeviceID = deviceID
	if s.Resources != nil {
		report.Resources, err = s.Resources.Assess(report)
		if err != nil {
			fail(w, 500, err)
			return
		}
	}
	if err = s.Store.create("reports", report.ID, report); errors.Is(err, os.ErrExist) {
		var old Report
		if err = s.Store.get("reports", report.ID, &old); err != nil {
			fail(w, 500, err)
			return
		}
		if old.Proof == nil || old.Proof.KeyID != key.ID || old.Proof.PayloadHash != report.Proof.PayloadHash {
			fail(w, 409, errors.New("challenge already used for another report"))
			return
		}
		reply(w, 200, map[string]string{"reportId": old.ID, "deviceId": deviceID, "keyId": key.ID, "proofLevel": old.Proof.Level})
		return
	} else if err != nil {
		fail(w, 500, err)
		return
	}
	reply(w, 201, map[string]string{"reportId": report.ID, "deviceId": deviceID, "keyId": key.ID, "proofLevel": proof.Level})
}
func (s *Server) enroll(w http.ResponseWriter, frame secureFrame, challenge storedChallenge, app App, security AppSecurity) {
	if challenge.KeyID != "" {
		fail(w, 403, errors.New("enrollment requires a new-key challenge"))
		return
	}
	payload, err := frame.decrypt(challenge.ServerPrivateKey, challenge.Nonce)
	if err != nil {
		fail(w, 403, err)
		return
	}
	var input struct {
		Chain []string `json:"certificateChain"`
	}
	if err = decodePayload(payload, &input); err != nil {
		fail(w, 400, err)
		return
	}
	certs, err := parseCertificateChain(input.Chain)
	if err != nil {
		fail(w, 400, err)
		return
	}
	id := certificateKeyID(certs[0])
	if id != frame.KeyID {
		fail(w, 403, errors.New("enrollment key digest mismatch"))
		return
	}
	if err = frame.verify(certs[0].PublicKey.(*ecdsa.PublicKey)); err != nil {
		fail(w, 403, err)
		return
	}
	proof := hardwareProof{Level: "signature_only", VerifiedAt: time.Now().UTC()}
	if security.RequireHardware {
		proof, err = s.Trust.verify(input.Chain, challenge.Nonce, app, security)
		if err != nil {
			fail(w, 403, err)
			return
		}
	}
	accepted := enrollment{id, digest(string(payload)), proof}
	if err = s.Store.create("enrollments", challenge.ID, accepted); errors.Is(err, os.ErrExist) {
		var old enrollment
		if err = s.Store.get("enrollments", challenge.ID, &old); err != nil {
			fail(w, 500, err)
			return
		}
		if old.KeyID != id || old.PayloadHash != accepted.PayloadHash {
			fail(w, 409, errors.New("challenge enrollment already used"))
			return
		}
		proof = old.Proof
	} else if err != nil {
		fail(w, 500, err)
		return
	}
	key := deviceKey{id, app.ID, input.Chain, challenge.Nonce, challenge.ID, time.Now().UTC(), proof}
	if err = s.Store.create("device-keys", app.ID+":"+id, key); err != nil && !errors.Is(err, os.ErrExist) {
		fail(w, 500, err)
		return
	}
	reply(w, 201, map[string]string{"keyId": id, "proofLevel": proof.Level})
}
func decodePayload(data []byte, v any) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(v); err != nil {
		return err
	}
	var extra any
	return requireEOF(decoder.Decode(&extra))
}
func requireEOF(err error) error {
	if err == io.EOF {
		return nil
	}
	if err != nil {
		return err
	}
	return errors.New("multiple payload objects")
}
func (s *Server) validateReportBinding(report Report, input eventInput) error {
	security, err := s.security(input.AppID)
	if err != nil {
		return err
	}
	if report.Proof == nil {
		if security.RequireSignedReports {
			return errors.New("signed device report required")
		}
		return nil
	}
	var challenge storedChallenge
	if err = s.Store.get("challenges", report.Proof.ChallengeID, &challenge); err != nil {
		return err
	}
	if challenge.AppID != input.AppID || challenge.EventID != input.BusinessID || challenge.AccountID != input.AccountID || challenge.Type != input.Type || challenge.ClientIP != input.ClientIP || !reflect.DeepEqual(challenge.Context, input.Context) {
		return errors.New("report is bound to a different business request")
	}
	if security.RequireHardware && report.Proof.Level != "tee" && report.Proof.Level != "strongbox" {
		return errors.New("hardware-backed report required")
	}
	return nil
}
