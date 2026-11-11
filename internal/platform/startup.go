package platform

import (
	"encoding/json"
	"errors"
	"net"
	"net/http"
)

// This public, app-scoped bootstrap only permits device observation. It never
// authenticates an account, issues a business permit, or exposes the app key.
func (d *DemoBusiness) prepareDevice(w http.ResponseWriter, r *http.Request) {
	var input struct {
		AppID          string `json:"appId"`
		InstallationID string `json:"installationId"`
		KeyID          string `json:"keyId"`
	}
	if err := decode(w, r, &input); err != nil {
		fail(w, 400, err)
		return
	}
	if input.AppID != d.Config.AppID {
		fail(w, 403, errors.New("bootstrap application mismatch"))
		return
	}
	if len(input.InstallationID) < 16 || len(input.InstallationID) > 128 {
		fail(w, 400, errors.New("installation observation ID required"))
		return
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		fail(w, 500, err)
		return
	}
	account := "guest:" + digest(input.InstallationID)
	eventID := "startup-" + token()
	kind := "device_init"
	if input.KeyID != "" {
		kind = "device_refresh"
	}
	intent, err := json.Marshal(input)
	if err != nil {
		fail(w, 500, err)
		return
	}
	context := BusinessContext{RequestHash: HashBusinessRequest("POST", "/device/prepare", account, intent)}
	var challenge json.RawMessage
	if err = d.call("/v1/challenges", map[string]any{"appId": d.Config.AppID, "eventId": eventID, "accountId": account, "type": kind, "context": context, "clientIp": ip, "keyId": input.KeyID}, &challenge); err != nil {
		fail(w, 502, err)
		return
	}
	reply(w, 201, map[string]any{"appId": d.Config.AppID, "platformUrl": d.Config.PlatformURL, "challenge": challenge})
}
