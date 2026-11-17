package platform

import (
	"fmt"
	"testing"
	"time"
)

func lanReport(id, link, ip, gateway, bssid string, at time.Time) Report {
	signals := map[string]Signal{
		"model": {Status: "available", Value: "M-" + id},
		"lan":   {Status: "available", Value: map[string]any{"transport": "wifi", "interface": "wlan0", "addresses": []any{map[string]any{"address": ip, "prefix": 24}}, "gateway": gateway}},
	}
	if bssid != "" {
		signals["bssid"] = Signal{Status: "available", Value: bssid}
	}
	return Report{ID: id, AppID: "a", InstallationID: "key:" + id, DeviceID: "d-" + id, LinkID: link, ObservedIP: "1.2.3.4", ReceivedAt: at, Signals: signals, Resources: &ResourceAssessment{}}
}

func TestLANClusterAndTargets(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Store: store}
	now := time.Now().UTC()
	for i := 0; i < 4; i++ {
		store.put("reports", fmt.Sprint("p", i), lanReport(fmt.Sprint("p", i), fmt.Sprint("L", i), fmt.Sprint("192.168.1.", 10+i), "192.168.1.1", "", now.Add(-time.Hour)))
	}
	// Same subnet and gateway behind another public IP: a different home.
	other := lanReport("x", "LX", "192.168.1.50", "192.168.1.1", "", now.Add(-time.Hour))
	other.ObservedIP = "5.6.7.8"
	store.put("reports", "x", other)
	// Different public IP but the same router BSSID: same room.
	b1 := lanReport("b1", "LB1", "10.0.0.5", "10.0.0.1", "AA:BB:CC:00:11:22", now.Add(-time.Hour))
	b1.ObservedIP = "9.9.9.9"
	store.put("reports", "b1", b1)

	self := lanReport("self", "LS", "192.168.1.20", "192.168.1.1", "", now)
	self.Signals["lanProbe"] = Signal{Status: "available", Value: map[string]any{"targets": 4, "reachable": []any{"192.168.1.10"}, "adb": []any{"192.168.1.11"}, "gateway": true}}
	store.put("reports", "self", self)
	if err := s.lanChecks(&self); err != nil {
		t.Fatal(err)
	}
	codes := map[string]ResourceCheck{}
	for _, c := range self.Resources.Checks {
		codes[c.Code] = c
	}
	if codes["lan_device_cluster"].Points != 20 || codes["lan_adb_peers"].Points != 30 || codes["lan_peers_reachable"].State != "matched" {
		t.Fatal(self.Resources.Checks)
	}

	targets, err := s.lanTargets("a", "key:self")
	if err != nil || len(targets) != 4 {
		t.Fatal(targets, err)
	}
	for _, ip := range targets {
		if ip == "192.168.1.50" || ip == "192.168.1.20" {
			t.Fatal("target from another LAN or self", targets)
		}
	}

	withBSSID := lanReport("b2", "LB2", "172.16.0.9", "172.16.0.1", "aa:bb:cc:00:11:22", now)
	if peers := lanPeers(withBSSID, []Report{b1}, lanClusterWindow); len(peers) != 1 {
		t.Fatal("bssid peers", peers)
	}

	// Isolation: gateway answers, no peer does; reported but not scored.
	iso := lanReport("iso", "LI", "192.168.1.30", "192.168.1.1", "", now)
	iso.Signals["lanProbe"] = Signal{Status: "available", Value: map[string]any{"targets": 3, "reachable": []any{}, "adb": []any{}, "gateway": true}}
	if err := s.lanChecks(&iso); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range iso.Resources.Checks {
		found = found || (c.Code == "lan_isolation" && c.Points == 0)
	}
	if !found {
		t.Fatal(iso.Resources.Checks)
	}

	// Cellular addresses are carrier-assigned: no LAN key.
	cell := lanReport("c", "LC", "10.20.30.40", "10.20.30.1", "", now)
	cell.Signals["lan"] = Signal{Status: "available", Value: map[string]any{"transport": "cellular", "addresses": []any{map[string]any{"address": "10.20.30.40", "prefix": 24}}, "gateway": "10.20.30.1"}}
	if len(lanKeys(cell)) != 0 {
		t.Fatal("cellular lan key")
	}
	// An eth0 LAN that the ROM labels cellular (box phones) still counts.
	box := lanReport("box", "LBox", "192.168.1.2", "192.168.0.1", "", now)
	box.Signals["lan"] = Signal{Status: "available", Value: map[string]any{"transport": "cellular", "interface": "eth0", "addresses": []any{map[string]any{"address": "192.168.1.2", "prefix": 22}}, "gateway": "192.168.0.1"}}
	if ip, subnet, _ := lanSegment(box); ip != "192.168.1.2" || subnet.String() != "192.168.0.0/22" {
		t.Fatal(ip, subnet)
	}
}
