package main

import (
	"device-trust-platform/internal/platform"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"
)

func main() {
	dir := flag.String("data", "data", "runtime data directory")
	build := flag.String("build-info", "config/build-info.json", "APK build information")
	ca := flag.String("ca", "config/ca.pem", "TLS CA public certificate")
	check := flag.Bool("check", false, "check both local HTTPS services with CA verification")
	flag.Parse()
	if *check {
		data, err := os.ReadFile(filepath.Join(*dir, "demo.json"))
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
		business.Client.Timeout = time.Second
		for _, url := range []string{config.PlatformURL + "/health", "https://localhost:9444/health"} {
			deadline := time.Now().Add(15 * time.Second)
			for {
				response, requestErr := business.Client.Get(url)
				if requestErr == nil {
					response.Body.Close()
					if response.StatusCode == 200 {
						break
					}
					requestErr = fmt.Errorf("HTTP %d", response.StatusCode)
				}
				if time.Now().After(deadline) {
					log.Fatalf("%s: %v", url, requestErr)
				}
				time.Sleep(300 * time.Millisecond)
			}
		}
		fmt.Println("Both HTTPS services ready; certificate validation passed.")
		return
	}
	data, err := os.ReadFile(*build)
	if err != nil {
		log.Fatal(err)
	}
	var info struct {
		APKSigner string `json:"apkSignerSha256"`
	}
	if err = json.Unmarshal(data, &info); err != nil {
		log.Fatal(err)
	}
	if len(info.APKSigner) != 64 {
		log.Fatal("missing APK signer SHA-256")
	}
	absolute, err := filepath.Abs(*dir)
	if err != nil {
		log.Fatal(err)
	}
	absoluteCA, err := filepath.Abs(*ca)
	if err != nil {
		log.Fatal(err)
	}
	if _, err = platform.InitializeDemo(absolute, absoluteCA, info.APKSigner); err != nil {
		log.Fatal(err)
	}
	fmt.Println("Demo configuration ready. Credentials: data/admin-token and data/demo.json")
}
