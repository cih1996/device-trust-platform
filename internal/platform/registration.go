package platform

import (
	"errors"
	"os"
	"time"
)

type DeviceRegistration struct {
	DeviceID  string    `json:"deviceId"`
	AppID     string    `json:"appId"`
	KeyID     string    `json:"keyId"`
	CreatedAt time.Time `json:"createdAt"`
}

// A public ID is an opaque label. Every report must still prove key possession.
func (s *Server) registerDevice(appID, keyID string) (string, error) {
	id := appID + ":" + keyID
	var current DeviceRegistration
	err := s.Store.get("registrations", id, &current)
	if err == nil {
		return current.DeviceID, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	current = DeviceRegistration{token(), appID, keyID, time.Now().UTC()}
	if err = s.Store.create("registrations", id, current); errors.Is(err, os.ErrExist) {
		err = s.Store.get("registrations", id, &current)
	}
	return current.DeviceID, err
}
