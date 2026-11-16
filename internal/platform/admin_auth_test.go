package platform

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAdminAccountLoginAndPasswordChange(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err = EnsureAdminUser(store, "admin", "initial-password-123"); err != nil {
		t.Fatal(err)
	}
	h := (&Server{Store: store, AdminUser: "admin"}).Handler(t.TempDir())
	login := func(password string) *httptest.ResponseRecorder {
		body, _ := json.Marshal(map[string]string{"username": "admin", "password": password})
		r := httptest.NewRequest(http.MethodPost, "/api/auth/login", bytes.NewReader(body))
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	if login("wrong-password-123").Code != http.StatusUnauthorized {
		t.Fatal("wrong password accepted")
	}
	w := login("initial-password-123")
	if w.Code != http.StatusOK {
		t.Fatalf("login failed: %s", w.Body.String())
	}
	var response map[string]any
	if err = json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	session := response["token"].(string)
	body, _ := json.Marshal(map[string]string{"currentPassword": "initial-password-123", "newPassword": "changed-password-456"})
	r := httptest.NewRequest(http.MethodPut, "/api/auth/password", bytes.NewReader(body))
	r.Header.Set("Authorization", "Bearer "+session)
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("password change failed: %s", w.Body.String())
	}
	if login("initial-password-123").Code != http.StatusUnauthorized || login("changed-password-456").Code != http.StatusOK {
		t.Fatal("password rotation did not take effect")
	}
}
