package platform

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
)

// Each record is published atomically. No shared mutable in-memory database.
type Store struct{ Root string }

func Open(root string) (*Store, error) {
	for _, name := range []string{"apps", "reports", "events", "policies", "outcomes", "reviews", "security", "challenges", "enrollments", "device-keys", "revoked-keys", "resolutions", "claims", "executions"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0700); err != nil {
			return nil, err
		}
	}
	return &Store{root}, nil
}
func token() string { return hex.EncodeToString(randomBytes(24)) }
func randomBytes(n int) []byte {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return b
}
func digest(v string) string                 { h := sha256.Sum256([]byte(v)); return hex.EncodeToString(h[:]) }
func (s *Store) path(kind, id string) string { return filepath.Join(s.Root, kind, digest(id)+".json") }
func (s *Store) put(kind, id string, v any) error {
	return s.publish(kind, id, v, false)
}

// A hard link publishes a complete immutable record only if its ID is absent.
// The temporary file and destination stay on the same filesystem.
func (s *Store) create(kind, id string, v any) error {
	return s.publish(kind, id, v, true)
}

func (s *Store) publish(kind, id string, v any, exclusive bool) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Join(s.Root, kind), ".write-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err = f.Write(b); err != nil {
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
	if exclusive {
		return os.Link(f.Name(), s.path(kind, id))
	}
	return os.Rename(f.Name(), s.path(kind, id))
}
func (s *Store) get(kind, id string, v any) error {
	b, err := os.ReadFile(s.path(kind, id))
	if err != nil {
		return err
	}
	return json.Unmarshal(b, v)
}
func records[T any](s *Store, kind string) ([]T, error) {
	entries, err := os.ReadDir(filepath.Join(s.Root, kind))
	if err != nil {
		return nil, err
	}
	result := []T{}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		b, err := os.ReadFile(filepath.Join(s.Root, kind, entry.Name()))
		if err != nil {
			return nil, err
		}
		var value T
		if err = json.Unmarshal(b, &value); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, nil
}
