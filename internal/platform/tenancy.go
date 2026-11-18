package platform

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Tenancy. Apps belong to a team; console users belong to one team with a
// role. The platform administrator (the account configured at startup, or
// any user marked Platform) sees every team. Every console read and write is
// limited to the apps of the caller's team: data from other teams is never
// returned, only the shared risk conclusion (see sharedrisk.go).

type Team struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Owner     string    `json:"owner"`
	CreatedAt time.Time `json:"createdAt"`
	Disabled  bool      `json:"disabled,omitempty"`
}

type Invite struct {
	Code      string    `json:"code"`
	TeamID    string    `json:"teamId"`
	Role      string    `json:"role"`
	CreatedBy string    `json:"createdBy"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
	UsedBy    string    `json:"usedBy,omitempty"`
}

type PlatformSettings struct {
	SignupOpen     bool `json:"signupOpen"`
	MaxAppsPerTeam int  `json:"maxAppsPerTeam"`
}

const (
	roleViewer  = "viewer"
	roleAnalyst = "analyst"
	roleAdmin   = "admin"
	inviteTTL   = 7 * 24 * time.Hour
)

var roleRank = map[string]int{roleViewer: 1, roleAnalyst: 2, roleAdmin: 3}
var usernamePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_.@-]{2,39}$`)

func normalizeUsername(name string) (string, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if !usernamePattern.MatchString(name) {
		return "", errors.New("账号需 3–40 位，可用小写字母、数字和 _ . @ -")
	}
	return name, nil
}

func (s *Server) settings() (PlatformSettings, error) {
	settings := PlatformSettings{SignupOpen: true, MaxAppsPerTeam: 10}
	err := s.Store.get("settings", "platform", &settings)
	if errors.Is(err, os.ErrNotExist) {
		err = nil
	}
	return settings, err
}

// access is what the signed-in caller may see and do.
type access struct {
	user     AdminUser
	platform bool
	team     Team
	apps     map[string]bool
}

func (a *access) can(appID string) bool { return a == nil || a.platform || a.apps[appID] }

func (a *access) role() string {
	if a.platform {
		return "platform"
	}
	return a.user.Role
}

func (a *access) atLeast(role string) bool {
	return a.platform || roleRank[a.user.Role] >= roleRank[role]
}

type accessKey struct{}

func accessFrom(r *http.Request) *access {
	a, _ := r.Context().Value(accessKey{}).(*access)
	return a
}

func withAccess(r *http.Request, a *access) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), accessKey{}, a))
}

// accessFor loads the user's team and its apps; disabled users and teams have none.
func (s *Server) accessFor(user AdminUser) (*access, error) {
	if user.Disabled {
		return nil, errors.New("账号已停用")
	}
	a := &access{user: user, platform: user.Platform || user.Username == s.AdminUser, apps: map[string]bool{}}
	if user.TeamID != "" {
		if err := s.Store.get("teams", user.TeamID, &a.team); err != nil {
			return nil, err
		}
		if a.team.Disabled && !a.platform {
			return nil, errors.New("所在团队已停用，请联系平台管理员")
		}
	}
	if a.platform {
		return a, nil
	}
	apps, err := records[App](s.Store, "apps")
	if err != nil {
		return nil, err
	}
	for _, app := range apps {
		if app.TeamID != "" && app.TeamID == user.TeamID {
			a.apps[app.ID] = true
		}
	}
	return a, nil
}

// requiredRole is the least role a console route needs.
func requiredRole(method, path string) string {
	switch {
	case strings.HasPrefix(path, "platform/"):
		return "platform"
	case path == "auth/password" || path == "auth/me":
		return roleViewer
	case strings.HasPrefix(path, "team"):
		return roleAdmin
	case method == "GET":
		return roleViewer
	case strings.HasPrefix(path, "events/") && (strings.HasSuffix(path, "/reviews") || strings.HasSuffix(path, "/resolve")):
		return roleAnalyst
	}
	return roleAdmin
}

// rateLimiter counts attempts per key in a sliding window.
type rateLimiter struct {
	mu   sync.Mutex
	hits map[string][]time.Time
}

func (l *rateLimiter) allow(key string, limit int, window time.Duration) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.hits == nil {
		l.hits = map[string][]time.Time{}
	}
	now := time.Now()
	kept := l.hits[key][:0]
	for _, t := range l.hits[key] {
		if now.Sub(t) < window {
			kept = append(kept, t)
		}
	}
	if len(kept) >= limit {
		l.hits[key] = kept
		return false
	}
	l.hits[key] = append(kept, now)
	if len(l.hits) > 10000 { // forget idle keys
		for k, v := range l.hits {
			if len(v) == 0 || now.Sub(v[len(v)-1]) > time.Hour {
				delete(l.hits, k)
			}
		}
	}
	return true
}

func remoteIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (s *Server) signupOptions(w http.ResponseWriter) {
	settings, err := s.settings()
	if err != nil {
		fail(w, 500, err)
		return
	}
	reply(w, 200, map[string]bool{"signupOpen": settings.SignupOpen})
}

// register creates a team and its first administrator.
func (s *Server) register(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Team     string `json:"team"`
	}
	if err := decode(w, r, &input); err != nil {
		fail(w, 400, err)
		return
	}
	settings, err := s.settings()
	if err != nil {
		fail(w, 500, err)
		return
	}
	if !settings.SignupOpen {
		fail(w, 403, errors.New("暂未开放注册，请联系平台管理员"))
		return
	}
	if !s.limits.allow("register|"+remoteIP(r), 5, time.Hour) {
		fail(w, 429, errors.New("注册过于频繁，请稍后再试"))
		return
	}
	name := strings.TrimSpace(input.Team)
	if name == "" || len([]rune(name)) > 40 {
		fail(w, 400, errors.New("请填写团队或公司名称（不超过 40 字）"))
		return
	}
	team := Team{ID: token(), Name: name, CreatedAt: time.Now().UTC()}
	user, err := s.createUser(input.Username, input.Password, team.ID, roleAdmin)
	if err != nil {
		fail(w, statusOf(err), err)
		return
	}
	team.Owner = user.Username
	if err = s.Store.put("teams", team.ID, team); err != nil {
		fail(w, 500, err)
		return
	}
	s.startSession(w, user)
}

// join redeems an invite into a new account of the inviting team.
func (s *Server) join(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Code     string `json:"code"`
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decode(w, r, &input); err != nil {
		fail(w, 400, err)
		return
	}
	if !s.limits.allow("join|"+remoteIP(r), 10, time.Hour) {
		fail(w, 429, errors.New("尝试过于频繁，请稍后再试"))
		return
	}
	var invite Invite
	if err := s.Store.get("invites", input.Code, &invite); err != nil || invite.UsedBy != "" || time.Now().After(invite.ExpiresAt) {
		fail(w, 400, errors.New("邀请链接无效、已使用或已过期"))
		return
	}
	var team Team
	if err := s.Store.get("teams", invite.TeamID, &team); err != nil || team.Disabled {
		fail(w, 400, errors.New("邀请所属团队不可用"))
		return
	}
	user, err := s.createUser(input.Username, input.Password, invite.TeamID, invite.Role)
	if err != nil {
		fail(w, statusOf(err), err)
		return
	}
	invite.UsedBy = user.Username
	if err = s.Store.put("invites", invite.Code, invite); err != nil {
		fail(w, 500, err)
		return
	}
	s.startSession(w, user)
}

var errUserExists = errors.New("账号已被使用")

func statusOf(err error) int {
	if errors.Is(err, errUserExists) {
		return 409
	}
	return 400
}

func (s *Server) createUser(username, password, teamID, role string) (AdminUser, error) {
	name, err := normalizeUsername(username)
	if err != nil {
		return AdminUser{}, err
	}
	user, err := newAdminUser(name, password)
	if err != nil {
		return user, err
	}
	now := time.Now().UTC()
	user.TeamID, user.Role, user.CreatedAt, user.LastLoginAt = teamID, role, now, &now
	if err = s.Store.create("admin-users", name, user); errors.Is(err, os.ErrExist) {
		return user, errUserExists
	}
	return user, err
}

func (s *Server) startSession(w http.ResponseWriter, user AdminUser) {
	session, err := s.createAdminSession(user.Username)
	if err != nil {
		fail(w, 500, err)
		return
	}
	reply(w, 201, map[string]any{"token": session, "username": user.Username})
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	a := accessFrom(r)
	reply(w, 200, map[string]any{"username": a.user.Username, "role": a.role(), "team": a.team})
}

type memberRow struct {
	Username    string     `json:"username"`
	Role        string     `json:"role"`
	TeamID      string     `json:"teamId,omitempty"`
	Disabled    bool       `json:"disabled,omitempty"`
	Platform    bool       `json:"platform,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	LastLoginAt *time.Time `json:"lastLoginAt,omitempty"`
}

func (s *Server) memberRows(teamID string, all bool) ([]memberRow, error) {
	users, err := records[AdminUser](s.Store, "admin-users")
	if err != nil {
		return nil, err
	}
	rows := []memberRow{}
	for _, u := range users {
		if all || u.TeamID == teamID {
			rows = append(rows, memberRow{Username: u.Username, Role: u.Role, TeamID: u.TeamID, Disabled: u.Disabled, Platform: u.Platform || u.Username == s.AdminUser, CreatedAt: u.CreatedAt, LastLoginAt: u.LastLoginAt})
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].CreatedAt.Before(rows[j].CreatedAt) })
	return rows, nil
}

// teamAPI serves team/... for a team administrator.
func (s *Server) teamAPI(w http.ResponseWriter, r *http.Request, path string) {
	a := accessFrom(r)
	if a.team.ID == "" {
		fail(w, 400, errors.New("当前账号不属于任何团队"))
		return
	}
	switch {
	case path == "team" && r.Method == "GET":
		members, err := s.memberRows(a.team.ID, false)
		if err != nil {
			fail(w, 500, err)
			return
		}
		invites, err := records[Invite](s.Store, "invites")
		if err != nil {
			fail(w, 500, err)
			return
		}
		open := []Invite{}
		for _, inv := range invites {
			if inv.TeamID == a.team.ID && inv.UsedBy == "" && time.Now().Before(inv.ExpiresAt) {
				open = append(open, inv)
			}
		}
		sort.Slice(open, func(i, j int) bool { return open[i].CreatedAt.After(open[j].CreatedAt) })
		reply(w, 200, map[string]any{"team": a.team, "members": members, "invites": open})
	case path == "team" && r.Method == "PUT":
		var input struct {
			Name string `json:"name"`
		}
		if err := decode(w, r, &input); err != nil {
			fail(w, 400, err)
			return
		}
		name := strings.TrimSpace(input.Name)
		if name == "" || len([]rune(name)) > 40 {
			fail(w, 400, errors.New("团队名称不能为空且不超过 40 字"))
			return
		}
		a.team.Name = name
		if err := s.Store.put("teams", a.team.ID, a.team); err != nil {
			fail(w, 500, err)
			return
		}
		reply(w, 200, a.team)
	case path == "team/invites" && r.Method == "POST":
		var input struct {
			Role string `json:"role"`
		}
		if err := decode(w, r, &input); err != nil {
			fail(w, 400, err)
			return
		}
		if roleRank[input.Role] == 0 {
			fail(w, 400, errors.New("未知角色"))
			return
		}
		now := time.Now().UTC()
		invite := Invite{Code: token(), TeamID: a.team.ID, Role: input.Role, CreatedBy: a.user.Username, CreatedAt: now, ExpiresAt: now.Add(inviteTTL)}
		if err := s.Store.create("invites", invite.Code, invite); err != nil {
			fail(w, 500, err)
			return
		}
		reply(w, 201, invite)
	case strings.HasPrefix(path, "team/invites/") && r.Method == "DELETE":
		var invite Invite
		code := strings.TrimPrefix(path, "team/invites/")
		if err := s.Store.get("invites", code, &invite); err != nil || invite.TeamID != a.team.ID {
			fail(w, 404, errors.New("邀请不存在"))
			return
		}
		invite.ExpiresAt = time.Now().UTC()
		if err := s.Store.put("invites", code, invite); err != nil {
			fail(w, 500, err)
			return
		}
		reply(w, 200, map[string]bool{"revoked": true})
	case strings.HasPrefix(path, "team/members/") && r.Method == "PUT":
		var input struct {
			Role     string `json:"role"`
			Disabled bool   `json:"disabled"`
		}
		if err := decode(w, r, &input); err != nil {
			fail(w, 400, err)
			return
		}
		name := strings.TrimPrefix(path, "team/members/")
		var user AdminUser
		if err := s.Store.get("admin-users", name, &user); err != nil || user.TeamID != a.team.ID {
			fail(w, 404, errors.New("成员不存在"))
			return
		}
		if roleRank[input.Role] == 0 {
			fail(w, 400, errors.New("未知角色"))
			return
		}
		if name == a.user.Username && (input.Disabled || input.Role != roleAdmin) {
			fail(w, 400, errors.New("不能停用自己或取消自己的管理员身份"))
			return
		}
		user.Role, user.Disabled = input.Role, input.Disabled
		if input.Disabled {
			user.SessionsAfter = time.Now().UTC()
		}
		if err := s.Store.put("admin-users", name, user); err != nil {
			fail(w, 500, err)
			return
		}
		reply(w, 200, map[string]string{"username": name})
	default:
		fail(w, 404, errors.New("接口不存在"))
	}
}

// platformAPI serves platform/... for the platform administrator.
func (s *Server) platformAPI(w http.ResponseWriter, r *http.Request, path string) {
	switch {
	case path == "platform/settings" && r.Method == "GET":
		settings, err := s.settings()
		if err != nil {
			fail(w, 500, err)
			return
		}
		reply(w, 200, settings)
	case path == "platform/settings" && r.Method == "PUT":
		var input PlatformSettings
		if err := decode(w, r, &input); err != nil {
			fail(w, 400, err)
			return
		}
		if input.MaxAppsPerTeam < 1 || input.MaxAppsPerTeam > 1000 {
			fail(w, 400, errors.New("每个团队的应用上限需在 1–1000 之间"))
			return
		}
		if err := s.Store.put("settings", "platform", input); err != nil {
			fail(w, 500, err)
			return
		}
		reply(w, 200, input)
	case path == "platform/teams" && r.Method == "GET":
		teams, err := records[Team](s.Store, "teams")
		if err != nil {
			fail(w, 500, err)
			return
		}
		apps, err := records[App](s.Store, "apps")
		if err != nil {
			fail(w, 500, err)
			return
		}
		users, err := s.memberRows("", true)
		if err != nil {
			fail(w, 500, err)
			return
		}
		type teamRow struct {
			Team
			Apps    []map[string]string `json:"apps"`
			Members []memberRow         `json:"members"`
		}
		byID := map[string]*teamRow{}
		rows := []*teamRow{}
		for _, t := range teams {
			row := &teamRow{Team: t, Apps: []map[string]string{}, Members: []memberRow{}}
			byID[t.ID] = row
			rows = append(rows, row)
		}
		// Apps and users without a team belong to the platform itself.
		platform := &teamRow{Team: Team{Name: "平台自有"}, Apps: []map[string]string{}, Members: []memberRow{}}
		for _, app := range apps {
			row := byID[app.TeamID]
			if row == nil {
				row = platform
			}
			row.Apps = append(row.Apps, map[string]string{"id": app.ID, "name": app.Name, "package": app.Package})
		}
		for _, u := range users {
			row := byID[u.TeamID]
			if row == nil {
				row = platform
			}
			row.Members = append(row.Members, u)
		}
		sort.Slice(rows, func(i, j int) bool { return rows[i].CreatedAt.After(rows[j].CreatedAt) })
		reply(w, 200, append([]*teamRow{platform}, rows...))
	case strings.HasPrefix(path, "platform/teams/") && r.Method == "PUT":
		var input struct {
			Disabled bool `json:"disabled"`
		}
		if err := decode(w, r, &input); err != nil {
			fail(w, 400, err)
			return
		}
		id := strings.TrimPrefix(path, "platform/teams/")
		var team Team
		if err := s.Store.get("teams", id, &team); err != nil {
			fail(w, 404, errors.New("团队不存在"))
			return
		}
		team.Disabled = input.Disabled
		if err := s.Store.put("teams", id, team); err != nil {
			fail(w, 500, err)
			return
		}
		reply(w, 200, team)
	case strings.HasPrefix(path, "platform/users/") && strings.HasSuffix(path, "/reset-password") && r.Method == "POST":
		name := strings.TrimSuffix(strings.TrimPrefix(path, "platform/users/"), "/reset-password")
		var user AdminUser
		if err := s.Store.get("admin-users", name, &user); err != nil {
			fail(w, 404, errors.New("账号不存在"))
			return
		}
		password := token()[:20]
		updated, err := newAdminUser(user.Username, password)
		if err != nil {
			fail(w, 500, err)
			return
		}
		user.Salt, user.Hash, user.Iterations, user.UpdatedAt, user.SessionsAfter = updated.Salt, updated.Hash, updated.Iterations, updated.UpdatedAt, updated.UpdatedAt
		if err = s.Store.put("admin-users", name, user); err != nil {
			fail(w, 500, err)
			return
		}
		reply(w, 200, map[string]string{"username": name, "password": password})
	case strings.HasPrefix(path, "platform/users/") && r.Method == "PUT":
		var input struct {
			Disabled bool `json:"disabled"`
		}
		if err := decode(w, r, &input); err != nil {
			fail(w, 400, err)
			return
		}
		name := strings.TrimPrefix(path, "platform/users/")
		var user AdminUser
		if err := s.Store.get("admin-users", name, &user); err != nil {
			fail(w, 404, errors.New("账号不存在"))
			return
		}
		if name == s.AdminUser && input.Disabled {
			fail(w, 400, errors.New("不能停用平台管理员"))
			return
		}
		user.Disabled = input.Disabled
		if input.Disabled {
			user.SessionsAfter = time.Now().UTC()
		}
		if err := s.Store.put("admin-users", name, user); err != nil {
			fail(w, 500, err)
			return
		}
		reply(w, 200, map[string]string{"username": name})
	default:
		fail(w, 404, errors.New("接口不存在"))
	}
}
