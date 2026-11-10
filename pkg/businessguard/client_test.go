package businessguard

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGuardRejectsChangedHashShadowAndExpiredPermit(t *testing.T) {
	permit := Permit{AppID: "app", OperationID: "op", RequestHash: strings.Repeat("a", 64), Mode: "enforce", Token: "private", ExpiresAt: time.Now().Add(time.Minute)}
	identity := sha256.Sum256([]byte("app:event"))
	permit.EventID = hex.EncodeToString(identity[:])
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer backend-secret" {
			t.Error("backend authentication missing")
		}
		json.NewEncoder(w).Encode(permit)
	}))
	defer server.Close()
	client := Client{BaseURL: server.URL, AppID: "app", AppKey: "backend-secret", HTTP: server.Client()}
	if _, err := client.Authorize(context.Background(), "event", "op", permit.RequestHash); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Authorize(context.Background(), "event", "op", strings.Repeat("b", 64)); err == nil {
		t.Fatal("changed amount/request hash accepted")
	}
	permit.Mode = "shadow"
	if _, err := client.Authorize(context.Background(), "event", "op", permit.RequestHash); err == nil {
		t.Fatal("shadow mode execution accepted")
	}
	permit.Mode = "enforce"
	permit.ExpiresAt = time.Now().Add(-time.Second)
	if _, err := client.Authorize(context.Background(), "event", "op", permit.RequestHash); err == nil {
		t.Fatal("expired execution accepted")
	}
}
