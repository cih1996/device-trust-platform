package platform

import (
	"bytes"
	"crypto/sha256"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// DemoBusiness is an isolated reference adapter with a durable demonstration
// ledger. A real integration supplies its own account and payment providers.
type DemoConfig struct {
	PlatformURL string `json:"platformUrl"`
	AppID       string `json:"appId"`
	AppKey      string `json:"appKey"`
	Password    string `json:"password"`
	CAFile      string `json:"caFile"`
	Data        string `json:"data"`
}
type DemoBusiness struct {
	Config DemoConfig
	Store  *Store
	Client *http.Client
}
type demoSession struct {
	AccountID string    `json:"accountId"`
	ExpiresAt time.Time `json:"expiresAt"`
}
type demoPending struct {
	EventID   string          `json:"eventId"`
	AccountID string          `json:"accountId"`
	Type      string          `json:"type"`
	Context   BusinessContext `json:"context"`
	ClientIP  string          `json:"clientIp"`
	CreatedAt time.Time       `json:"createdAt"`
}
type demoOperation struct {
	EventID   string    `json:"eventId"`
	AccountID string    `json:"accountId"`
	Type      string    `json:"type"`
	ReportID  string    `json:"reportId"`
	CreatedAt time.Time `json:"createdAt"`
}
type upstreamError struct {
	Status int
	Body   string
}

func (e *upstreamError) Error() string { return fmt.Sprintf("platform HTTP %d: %s", e.Status, e.Body) }
func NewDemoBusiness(config DemoConfig) (*DemoBusiness, error) {
	if config.AppID == "" || config.AppKey == "" || len(config.Password) < 16 {
		return nil, errors.New("demo application credentials and a 16-character password required")
	}
	ca, err := os.ReadFile(config.CAFile)
	if err != nil {
		return nil, err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		return nil, errors.New("invalid demo server CA")
	}
	for _, dir := range []string{"sessions", "pending", "operations", "report-bindings"} {
		if err = os.MkdirAll(filepath.Join(config.Data, dir), 0700); err != nil {
			return nil, err
		}
	}
	client := &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &DemoBusiness{config, &Store{Root: config.Data}, client}, nil
}
func (d *DemoBusiness) call(path string, input, output any) error {
	data, err := json.Marshal(input)
	if err != nil {
		return err
	}
	req, err := http.NewRequest("POST", d.Config.PlatformURL+path, bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+d.Config.AppKey)
	req.Header.Set("Content-Type", "application/json")
	response, err := d.Client.Do(req)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	body, err := io.ReadAll(io.LimitReader(response.Body, 512<<10))
	if err != nil {
		return err
	}
	if response.StatusCode != 200 && response.StatusCode != 201 {
		return &upstreamError{response.StatusCode, string(body)}
	}
	return json.Unmarshal(body, output)
}
func (d *DemoBusiness) session(w http.ResponseWriter, r *http.Request) (demoSession, bool) {
	var session demoSession
	if err := d.Store.get("sessions", digest(bearer(r)), &session); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fail(w, 401, errors.New("请先登录"))
		} else {
			fail(w, 500, err)
		}
		return session, false
	}
	if time.Now().After(session.ExpiresAt) {
		fail(w, 401, errors.New("登录已过期"))
		return session, false
	}
	return session, true
}
func (d *DemoBusiness) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /login", d.login)
	mux.HandleFunc("POST /prepare", d.prepare)
	mux.HandleFunc("POST /complete", d.complete)
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) { reply(w, 200, map[string]string{"status": "ok"}) })
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		mux.ServeHTTP(w, r)
	})
}
func (d *DemoBusiness) login(w http.ResponseWriter, r *http.Request) {
	var input struct {
		AccountID string `json:"accountId"`
		Password  string `json:"password"`
	}
	if err := decode(w, r, &input); err != nil {
		fail(w, 400, err)
		return
	}
	expected := sha256.Sum256([]byte(d.Config.Password))
	provided := sha256.Sum256([]byte(input.Password))
	if input.AccountID == "" || subtle.ConstantTimeCompare(expected[:], provided[:]) != 1 {
		fail(w, 401, errors.New("账号或密码错误"))
		return
	}
	token := token()
	session := demoSession{input.AccountID, time.Now().UTC().Add(30 * time.Minute)}
	if err := d.Store.create("sessions", digest(token), session); err != nil {
		fail(w, 500, err)
		return
	}
	response := map[string]any{"token": token, "appId": d.Config.AppID, "platformUrl": d.Config.PlatformURL}
	pending, err := records[demoPending](d.Store, "pending")
	if err != nil {
		fail(w, 500, err)
		return
	}
	var latest demoPending
	for _, p := range pending {
		if p.AccountID == session.AccountID && p.CreatedAt.After(latest.CreatedAt) {
			latest = p
		}
	}
	if latest.EventID != "" {
		var binding struct {
			ReportID string `json:"reportId"`
		}
		if err = d.Store.get("report-bindings", latest.EventID, &binding); err == nil {
			response["pending"] = map[string]string{"eventId": latest.EventID, "reportId": binding.ReportID}
		} else if !errors.Is(err, os.ErrNotExist) {
			fail(w, 500, err)
			return
		}
	}
	reply(w, 200, response)
}
func (d *DemoBusiness) prepare(w http.ResponseWriter, r *http.Request) {
	session, ok := d.session(w, r)
	if !ok {
		return
	}
	var input struct {
		Type  string `json:"type"`
		KeyID string `json:"keyId"`
	}
	if err := decode(w, r, &input); err != nil {
		fail(w, 400, err)
		return
	}
	switch input.Type {
	case "register", "reward", "payment":
	default:
		fail(w, 400, errors.New("不支持的体验场景"))
		return
	}
	id := token()
	context := BusinessContext{}
	if input.Type == "reward" {
		context.CampaignID = "demo-campaign"
		context.RecipientID = "demo-recipient"
	}
	if input.Type == "payment" {
		context.OrderID = "demo-order-" + id
		context.PaymentMethodID = "demo-method-" + session.AccountID
		context.AmountMinor = 100
		context.Currency = "CNY"
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		fail(w, 500, err)
		return
	}
	pending := demoPending{id, session.AccountID, input.Type, context, ip, time.Now().UTC()}
	var challenge Challenge
	if err = d.call("/v1/challenges", map[string]any{"appId": d.Config.AppID, "eventId": id, "accountId": session.AccountID, "type": input.Type, "context": context, "clientIp": ip, "keyId": input.KeyID}, &challenge); err != nil {
		fail(w, 502, err)
		return
	}
	if err = d.Store.create("pending", id, pending); err != nil {
		fail(w, 500, err)
		return
	}
	reply(w, 201, map[string]any{"eventId": id, "challenge": challenge})
}
func (d *DemoBusiness) complete(w http.ResponseWriter, r *http.Request) {
	session, ok := d.session(w, r)
	if !ok {
		return
	}
	var input struct {
		EventID  string `json:"eventId"`
		ReportID string `json:"reportId"`
	}
	if err := decode(w, r, &input); err != nil {
		fail(w, 400, err)
		return
	}
	var pending demoPending
	if err := d.Store.get("pending", input.EventID, &pending); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fail(w, 404, errors.New("业务请求不存在"))
		} else {
			fail(w, 500, err)
		}
		return
	}
	if pending.AccountID != session.AccountID {
		fail(w, 403, errors.New("业务账号不匹配"))
		return
	}
	binding := struct {
		ReportID string `json:"reportId"`
	}{input.ReportID}
	if err := d.Store.create("report-bindings", input.EventID, binding); errors.Is(err, os.ErrExist) {
		var old struct {
			ReportID string `json:"reportId"`
		}
		if err = d.Store.get("report-bindings", input.EventID, &old); err != nil {
			fail(w, 500, err)
			return
		}
		if old.ReportID != input.ReportID {
			fail(w, 409, errors.New("业务请求已绑定另一份报告"))
			return
		}
	} else if err != nil {
		fail(w, 500, err)
		return
	}
	var event Event
	if err := d.call("/v1/events", eventInput{BusinessID: pending.EventID, AppID: d.Config.AppID, ReportID: input.ReportID, AccountID: session.AccountID, Type: pending.Type, ClientIP: pending.ClientIP, Context: pending.Context}, &event); err != nil {
		fail(w, 502, err)
		return
	}
	var claim Claim
	if err := d.call("/v1/decisions/claim", map[string]string{"appId": d.Config.AppID, "eventId": pending.EventID, "operationId": pending.EventID}, &claim); err != nil {
		var upstream *upstreamError
		if errors.As(err, &upstream) && upstream.Status == 403 {
			var state struct {
				State string `json:"state"`
			}
			if json.Unmarshal([]byte(upstream.Body), &state) == nil && (state.State == "verify" || state.State == "hold") {
				reply(w, 200, map[string]any{"state": state.State, "eventId": pending.EventID, "score": event.Score, "reasons": event.Reasons})
				return
			}
		}
		fail(w, 502, err)
		return
	}
	var operation demoOperation
	if err := d.Store.get("operations", pending.EventID, &operation); errors.Is(err, os.ErrNotExist) {
		if time.Now().After(claim.ExpiresAt) {
			fail(w, 410, errors.New("本次授权已过期"))
			return
		}
		operation = demoOperation{pending.EventID, session.AccountID, pending.Type, input.ReportID, time.Now().UTC()}
		if err = d.Store.create("operations", pending.EventID, operation); errors.Is(err, os.ErrExist) {
			if err = d.Store.get("operations", pending.EventID, &operation); err != nil {
				fail(w, 500, err)
				return
			}
		} else if err != nil {
			fail(w, 500, err)
			return
		}
	} else if err != nil {
		fail(w, 500, err)
		return
	}
	var receipt Execution
	if err := d.call("/v1/executions", map[string]string{"appId": d.Config.AppID, "eventId": pending.EventID, "operationId": pending.EventID, "token": claim.Token, "receiptId": "demo-" + pending.EventID, "status": "succeeded", "reference": "demo-ledger-" + pending.EventID}, &receipt); err != nil {
		fail(w, 502, err)
		return
	}
	reply(w, 200, map[string]any{"state": "completed", "eventId": pending.EventID, "score": event.Score, "reasons": event.Reasons, "operation": operation})
}

// InitializeDemo creates new, isolated credentials once. Existing state is read
// and retained. Passwords and application keys are never put into the APK.
func InitializeDemo(dir, caFile, apkSigner string) (DemoConfig, error) {
	configFile := filepath.Join(dir, "demo.json")
	var config DemoConfig
	if data, err := os.ReadFile(configFile); err == nil {
		err = json.Unmarshal(data, &config)
		return config, err
	} else if !errors.Is(err, os.ErrNotExist) {
		return config, err
	}
	store, err := Open(filepath.Join(dir, "platform"))
	if err != nil {
		return config, err
	}
	appKey := token()
	app := App{ID: token(), Name: "知衡体验", Package: "com.device.trust.sample", KeyHash: digest(appKey), CreatedAt: time.Now().UTC()}
	if err = store.put("security", app.ID, AppSecurity{RequireSignedReports: true, SigningDigests: []string{apkSigner}, MaxEnrollmentHours: 24}); err != nil {
		return config, err
	}
	policy := DefaultPolicy()
	policy.Mode = "enforce"
	policy.Version = token()
	if err = store.put("policies", app.ID, policy); err != nil {
		return config, err
	}
	if err = store.put("apps", app.ID, app); err != nil {
		return config, err
	}
	config = DemoConfig{PlatformURL: "https://localhost:9443", AppID: app.ID, AppKey: appKey, Password: token(), CAFile: caFile, Data: filepath.Join(dir, "business")}
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return config, err
	}
	if err = os.WriteFile(configFile, data, 0600); err != nil {
		return config, err
	}
	if err = os.WriteFile(filepath.Join(dir, "admin-token"), []byte(token()), 0600); err != nil {
		return config, err
	}
	return config, nil
}
