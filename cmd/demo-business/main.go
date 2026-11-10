package main

import (
	"device-trust-platform/internal/platform"
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	configFile := flag.String("config", "data/demo.json", "private demo configuration")
	addr := flag.String("addr", "127.0.0.1:9444", "HTTPS listen address")
	cert := flag.String("tls-cert", "config/localhost-cert.pem", "TLS certificate")
	key := flag.String("tls-key", "config/localhost-key.pem", "TLS key")
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
	server := http.Server{Addr: *addr, Handler: business.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 90 * time.Second, IdleTimeout: 60 * time.Second}
	log.Printf("Demo business: https://%s", *addr)
	log.Fatal(server.ListenAndServeTLS(*cert, *key))
}
