package platform

import (
	"testing"
	"time"
)

func TestRecoveryDiscount(t *testing.T) {
	soft := []RuleHit{{Code: "ip_gps_region_mismatch", Points: 10, Source: "resource"}, {Code: "lan_device_cluster", Points: 15, Source: "resource"}}
	cases := []struct {
		hits []RuleHit
		days int
		want int
	}{
		{soft, 0, 0},
		{soft, 7, 18},  // 10 fully + 15 at most half
		{soft, 30, 18}, // capped at seven days
		{soft, 3, 8},   // (10 + 7.5) * 3/7
		{append(soft, RuleHit{Code: "adb_enabled", Points: 5, Source: "client"}), 7, 0},
		{append(soft, RuleHit{Code: "frequent_events_1m", Points: 30, Source: "business"}), 7, 0},
	}
	for i, c := range cases {
		if got := recoveryDiscount(c.hits, c.days); got != c.want {
			t.Errorf("case %d: got %d want %d", i, got, c.want)
		}
	}
}

func TestRecoveryEarnedOverDaysAndLostOnHardRisk(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Store: store}
	start := time.Date(2026, 11, 1, 2, 0, 0, 0, time.UTC) // 10:00 Beijing
	report := func(at time.Time, adb bool) Report {
		r := Report{AppID: "a", InstallationID: "i", ReceivedAt: at, Signals: map[string]Signal{"adbEnabled": {Status: "available", Value: adb}},
			Resources: &ResourceAssessment{Checks: []ResourceCheck{{Code: "ip_gps_region_mismatch", State: "conflict", Points: 10}, {Code: "lan_device_cluster", State: "conflict", Points: 15}}}}
		if err := s.applyRecovery(&r); err != nil {
			t.Fatal(err)
		}
		return r
	}
	// Many reports on one day earn nothing that day.
	for i := 0; i < 5; i++ {
		if r := report(start.Add(time.Duration(i)*time.Hour), false); r.Recovery.Discount != 0 {
			t.Fatal(r.Recovery)
		}
	}
	for day := 1; day <= 7; day++ {
		r := report(start.AddDate(0, 0, day), false)
		if r.Recovery.GoodDays != day {
			t.Fatalf("day %d: %+v", day, r.Recovery)
		}
	}
	r := report(start.AddDate(0, 0, 8), false)
	score := 0
	for _, hit := range environmentHits(r) {
		score += hit.Points
	}
	if r.Recovery.Discount != 18 || score != 7 {
		t.Fatalf("discount %+v score %d", r.Recovery, score)
	}
	// USB debugging is hard risk: no discount and the standing restarts.
	if r = report(start.AddDate(0, 0, 9), true); r.Recovery.Discount != 0 {
		t.Fatal(r.Recovery)
	}
	if r = report(start.AddDate(0, 0, 10), false); r.Recovery.GoodDays != 0 || r.Recovery.Discount != 0 {
		t.Fatal(r.Recovery)
	}
	if r = report(start.AddDate(0, 0, 11), false); r.Recovery.GoodDays != 1 {
		t.Fatal(r.Recovery)
	}
	// A business-rule hit restarts it too, and that day does not count.
	if err = s.resetRecovery("a", "i", "业务规则命中"); err != nil {
		t.Fatal(err)
	}
	if r = report(time.Now().UTC(), false); r.Recovery.GoodDays != 0 {
		t.Fatal(r.Recovery)
	}
	if r = report(time.Now().UTC().AddDate(0, 0, 1), false); r.Recovery.GoodDays != 0 {
		t.Fatal(r.Recovery)
	}
}

func TestEventScoreUsesRecovery(t *testing.T) {
	r := Report{AppID: "a", InstallationID: "i", ReceivedAt: time.Now().UTC(), Signals: map[string]Signal{},
		Resources: &ResourceAssessment{Checks: []ResourceCheck{{Code: "ip_gps_region_mismatch", State: "conflict", Points: 10}}},
		Recovery:  &ReportRecovery{GoodDays: 7, Discount: 10}}
	e := Event{AppID: "a", InstallationID: "i", Type: "login", ReceivedAt: time.Now().UTC()}
	evaluateBusiness(&e, r, nil, nil, DefaultPolicy())
	if e.Score != 0 || businessPoints(e) != 0 {
		t.Fatal(e.Score, e.Hits)
	}
}
