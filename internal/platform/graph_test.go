package platform

import (
	"fmt"
	"testing"
	"time"
)

func TestRelationGraph(t *testing.T) {
	now := time.Now().UTC()
	var reports []Report
	// A studio: six phones on one LAN.
	for i := 0; i < 6; i++ {
		id := fmt.Sprint("s", i)
		reports = append(reports, lanReport(id, "L"+id, fmt.Sprint("192.168.1.", 10+i), "192.168.1.1", "", now.Add(-time.Hour)))
	}
	// Two phones that only share an account.
	a, b := lanReport("a", "La", "10.0.0.2", "10.0.0.1", "", now), lanReport("b", "Lb", "10.1.0.2", "10.1.0.1", "", now)
	a.ObservedIP, b.ObservedIP = "7.7.7.7", "8.8.8.8"
	// Strangers on the same carrier NAT address: not related.
	c, d := Report{ID: "c", AppID: "a", InstallationID: "key:c", LinkID: "Lc", ObservedIP: "9.9.9.9", ReceivedAt: now, Signals: map[string]Signal{"transport": {Status: "available", Value: "cellular"}}},
		Report{ID: "d", AppID: "a", InstallationID: "key:d", LinkID: "Ld", ObservedIP: "9.9.9.9", ReceivedAt: now, Signals: map[string]Signal{"transport": {Status: "available", Value: "cellular"}}}
	// Old report outside the window.
	old := lanReport("old", "Lold", "192.168.1.99", "192.168.1.1", "", now.Add(-30*24*time.Hour))
	reports = append(reports, a, b, c, d, old)
	events := []Event{
		{AppID: "a", InstallationID: "key:a", AccountID: "u1", ReceivedAt: now},
		{AppID: "a", InstallationID: "key:b", AccountID: "u1", ReceivedAt: now},
	}
	g := relationGraph(reports, events, "", now.Add(-7*24*time.Hour))
	if len(g.Clusters) != 2 || g.Clusters[0].Size != 6 || g.Clusters[0].Edges != 15 || g.Clusters[1].Size != 2 {
		t.Fatalf("%+v", g.Clusters)
	}
	// LAN + same broadband egress on the same day: 1-(0.4*0.7) = 0.72.
	if g.Edges[0].Score != 0.8 || g.Edges[0].Reasons[0].Kind != "account" {
		t.Fatalf("%+v", g.Edges[0])
	}
	if e := g.Edges[1]; e.Score != 0.72 || len(e.Reasons) != 2 || e.Reasons[0].Kind != "lan" || e.Reasons[1].Kind != "ip" {
		t.Fatalf("%+v", e)
	}
	for _, n := range g.Nodes {
		if n.ID == digest("a:key:c") || n.ID == digest("a:key:old") {
			t.Fatal("unrelated or stale node included", n)
		}
	}
	if only := relationGraph(reports, events, "other", now.Add(-7*24*time.Hour)); len(only.Nodes) != 0 {
		t.Fatal(only.Nodes)
	}
}
