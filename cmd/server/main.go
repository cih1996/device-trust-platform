package main

import (
	"device-trust-platform/internal/platform"
	"flag"
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	addr := flag.String("addr", "127.0.0.1:8099", "listen address")
	data := flag.String("data", "data", "record directory")
	web := flag.String("web", "web/dist", "console assets")
	flag.Parse()
	token := os.Getenv("DTP_ADMIN_TOKEN")
	if len(token) < 24 {
		log.Fatal("DTP_ADMIN_TOKEN must contain at least 24 characters")
	}
	store, err := platform.Open(*data)
	if err != nil {
		log.Fatal(err)
	}
	srv := http.Server{Addr: *addr, Handler: (&platform.Server{Store: store, AdminToken: token}).Handler(*web), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	log.Printf("Device Trust: http://%s", *addr)
	log.Fatal(srv.ListenAndServe())
}
