// cell-sync downloads one country's OpenCelliD cell file, checks it with the
// platform's own parser and replaces the served copy only if it is sound.
// Data © OpenCelliD contributors, CC BY-SA 4.0.
package main

import (
	"device-trust-platform/internal/platform"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	tokenFile := flag.String("token-file", "", "file holding the OpenCelliD API token")
	mcc := flag.String("mcc", "460", "mobile country code to download")
	out := flag.String("out", "", "directory holding <mcc>.csv.gz")
	source := flag.String("source", "https://opencellid.org/ocid/downloads", "download endpoint")
	flag.Parse()
	if *tokenFile == "" || *out == "" {
		log.Fatal("-token-file and -out are required")
	}
	raw, err := os.ReadFile(*tokenFile)
	if err != nil {
		log.Fatal(err)
	}
	token := strings.TrimSpace(string(raw))
	target := filepath.Join(*out, *mcc+".csv.gz")
	if err = sync(*source, token, *mcc, target); err != nil {
		log.Fatal(err)
	}
}

func sync(source, token, mcc, target string) error {
	tmp, err := os.CreateTemp(filepath.Dir(target), ".cells-*.csv.gz")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	query := url.Values{"token": {token}, "type": {"mcc"}, "file": {mcc + ".csv.gz"}}
	client := http.Client{Timeout: 10 * time.Minute}
	response, err := client.Get(source + "?" + query.Encode())
	if err != nil {
		// The URL carries the token; report the failure without it.
		var u *url.Error
		if errors.As(err, &u) {
			err = u.Err
		}
		return fmt.Errorf("download: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		return fmt.Errorf("download: HTTP %d", response.StatusCode)
	}
	if _, err = io.Copy(tmp, io.LimitReader(response.Body, 512<<20)); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	// A quota or token error comes back as a small text body, not gzip; the
	// parser rejects it, and a truncated file shrinks far below the last one.
	fresh, err := platform.LoadCellDB(tmp.Name())
	if err != nil {
		return fmt.Errorf("downloaded file rejected: %w", err)
	}
	if old, err := platform.LoadCellDB(target); err == nil && fresh.Size() < old.Size()/2 {
		return fmt.Errorf("downloaded file has %d cells, previous %d; keeping previous", fresh.Size(), old.Size())
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("previous file unreadable, replacing: %v", err)
	}
	if err = os.Chmod(tmp.Name(), 0644); err != nil {
		return err
	}
	if _, err = os.Stat(target); err == nil {
		if err = os.Rename(target, strings.TrimSuffix(target, ".csv.gz")+".prev.csv.gz"); err != nil {
			return err
		}
	}
	if err = os.Rename(tmp.Name(), target); err != nil {
		return err
	}
	fmt.Printf("%s: %d cells, newest measurement %s\n", target, fresh.Size(), fresh.Updated.Format(time.RFC3339))
	return nil
}
