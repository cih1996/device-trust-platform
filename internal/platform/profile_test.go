package platform

import (
	"testing"
	"time"
)

func TestProfilesUseApplicationScopeAndLatestSample(t *testing.T) {
	now := time.Now().UTC()
	old := Report{ID: "old", AppID: "one", InstallationID: "shared", ReceivedAt: now.Add(-time.Hour)}
	latest := Report{ID: "new", AppID: "one", InstallationID: "shared", ReceivedAt: now}
	other := Report{ID: "other", AppID: "two", InstallationID: "shared", ReceivedAt: now}
	history := []Event{{AppID: "one", InstallationID: "shared", AccountID: "a"}, {AppID: "one", InstallationID: "shared", AccountID: "a"}, {AppID: "one", InstallationID: "shared", AccountID: "b"}, {AppID: "two", InstallationID: "shared", AccountID: "c"}}
	result := profiles([]Report{latest, other, old}, history, "one")
	if len(result) != 1 {
		t.Fatal(result)
	}
	p := result[0]
	if p.LatestReport.ID != "new" || p.ReportCount != 2 || p.EventCount != 3 || p.AccountCount != 2 || !p.FirstSeen.Equal(old.ReceivedAt) {
		t.Fatal(p)
	}
	if all := profiles([]Report{latest, other, old}, history, ""); len(all) != 2 {
		t.Fatal(all)
	}
}

// USB debugging during an event must not keep the device high once a clean report arrives.
func TestProfileRecoversWhenEnvironmentIsClean(t *testing.T) {
	now := time.Now().UTC()
	clean := Report{AppID: "one", InstallationID: "i", ReceivedAt: now, Signals: map[string]Signal{}}
	risky := Event{AppID: "one", InstallationID: "i", ReceivedAt: now.Add(-time.Hour), Score: 80, Hits: []RuleHit{
		{Code: "adb_enabled", Points: 5, Source: "client"},
		{Code: "injected_touches", Points: 30, Source: "client"},
		{Code: "exposed_listening_ports", Points: 15, Source: "client"},
		{Code: "frequent_events_1m", Points: 30, Source: "business"},
	}}
	p := profiles([]Report{clean}, []Event{risky}, "one")[0]
	if p.EnvScore != 0 || p.BusinessScore != 30 || p.RiskScore != 30 || p.RiskLevel != "medium" {
		t.Fatalf("env %d business %d risk %d %s", p.EnvScore, p.BusinessScore, p.RiskScore, p.RiskLevel)
	}
}
