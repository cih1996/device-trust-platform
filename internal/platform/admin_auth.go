package platform

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"net/http"
	"os"
	"strings"
	"time"
)

const adminPasswordIterations = 120000

type AdminUser struct {
	Username   string    `json:"username"`
	Salt       string    `json:"salt"`
	Hash       string    `json:"hash"`
	Iterations int       `json:"iterations"`
	UpdatedAt  time.Time `json:"updatedAt"`
	// Tenancy. The startup account has no team and is the platform administrator.
	TeamID   string `json:"teamId,omitempty"`
	Role     string `json:"role,omitempty"`
	Platform bool   `json:"platform,omitempty"`
	Disabled bool   `json:"disabled,omitempty"`
	// Demo is a public experience account: its password is published, so it
	// cannot change the password and many visitors may sign in at once.
	Demo        bool       `json:"demo,omitempty"`
	CreatedAt   time.Time  `json:"createdAt"`
	LastLoginAt *time.Time `json:"lastLoginAt,omitempty"`
	// Sessions issued before this instant are void (password reset, disable).
	SessionsAfter time.Time `json:"sessionsAfter,omitempty"`
}

type AdminSession struct {
	Username  string    `json:"username"`
	CreatedAt time.Time `json:"createdAt"`
	ExpiresAt time.Time `json:"expiresAt"`
}

func deriveAdminPassword(password string, salt []byte, iterations int) []byte {
	if iterations < 1 {
		iterations = adminPasswordIterations
	}
	mac := hmac.New(sha256.New, salt)
	mac.Write([]byte(password))
	var block [4]byte
	binary.BigEndian.PutUint32(block[:], 1)
	mac.Write(block[:])
	u := mac.Sum(nil)
	t := append([]byte(nil), u...)
	for i := 1; i < iterations; i++ {
		mac = hmac.New(sha256.New, salt)
		mac.Write(u)
		u = mac.Sum(nil)
		for j := range t {
			t[j] ^= u[j]
		}
	}
	return t
}

func newAdminUser(username, password string) (AdminUser, error) {
	if strings.TrimSpace(username) == "" || len(password) < 6 {
		return AdminUser{}, errors.New("账号和至少 6 位密码为必填项")
	}
	salt := randomBytes(16)
	return AdminUser{Username: strings.TrimSpace(username), Salt: base64.RawStdEncoding.EncodeToString(salt), Hash: base64.RawStdEncoding.EncodeToString(deriveAdminPassword(password, salt, adminPasswordIterations)), Iterations: adminPasswordIterations, UpdatedAt: time.Now().UTC()}, nil
}

// passwordPolicy applies to passwords people choose: at least 6 characters
// with a letter and a digit. Generated and bootstrap passwords skip it.
func passwordPolicy(password string) error {
	letter, digit := false, false
	for _, c := range password {
		letter = letter || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
		digit = digit || c >= '0' && c <= '9'
	}
	if len(password) < 6 || !letter || !digit {
		return errors.New("密码至少 6 位，需同时包含字母和数字")
	}
	return nil
}

func adminPasswordMatches(user AdminUser, password string) bool {
	salt, err := base64.RawStdEncoding.DecodeString(user.Salt)
	if err != nil || user.Hash == "" || user.Iterations < 1 {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(user.Hash)
	if err != nil {
		return false
	}
	return hmac.Equal(want, deriveAdminPassword(password, salt, user.Iterations))
}

// EnsureAdminUser performs the one-time migration from the original private
// admin token to an account/password record. Existing passwords are retained.
func EnsureAdminUser(store *Store, username, bootstrapPassword string) error {
	var user AdminUser
	err := store.get("admin-users", username, &user)
	if err == nil {
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	user, err = newAdminUser(username, bootstrapPassword)
	if err != nil {
		return err
	}
	return store.put("admin-users", username, user)
}

func (s *Server) createAdminSession(username string) (string, error) {
	value := token()
	now := time.Now().UTC()
	session := AdminSession{Username: username, CreatedAt: now, ExpiresAt: now.Add(12 * time.Hour)}
	return value, s.Store.create("admin-sessions", value, session)
}

func (s *Server) adminLogin(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decode(w, r, &input); err != nil {
		fail(w, 400, err)
		return
	}
	if s.AdminUser == "" {
		fail(w, 500, errors.New("admin account authentication is not configured"))
		return
	}
	name := strings.ToLower(strings.TrimSpace(input.Username))
	if !s.limits.allow("login|"+remoteIP(r), 30, 15*time.Minute) {
		fail(w, 429, errors.New("登录尝试过于频繁，请 15 分钟后再试"))
		return
	}
	var user AdminUser
	err := s.Store.get("admin-users", name, &user)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		fail(w, 500, err)
		return
	}
	// A public account is shared by every visitor; only the address limit applies.
	if !user.Demo && !s.limits.allow("login|"+name, 10, 15*time.Minute) {
		fail(w, 429, errors.New("登录尝试过于频繁，请 15 分钟后再试"))
		return
	}
	if err != nil || !adminPasswordMatches(user, input.Password) {
		fail(w, 401, errors.New("账号或密码错误"))
		return
	}
	if _, err = s.accessFor(user); err != nil {
		fail(w, 403, err)
		return
	}
	now := time.Now().UTC()
	user.LastLoginAt = &now
	if err = s.Store.put("admin-users", user.Username, user); err != nil {
		fail(w, 500, err)
		return
	}
	session, err := s.createAdminSession(user.Username)
	if err != nil {
		fail(w, 500, err)
		return
	}
	reply(w, 200, map[string]any{"token": session, "username": user.Username, "expiresAt": now.Add(12 * time.Hour)})
}

// authorizeAdmin returns the caller's access, or nil when not signed in.
func (s *Server) authorizeAdmin(r *http.Request) (*access, error) {
	if s.AdminUser == "" {
		if s.AdminToken != "" && same(bearer(r), s.AdminToken) {
			return &access{platform: true, apps: map[string]bool{}}, nil
		}
		return nil, nil
	}
	var session AdminSession
	if err := s.Store.get("admin-sessions", bearer(r), &session); err != nil || !time.Now().UTC().Before(session.ExpiresAt) {
		return nil, nil
	}
	var user AdminUser
	if err := s.Store.get("admin-users", session.Username, &user); err != nil {
		return nil, nil
	}
	if session.CreatedAt.Before(user.SessionsAfter) {
		return nil, nil
	}
	return s.accessFor(user)
}

func (s *Server) changeAdminPassword(w http.ResponseWriter, r *http.Request) {
	if accessFrom(r).user.Demo {
		fail(w, 403, errors.New("体验账号不能修改密码"))
		return
	}
	var input struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := decode(w, r, &input); err != nil {
		fail(w, 400, err)
		return
	}
	var user AdminUser
	if err := s.Store.get("admin-users", accessFrom(r).user.Username, &user); err != nil {
		fail(w, 500, err)
		return
	}
	if !adminPasswordMatches(user, input.CurrentPassword) {
		fail(w, 401, errors.New("当前密码错误"))
		return
	}
	if err := passwordPolicy(input.NewPassword); err != nil {
		fail(w, 400, err)
		return
	}
	updated, err := newAdminUser(user.Username, input.NewPassword)
	if err != nil {
		fail(w, 400, err)
		return
	}
	user.Salt, user.Hash, user.Iterations, user.UpdatedAt = updated.Salt, updated.Hash, updated.Iterations, updated.UpdatedAt
	if err = s.Store.put("admin-users", user.Username, user); err != nil {
		fail(w, 500, err)
		return
	}
	reply(w, 200, map[string]string{"username": user.Username})
}

func (s *Server) adminAuth(w http.ResponseWriter, r *http.Request) (*access, bool) {
	a, err := s.authorizeAdmin(r)
	if err != nil {
		fail(w, 403, err)
		return nil, false
	}
	if a == nil {
		fail(w, 401, errors.New("请重新登录"))
		return nil, false
	}
	return a, true
}
