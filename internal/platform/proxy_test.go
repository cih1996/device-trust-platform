package platform

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestProxyPreservesClientAndRejectsUntrustedHeader(t *testing.T) {
	for _, test := range []struct{ peer, header, want string }{
		{"127.0.0.1:2", "203.0.113.7", "203.0.113.7:2"},
		{"[::1]:2", "2001:db8::1", "[2001:db8::1]:2"},
		{"198.51.100.4:2", "203.0.113.7", "198.51.100.4:2"},
		{"127.0.0.1:2", "invalid", "127.0.0.1:2"},
	} {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = test.peer
		r.Header.Set("X-Real-IP", test.header)
		TrustLoopbackProxy(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.RemoteAddr != test.want {
				t.Errorf("got %s want %s", r.RemoteAddr, test.want)
			}
		})).ServeHTTP(httptest.NewRecorder(), r)
	}
}
