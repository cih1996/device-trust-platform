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
