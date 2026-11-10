package main

import (
	"device-trust-platform/internal/platform"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

func fetch(url string) ([]byte, error) {
	client := http.Client{Timeout: 20 * time.Second}
	response, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return nil, fmt.Errorf("%s: HTTP %d", url, response.StatusCode)
	}
	return io.ReadAll(io.LimitReader(response.Body, 4<<20))
}
func publish(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".trust-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
func main() {
	dir := flag.String("out", "data", "trust snapshot directory")
	flag.Parse()
	roots, err := fetch("https://android.googleapis.com/attestation/root")
	if err != nil {
		log.Fatal(err)
	}
	bundle, err := platform.RootsJSONToPEM(roots)
	if err != nil {
		log.Fatal(err)
	}
	status, err := fetch("https://android.googleapis.com/attestation/status")
	if err != nil {
		log.Fatal(err)
	}
	var parsed struct {
		Entries map[string]any `json:"entries"`
	}
	if err = json.Unmarshal(status, &parsed); err != nil {
		log.Fatal(err)
	}
	if parsed.Entries == nil {
		log.Fatal("revocation snapshot missing entries")
	}
	if err = os.MkdirAll(*dir, 0700); err != nil {
		log.Fatal(err)
	}
	if err = publish(filepath.Join(*dir, "attestation-roots.pem"), bundle); err != nil {
		log.Fatal(err)
	}
	if err = publish(filepath.Join(*dir, "attestation-status.json"), status); err != nil {
		log.Fatal(err)
	}
	fmt.Printf("Official attestation roots and %d revocation entries refreshed.\n", len(parsed.Entries))
}
