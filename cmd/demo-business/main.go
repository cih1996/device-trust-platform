package main

import (
	"device-trust-platform/internal/platform"
	"encoding/json"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"time"
)

func main() {
	configFile := flag.String("config", "data/demo.json", "private demo configuration")
	addr := flag.String("addr", "127.0.0.1:9444", "HTTPS listen address")
	cert := flag.String("tls-cert", "config/localhost-cert.pem", "TLS certificate")
	key := flag.String("tls-key", "config/localhost-key.pem", "TLS key")
	proxy := flag.Bool("trusted-loopback-proxy", false, "trust X-Real-IP from local Nginx")
	flag.Parse()
	data, err := os.ReadFile(*configFile)
	if err != nil {
		log.Fatal(err)
	}
	var config platform.DemoConfig
	if err = json.Unmarshal(data, &config); err != nil {
		log.Fatal(err)
	}
	business, err := platform.NewDemoBusiness(config)
	if err != nil {
		log.Fatal(err)
	}
	var handler http.Handler = business.Handler()
	if *proxy {
		handler = platform.TrustLoopbackProxy(handler)
	}
	server := http.Server{Addr: *addr, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 90 * time.Second, IdleTimeout: 60 * time.Second}
	if *cert == "" && *key == "" {
		host, _, err := net.SplitHostPort(*addr)
		if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
			log.Fatal("HTTP business listener must use loopback")
		}
		log.Printf("Demo business behind TLS proxy: http://%s", *addr)
		log.Fatal(server.ListenAndServe())
	}
	log.Printf("Demo business: https://%s", *addr)
	log.Fatal(server.ListenAndServeTLS(*cert, *key))
}
