package platform

import (
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"
)

// Insights: one compact row per device active in the range, with every
// dimension already bucketed, so the console can cross-filter and re-count
// buckets without another request. Values are dictionary-encoded per dimension.

type insightDim struct {
	Key, Label, Group string
	Multi             bool // the device can fall in several buckets (risk factors)
	value             func(p Profile) []string
}

var simOperators = map[string]string{
	"46000": "中国移动", "46002": "中国移动", "46004": "中国移动", "46007": "中国移动", "46008": "中国移动", "46013": "中国移动",
	"46001": "中国联通", "46006": "中国联通", "46009": "中国联通", "46010": "中国联通",
	"46003": "中国电信", "46005": "中国电信", "46011": "中国电信", "46012": "中国电信",
	"46015": "中国广电", "46020": "中国铁通",
}

func one(v string) []string {
	if v == "" {
		return []string{"未知"}
	}
	return []string{v}
}

func profileRegion(p Profile) *IPRegion {
	if p.Region != nil && p.Region.Country != "" {
		return p.Region
	}
	return nil
}

func numberIn(m map[string]any, key string) (float64, bool) {
	v, ok := m[key].(float64)
	return v, ok
}

var insightDims = []insightDim{
	{Key: "level", Label: "风险等级", Group: "risk", value: func(p Profile) []string { return []string{p.RiskLevel} }},
	{Key: "factor", Label: "风险线索", Group: "risk", Multi: true, value: func(p Profile) []string {
		seen, out := map[string]bool{}, []string{}
		for _, f := range p.Factors {
			if !seen[f.Code] {
				seen[f.Code] = true
				out = append(out, f.Code)
			}
		}
		return out
	}},
	{Key: "proof", Label: "设备密钥", Group: "risk", value: func(p Profile) []string {
		switch {
		case p.KeyStatus == "revoked":
			return []string{"已吊销"}
		case p.LatestReport.Proof == nil:
			return []string{"未签名上报"}
		case p.LatestReport.Proof.Level != "":
			return []string{p.LatestReport.Proof.Level}
		}
		return []string{"未知"}
	}},
	{Key: "province", Label: "省份", Group: "geo", value: func(p Profile) []string {
		r := profileRegion(p)
		switch {
		case r == nil:
			return []string{"未知"}
		case r.CountryCode != "" && r.CountryCode != "CN" || r.CountryCode == "" && r.Country != "中国":
			return []string{"境外 · " + r.Country}
		}
		return one(r.Province)
	}},
	{Key: "city", Label: "城市", Group: "geo", value: func(p Profile) []string {
		if r := profileRegion(p); r != nil && r.City != "" {
			return []string{strings.TrimSpace(r.Province + " " + r.City)}
		}
		return []string{"未知"}
	}},
	{Key: "isp", Label: "宽带运营商（IP）", Group: "geo", value: func(p Profile) []string {
		if r := profileRegion(p); r != nil {
			return one(r.ISP)
		}
		return []string{"未知"}
	}},
	{Key: "network", Label: "上网方式", Group: "network", value: func(p Profile) []string {
		r := p.LatestReport
		iface := stringSignal(r, "activeInterface")
		switch {
		case signalTrue(r, "vpn"):
			return []string{"VPN"}
		case strings.HasPrefix(iface, "eth"):
			return []string{"有线 / 盒子网卡"}
		}
		switch stringSignal(r, "transport") {
		case "wifi":
			return []string{"WiFi"}
		case "cellular":
			return []string{"移动网络"}
		case "ethernet":
			return []string{"有线 / 盒子网卡"}
		}
		return []string{"未知"}
	}},
	{Key: "proxy", Label: "代理", Group: "network", value: func(p Profile) []string {
		if signalTrue(p.LatestReport, "httpProxy") {
			return []string{"设置了 HTTP 代理"}
		}
		if p.LatestReport.Signals["httpProxy"].Status == "available" {
			return []string{"无代理"}
		}
		return []string{"未知"}
	}},
	{Key: "sim", Label: "SIM 卡运营商", Group: "network", value: func(p Profile) []string {
		code := stringSignal(p.LatestReport, "simOperator")
		if p.LatestReport.Signals["simOperator"].Status == "available" && code == "" {
			return []string{"无 SIM 卡"}
		}
		if name := simOperators[code]; name != "" {
			return []string{name}
		}
		if code != "" {
			return []string{"其他 " + code}
		}
		return []string{"未知"}
	}},
	{Key: "brand", Label: "品牌", Group: "device", value: func(p Profile) []string {
		return one(strings.ToLower(stringSignal(p.LatestReport, "manufacturer")))
	}},
	{Key: "model", Label: "机型", Group: "device", value: func(p Profile) []string {
		r := p.LatestReport
		return one(strings.TrimSpace(strings.ToLower(stringSignal(r, "manufacturer")) + " " + stringSignal(r, "model")))
	}},
	{Key: "android", Label: "Android 版本", Group: "device", value: func(p Profile) []string {
		if release, ok := signalMap(p.LatestReport, "osInfo")["release"].(string); ok && release != "" {
			return []string{"Android " + release}
		}
		return []string{"未知"}
	}},
	{Key: "patch", Label: "安全补丁", Group: "device", value: func(p Profile) []string {
		patch := stringSignal(p.LatestReport, "securityPatch")
		if len(patch) < 7 {
			return []string{"未知"}
		}
		t, err := time.Parse("2006-01", patch[:7])
		if err != nil {
			return []string{"未知"}
		}
		switch age := time.Since(t); {
		case age < 180*24*time.Hour:
			return []string{"半年内"}
		case age < 365*24*time.Hour:
			return []string{"半年 ~ 1 年"}
		case age < 3*365*24*time.Hour:
			return []string{"1 ~ 3 年"}
		}
		return []string{"3 年以上"}
	}},
	{Key: "memory", Label: "内存", Group: "device", value: func(p Profile) []string {
		total, ok := numberIn(signalMap(p.LatestReport, "memory"), "totalBytes")
		if !ok || total <= 0 {
			return []string{"未知"}
		}
		// Reported totals sit below the marketed size; round up to the next GiB.
		return []string{fmt.Sprintf("%.0f GB", math.Ceil(total/(1<<30)))}
	}},
	{Key: "screen", Label: "分辨率", Group: "device", value: func(p Profile) []string {
		d := signalMap(p.LatestReport, "display")
		w, ok1 := numberIn(d, "widthPixels")
		h, ok2 := numberIn(d, "heightPixels")
		if !ok1 || !ok2 || w <= 0 {
			return []string{"未知"}
		}
		return []string{fmt.Sprintf("%.0f×%.0f", min(w, h), max(w, h))}
	}},
	{Key: "appVersion", Label: "应用版本", Group: "device", value: func(p Profile) []string {
		v, _ := signalMap(p.LatestReport, "application")["versionName"].(string)
		return one(v)
	}},
	{Key: "battery", Label: "电量与充电", Group: "habit", value: func(p Profile) []string {
		b := signalMap(p.LatestReport, "battery")
		level, ok := numberIn(b, "level")
		if !ok {
			return []string{"未知"}
		}
		if scale, ok := numberIn(b, "scale"); ok && scale > 0 {
			level = level * 100 / scale
		}
		plugged, _ := numberIn(b, "plugged")
		switch {
		case plugged != 0 && level >= 100:
			return []string{"充电中 · 满电"}
		case plugged != 0:
			return []string{"充电中"}
		case level < 20:
			return []string{"未充电 · 低于 20%"}
		}
		return []string{"未充电"}
	}},
	{Key: "uptime", Label: "开机时长", Group: "habit", value: func(p Profile) []string {
		ms, ok := numberIn(signalMap(p.LatestReport, "osInfo"), "uptimeMillis")
		if !ok || ms <= 0 {
			return []string{"未知"}
		}
		switch d := time.Duration(ms) * time.Millisecond; {
		case d < time.Hour:
			return []string{"1 小时内"}
		case d < 24*time.Hour:
			return []string{"1 小时 ~ 1 天"}
		case d < 7*24*time.Hour:
			return []string{"1 ~ 7 天"}
		case d < 30*24*time.Hour:
			return []string{"7 ~ 30 天"}
		}
		return []string{"30 天以上"}
	}},
	{Key: "age", Label: "首次出现", Group: "habit", value: func(p Profile) []string {
		switch d := time.Since(p.FirstSeen); {
		case d < 24*time.Hour:
			return []string{"24 小时内"}
		case d < 7*24*time.Hour:
			return []string{"1 ~ 7 天前"}
		case d < 30*24*time.Hour:
			return []string{"7 ~ 30 天前"}
		}
		return []string{"30 天以前"}
	}},
	{Key: "accounts", Label: "关联账号数", Group: "habit", value: func(p Profile) []string {
		switch n := p.AccountCount; {
		case n == 0:
			return []string{"无账号"}
		case n == 1:
			return []string{"1 个"}
		case n <= 3:
			return []string{"2 ~ 3 个"}
		}
		return []string{"4 个以上"}
	}},
}

const insightRowLimit = 50000

func (s *Server) insights(w http.ResponseWriter, r *http.Request) {
	appID := r.URL.Query().Get("appId")
	name, span, _, err := parseRange(r.URL.Query().Get("range"))
	if err != nil {
		fail(w, 400, err)
		return
	}
	reports, events, err := s.deviceData(accessFrom(r))
	if err != nil {
		fail(w, 500, err)
		return
	}
	now := time.Now().UTC()
	start := now.Add(-span)
	all := profiles(reports, events, appID)
	if err = s.withKeyStatus(all); err != nil {
		fail(w, 500, err)
		return
	}
	active := []Profile{}
	for _, p := range all {
		if p.LastSeen.Before(start) && (p.LastEventAt == nil || p.LastEventAt.Before(start)) {
			continue
		}
		active = append(active, p)
	}
	sort.Slice(active, func(i, j int) bool {
		if active[i].RiskScore != active[j].RiskScore {
			return active[i].RiskScore > active[j].RiskScore
		}
		return active[i].LastSeen.After(active[j].LastSeen)
	})
	truncated := len(active) > insightRowLimit
	active = active[:min(len(active), insightRowLimit)]

	type dimOut struct {
		Key    string   `json:"key"`
		Label  string   `json:"label"`
		Group  string   `json:"group"`
		Multi  bool     `json:"multi,omitempty"`
		Values []string `json:"values"`
	}
	dims := make([]dimOut, len(insightDims))
	index := make([]map[string]int, len(insightDims))
	for i, d := range insightDims {
		dims[i] = dimOut{Key: d.Key, Label: d.Label, Group: d.Group, Multi: d.Multi, Values: []string{}}
		index[i] = map[string]int{}
	}
	code := func(i int, v string) int {
		n, ok := index[i][v]
		if !ok {
			n = len(dims[i].Values)
			index[i][v] = n
			dims[i].Values = append(dims[i].Values, v)
		}
		return n
	}
	type row struct {
		ID      string    `json:"id"`
		AppID   string    `json:"appId"`
		Model   string    `json:"model"`
		Score   int       `json:"score"`
		Handset string    `json:"handset"`
		Seen    time.Time `json:"lastSeen"`
		D       []any     `json:"d"`
	}
	rows := make([]row, 0, len(active))
	for _, p := range active {
		handset := p.LinkID
		if handset == "" {
			handset = "device:" + p.DeviceID
		}
		if p.LinkID == "" && p.DeviceID == "" {
			handset = "profile:" + p.ID
		}
		out := row{ID: p.ID, AppID: p.AppID, Model: stringSignal(p.LatestReport, "model"), Score: p.RiskScore, Handset: digest(handset)[:12], Seen: p.LastSeen, D: make([]any, len(insightDims))}
		for i, d := range insightDims {
			values := d.value(p)
			if d.Multi {
				codes := make([]int, len(values))
				for j, v := range values {
					codes[j] = code(i, v)
				}
				out.D[i] = codes
			} else {
				out.D[i] = code(i, values[0])
			}
		}
		rows = append(rows, out)
	}

	// Activity by local weekday and hour: signed reports plus business events.
	var heat [7][24]int
	for _, rep := range reports {
		if (appID == "" || rep.AppID == appID) && !rep.ReceivedAt.Before(start) {
			t := rep.ReceivedAt.In(time.Local)
			heat[int(t.Weekday()+6)%7][t.Hour()]++
		}
	}
	for _, e := range events {
		if (appID == "" || e.AppID == appID) && !e.ReceivedAt.Before(start) {
			t := e.ReceivedAt.In(time.Local)
			heat[int(t.Weekday()+6)%7][t.Hour()]++
		}
	}
	reply(w, 200, map[string]any{"range": name, "from": start, "to": now, "dims": dims, "rows": rows, "truncated": truncated, "activity": heat})
}
