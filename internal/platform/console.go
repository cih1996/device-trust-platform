package platform

import (
	"errors"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Console read models: analytics, paged events and event details for the admin workspace.

type controlState struct {
	feedback    map[string]Review
	resolutions map[string]Resolution
	executions  map[string]Execution
	outcomes    map[string]Outcome
}

func (s *Server) controlState() (controlState, error) {
	c := controlState{map[string]Review{}, map[string]Resolution{}, map[string]Execution{}, map[string]Outcome{}}
	reviews, err := records[Review](s.Store, "reviews")
	if err != nil {
		return c, err
	}
	for _, item := range reviews {
		if old, ok := c.feedback[item.EventID]; !ok || item.ReceivedAt.After(old.ReceivedAt) {
			c.feedback[item.EventID] = item
		}
	}
	resolutions, err := records[Resolution](s.Store, "resolutions")
	if err != nil {
		return c, err
	}
	for _, item := range resolutions {
		c.resolutions[item.EventID] = item
	}
	executions, err := records[Execution](s.Store, "executions")
	if err != nil {
		return c, err
	}
	for _, item := range executions {
		if old, ok := c.executions[item.EventID]; !ok || item.ReceivedAt.After(old.ReceivedAt) {
			c.executions[item.EventID] = item
		}
	}
	outcomes, err := records[Outcome](s.Store, "outcomes")
	if err != nil {
		return c, err
	}
	for _, item := range outcomes {
		key := digest(item.AppID + ":" + item.EventID)
		old, ok := c.outcomes[key]
		if !ok || item.OccurredAt.After(old.OccurredAt) || (item.OccurredAt.Equal(old.OccurredAt) && item.ReceivedAt.After(old.ReceivedAt)) {
			c.outcomes[key] = item
		}
	}
	return c, nil
}

type eventRow struct {
	Event
	ProfileID  string      `json:"profileId"`
	RiskLevel  string      `json:"riskLevel"`
	Status     string      `json:"status"`
	Feedback   *Review     `json:"feedback,omitempty"`
	Resolution *Resolution `json:"resolution,omitempty"`
	Execution  *Execution  `json:"execution,omitempty"`
	Outcome    *Outcome    `json:"outcome,omitempty"`
}

// status mirrors how the business gate sees the event.
func (c controlState) row(e Event) eventRow {
	if e.Scene == "" {
		e.Scene = sceneFor(e.Type)
	}
	row := eventRow{Event: e, ProfileID: digest(e.AppID + ":" + e.InstallationID), RiskLevel: riskLevel(e.Score)}
	if v, ok := c.feedback[e.ID]; ok {
		row.Feedback = &v
	}
	if v, ok := c.resolutions[e.ID]; ok {
		row.Resolution = &v
	}
	if v, ok := c.executions[e.ID]; ok {
		row.Execution = &v
	}
	if v, ok := c.outcomes[e.ID]; ok {
		row.Outcome = &v
	}
	switch {
	case row.Execution != nil && row.Execution.Status == "succeeded":
		row.Status = "executed"
	case row.Execution != nil:
		row.Status = "failed"
	case row.Resolution != nil:
		row.Status = row.Resolution.Status
	case e.Mode != "enforce" || e.Action == "observe":
		row.Status = "observed"
	default:
		row.Status = "pending"
	}
	return row
}

var consoleRanges = map[string]struct {
	span, step time.Duration
}{
	"24h": {24 * time.Hour, time.Hour},
	"7d":  {7 * 24 * time.Hour, 24 * time.Hour},
	"30d": {30 * 24 * time.Hour, 24 * time.Hour},
}

func parseRange(value string) (string, time.Duration, time.Duration, error) {
	if value == "" {
		value = "24h"
	}
	r, ok := consoleRanges[value]
	if !ok {
		return "", 0, 0, errors.New("invalid range")
	}
	return value, r.span, r.step, nil
}

func pagination(r *http.Request, defaultLimit int) (int, int, error) {
	offset, limit := 0, defaultLimit
	for name, target := range map[string]*int{"offset": &offset, "limit": &limit} {
		if text := r.URL.Query().Get(name); text != "" {
			value, err := strconv.Atoi(text)
			if err != nil || value < 0 || (name == "limit" && (value < 1 || value > 200)) {
				return 0, 0, errors.New("invalid pagination")
			}
			*target = value
		}
	}
	return offset, limit, nil
}

func page[T any](items []T, offset, limit int) []T {
	start := min(offset, len(items))
	return items[start:min(start+limit, len(items))]
}

type counted struct {
	Key   string `json:"key"`
	Count int    `json:"count"`
}

func ranked(counts map[string]int, limit int) []counted {
	list := make([]counted, 0, len(counts))
	for k, v := range counts {
		list = append(list, counted{k, v})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Count != list[j].Count {
			return list[i].Count > list[j].Count
		}
		return list[i].Key < list[j].Key
	})
	if len(list) > limit {
		list = list[:limit]
	}
	return list
}

func stringSignal(r Report, key string) string {
	if s := r.Signals[key]; s.Status == "available" {
		if v, ok := s.Value.(string); ok {
			return v
		}
	}
	return ""
}

type periodSummary struct {
	Events       int `json:"events"`
	Devices      int `json:"devices"`
	NewDevices   int `json:"newDevices"`
	Accounts     int `json:"accounts"`
	IPs          int `json:"ips"`
	HighRisk     int `json:"highRisk"`
	Review       int `json:"review"`
	AverageRisk  int `json:"averageRisk"`
	DwellSeconds int `json:"dwellSeconds"`
	DwellSamples int `json:"dwellSamples"`
}

func summarize(events []Event, reports []Report, firstSeen map[string]time.Time, from, to time.Time) periodSummary {
	var out periodSummary
	accounts, ips, devices := map[string]bool{}, map[string]bool{}, map[string]bool{}
	risk, dwell := 0, int64(0)
	for _, e := range events {
		if e.ReceivedAt.Before(from) || !e.ReceivedAt.Before(to) {
			continue
		}
		out.Events++
		risk += e.Score
		if e.Score >= 70 {
			out.HighRisk++
		}
		if e.Decision == "review" {
			out.Review++
		}
		if e.AccountID != "" {
			accounts[e.AccountID] = true
		}
		if e.ClientIP != "" {
			ips[e.ClientIP] = true
		}
		if e.Context.PageDurationMs > 0 {
			dwell += e.Context.PageDurationMs
			out.DwellSamples++
		}
	}
	for _, r := range reports {
		if r.ReceivedAt.Before(from) || !r.ReceivedAt.Before(to) {
			continue
		}
		key := r.AppID + ":" + r.InstallationID
		if !devices[key] {
			devices[key] = true
			if first := firstSeen[key]; !first.Before(from) {
				out.NewDevices++
			}
		}
	}
	out.Devices, out.Accounts, out.IPs = len(devices), len(accounts), len(ips)
	if out.Events > 0 {
		out.AverageRisk = risk / out.Events
	}
	if out.DwellSamples > 0 {
		out.DwellSeconds = int(dwell / int64(out.DwellSamples) / 1000)
	}
	return out
}

func (s *Server) analytics(w http.ResponseWriter, r *http.Request) {
	appID := r.URL.Query().Get("appId")
	name, span, step, err := parseRange(r.URL.Query().Get("range"))
	if err != nil {
		fail(w, 400, err)
		return
	}
	allReports, allEvents, err := s.deviceData(accessFrom(r))
	if err != nil {
		fail(w, 500, err)
		return
	}
	reports, events := []Report{}, []Event{}
	firstSeen := map[string]time.Time{}
	for _, rep := range allReports {
		if appID != "" && rep.AppID != appID {
			continue
		}
		reports = append(reports, rep)
		key := rep.AppID + ":" + rep.InstallationID
		if first, ok := firstSeen[key]; !ok || rep.ReceivedAt.Before(first) {
			firstSeen[key] = rep.ReceivedAt
		}
	}
	for _, e := range allEvents {
		if appID == "" || e.AppID == appID {
			if e.Scene == "" {
				e.Scene = sceneFor(e.Type)
			}
			events = append(events, e)
		}
	}
	now := time.Now().UTC()
	end := now.Truncate(step).Add(step)
	if step == 24*time.Hour {
		y, m, d := now.In(time.Local).Date()
		end = time.Date(y, m, d, 0, 0, 0, 0, time.Local).AddDate(0, 0, 1).UTC()
	}
	start := end.Add(-span)

	type bucket struct {
		Start   time.Time `json:"start"`
		Events  int       `json:"events"`
		Low     int       `json:"low"`
		Medium  int       `json:"medium"`
		High    int       `json:"high"`
		Devices int       `json:"devices"`
		Risk    int       `json:"averageRisk"`
		total   int
		seen    map[string]bool
	}
	series := make([]*bucket, int(span/step))
	for i := range series {
		series[i] = &bucket{Start: start.Add(time.Duration(i) * step), seen: map[string]bool{}}
	}
	index := func(t time.Time) int {
		if t.Before(start) || !t.Before(end) {
			return -1
		}
		return int(t.Sub(start) / step)
	}
	levels := map[string]int{"low": 0, "medium": 0, "high": 0}
	scenes := map[string]int{"registration": 0, "marketing": 0, "payment": 0, "general": 0}
	actions := map[string]int{"observe": 0, "verify": 0, "hold": 0}
	reasons, regions := map[string]int{}, map[string]int{}
	type ipStat struct {
		IP       string    `json:"ip"`
		Count    int       `json:"count"`
		Devices  int       `json:"devices"`
		Accounts int       `json:"accounts"`
		MaxScore int       `json:"maxScore"`
		Region   *IPRegion `json:"region,omitempty"`
		devices  map[string]bool
		accounts map[string]bool
	}
	ips := map[string]*ipStat{}
	recent := []Event{}
	for _, e := range events {
		i := index(e.ReceivedAt)
		if i < 0 {
			continue
		}
		b := series[i]
		b.Events++
		b.total += e.Score
		level := riskLevel(e.Score)
		levels[level]++
		switch level {
		case "high":
			b.High++
		case "medium":
			b.Medium++
		default:
			b.Low++
		}
		scenes[e.Scene]++
		actions[e.Action]++
		// Hits mirror Reasons; count each rule once per event.
		for _, hit := range e.Hits {
			reasons[hit.Code]++
		}
		if len(e.Hits) == 0 {
			for _, code := range e.Reasons {
				reasons[code]++
			}
		}
		var region *IPRegion
		if e.Resources != nil {
			region = e.Resources.IP
		}
		if region != nil && region.Province != "" {
			regions[region.Province]++
		}
		if e.ClientIP != "" {
			stat := ips[e.ClientIP]
			if stat == nil {
				stat = &ipStat{IP: e.ClientIP, devices: map[string]bool{}, accounts: map[string]bool{}}
				ips[e.ClientIP] = stat
			}
			stat.Count++
			stat.devices[e.InstallationID] = true
			stat.accounts[e.AccountID] = true
			stat.MaxScore = max(stat.MaxScore, e.Score)
			if region != nil {
				stat.Region = region
			}
		}
		if e.Score >= 30 {
			recent = append(recent, e)
		}
	}
	for _, rep := range reports {
		if i := index(rep.ReceivedAt); i >= 0 {
			series[i].seen[rep.AppID+":"+rep.InstallationID] = true
		}
	}
	for _, b := range series {
		b.Devices = len(b.seen)
		if b.Events > 0 {
			b.Risk = b.total / b.Events
		}
	}
	ipList := []*ipStat{}
	for _, stat := range ips {
		stat.Devices, stat.Accounts = len(stat.devices), len(stat.accounts)
		ipList = append(ipList, stat)
	}
	sort.Slice(ipList, func(i, j int) bool {
		if ipList[i].Count != ipList[j].Count {
			return ipList[i].Count > ipList[j].Count
		}
		return ipList[i].IP < ipList[j].IP
	})
	ipList = ipList[:min(10, len(ipList))]

	all := profiles(reports, events, "")
	if err = s.withKeyStatus(all); err != nil {
		fail(w, 500, err)
		return
	}
	deviceLevels := map[string]int{"low": 0, "medium": 0, "high": 0}
	brands := map[string]int{}
	active := []Profile{}
	for _, p := range all {
		if p.LastSeen.Before(start) && (p.LastEventAt == nil || p.LastEventAt.Before(start)) {
			continue
		}
		deviceLevels[p.RiskLevel]++
		brand := stringSignal(p.LatestReport, "manufacturer")
		if brand == "" {
			brand = "未识别"
		}
		brands[brand]++
		active = append(active, p)
	}
	sort.Slice(active, func(i, j int) bool {
		if active[i].RiskScore != active[j].RiskScore {
			return active[i].RiskScore > active[j].RiskScore
		}
		return active[i].LastSeen.After(active[j].LastSeen)
	})
	topDevices := []map[string]any{}
	for _, p := range active[:min(8, len(active))] {
		topDevices = append(topDevices, deviceSummary(p))
	}
	sort.Slice(recent, func(i, j int) bool { return recent[i].ReceivedAt.After(recent[j].ReceivedAt) })
	control, err := s.controlState()
	if err != nil {
		fail(w, 500, err)
		return
	}
	recentRows := []eventRow{}
	for _, e := range recent[:min(8, len(recent))] {
		recentRows = append(recentRows, control.row(e))
	}
	reasonList := ranked(reasons, 10)
	reply(w, 200, map[string]any{
		"range": name, "step": step.String(), "from": start, "to": end,
		"summary":      summarize(events, reports, firstSeen, start, end),
		"previous":     summarize(events, reports, firstSeen, start.Add(-span), start),
		"series":       series,
		"riskLevels":   levels,
		"deviceLevels": deviceLevels,
		"scenes":       scenes,
		"actions":      actions,
		"reasons":      reasonList,
		"regions":      ranked(regions, 10),
		"brands":       ranked(brands, 8),
		"ips":          ipList,
		"topDevices":   topDevices,
		"recent":       recentRows,
	})
}

func deviceSummary(p Profile) map[string]any {
	return map[string]any{
		"id": p.ID, "deviceId": p.DeviceID, "appId": p.AppID,
		"model": stringSignal(p.LatestReport, "model"), "brand": stringSignal(p.LatestReport, "manufacturer"),
		"riskScore": p.RiskScore, "riskLevel": p.RiskLevel, "events": p.EventCount, "accounts": p.AccountCount,
		"lastIp": p.LastIP, "lastSeen": p.LastSeen, "factors": p.Factors,
	}
}

func (s *Server) withKeyStatus(list []Profile) error {
	for i := range list {
		if err := s.keyStatus(&list[i]); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) listEvents(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	appID, scene, level, status, deviceID := q.Get("appId"), q.Get("scene"), q.Get("level"), q.Get("status"), q.Get("profileId")
	text := strings.ToLower(strings.TrimSpace(q.Get("q")))
	offset, limit, err := pagination(r, 50)
	if err != nil {
		fail(w, 400, err)
		return
	}
	var from time.Time
	if q.Get("range") != "" && q.Get("range") != "all" {
		_, span, _, err := parseRange(q.Get("range"))
		if err != nil {
			fail(w, 400, err)
			return
		}
		from = time.Now().UTC().Add(-span)
	}
	events, err := records[Event](s.Store, "events")
	if err != nil {
		fail(w, 500, err)
		return
	}
	control, err := s.controlState()
	if err != nil {
		fail(w, 500, err)
		return
	}
	rows := []eventRow{}
	counts := map[string]int{"low": 0, "medium": 0, "high": 0, "pending": 0}
	for _, e := range events {
		if (appID != "" && e.AppID != appID) || e.ReceivedAt.Before(from) || !accessFrom(r).can(e.AppID) {
			continue
		}
		row := control.row(e)
		if (scene != "" && row.Scene != scene) || (deviceID != "" && row.ProfileID != deviceID) {
			continue
		}
		if text != "" && !strings.Contains(strings.ToLower(strings.Join([]string{e.AccountID, e.Type, e.BusinessID, e.DeviceID, e.InstallationID, e.ClientIP}, " ")), text) {
			continue
		}
		counts[row.RiskLevel]++
		if row.Status == "pending" {
			counts["pending"]++
		}
		if (level != "" && row.RiskLevel != level) || (status != "" && row.Status != status) {
			continue
		}
		rows = append(rows, row)
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].ReceivedAt.After(rows[j].ReceivedAt) })
	reply(w, 200, map[string]any{"items": page(rows, offset, limit), "total": len(rows), "offset": offset, "limit": limit, "counts": counts})
}

func (s *Server) eventDetails(w http.ResponseWriter, r *http.Request, id string) {
	var e Event
	if err := s.Store.get("events", id, &e); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fail(w, 404, errors.New("event not found"))
			return
		}
		fail(w, 500, err)
		return
	}
	control, err := s.controlState()
	if err != nil {
		fail(w, 500, err)
		return
	}
	reply(w, 200, control.row(e))
}
