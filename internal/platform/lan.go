package platform

import (
	"fmt"
	"net"
	"sort"
	"strings"
	"time"
)

// Studio clusters: many handsets behind one router. Reports are grouped by the
// LAN they sit on, and the SDK confirms the grouping by probing the other SDK
// devices the server places on the same LAN (see lanTargets).

const (
	lanClusterWindow  = 24 * time.Hour
	lanClusterDevices = 5
	lanTargetWindow   = 2 * time.Hour
	lanTargetLimit    = 32
	lanAdbPort        = 5555
)

type lanAddress struct {
	Address string `json:"address"`
	Prefix  int    `json:"prefix"`
}

type lanObservation struct {
	Transport  string       `json:"transport"`
	Interface  string       `json:"interface"`
	Addresses  []lanAddress `json:"addresses"`
	Gateway    string       `json:"gateway"`
	DNS        []string     `json:"dns"`
	DHCPServer string       `json:"dhcpServer"`
}

type lanProbeResult struct {
	Targets   int      `json:"targets"`
	Reachable []string `json:"reachable"`
	ADB       []string `json:"adb"`
	Gateway   *bool    `json:"gateway"`
	Millis    int      `json:"millis"`
}

// lanSegment returns the device's private IPv4 address and its subnet that
// contains the default gateway. Only Wi-Fi/Ethernet networks or LAN NICs
// (eth*, wlan*) count: cellular addresses are carrier-assigned and say nothing
// about the room. Cloud and box phones label an eth0 LAN as cellular.
func lanSegment(r Report) (ip string, subnet *net.IPNet, gateway string) {
	var lan lanObservation
	if signalObject(r, "lan", &lan) != nil {
		return "", nil, ""
	}
	lanNIC := strings.HasPrefix(lan.Interface, "eth") || strings.HasPrefix(lan.Interface, "wlan")
	if lan.Transport != "wifi" && lan.Transport != "ethernet" && !lanNIC {
		return "", nil, ""
	}
	gw := net.ParseIP(lan.Gateway).To4()
	if gw == nil {
		return "", nil, ""
	}
	for _, a := range lan.Addresses {
		addr := net.ParseIP(a.Address).To4()
		if addr == nil || !addr.IsPrivate() || a.Prefix < 8 || a.Prefix > 30 {
			continue
		}
		network := &net.IPNet{IP: addr.Mask(net.CIDRMask(a.Prefix, 32)), Mask: net.CIDRMask(a.Prefix, 32)}
		if network.Contains(gw) {
			return addr.String(), network, gw.String()
		}
	}
	return "", nil, ""
}

// lanKeys identify the LAN of a report. The router BSSID is the strongest key
// (needs location permission); otherwise public egress + subnet + gateway,
// since 192.168.1.0/24 behind one carrier NAT address can be two homes.
func lanKeys(r Report) []string {
	var keys []string
	if bssid, _ := signalValue(r, "bssid").(string); bssid != "" {
		keys = append(keys, "bssid:"+strings.ToLower(bssid))
	}
	if _, subnet, gateway := lanSegment(r); subnet != nil && r.ObservedIP != "" {
		keys = append(keys, "net:"+r.ObservedIP+"|"+subnet.String()+"|"+gateway)
	}
	return keys
}

func sharesKey(a, b []string) bool {
	for _, x := range a {
		for _, y := range b {
			if x == y {
				return true
			}
		}
	}
	return false
}

func handset(r Report) string {
	if r.LinkID != "" {
		return "link:" + r.LinkID
	}
	return "device:" + r.AppID + ":" + r.DeviceID
}

// LANPeer is another handset seen on the same LAN.
type LANPeer struct {
	ID       string    `json:"id"`
	AppID    string    `json:"appId"`
	DeviceID string    `json:"deviceId"`
	Model    string    `json:"model,omitempty"`
	LanIP    string    `json:"lanIp,omitempty"`
	LastSeen time.Time `json:"lastSeen"`
}

// lanPeers returns the latest report of every other handset that shared a LAN
// key with r within window before r.
func lanPeers(r Report, reports []Report, window time.Duration) []Report {
	keys := lanKeys(r)
	if len(keys) == 0 {
		return nil
	}
	self := handset(r)
	latest := map[string]Report{}
	for _, other := range reports {
		age := r.ReceivedAt.Sub(other.ReceivedAt)
		if other.ID == r.ID || age < 0 || age > window || handset(other) == self || !sharesKey(keys, lanKeys(other)) {
			continue
		}
		if old, ok := latest[handset(other)]; !ok || other.ReceivedAt.After(old.ReceivedAt) {
			latest[handset(other)] = other
		}
	}
	peers := make([]Report, 0, len(latest))
	for _, p := range latest {
		peers = append(peers, p)
	}
	sort.Slice(peers, func(i, j int) bool { return peers[i].ReceivedAt.After(peers[j].ReceivedAt) })
	return peers
}

// lanChecks scores the LAN cluster and the SDK's probe of its peers.
func (s *Server) lanChecks(r *Report) error {
	if r.Resources == nil || len(lanKeys(*r)) == 0 {
		return nil
	}
	reports, err := records[Report](s.Store, "reports")
	if err != nil {
		return err
	}
	add := func(code, state, detail string, points int) {
		r.Resources.Checks = append(r.Resources.Checks, ResourceCheck{Code: code, State: state, Dimension: "network", Detail: detail, Source: "client", Points: points})
	}
	handsets := len(lanPeers(*r, reports, lanClusterWindow)) + 1
	if handsets >= lanClusterDevices {
		add("lan_device_cluster", "conflict", fmt.Sprintf("24 小时内同一局域网 %d 台设备", handsets), 20)
	} else {
		add("lan_cluster", "matched", fmt.Sprintf("24 小时内同一局域网 %d 台设备", handsets), 0)
	}

	var probe lanProbeResult
	if signalObject(*r, "lanProbe", &probe) != nil || probe.Targets == 0 {
		return nil
	}
	if len(probe.ADB) > 0 {
		add("lan_adb_peers", "conflict", fmt.Sprintf("同局域网 %d 台设备开放 ADB 无线调试端口 %d", len(probe.ADB), lanAdbPort), 30)
	}
	switch {
	case len(probe.Reachable) > 0:
		add("lan_peers_reachable", "matched", fmt.Sprintf("探测 %d 台同网设备，%d 台可达", probe.Targets, len(probe.Reachable)), 0)
	case probe.Gateway != nil && *probe.Gateway && probe.Targets >= 2:
		// Routers in hotels and offices isolate clients too, so this is a lead, not a score.
		add("lan_isolation", "unknown", fmt.Sprintf("网关可达，但 %d 台同网设备均不可达，疑似开启 AP 隔离", probe.Targets), 0)
	}
	return nil
}

// lanTargets lists the LAN addresses of other handsets recently seen on the
// same LAN as the installation's latest report, for the SDK to probe.
func (s *Server) lanTargets(appID, installationID string) ([]string, error) {
	reports, err := records[Report](s.Store, "reports")
	if err != nil {
		return nil, err
	}
	var latest *Report
	for i := range reports {
		r := &reports[i]
		if r.AppID == appID && r.InstallationID == installationID && (latest == nil || r.ReceivedAt.After(latest.ReceivedAt)) {
			latest = r
		}
	}
	if latest == nil {
		return nil, nil
	}
	own, subnet, _ := lanSegment(*latest)
	if subnet == nil {
		return nil, nil
	}
	now := *latest
	now.ReceivedAt = time.Now().UTC()
	var targets []string
	seen := map[string]bool{own: true}
	for _, p := range lanPeers(now, reports, lanTargetWindow) {
		ip, _, _ := lanSegment(p)
		if ip == "" || seen[ip] || !subnet.Contains(net.ParseIP(ip)) {
			continue
		}
		seen[ip] = true
		targets = append(targets, ip)
		if len(targets) == lanTargetLimit {
			break
		}
	}
	return targets, nil
}

func lanPeerRows(peers []Report) []LANPeer {
	rows := make([]LANPeer, 0, len(peers))
	for _, p := range peers {
		model, _ := signalValue(p, "model").(string)
		rows = append(rows, LANPeer{ID: digest(p.AppID + ":" + p.InstallationID), AppID: p.AppID, DeviceID: p.DeviceID, Model: model, LanIP: lanIP(p), LastSeen: p.ReceivedAt})
	}
	return rows
}

func signalValue(r Report, key string) any {
	if s := r.Signals[key]; s.Status == "available" {
		return s.Value
	}
	return nil
}

// lanIP is the device's LAN IPv4 address for display and search.
func lanIP(r Report) string {
	if ip, _, _ := lanSegment(r); ip != "" {
		return ip
	}
	var lan lanObservation
	if signalObject(r, "lan", &lan) == nil && len(lan.Addresses) > 0 {
		return lan.Addresses[0].Address
	}
	return ""
}
