// Package businessguard belongs in the authenticated BUSINESS BACKEND, never in an APK.
package businessguard

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type Client struct {
	BaseURL string
	AppID   string
	AppKey  string
	HTTP    *http.Client
}

// HashRequest binds a backend-normalized business intent. Use the same account,
// route and complete intent when preparing the challenge and authorizing effect.
func HashRequest(method, path, account string, body []byte) string {
	hash := sha256.Sum256(body)
	value := struct {
		Version  string `json:"v"`
		Method   string `json:"method"`
		Path     string `json:"path"`
		Account  string `json:"account"`
		BodyHash string `json:"bodyHash"`
	}{"dtp-request-1", strings.ToUpper(method), path, account, hex.EncodeToString(hash[:])}
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:])
}

type Permit struct {
	AppID       string    `json:"appId"`
	EventID     string    `json:"eventId"`
	OperationID string    `json:"operationId"`
	RequestHash string    `json:"requestHash"`
	Token       string    `json:"token"`
	Mode        string    `json:"mode"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

func (c Client) post(ctx context.Context, path string, input, output any) error {
	if !strings.HasPrefix(c.BaseURL, "https://") {
		return fmt.Errorf("business guard requires HTTPS")
	}
	data, err := json.Marshal(input)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, "POST", strings.TrimRight(c.BaseURL, "/")+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.AppKey)
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	copy := *client
	copy.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := copy.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 128<<10))
	if err != nil {
		return err
	}
	if response.StatusCode != 200 && response.StatusCode != 201 {
		return fmt.Errorf("risk HTTP %d: %s", response.StatusCode, string(body))
	}
	return json.Unmarshal(body, output)
}

// Authorize must run after account authentication and normalization of the REAL
// request. A successful permit still requires a unique operation ID in the
// business database and the same idempotency key at the payment provider.
func (c Client) Authorize(ctx context.Context, eventID, operationID, requestHash string) (Permit, error) {
	var permit Permit
	if len(requestHash) != 64 {
		return permit, fmt.Errorf("normalized business request SHA-256 required")
	}
	err := c.post(ctx, "/v1/decisions/claim", map[string]string{"appId": c.AppID, "eventId": eventID, "operationId": operationID, "requestHash": requestHash}, &permit)
	if err != nil {
		return permit, err
	}
	identity := sha256.Sum256([]byte(c.AppID + ":" + eventID))
	if permit.EventID != hex.EncodeToString(identity[:]) || permit.AppID != c.AppID || permit.OperationID != operationID || permit.RequestHash != requestHash || permit.Mode != "enforce" || permit.Token == "" {
		return permit, fmt.Errorf("risk execution permit binding/mode mismatch")
	}
	if !time.Now().Before(permit.ExpiresAt) {
		return permit, fmt.Errorf("risk execution permit expired")
	}
	return permit, nil
}

// Receipt can recover a previously committed effect after the original permit
// expires. The business database must supply the stored permit and result.
func (c Client) Receipt(ctx context.Context, eventID, receiptID, status, reference string, permit Permit) error {
	var response map[string]any
	return c.post(ctx, "/v1/executions", map[string]string{"appId": c.AppID, "eventId": eventID, "operationId": permit.OperationID, "token": permit.Token, "receiptId": receiptID, "status": status, "reference": reference}, &response)
}

// Verdict is the platform's assessment of one device. Score and level combine
// the device environment with recent business risk; Linked covers the same
// handset in your other apps.
type Verdict struct {
	ReportID      string    `json:"reportId"`
	DeviceID      string    `json:"deviceId"`
	LinkID        string    `json:"linkId"`
	KeyStatus     string    `json:"keyStatus"`
	RiskScore     int       `json:"riskScore"`
	RiskLevel     string    `json:"riskLevel"`
	EnvScore      int       `json:"envScore"`
	BusinessScore int       `json:"businessScore"`
	ObservedIP    string    `json:"observedIp"`
	ReportedAt    time.Time `json:"reportedAt"`
	Factors       []struct {
		Code   string `json:"code"`
		Points int    `json:"points"`
		Source string `json:"source"`
	} `json:"factors"`
	App struct {
		Package     string `json:"package"`
		Signer      string `json:"signer"`
		Attestation string `json:"attestation"`
		Issue       string `json:"issue"`
	} `json:"app"`
	Linked struct {
		Devices      int `json:"devices"`
		Apps         int `json:"apps"`
		MaxRiskScore int `json:"maxRiskScore"`
		HighRisk     int `json:"highRisk"`
	} `json:"linked"`
}

// Verdict assesses a fresh signed report. eventID and accountID must be the
// ones the backend used when it requested the report's challenge.
func (c Client) Verdict(ctx context.Context, reportID, eventID, accountID string) (Verdict, error) {
	var v Verdict
	err := c.post(ctx, "/v1/verdicts", map[string]string{"appId": c.AppID, "reportId": reportID, "eventId": eventID, "accountId": accountID}, &v)
	if err == nil && v.ReportID != reportID {
		err = fmt.Errorf("verdict report mismatch")
	}
	return v, err
}

// Device returns the latest known verdict of a registered device ID. It is
// history, not proof the device is present; use Verdict for live requests.
func (c Client) Device(ctx context.Context, deviceID string) (Verdict, error) {
	var v Verdict
	err := c.post(ctx, "/v1/devices/query", map[string]string{"appId": c.AppID, "deviceId": deviceID}, &v)
	if err == nil && v.DeviceID != deviceID {
		err = fmt.Errorf("verdict device mismatch")
	}
	return v, err
}
