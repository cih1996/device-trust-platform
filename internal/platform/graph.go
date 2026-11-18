package platform

import (
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"
)

// The relation graph joins device profiles that share evidence of sitting
// together: the same handset in two apps, a shared account, a LAN, a serving
// cell in the same hour, a broadband egress on the same day. Each kind has a
// weight; a pair's coefficient is 1 - Π(1 - w) over its kinds, and pairs at or
// above graphLinkScore form clusters (studio phones end up in one cluster).

type relationKind struct {
	Kind   string
	Weight float64
	// Groups larger than this say nothing about the members (a city cell, a
	// carrier NAT address); they are skipped rather than turned into cliques.
	MaxGroup int
}

var relationKinds = map[string]relationKind{
	"handset": {"handset", 1.0, 50},
	"account": {"account", 0.8, 50},
	"lan":     {"lan", 0.6, 300},
	"cell":    {"cell", 0.4, 100},
	"ip":      {"ip", 0.3, 50},
}

const (
	graphLinkScore = 0.5
	graphMaxNodes  = 600
)

type GraphNode struct {
	ID        string    `json:"id"`
	AppID     string    `json:"appId"`
	DeviceID  string    `json:"deviceId,omitempty"`
	Model     string    `json:"model"`
	LanIP     string    `json:"lanIp,omitempty"`
	RiskScore int       `json:"riskScore"`
	RiskLevel string    `json:"riskLevel"`
	LastSeen  time.Time `json:"lastSeen"`
	Cluster   int       `json:"cluster"`
}

type GraphReason struct {
	Kind   string  `json:"kind"`
	Weight float64 `json:"weight"`
	Detail string  `json:"detail"`
}

type GraphEdge struct {
	Source  string        `json:"source"`
	Target  string        `json:"target"`
	Score   float64       `json:"score"`
	Reasons []GraphReason `json:"reasons"`
}

type GraphCluster struct {
	ID        int            `json:"id"`
	Size      int            `json:"size"`
	Edges     int            `json:"edges"`
	Apps      int            `json:"apps"`
	MaxRisk   int            `json:"maxRisk"`
	HighRisk  int            `json:"highRisk"`
	Kinds     map[string]int `json:"kinds"`
	Strongest float64        `json:"strongest"`
}

type RelationGraph struct {
	Since    time.Time      `json:"since"`
	Nodes    []GraphNode    `json:"nodes"`
	Edges    []GraphEdge    `json:"edges"`
	Clusters []GraphCluster `json:"clusters"`
	Skipped  map[string]int `json:"skippedGroups"`
	Trimmed  bool           `json:"trimmed"`
}

func profileKey(appID, installationID string) string { return appID + ":" + installationID }

// relationGraph builds the graph over reports and events since `since`,
// limited to one app when appID is set.
func relationGraph(reports []Report, events []Event, appID string, since time.Time) RelationGraph {
	g := RelationGraph{Since: since, Skipped: map[string]int{}}
	inScope := func(app string, at time.Time) bool { return !at.Before(since) && (appID == "" || app == appID) }

	// groups[kind][key] = profile keys; detail[kind][key] = human-readable key.
	groups := map[string]map[string]map[string]bool{}
	detail := map[string]map[string]string{}
	join := func(kind, key, text, profile string) {
		if groups[kind] == nil {
			groups[kind], detail[kind] = map[string]map[string]bool{}, map[string]string{}
		}
		if groups[kind][key] == nil {
			groups[kind][key] = map[string]bool{}
			detail[kind][key] = text
		}
		groups[kind][key][profile] = true
	}
	for _, r := range reports {
		if !inScope(r.AppID, r.ReceivedAt) {
			continue
		}
		p := profileKey(r.AppID, r.InstallationID)
		if r.LinkID != "" {
			join("handset", r.LinkID, "同一台手机（跨应用关联）", p)
		}
		for _, key := range lanKeys(r) {
			text := "同一路由器 " + strings.TrimPrefix(key, "bssid:")
			if !strings.HasPrefix(key, "bssid:") {
				_, subnet, gateway := lanSegment(r)
				text = "同一局域网 " + subnet.String() + " 网关 " + gateway
			}
			join("lan", key, text, p)
		}
		if cell := servingCellKey(r); cell != "" {
			hour := r.ReceivedAt.UTC().Truncate(time.Hour)
			join("cell", cell+"@"+hour.Format(time.RFC3339), "同一小时注册同一基站 "+cell, p)
		}
		// Carrier NAT addresses are shared by strangers; only broadband counts.
		if transport, _ := signalValue(r, "transport").(string); r.ObservedIP != "" && transport != "cellular" {
			day := r.ReceivedAt.UTC().Format("2006-01-02")
			join("ip", r.ObservedIP+"@"+day, "同一天同一宽带出口 "+r.ObservedIP, p)
		}
	}
	for _, e := range events {
		if e.AccountID != "" && inScope(e.AppID, e.ReceivedAt) {
			join("account", e.AppID+"\x00"+e.AccountID, "共用账号 "+e.AccountID, profileKey(e.AppID, e.InstallationID))
		}
	}

	type pairKey struct{ a, b string }
	pairs := map[pairKey]map[string]GraphReason{}
	for kind, byKey := range groups {
		rel := relationKinds[kind]
		for key, members := range byKey {
			if len(members) < 2 {
				continue
			}
			if len(members) > rel.MaxGroup {
				g.Skipped[kind]++
				continue
			}
			list := make([]string, 0, len(members))
			for m := range members {
				list = append(list, m)
			}
			sort.Strings(list)
			for i := range list {
				for j := i + 1; j < len(list); j++ {
					k := pairKey{list[i], list[j]}
					if pairs[k] == nil {
						pairs[k] = map[string]GraphReason{}
					}
					if _, ok := pairs[k][kind]; !ok {
						pairs[k][kind] = GraphReason{Kind: kind, Weight: rel.Weight, Detail: detail[kind][key]}
					}
				}
			}
		}
	}

	// Union-find over pairs strong enough to call related.
	parent := map[string]string{}
	var find func(string) string
	find = func(x string) string {
		if parent[x] == "" || parent[x] == x {
			parent[x] = x
			return x
		}
		parent[x] = find(parent[x])
		return parent[x]
	}
	var edges []GraphEdge
	for k, reasons := range pairs {
		miss := 1.0
		list := make([]GraphReason, 0, len(reasons))
		for _, reason := range reasons {
			miss *= 1 - reason.Weight
			list = append(list, reason)
		}
		score := 1 - miss
		if score < graphLinkScore {
			continue
		}
		sort.Slice(list, func(i, j int) bool { return list[i].Weight > list[j].Weight })
		edges = append(edges, GraphEdge{Source: k.a, Target: k.b, Score: float64(int(score*1000+0.5)) / 1000, Reasons: list})
		parent[find(k.a)] = find(k.b)
	}

	byProfile := map[string]Profile{}
	for _, p := range profiles(reports, events, appID) {
		byProfile[profileKey(p.AppID, p.InstallationID)] = p
	}
	members := map[string][]string{}
	for p := range parent {
		if _, ok := byProfile[p]; !ok {
			continue // an account seen only in events, never reported
		}
		root := find(p)
		members[root] = append(members[root], p)
	}
	for root, list := range members {
		if len(list) < 2 {
			delete(members, root)
		}
	}
	roots := make([]string, 0, len(members))
	for root := range members {
		roots = append(roots, root)
	}
	sort.Slice(roots, func(i, j int) bool {
		if len(members[roots[i]]) != len(members[roots[j]]) {
			return len(members[roots[i]]) > len(members[roots[j]])
		}
		return roots[i] < roots[j]
	})

	clusterOf := map[string]int{}
	for i, root := range roots {
		if len(g.Nodes)+len(members[root]) > graphMaxNodes {
			g.Trimmed = true
			break
		}
		sort.Strings(members[root])
		for _, m := range members[root] {
			p := byProfile[m]
			model := stringSignal(p.LatestReport, "model")
			if model == "" {
				model = "未识别机型"
			}
			g.Nodes = append(g.Nodes, GraphNode{ID: p.ID, AppID: p.AppID, DeviceID: p.DeviceID, Model: model, LanIP: lanIP(p.LatestReport), RiskScore: p.RiskScore, RiskLevel: p.RiskLevel, LastSeen: p.LastSeen, Cluster: i + 1})
			clusterOf[m] = i + 1
		}
		g.Clusters = append(g.Clusters, GraphCluster{ID: i + 1, Size: len(members[root]), Kinds: map[string]int{}})
	}
	ids := map[string]string{}
	for key, p := range byProfile {
		ids[key] = p.ID
	}
	for _, e := range edges {
		c := clusterOf[e.Source]
		if c == 0 || clusterOf[e.Target] == 0 {
			continue
		}
		cl := &g.Clusters[c-1]
		cl.Edges++
		cl.Strongest = max(cl.Strongest, e.Score)
		for _, r := range e.Reasons {
			cl.Kinds[r.Kind]++
		}
		e.Source, e.Target = ids[e.Source], ids[e.Target]
		g.Edges = append(g.Edges, e)
	}
	sort.Slice(g.Edges, func(i, j int) bool { return g.Edges[i].Score > g.Edges[j].Score })
	apps := map[int]map[string]bool{}
	for _, n := range g.Nodes {
		cl := &g.Clusters[n.Cluster-1]
		cl.MaxRisk = max(cl.MaxRisk, n.RiskScore)
		if n.RiskLevel == "high" {
			cl.HighRisk++
		}
		if apps[n.Cluster] == nil {
			apps[n.Cluster] = map[string]bool{}
		}
		apps[n.Cluster][n.AppID] = true
	}
	for i := range g.Clusters {
		g.Clusters[i].Apps = len(apps[g.Clusters[i].ID])
	}
	if g.Nodes == nil {
		g.Nodes, g.Edges, g.Clusters = []GraphNode{}, []GraphEdge{}, []GraphCluster{}
	}
	if g.Edges == nil {
		g.Edges = []GraphEdge{}
	}
	return g
}

func (s *Server) graph(w http.ResponseWriter, r *http.Request) {
	days := 7
	if v := r.URL.Query().Get("days"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 30 {
			fail(w, 400, errors.New("days must be 1-30"))
			return
		}
		days = n
	}
	reports, events, err := s.deviceData(accessFrom(r))
	if err != nil {
		fail(w, 500, err)
		return
	}
	since := time.Now().UTC().Add(-time.Duration(days) * 24 * time.Hour)
	reply(w, 200, relationGraph(reports, events, r.URL.Query().Get("appId"), since))
}
