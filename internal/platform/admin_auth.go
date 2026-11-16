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
}

type AdminSession struct {
	Username  string    `json:"username"`
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
	if strings.TrimSpace(username) == "" || len(password) < 12 {
		return AdminUser{}, errors.New("管理账号和至少 12 位密码为必填项")
	}
	salt := randomBytes(16)
	return AdminUser{Username: strings.TrimSpace(username), Salt: base64.RawStdEncoding.EncodeToString(salt), Hash: base64.RawStdEncoding.EncodeToString(deriveAdminPassword(password, salt, adminPasswordIterations)), Iterations: adminPasswordIterations, UpdatedAt: time.Now().UTC()}, nil
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

func (s *Server) adminUser() (AdminUser, error) {
	var user AdminUser
	if s.AdminUser == "" {
		return user, errors.New("admin account authentication is not configured")
	}
	if err := s.Store.get("admin-users", s.AdminUser, &user); err != nil {
		return user, err
	}
	return user, nil
}

func (s *Server) createAdminSession(username string) (string, error) {
	value := token()
	session := AdminSession{Username: username, ExpiresAt: time.Now().UTC().Add(12 * time.Hour)}
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
	user, err := s.adminUser()
	if err != nil {
		fail(w, 500, err)
		return
	}
	if input.Username != user.Username || !adminPasswordMatches(user, input.Password) {
		fail(w, 401, errors.New("账号或密码错误"))
		return
	}
	session, err := s.createAdminSession(user.Username)
	if err != nil {
		fail(w, 500, err)
		return
	}
	reply(w, 200, map[string]any{"token": session, "username": user.Username, "expiresAt": time.Now().UTC().Add(12 * time.Hour)})
}

func (s *Server) authorizeAdmin(r *http.Request) bool {
	if s.AdminUser == "" {
		return s.AdminToken != "" && same(bearer(r), s.AdminToken)
	}
	var session AdminSession
	if err := s.Store.get("admin-sessions", bearer(r), &session); err != nil {
		return false
	}
	return session.Username == s.AdminUser && time.Now().UTC().Before(session.ExpiresAt)
}

func (s *Server) changeAdminPassword(w http.ResponseWriter, r *http.Request) {
	var input struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	if err := decode(w, r, &input); err != nil {
		fail(w, 400, err)
		return
	}
	user, err := s.adminUser()
	if err != nil {
		fail(w, 500, err)
		return
	}
	if !adminPasswordMatches(user, input.CurrentPassword) {
		fail(w, 401, errors.New("当前密码错误"))
		return
	}
	updated, err := newAdminUser(user.Username, input.NewPassword)
	if err != nil {
		fail(w, 400, err)
		return
	}
	if err = s.Store.put("admin-users", user.Username, updated); err != nil {
		fail(w, 500, err)
		return
	}
	reply(w, 200, map[string]string{"username": updated.Username})
}

func (s *Server) adminAuth(w http.ResponseWriter, r *http.Request) bool {
	if s.authorizeAdmin(r) {
		return true
	}
	fail(w, 401, errors.New("请重新登录"))
	return false
}
