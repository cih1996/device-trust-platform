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
	"reflect"
	"sort"
	"strings"
	"time"
)

type Server struct {
	Store      *Store
	AdminToken string
	AdminUser  string
	Trust      *AttestationVerifier
	Resources  *ResourceEngine
	limits     rateLimiter
}

func (s *Server) Handler(web string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) { reply(w, 200, map[string]string{"status": "ok"}) })
	mux.HandleFunc("/api/", s.admin)
	mux.HandleFunc("POST /v1/reports", s.report)
	mux.HandleFunc("POST /v1/events", s.event)
	mux.HandleFunc("POST /v1/outcomes", s.outcome)
	mux.HandleFunc("POST /v1/challenges", s.challenge)
	mux.HandleFunc("POST /v1/secure", s.secure)
	mux.HandleFunc("POST /v1/decisions/claim", s.claim)
	mux.HandleFunc("POST /v1/decisions/resolve", s.resolve)
	mux.HandleFunc("POST /v1/executions", s.execution)
	mux.HandleFunc("POST /v1/verdicts", s.reportVerdict)
	mux.HandleFunc("POST /v1/devices/query", s.deviceVerdict)
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
	// A disabled team's apps stop serving the SDK and the business server.
	if a.TeamID != "" {
		var team Team
		if err = s.Store.get("teams", a.TeamID, &team); err != nil {
			fail(w, 500, err)
			return a, false
		}
		if team.Disabled {
			fail(w, 403, errors.New("应用所属团队已停用"))
			return a, false
		}
	}
	return a, true
}
func (s *Server) admin(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/api/")
	switch {
	case path == "auth/login" && r.Method == "POST":
		s.adminLogin(w, r)
		return
	case path == "auth/options" && r.Method == "GET":
		s.signupOptions(w)
		return
	case path == "auth/register" && r.Method == "POST":
		s.register(w, r)
		return
	case path == "auth/join" && r.Method == "POST":
		s.join(w, r)
		return
	}
	a, ok := s.adminAuth(w, r)
	if !ok {
		return
	}
	r = withAccess(r, a)
	if need := requiredRole(r.Method, path); need == "platform" && !a.platform || need != "platform" && !a.atLeast(need) {
		fail(w, 403, errors.New("当前账号没有此操作的权限"))
		return
	}
	// Every app named by the request must belong to the caller's team.
	if id := r.URL.Query().Get("appId"); id != "" && !a.can(id) {
		fail(w, 404, errors.New("application not found"))
		return
	}
	if strings.HasPrefix(path, "apps/") {
		if id, _, _ := strings.Cut(strings.TrimPrefix(path, "apps/"), "/"); !a.can(id) {
			fail(w, 404, errors.New("application not found"))
			return
		}
	}
	if strings.HasPrefix(path, "events/") {
		id, _, _ := strings.Cut(strings.TrimPrefix(path, "events/"), "/")
		var e Event
		if err := s.Store.get("events", id, &e); err == nil && !a.can(e.AppID) {
			fail(w, 404, errors.New("event not found"))
			return
		}
	}
	if r.URL.Path == "/api/resources" && r.Method == "GET" {
		if s.Resources == nil {
			reply(w, 200, map[string]any{"configured": false})
			return
		}
		e := s.Resources
		cells := map[string]any{"loaded": e.Cells != nil}
		if e.Cells != nil {
			cells["count"], cells["source"], cells["updated"] = e.Cells.Size(), e.Cells.Source, e.Cells.Updated
		}
		reply(w, 200, map[string]any{"configured": true, "version": e.Catalog.Version, "devices": len(e.Catalog.Devices), "cells": cells, "regions": len(e.Catalog.Regions), "ipv4": e.V4 != nil, "ipv6": e.V6 != nil})
		return
	}
	switch {
	case path == "auth/password" && r.Method == "PUT":
		s.changeAdminPassword(w, r)
	case path == "auth/me" && r.Method == "GET":
		s.me(w, r)
	case strings.HasPrefix(path, "team"):
		s.teamAPI(w, r, path)
	case strings.HasPrefix(path, "platform/"):
		s.platformAPI(w, r, path)
	case path == "apps" && r.Method == "GET":
		apps, err := records[App](s.Store, "apps")
		if err != nil {
			fail(w, 500, err)
			return
		}
		visible := apps[:0]
		for _, app := range apps {
			if a.can(app.ID) {
				visible = append(visible, app)
			}
		}
		apps = visible
		for i := range apps {
			apps[i].KeyHash = ""
			p, err := s.policy(apps[i].ID)
			if err != nil {
				fail(w, 500, err)
				return
			}
			apps[i].Policy = &p
			config, err := s.security(apps[i].ID)
			if err != nil {
				fail(w, 500, err)
				return
			}
			apps[i].Security = &config
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
		if !a.platform {
			settings, err := s.settings()
			if err != nil {
				fail(w, 500, err)
				return
			}
			if len(a.apps) >= settings.MaxAppsPerTeam {
				fail(w, 403, fmt.Errorf("每个团队最多创建 %d 个应用", settings.MaxAppsPerTeam))
				return
			}
		}
		key := token()
		app := App{ID: token(), TeamID: a.team.ID, Name: input.Name, Package: input.Package, KeyHash: digest(key), CreatedAt: time.Now().UTC()}
		if err := s.Store.put("security", app.ID, AppSecurity{RequireSignedReports: true, RequireRequestBinding: true, MaxEnrollmentHours: 24, SigningDigests: []string{}}); err != nil {
			fail(w, 500, err)
			return
		}
		if err := s.Store.put("apps", app.ID, app); err != nil {
			fail(w, 500, err)
			return
		}
		app.KeyHash = ""
		reply(w, 201, map[string]any{"app": app, "key": key})
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
	case path == "overview" && r.Method == "GET" && a.platform:
		s.overview(w, r)
	case path == "analytics" && r.Method == "GET":
		s.analytics(w, r)
	case path == "events" && r.Method == "GET":
		s.listEvents(w, r)
	case strings.HasPrefix(path, "events/") && !strings.Contains(strings.TrimPrefix(path, "events/"), "/") && r.Method == "GET":
		s.eventDetails(w, r, strings.TrimPrefix(path, "events/"))
	case path == "insights" && r.Method == "GET":
		s.insights(w, r)
	case path == "graph" && r.Method == "GET":
		s.graph(w, r)
	case path == "devices" && r.Method == "GET":
		s.listDevices(w, r)
	case strings.HasPrefix(path, "devices/") && r.Method == "GET":
		s.deviceDetails(w, r, strings.TrimPrefix(path, "devices/"))
	case path == "device-keys/revoke" && r.Method == "POST":
		s.revokeKey(w, r)
	case strings.HasPrefix(path, "apps/") && strings.HasSuffix(path, "/policy") && r.Method == "PUT":
		s.savePolicy(w, r, strings.TrimSuffix(strings.TrimPrefix(path, "apps/"), "/policy"))
	case strings.HasPrefix(path, "apps/") && strings.HasSuffix(path, "/security") && r.Method == "PUT":
		s.saveSecurity(w, r, strings.TrimSuffix(strings.TrimPrefix(path, "apps/"), "/security"))
	case strings.HasPrefix(path, "events/") && strings.HasSuffix(path, "/reviews") && r.Method == "POST":
		s.review(w, r, strings.TrimSuffix(strings.TrimPrefix(path, "events/"), "/reviews"))
	case strings.HasPrefix(path, "events/") && strings.HasSuffix(path, "/resolve") && r.Method == "POST":
		s.adminResolve(w, r, strings.TrimSuffix(strings.TrimPrefix(path, "events/"), "/resolve"))
	default:
		fail(w, 404, errors.New("接口不存在"))
	}
}
func (s *Server) report(w http.ResponseWriter, r *http.Request) {
	var input reportInput
	if err := decode(w, r, &input); err != nil {
		fail(w, 400, err)
		return
	}
	a, ok := s.app(input.AppID, w)
	if !ok {
		return
	}
	security, err := s.security(a.ID)
	if err != nil {
		fail(w, 500, err)
		return
	}
	if security.RequireSignedReports {
		fail(w, 403, errors.New("signed binary report required"))
		return
	}
	s.legacyReport(w, r, input, a)
}

type reportInput struct {
	AppID          string            `json:"appId"`
	InstallationID string            `json:"installationId"`
	Package        string            `json:"package"`
	Signals        map[string]Signal `json:"signals"`
}

func validateSignals(signals map[string]Signal) error {
	for name, signal := range signals {
		switch signal.Status {
		case "available", "permission_denied", "unavailable", "disabled", "error":
		default:
			return fmt.Errorf("invalid signal status: %s", name)
		}
	}
	return nil
}
func (s *Server) legacyReport(w http.ResponseWriter, r *http.Request, input reportInput, a App) {
	if input.InstallationID == "" || input.Package == "" || len(input.Signals) == 0 {
		fail(w, 400, errors.New("installationId, package and signals required"))
		return
	}
	if err := validateSignals(input.Signals); err != nil {
		fail(w, 400, err)
		return
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
	var input eventInput
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
	if input.BusinessID == "" || input.ReportID == "" || input.AccountID == "" || input.Type == "" {
		fail(w, 400, errors.New("eventId, reportId, accountId and type required"))
		return
	}
	if input.ClientIP != "" && net.ParseIP(input.ClientIP) == nil {
		fail(w, 400, errors.New("invalid clientIp"))
		return
	}
	if input.ClientIP != "" {
		input.ClientIP = net.ParseIP(input.ClientIP).String()
	}
	if input.Context.AmountMinor < 0 || (input.Context.AmountMinor > 0 && input.Context.Currency == "") {
		fail(w, 400, errors.New("amountMinor must be non-negative and a positive amount requires currency"))
		return
	}
	id := digest(a.ID + ":" + input.BusinessID)
	var existing Event
	if err := s.Store.get("events", id, &existing); err == nil {
		replay(w, existing, input)
		return
	} else if !errors.Is(err, os.ErrNotExist) {
		fail(w, 500, err)
		return
	}
	s.createEvent(w, input, a, id)
}

type eventInput struct {
	BusinessID string          `json:"eventId"`
	AppID      string          `json:"appId"`
	ReportID   string          `json:"reportId"`
	AccountID  string          `json:"accountId"`
	Type       string          `json:"type"`
	ClientIP   string          `json:"clientIp"`
	Context    BusinessContext `json:"context"`
}

func replay(w http.ResponseWriter, existing Event, input eventInput) {
	if existing.ReportID != input.ReportID || existing.AccountID != input.AccountID || existing.Type != input.Type || existing.ClientIP != input.ClientIP || !reflect.DeepEqual(existing.Context, input.Context) {
		fail(w, 409, errors.New("eventId already used with different event data"))
		return
	}
	reply(w, 200, existing)
}

func (s *Server) createEvent(w http.ResponseWriter, input eventInput, a App, id string) {
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
	if err := s.validateReportBinding(report, input); err != nil {
		fail(w, 403, err)
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
	outcomes, err := records[Outcome](s.Store, "outcomes")
	if err != nil {
		fail(w, 500, err)
		return
	}
	p, err := s.policy(a.ID)
	if err != nil {
		fail(w, 500, err)
		return
	}
	e := Event{ID: id, BusinessID: input.BusinessID, AppID: a.ID, ReportID: report.ID, AccountID: input.AccountID, Type: input.Type, ClientIP: input.ClientIP, InstallationID: report.InstallationID, Context: input.Context, ReceivedAt: time.Now().UTC()}
	e.ReportProof = report.Proof
	e.DeviceID = report.DeviceID
	e.Resources = report.Resources
	evaluateBusiness(&e, report, history, outcomes, p)
	if err = s.Store.create("events", e.ID, e); errors.Is(err, os.ErrExist) {
		var saved Event
		if err = s.Store.get("events", id, &saved); err != nil {
			fail(w, 500, err)
			return
		}
		replay(w, saved, input)
		return
	} else if err != nil {
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
	resolutions, err := records[Resolution](s.Store, "resolutions")
	if err != nil {
		fail(w, 500, err)
		return
	}
	resolutionMap := map[string]Resolution{}
	for _, item := range resolutions {
		resolutionMap[item.EventID] = item
	}
	executions, err := records[Execution](s.Store, "executions")
	if err != nil {
		fail(w, 500, err)
		return
	}
	executionMap := map[string]Execution{}
	for _, item := range executions {
		if appID != "" && item.AppID != appID {
			continue
		}
		old, ok := executionMap[item.EventID]
		if !ok || item.ReceivedAt.After(old.ReceivedAt) {
			executionMap[item.EventID] = item
		}
	}
	reviews, err := records[Review](s.Store, "reviews")
	if err != nil {
		fail(w, 500, err)
		return
	}
	feedback := map[string]Review{}
	for _, review := range reviews {
		if appID != "" && review.AppID != appID {
			continue
		}
		prior, ok := feedback[review.EventID]
		if !ok || review.ReceivedAt.After(prior.ReceivedAt) {
			feedback[review.EventID] = review
		}
	}
	results, err := records[Outcome](s.Store, "outcomes")
	if err != nil {
		fail(w, 500, err)
		return
	}
	latestOutcomes := map[string]Outcome{}
	for _, outcome := range results {
		if appID != "" && outcome.AppID != appID {
			continue
		}
		key := digest(outcome.AppID + ":" + outcome.EventID)
		old, exists := latestOutcomes[key]
		if !exists || outcome.OccurredAt.After(old.OccurredAt) || (outcome.OccurredAt.Equal(old.OccurredAt) && outcome.ReceivedAt.After(old.ReceivedAt)) {
			latestOutcomes[key] = outcome
		}
	}
	scenes := map[string]int{"registration": 0, "marketing": 0, "payment": 0, "general": 0}
	devices := map[string]bool{}
	now := time.Now().UTC()
	ipCounts := map[string]int{}
	riskBuckets := map[string]int{"low": 0, "medium": 0, "high": 0}
	type deviceRiskStat struct {
		count    int
		total    int
		maxScore int
		lastIP   string
		lastSeen time.Time
		accounts map[string]bool
	}
	deviceRisk := map[string]*deviceRiskStat{}
	reasonCounts := map[string]int{}
	activity := make([]map[string]any, 12)
	for i := range activity {
		activity[i] = map[string]any{"label": now.Add(-time.Duration(11-i) * time.Hour).Format("15:04"), "events": 0, "risk": 0}
	}
	review := 0
	today := 0
	riskTotal := 0
	highRisk := 0
	durationTotal := int64(0)
	durationCount := 0
	start := now.Add(-24 * time.Hour)
	for _, e := range events {
		if appID != "" && appID != e.AppID {
			continue
		}
		if e.Scene == "" {
			e.Scene = sceneFor(e.Type)
		}
		filtered = append(filtered, e)
		if e.ReceivedAt.After(start) {
			scenes[sceneFor(e.Type)]++
			today++
			riskTotal += e.Score
			if e.Context.PageDurationMs > 0 {
				durationTotal += e.Context.PageDurationMs
				durationCount++
			}
			if e.ClientIP != "" {
				ipCounts[e.ClientIP]++
			}
			bucket := "low"
			if e.Score >= 70 {
				bucket = "high"
				highRisk++
			} else if e.Score >= 30 {
				bucket = "medium"
			}
			riskBuckets[bucket]++
			hourIndex := int(now.Sub(e.ReceivedAt) / time.Hour)
			if hourIndex >= 0 && hourIndex < len(activity) {
				item := activity[len(activity)-1-hourIndex]
				item["events"] = item["events"].(int) + 1
				item["risk"] = item["risk"].(int) + e.Score
			}
			if e.Decision == "review" {
				review++
			}
			deviceKey := e.DeviceID
			if deviceKey == "" {
				deviceKey = e.InstallationID
			}
			if deviceKey == "" {
				deviceKey = "未登记设备"
			}
			stat := deviceRisk[deviceKey]
			if stat == nil {
				stat = &deviceRiskStat{accounts: map[string]bool{}}
				deviceRisk[deviceKey] = stat
			}
			stat.count++
			stat.total += e.Score
			if e.Score > stat.maxScore {
				stat.maxScore = e.Score
			}
			if e.ClientIP != "" {
				stat.lastIP = e.ClientIP
			}
			if e.ReceivedAt.After(stat.lastSeen) {
				stat.lastSeen = e.ReceivedAt
			}
			if e.AccountID != "" {
				stat.accounts[e.AccountID] = true
			}
			for _, hit := range e.Hits {
				reasonCounts[hit.Code]++
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
	if appID != "" {
		allowed := map[string]bool{}
		for _, e := range events {
			if e.AppID == appID {
				allowed[e.ID] = true
			}
		}
		for id := range resolutionMap {
			if !allowed[id] {
				delete(resolutionMap, id)
			}
		}
	}
	accesses := make([]map[string]any, 0, len(filtered))
	for _, e := range filtered {
		riskLevel := "low"
		if e.Score >= 70 {
			riskLevel = "high"
		} else if e.Score >= 30 {
			riskLevel = "medium"
		}
		accesses = append(accesses, map[string]any{"id": e.ID, "appId": e.AppID, "receivedAt": e.ReceivedAt, "deviceId": e.DeviceID, "ip": e.ClientIP, "accountId": e.AccountID, "type": e.Type, "scene": e.Scene, "score": e.Score, "riskLevel": riskLevel, "decision": e.Decision, "action": e.Action, "pageName": e.Context.PageName, "pageDurationMs": e.Context.PageDurationMs})
	}
	uniqueIPs := len(ipCounts)
	averageRisk := 0
	if today > 0 {
		averageRisk = riskTotal / today
	}
	averageDwellSeconds := int64(0)
	if durationCount > 0 {
		averageDwellSeconds = durationTotal / int64(durationCount) / 1000
	}
	topIPs := make([]map[string]any, 0, len(ipCounts))
	for ip, count := range ipCounts {
		topIPs = append(topIPs, map[string]any{"ip": ip, "count": count})
	}
	sort.Slice(topIPs, func(i, j int) bool { return topIPs[i]["count"].(int) > topIPs[j]["count"].(int) })
	if len(topIPs) > 8 {
		topIPs = topIPs[:8]
	}
	topDevices := make([]map[string]any, 0, len(deviceRisk))
	for id, stat := range deviceRisk {
		average := 0
		if stat.count > 0 {
			average = stat.total / stat.count
		}
		topDevices = append(topDevices, map[string]any{"deviceId": id, "count": stat.count, "averageScore": average, "maxScore": stat.maxScore, "lastIP": stat.lastIP, "lastSeen": stat.lastSeen, "accounts": len(stat.accounts)})
	}
	sort.Slice(topDevices, func(i, j int) bool {
		if topDevices[i]["maxScore"].(int) != topDevices[j]["maxScore"].(int) {
			return topDevices[i]["maxScore"].(int) > topDevices[j]["maxScore"].(int)
		}
		return topDevices[i]["count"].(int) > topDevices[j]["count"].(int)
	})
	if len(topDevices) > 8 {
		topDevices = topDevices[:8]
	}
	topReasons := make([]map[string]any, 0, len(reasonCounts))
	for code, count := range reasonCounts {
		topReasons = append(topReasons, map[string]any{"code": code, "count": count})
	}
	sort.Slice(topReasons, func(i, j int) bool { return topReasons[i]["count"].(int) > topReasons[j]["count"].(int) })
	if len(topReasons) > 8 {
		topReasons = topReasons[:8]
	}
	analytics := map[string]any{"activeDevices": len(devices), "uniqueIPs": uniqueIPs, "averageRisk": averageRisk, "highRisk": highRisk, "averageDwellSeconds": averageDwellSeconds, "dwellSamples": durationCount, "riskBuckets": riskBuckets, "activity": activity, "topIPs": topIPs, "topDevices": topDevices, "topReasons": topReasons}
	reply(w, 200, map[string]any{"events": filtered, "accesses": accesses, "resolutions": resolutionMap, "executions": executionMap, "feedback": feedback, "outcomes": latestOutcomes, "scenes": scenes, "profiles": profiles(reports, events, appID), "analytics": analytics, "stats": map[string]int{"events": today, "devices": len(devices), "review": review}})
}
