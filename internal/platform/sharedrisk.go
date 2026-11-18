package platform

import (
	"fmt"
	"time"
)

// Shared risk across teams. A handset (link ID) that another team has seen
// as high risk raises the score here, and the console shows how many other
// teams saw it and how risky it was. Nothing else crosses teams: no app
// names, accounts, events or signals.

const (
	sharedRiskCode   = "linked_risk_elsewhere"
	sharedRiskWindow = 30 * 24 * time.Hour
	sharedRiskPoints = 20
)

type SharedRisk struct {
	Teams    int        `json:"teams"`
	Devices  int        `json:"devices"`
	HighRisk int        `json:"highRisk"`
	MaxScore int        `json:"maxScore"`
	LastSeen *time.Time `json:"lastSeen,omitempty"`
}

// teamOf maps app IDs to their team; apps without one belong to the platform.
func (s *Server) teamOf() (map[string]string, error) {
	apps, err := records[App](s.Store, "apps")
	if err != nil {
		return nil, err
	}
	teams := map[string]string{}
	for _, a := range apps {
		teams[a.ID] = "team:" + a.TeamID
	}
	return teams, nil
}

// withoutShared is a score with the shared-risk bonus taken out, so a bonus
// never feeds another team's bonus.
func withoutShared(score int, hits []RuleHit) int {
	for _, h := range hits {
		if h.Code == sharedRiskCode {
			score -= h.Points
		}
	}
	return max(score, 0)
}

func sharedRisk(appID, linkID string, teams map[string]string, reports []Report, events []Event) SharedRisk {
	var out SharedRisk
	if linkID == "" {
		return out
	}
	own, since := teams[appID], time.Now().Add(-sharedRiskWindow)
	type device struct {
		team  string
		score int
		seen  time.Time
	}
	devices := map[string]*device{}
	for _, r := range reports {
		if r.LinkID != linkID || teams[r.AppID] == own || r.ReceivedAt.Before(since) {
			continue
		}
		key := r.AppID + ":" + r.InstallationID
		d := devices[key]
		if d == nil {
			d = &device{team: teams[r.AppID]}
			devices[key] = d
		}
		if r.ReceivedAt.After(d.seen) {
			hits := environmentHits(r)
			total := 0
			for _, h := range hits {
				total += h.Points
			}
			d.seen, d.score = r.ReceivedAt, max(d.score, withoutShared(min(total, 100), hits))
		}
	}
	for _, e := range events {
		if d := devices[e.AppID+":"+e.InstallationID]; d != nil && !e.ReceivedAt.Before(since) {
			d.score = max(d.score, withoutShared(e.Score, e.Hits))
		}
	}
	seenTeams := map[string]bool{}
	for _, d := range devices {
		out.Devices++
		seenTeams[d.team] = true
		out.MaxScore = max(out.MaxScore, d.score)
		if d.score >= 70 {
			out.HighRisk++
		}
		if out.LastSeen == nil || d.seen.After(*out.LastSeen) {
			seen := d.seen
			out.LastSeen = &seen
		}
	}
	out.Teams = len(seenTeams)
	return out
}

// sharedRiskCheck adds the bonus to a new report when another team saw the
// same handset as high risk.
func (s *Server) sharedRiskCheck(r *Report) error {
	if r.LinkID == "" {
		return nil
	}
	teams, err := s.teamOf()
	if err != nil {
		return err
	}
	reports, events, err := s.deviceData(nil)
	if err != nil {
		return err
	}
	shared := sharedRisk(r.AppID, r.LinkID, teams, reports, events)
	if shared.HighRisk > 0 {
		r.Resources.Checks = append(r.Resources.Checks, ResourceCheck{Code: sharedRiskCode, State: "conflict", Dimension: "device", Source: "server", Points: sharedRiskPoints,
			Detail: fmt.Sprintf("同一台手机在 %d 个其他接入方被评为高风险（最高 %d 分）", shared.Teams, shared.MaxScore)})
	}
	return nil
}
