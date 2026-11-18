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
	LinkID         string     `json:"linkId,omitempty"`
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
	hits = append(hits, resourceHits(r)...)
	return append(hits, recoveryHit(r)...)
}

// businessPoints is an event's score without its device-environment hits. The
// environment is judged from the latest report only, so a device recovers as
// soon as it reports a clean environment (e.g. USB debugging switched off).
func businessPoints(e Event) int {
	points := 0
	for _, hit := range e.Hits {
		if hit.Source != "client" && hit.Source != "resource" && hit.Source != "recovery" {
			points += hit.Points
		}
	}
	return min(points, 100)
}

// Profiles are scoped to the app and the untrusted client installation identifier.
// A device score is its current environment score (latest report) plus the
// peak business-behaviour score within deviceRiskWindow.
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
			p.LinkID = r.LinkID
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
			if score := businessPoints(e); p.peakEvent == nil || score > p.BusinessScore {
				p.peakEvent = &events[i]
				p.BusinessScore = score
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
		p.EnvScore = max(min(p.EnvScore, 100), 0)
		if p.peakEvent != nil {
			for _, hit := range p.peakEvent.Hits {
				if hit.Source != "client" && hit.Source != "resource" && hit.Source != "recovery" {
					p.Factors = append(p.Factors, hit)
				}
			}
		}
		p.RiskScore = min(p.EnvScore+p.BusinessScore, 100)
		p.RiskLevel = riskLevel(p.RiskScore)
		result = append(result, *p)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].LastSeen.After(result[j].LastSeen) })
	return result
}
