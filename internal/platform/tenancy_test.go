package platform

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestTeamsAreIsolated(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = EnsureAdminUser(store, "admin", "platform-password-123"); err != nil {
		t.Fatal(err)
	}
	h := (&Server{Store: store, AdminUser: "admin"}).Handler(t.TempDir())
	login := func(name, password string) string {
		return request(t, h, "POST", "/api/auth/login", "", map[string]string{"username": name, "password": password}, 200)["token"].(string)
	}
	root := login("admin", "platform-password-123")
	signup := func(name, team string) string {
		return request(t, h, "POST", "/api/auth/register", "", map[string]string{"username": name, "password": "a-long-password", "team": team}, 201)["token"].(string)
	}
	alice, bob := signup("Alice", "甲公司"), signup("bob", "乙公司")
	request(t, h, "POST", "/api/auth/register", "", map[string]string{"username": "alice", "password": "a-long-password", "team": "x"}, 409)
	request(t, h, "POST", "/api/auth/register", "", map[string]string{"username": "admin", "password": "a-long-password", "team": "x"}, 409)

	newApp := func(session, name string) (string, string) {
		a := request(t, h, "POST", "/api/apps", session, map[string]string{"name": name, "package": "com.example." + name}, 201)
		id := a["app"].(map[string]any)["id"].(string)
		request(t, h, "PUT", "/api/apps/"+id+"/security", session, AppSecurity{MaxEnrollmentHours: 24}, 200)
		return id, a["key"].(string)
	}
	appA, keyA := newApp(alice, "a")
	appB, _ := newApp(bob, "b")
	// One handset reports to both teams; it looks rooted in team B.
	av := func(v any) Signal { return Signal{Status: "available", Value: v} }
	now := time.Now().UTC()
	for _, r := range []Report{
		{ID: "rb", AppID: appB, InstallationID: "ib", LinkID: "L", ReceivedAt: now.Add(-time.Hour), Signals: map[string]Signal{"emulatorHint": av(true), "debugger": av(true), "rootBeerSuBinary": av(true), "rootBeerMagiskBinary": av(true), "rootBeerNativeSu": av(true), "adbEnabled": av(true)}},
		{ID: "ra", AppID: appA, InstallationID: "ia", LinkID: "L", ReceivedAt: now},
	} {
		if err = store.put("reports", r.ID, r); err != nil {
			t.Fatal(err)
		}
	}
	if err = store.put("events", "eb", Event{ID: "eb", AppID: appB, InstallationID: "ib", AccountID: "x", Score: 85, ReceivedAt: now.Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	request(t, h, "POST", "/v1/events", keyA, map[string]any{"eventId": "e1", "appId": appA, "reportId": "ra", "accountId": "u1", "type": "login"}, 201)

	apps := func(session string) int { return len(requestList(t, h, "/api/apps", session)) }
	if apps(alice) != 1 || apps(bob) != 1 || apps(root) != 2 {
		t.Fatalf("apps visible: alice %d bob %d root %d", apps(alice), apps(bob), apps(root))
	}
	devices := request(t, h, "GET", "/api/devices", alice, nil, 200)["items"].([]any)
	if len(devices) != 1 || devices[0].(map[string]any)["appId"] != appA {
		t.Fatalf("alice sees %v", devices)
	}
	request(t, h, "GET", "/api/devices?appId="+appB, alice, nil, 404)
	request(t, h, "GET", "/api/devices/"+digest(appB+":ib"), alice, nil, 404)
	request(t, h, "GET", "/api/events/e1", bob, nil, 404)
	request(t, h, "PUT", "/api/apps/"+appA+"/security", bob, AppSecurity{MaxEnrollmentHours: 24}, 404)
	request(t, h, "GET", "/api/overview", alice, nil, 404)
	request(t, h, "GET", "/api/platform/teams", alice, nil, 403)
	if events := request(t, h, "GET", "/api/events", bob, nil, 200)["items"].([]any); len(events) != 1 || events[0].(map[string]any)["id"] != "eb" {
		t.Fatalf("bob sees alice's events: %v", events)
	}
	for _, path := range []string{"/api/analytics?range=7d", "/api/insights?range=7d", "/api/graph?days=7"} {
		request(t, h, "GET", path, alice, nil, 200)
	}
	if rows := request(t, h, "GET", "/api/insights?range=7d", alice, nil, 200)["rows"].([]any); len(rows) != 1 {
		t.Fatalf("insights leak: %d rows", len(rows))
	}

	// Shared risk: alice learns only that another team saw the handset as high risk.
	detail := request(t, h, "GET", "/api/devices/"+digest(appA+":ia"), alice, nil, 200)
	shared := detail["shared"].(map[string]any)
	if shared["teams"].(float64) != 1 || shared["highRisk"].(float64) != 1 || len(detail["linked"].([]any)) != 0 {
		t.Fatalf("shared %v linked %v", shared, detail["linked"])
	}

	// Invites and roles.
	invite := request(t, h, "POST", "/api/team/invites", alice, map[string]string{"role": "viewer"}, 201)["code"].(string)
	carol := request(t, h, "POST", "/api/auth/join", "", map[string]string{"code": invite, "username": "carol", "password": "a-long-password"}, 201)["token"].(string)
	request(t, h, "POST", "/api/auth/join", "", map[string]string{"code": invite, "username": "dave", "password": "a-long-password"}, 400)
	if apps(carol) != 1 {
		t.Fatal("viewer cannot see team apps")
	}
	request(t, h, "POST", "/api/apps/"+appA+"/rotate-key", carol, map[string]any{}, 403)
	request(t, h, "POST", "/api/team/invites", carol, map[string]string{"role": "admin"}, 403)
	me := request(t, h, "GET", "/api/auth/me", carol, nil, 200)
	if me["role"] != "viewer" || me["team"].(map[string]any)["name"] != "甲公司" {
		t.Fatal(me)
	}
	request(t, h, "PUT", "/api/team/members/alice", alice, map[string]any{"role": "viewer"}, 400)
	request(t, h, "PUT", "/api/team/members/bob", alice, map[string]any{"role": "viewer"}, 404)
	request(t, h, "PUT", "/api/team/members/carol", alice, map[string]any{"role": "viewer", "disabled": true}, 200)
	request(t, h, "GET", "/api/apps", carol, nil, 401)

	// Disabling a team signs its members out and stops its apps.
	var teamA string
	for _, row := range requestList(t, h, "/api/platform/teams", root) {
		if row.(map[string]any)["name"] == "甲公司" {
			teamA = row.(map[string]any)["id"].(string)
		}
	}
	request(t, h, "PUT", "/api/platform/teams/"+teamA, root, map[string]any{"disabled": true}, 200)
	request(t, h, "GET", "/api/apps", alice, nil, 403)
	request(t, h, "POST", "/v1/events", keyA, map[string]any{"eventId": "e2", "appId": appA, "reportId": "ra", "accountId": "u1", "type": "login"}, 403)
	request(t, h, "POST", "/api/auth/login", "", map[string]string{"username": "alice", "password": "a-long-password"}, 403)

	// Platform switches sign-up off and resets a password.
	request(t, h, "PUT", "/api/platform/settings", root, PlatformSettings{SignupOpen: false, MaxAppsPerTeam: 1}, 200)
	request(t, h, "POST", "/api/auth/register", "", map[string]string{"username": "eve", "password": "a-long-password", "team": "x"}, 403)
	request(t, h, "POST", "/api/apps", bob, map[string]string{"name": "b2", "package": "com.example.b2"}, 403)
	reset := request(t, h, "POST", "/api/platform/users/bob/reset-password", root, map[string]any{}, 200)["password"].(string)
	request(t, h, "GET", "/api/apps", bob, nil, 401)
	login("bob", reset)
}

func requestList(t *testing.T, h interface {
	ServeHTTP(w http.ResponseWriter, r *http.Request)
}, path, key string) []any {
	t.Helper()
	r := httptest.NewRequest("GET", path, nil)
	r.Header.Set("Authorization", "Bearer "+key)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	var out []any
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &out) != nil {
		t.Fatalf("GET %s: %d %s", path, w.Code, w.Body.String())
	}
	return out
}
