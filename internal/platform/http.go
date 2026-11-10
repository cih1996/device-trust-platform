package platform

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

type Server struct {
	Store      *Store
	AdminToken string
}

func (s *Server) Handler(web string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) { reply(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("/api/", s.admin)
	mux.HandleFunc("POST /v1/reports", s.report)
	mux.HandleFunc("POST /v1/events", s.event)
	mux.Handle("/", http.FileServer(http.Dir(web)))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		if strings.HasPrefix(r.URL.Path, "/api/") || strings.HasPrefix(r.URL.Path, "/v1/") {
			w.Header().Set("Cache-Control", "no-store")
		}
		mux.ServeHTTP(w, r)
	})
}
func reply(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}
func fail(w http.ResponseWriter, status int, err error) {
	reply(w, status, map[string]string{"error": err.Error()})
}
func decode(w http.ResponseWriter, r *http.Request, v any) error {
	d := json.NewDecoder(http.MaxBytesReader(w, r.Body, 128<<10))
	d.DisallowUnknownFields()
	if err := d.Decode(v); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return errors.New("request must contain one JSON object")
	}
	return nil
}
func bearer(r *http.Request) string {
	return strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
}
func same(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }
func (s *Server) app(id string, w http.ResponseWriter) (App, bool) {
	var a App
	err := s.Store.get("apps", id, &a)
	if errors.Is(err, os.ErrNotExist) {
		fail(w, 404, errors.New("application not found"))
		return a, false
	}
	if err != nil {
		fail(w, 500, err)
		return a, false
	}
	return a, true
}
func (s *Server) admin(w http.ResponseWriter, r *http.Request) {
	if s.AdminToken == "" || !same(bearer(r), s.AdminToken) {
		fail(w, 401, errors.New("请重新输入管理口令"))
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/")
	switch {
	case path == "apps" && r.Method == "GET":
		apps, err := records[App](s.Store, "apps")
		if err != nil {
			fail(w, 500, err)
			return
		}
		for i := range apps {
			apps[i].KeyHash = ""
		}
		reply(w, 200, apps)
	case path == "apps" && r.Method == "POST":
		var input struct {
			Name    string `json:"name"`
			Package string `json:"package"`
		}
		if err := decode(w, r, &input); err != nil {
			fail(w, 400, err)
			return
		}
		if strings.TrimSpace(input.Name) == "" || strings.TrimSpace(input.Package) == "" {
			fail(w, 400, errors.New("请输入应用名称和包名"))
			return
		}
		key := token()
		a := App{ID: token(), Name: input.Name, Package: input.Package, KeyHash: digest(key), CreatedAt: time.Now().UTC()}
		if err := s.Store.put("apps", a.ID, a); err != nil {
			fail(w, 500, err)
			return
		}
		a.KeyHash = ""
		reply(w, 201, map[string]any{"app": a, "key": key})
	case strings.HasPrefix(path, "apps/") && strings.HasSuffix(path, "/rotate-key") && r.Method == "POST":
		id := strings.TrimSuffix(strings.TrimPrefix(path, "apps/"), "/rotate-key")
		a, ok := s.app(id, w)
		if !ok {
			return
		}
		key := token()
		a.KeyHash = digest(key)
		if err := s.Store.put("apps", a.ID, a); err != nil {
			fail(w, 500, err)
			return
		}
		reply(w, 200, map[string]string{"key": key})
	case path == "overview" && r.Method == "GET":
		s.overview(w, r)
	default:
		fail(w, 404, errors.New("接口不存在"))
	}
}
func (s *Server) report(w http.ResponseWriter, r *http.Request) {
	var input struct {
		AppID          string            `json:"appId"`
		InstallationID string            `json:"installationId"`
		Package        string            `json:"package"`
		Signals        map[string]Signal `json:"signals"`
	}
	if err := decode(w, r, &input); err != nil {
		fail(w, 400, err)
		return
	}
	a, ok := s.app(input.AppID, w)
	if !ok {
		return
	}
	if input.InstallationID == "" || input.Package == "" || len(input.Signals) == 0 {
		fail(w, 400, errors.New("installationId, package and signals required"))
		return
	}
	for name, signal := range input.Signals {
		switch signal.Status {
		case "available", "permission_denied", "unavailable", "disabled":
		default:
			fail(w, 400, fmt.Errorf("invalid signal status: %s", name))
			return
		}
	}
	input.Signals["expectedPackage"] = Signal{Status: "available", Value: input.Package == a.Package}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		fail(w, 500, err)
		return
	}
	report := Report{ID: token(), AppID: a.ID, InstallationID: input.InstallationID, Package: input.Package, Signals: input.Signals, ObservedIP: ip, ReceivedAt: time.Now().UTC()}
	if err = s.Store.put("reports", report.ID, report); err != nil {
		fail(w, 500, err)
		return
	}
	reply(w, 201, map[string]string{"reportId": report.ID})
}
func (s *Server) event(w http.ResponseWriter, r *http.Request) {
	var input struct {
		AppID     string `json:"appId"`
		ReportID  string `json:"reportId"`
		AccountID string `json:"accountId"`
		Type      string `json:"type"`
		ClientIP  string `json:"clientIp"`
	}
	if err := decode(w, r, &input); err != nil {
		fail(w, 400, err)
		return
	}
	a, ok := s.app(input.AppID, w)
	if !ok {
		return
	}
	if !same(digest(bearer(r)), a.KeyHash) {
		fail(w, 401, errors.New("invalid application key"))
		return
	}
	if input.ReportID == "" || input.AccountID == "" || input.Type == "" {
		fail(w, 400, errors.New("reportId, accountId and type required"))
		return
	}
	if input.ClientIP != "" && net.ParseIP(input.ClientIP) == nil {
		fail(w, 400, errors.New("invalid clientIp"))
		return
	}
	var report Report
	if err := s.Store.get("reports", input.ReportID, &report); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fail(w, 404, errors.New("report not found"))
		} else {
			fail(w, 500, err)
		}
		return
	}
	if report.AppID != a.ID {
		fail(w, 403, errors.New("report belongs to another application"))
		return
	}
	if time.Since(report.ReceivedAt) > 15*time.Minute {
		fail(w, 400, errors.New("report expired; collect a new report"))
		return
	}
	history, err := records[Event](s.Store, "events")
	if err != nil {
		fail(w, 500, err)
		return
	}
	e := Event{ID: token(), AppID: a.ID, ReportID: report.ID, AccountID: input.AccountID, Type: input.Type, ClientIP: input.ClientIP, InstallationID: report.InstallationID, ReceivedAt: time.Now().UTC()}
	evaluate(&e, report, history)
	if err = s.Store.put("events", e.ID, e); err != nil {
		fail(w, 500, err)
		return
	}
	reply(w, 201, e)
}
func (s *Server) overview(w http.ResponseWriter, r *http.Request) {
	appID := r.URL.Query().Get("appId")
	events, err := records[Event](s.Store, "events")
	if err != nil {
		fail(w, 500, err)
		return
	}
	reports, err := records[Report](s.Store, "reports")
	if err != nil {
		fail(w, 500, err)
		return
	}
	filtered := []Event{}
	devices := map[string]bool{}
	review := 0
	today := 0
	now := time.Now().UTC()
	start := now.Add(-24 * time.Hour)
	for _, e := range events {
		if appID != "" && appID != e.AppID {
			continue
		}
		filtered = append(filtered, e)
		if e.ReceivedAt.After(start) {
			today++
			if e.Decision == "review" {
				review++
			}
		}
	}
	for _, p := range reports {
		if (appID == "" || p.AppID == appID) && p.ReceivedAt.After(start) {
			devices[p.AppID+":"+p.InstallationID] = true
		}
	}
	sort.Slice(filtered, func(i, j int) bool { return filtered[i].ReceivedAt.After(filtered[j].ReceivedAt) })
	if len(filtered) > 100 {
		filtered = filtered[:100]
	}
	reply(w, 200, map[string]any{"events": filtered, "profiles": profiles(reports, events, appID), "stats": map[string]int{"events": today, "devices": len(devices), "review": review}})
}
