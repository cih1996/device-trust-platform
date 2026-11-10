package platform

import (
	"net"
	"net/http"
)

// TrustLoopbackProxy is enabled only for a loopback listener behind our Nginx.
// Nginx must overwrite X-Real-IP with $remote_addr.
func TrustLoopbackProxy(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host, port, err := net.SplitHostPort(r.RemoteAddr)
		if err == nil && net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback() {
			if ip := net.ParseIP(r.Header.Get("X-Real-IP")); ip != nil {
				r = r.Clone(r.Context())
				r.RemoteAddr = net.JoinHostPort(ip.String(), port)
			}
		}
		next.ServeHTTP(w, r)
	})
}
