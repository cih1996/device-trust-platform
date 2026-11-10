package platform

import (
	"encoding/json"
	"errors"
	"strings"
)

// HashBusinessRequest is computed by the authenticated business backend, never
// accepted from the app as an authoritative request digest. body must be the
// backend's normalized, complete business intent (including amount/recipient).
func HashBusinessRequest(method, path, account string, body []byte) string {
	value := struct {
		Version  string `json:"v"`
		Method   string `json:"method"`
		Path     string `json:"path"`
		Account  string `json:"account"`
		BodyHash string `json:"bodyHash"`
	}{"dtp-request-1", strings.ToUpper(method), path, account, digest(string(body))}
	data, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return digest(string(data))
}
func validRequestHash(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, c := range value {
		if c < '0' || c > '9' {
			if c < 'a' || c > 'f' {
				return false
			}
		}
	}
	return true
}

var errRequestBinding = errors.New("authenticated business request SHA-256 binding required")
