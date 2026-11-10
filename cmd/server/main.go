package main

import (
	"device-trust-platform/internal/platform"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8099", "listen address")
	data := flag.String("data", "data", "record directory")
	web := flag.String("web", "web/dist", "console assets")
	cert := flag.String("tls-cert", "", "TLS certificate PEM")
	key := flag.String("tls-key", "", "TLS private key PEM")
	roots := flag.String("attestation-roots", "", "trusted attestation root PEM bundle")
	revocations := flag.String("attestation-revocations", "", "fresh Google revocation snapshot JSON")
	adminFile := flag.String("admin-token-file", "", "private file containing administrator token")
	adminAddr := flag.String("admin-addr", "", "optional loopback-only HTTP console")
	flag.Parse()
	token := os.Getenv("DTP_ADMIN_TOKEN")
	if *adminFile != "" {
		value, err := os.ReadFile(*adminFile)
		if err != nil {
			log.Fatal(err)
		}
		token = strings.TrimSpace(string(value))
	}
	if len(token) < 24 {
		log.Fatal("DTP_ADMIN_TOKEN must contain at least 24 characters")
	}
	store, err := platform.Open(*data)
	if err != nil {
		log.Fatal(err)
	}
	var trust *platform.AttestationVerifier
	if *roots != "" || *revocations != "" {
		if *roots == "" || *revocations == "" {
			log.Fatal("both attestation roots and revocations are required")
		}
		trust, err = platform.LoadAttestationVerifier(*roots, *revocations)
		if err != nil {
			log.Fatal(err)
		}
	}
	if (*cert == "") != (*key == "") {
		log.Fatal("both TLS certificate and key are required")
	}
	if *cert == "" {
		host, _, err := net.SplitHostPort(*addr)
		if err != nil {
			log.Fatal(err)
		}
		ip := net.ParseIP(host)
		if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
			log.Fatal("TLS is required for non-loopback listeners")
		}
	}
	handler := (&platform.Server{Store: store, AdminToken: token, Trust: trust}).Handler(*web)
	if *adminAddr != "" {
		host, _, err := net.SplitHostPort(*adminAddr)
		if err != nil {
			log.Fatal(err)
		}
		ip := net.ParseIP(host)
		if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
			log.Fatal("HTTP console must listen on loopback")
		}
		admin := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if strings.HasPrefix(r.URL.Path, "/v1/") {
				http.NotFound(w, r)
				return
			}
			handler.ServeHTTP(w, r)
		})
		go func() {
			log.Printf("Local console: http://%s", *adminAddr)
			server := http.Server{Addr: *adminAddr, Handler: admin, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
			log.Fatal(server.ListenAndServe())
		}()
	}
	srv := http.Server{Addr: *addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	if *cert != "" {
		log.Printf("Device Trust: https://%s", *addr)
		log.Fatal(srv.ListenAndServeTLS(*cert, *key))
		return
	}
	log.Printf("Device Trust: http://%s", *addr)
	log.Fatal(srv.ListenAndServe())
}
