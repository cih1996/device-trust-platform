package platform

import (
	"sort"
	"time"
)

type Profile struct {
	RegisteredAt   *time.Time `json:"registeredAt,omitempty"`
	KeyStatus      string     `json:"keyStatus"`
	ID             string     `json:"id"`
	DeviceID       string     `json:"deviceId,omitempty"`
	RiskScore      int        `json:"riskScore"`
	LastAction     string     `json:"lastAction,omitempty"`
	lastEvent      time.Time
	AppID          string    `json:"appId"`
	InstallationID string    `json:"installationId"`
	FirstSeen      time.Time `json:"firstSeen"`
	LastSeen       time.Time `json:"lastSeen"`
	ReportCount    int       `json:"reportCount"`
	EventCount     int       `json:"eventCount"`
	AccountCount   int       `json:"accountCount"`
	LatestReport   Report    `json:"latestReport"`
}

// Profiles are scoped to the app and the untrusted client installation identifier.
func profiles(reports []Report, events []Event, appID string) []Profile {
	devices := map[string]*Profile{}
	accounts := map[string]map[string]bool{}
	for _, r := range reports {
		if appID != "" && r.AppID != appID {
			continue
		}
		key := r.AppID + ":" + r.InstallationID
		p := devices[key]
		if p == nil {
			p = &Profile{ID: digest(key), AppID: r.AppID, InstallationID: r.InstallationID, FirstSeen: r.ReceivedAt}
			devices[key] = p
			accounts[key] = map[string]bool{}
		}
		p.ReportCount++
		if r.ReceivedAt.Before(p.FirstSeen) {
			p.FirstSeen = r.ReceivedAt
		}
		if r.ReceivedAt.After(p.LastSeen) {
			p.LastSeen = r.ReceivedAt
			p.LatestReport = r
			p.DeviceID = r.DeviceID
		}
	}
	for _, e := range events {
		key := e.AppID + ":" + e.InstallationID
		if p := devices[key]; p != nil {
			p.EventCount++
			accounts[key][e.AccountID] = true
			if e.ReceivedAt.After(p.lastEvent) {
				p.lastEvent = e.ReceivedAt
				p.RiskScore = e.Score
				p.LastAction = e.Action
			}
		}
	}
	result := []Profile{}
	for key, p := range devices {
		p.AccountCount = len(accounts[key])
		result = append(result, *p)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].LastSeen.After(result[j].LastSeen) })
	return result
}
