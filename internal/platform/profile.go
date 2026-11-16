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
	RiskLevel      string     `json:"riskLevel"`
	EnvScore       int        `json:"envScore"`
	BusinessScore  int        `json:"businessScore"`
	Factors        []RuleHit  `json:"factors"`
	HighEvents     int        `json:"highEvents"`
	LastAction     string     `json:"lastAction,omitempty"`
	lastEvent      time.Time
	peakEvent      *Event
	AppID          string     `json:"appId"`
	InstallationID string     `json:"installationId"`
	FirstSeen      time.Time  `json:"firstSeen"`
	LastSeen       time.Time  `json:"lastSeen"`
	LastEventAt    *time.Time `json:"lastEventAt,omitempty"`
	ReportCount    int        `json:"reportCount"`
	EventCount     int        `json:"eventCount"`
	AccountCount   int        `json:"accountCount"`
	LastIP         string     `json:"lastIp,omitempty"`
	Region         *IPRegion  `json:"region,omitempty"`
	LatestReport   Report     `json:"latestReport"`
}

// Business risk on a device decays out of the profile after this window.
const deviceRiskWindow = 7 * 24 * time.Hour

func riskLevel(score int) string {
	switch {
	case score >= 70:
		return "high"
	case score >= 30:
		return "medium"
	default:
		return "low"
	}
}

// environmentHits are the device-only signals from one report, without business history.
func environmentHits(r Report) []RuleHit {
	hits := []RuleHit{}
	evaluateDevice(&Event{}, r, nil, func(points int, code string) {
		hits = append(hits, RuleHit{Code: code, Points: points, Source: "client", Dimension: "client_environment"})
	})
	if roots := rootIndicators(r); roots >= 2 {
		hits = append(hits, RuleHit{Code: "root_indicators", Points: 20, Source: "client", Dimension: "client_environment", Observed: roots, Threshold: 2})
	}
	return append(hits, resourceHits(r)...)
}

// Profiles are scoped to the app and the untrusted client installation identifier.
// A device score is the higher of its current environment score and its
// peak business event score within deviceRiskWindow.
func profiles(reports []Report, events []Event, appID string) []Profile {
	now := time.Now().UTC()
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
	for i := range events {
		e := events[i]
		key := e.AppID + ":" + e.InstallationID
		p := devices[key]
		if p == nil {
			continue
		}
		p.EventCount++
		accounts[key][e.AccountID] = true
		if e.ReceivedAt.After(p.lastEvent) {
			p.lastEvent = e.ReceivedAt
			p.LastAction = e.Action
			p.LastIP = e.ClientIP
			at := e.ReceivedAt
			p.LastEventAt = &at
		}
		if now.Sub(e.ReceivedAt) <= deviceRiskWindow {
			if e.Score >= 70 {
				p.HighEvents++
			}
			if p.peakEvent == nil || e.Score > p.peakEvent.Score {
				p.peakEvent = &events[i]
			}
		}
	}
	result := []Profile{}
	for key, p := range devices {
		p.AccountCount = len(accounts[key])
		if p.LastIP == "" {
			p.LastIP = p.LatestReport.ObservedIP
		}
		if p.LatestReport.Resources != nil {
			p.Region = p.LatestReport.Resources.IP
		}
		p.Factors = environmentHits(p.LatestReport)
		for _, hit := range p.Factors {
			p.EnvScore += hit.Points
		}
		p.EnvScore = min(p.EnvScore, 100)
		if p.peakEvent != nil {
			p.BusinessScore = p.peakEvent.Score
			for _, hit := range p.peakEvent.Hits {
				if hit.Source != "client" && hit.Source != "resource" {
					p.Factors = append(p.Factors, hit)
				}
			}
		}
		p.RiskScore = max(p.EnvScore, p.BusinessScore)
		p.RiskLevel = riskLevel(p.RiskScore)
		result = append(result, *p)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].LastSeen.After(result[j].LastSeen) })
	return result
}
