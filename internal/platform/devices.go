package platform

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"sort"
	"strings"
	"time"
)

// deviceData loads the reports and events the caller may see; nil sees all.
func (s *Server) deviceData(a *access) (reports []Report, events []Event, err error) {
	reports, err = records[Report](s.Store, "reports")
	if err != nil {
		return
	}
	events, err = records[Event](s.Store, "events")
	if err != nil || a == nil || a.platform {
		return
	}
	visibleReports := reports[:0]
	for _, r := range reports {
		if a.can(r.AppID) {
			visibleReports = append(visibleReports, r)
		}
	}
	visibleEvents := events[:0]
	for _, e := range events {
		if a.can(e.AppID) {
			visibleEvents = append(visibleEvents, e)
		}
	}
	return visibleReports, visibleEvents, nil
}
func (s *Server) keyStatus(p *Profile) error {
	if p.LatestReport.Proof == nil {
		p.KeyStatus = "legacy"
		return nil
	}
	if p.DeviceID != "" {
		var registration DeviceRegistration
		if err := s.Store.get("registrations", p.AppID+":"+p.LatestReport.Proof.KeyID, &registration); err != nil {
			return err
		}
		if registration.DeviceID != p.DeviceID {
			return errors.New("device registration mismatch")
		}
		p.RegisteredAt = &registration.CreatedAt
	}
	var record json.RawMessage
	err := s.Store.get("revoked-keys", p.AppID+":"+p.LatestReport.Proof.KeyID, &record)
	if err == nil {
		p.KeyStatus = "revoked"
		return nil
	}
	if errors.Is(err, os.ErrNotExist) {
		p.KeyStatus = "active"
		return nil
	}
	return err
}
func (s *Server) listDevices(w http.ResponseWriter, r *http.Request) {
	reports, events, err := s.deviceData(accessFrom(r))
	if err != nil {
		fail(w, 500, err)
		return
	}
	query := r.URL.Query()
	all := profiles(reports, events, query.Get("appId"))
	if err = s.withKeyStatus(all); err != nil {
		fail(w, 500, err)
		return
	}
	offset, limit, err := pagination(r, 50)
	if err != nil {
		fail(w, 400, err)
		return
	}
	stats := map[string]int{"total": len(all), "active": 0, "conflict": 0, "incomplete": 0, "low": 0, "medium": 0, "high": 0, "revoked": 0}
	for _, p := range all {
		if time.Since(p.LastSeen) <= 24*time.Hour {
			stats["active"]++
		}
		stats[p.RiskLevel]++
		if p.KeyStatus == "revoked" {
			stats["revoked"]++
		}
		conflict, incomplete := false, p.LatestReport.Resources == nil
		if p.LatestReport.Resources != nil {
			for _, check := range p.LatestReport.Resources.Checks {
				if check.State == "conflict" {
					conflict = true
				}
				if check.State == "unknown" || check.State == "stale" || check.State == "error" {
					incomplete = true
				}
			}
		}
		if conflict {
			stats["conflict"]++
		}
		if incomplete {
			stats["incomplete"]++
		}
	}
	q := strings.ToLower(strings.TrimSpace(query.Get("q")))
	level := query.Get("level")
	filtered := []Profile{}
	for _, p := range all {
		if level != "" && p.RiskLevel != level {
			continue
		}
		value := p.DeviceID + " " + p.InstallationID + " " + p.LatestReport.ObservedIP + " " + p.LastIP + " " + lanIP(p.LatestReport)
		for _, key := range []string{"manufacturer", "model"} {
			value += " " + stringSignal(p.LatestReport, key)
		}
		if q == "" || strings.Contains(strings.ToLower(value), q) {
			filtered = append(filtered, p)
		}
	}
	less := map[string]func(a, b Profile) bool{
		"lastSeen": func(a, b Profile) bool { return a.LastSeen.After(b.LastSeen) },
		"risk":     func(a, b Profile) bool { return a.RiskScore > b.RiskScore },
		"accounts": func(a, b Profile) bool { return a.AccountCount > b.AccountCount },
		"events":   func(a, b Profile) bool { return a.EventCount > b.EventCount },
	}
	sortKey := query.Get("sort")
	if sortKey == "" {
		sortKey = "lastSeen"
	}
	order, ok := less[sortKey]
	if !ok {
		fail(w, 400, errors.New("invalid device sort"))
		return
	}
	sort.SliceStable(filtered, func(i, j int) bool {
		a, b := filtered[i], filtered[j]
		if order(a, b) != order(b, a) {
			return order(a, b)
		}
		return a.LastSeen.After(b.LastSeen)
	})
	reply(w, 200, map[string]any{"items": page(filtered, offset, limit), "total": len(filtered), "offset": offset, "limit": limit, "stats": stats})
}

type deviceAccount struct {
	AccountID string    `json:"accountId"`
	Events    int       `json:"events"`
	LastSeen  time.Time `json:"lastSeen"`
}

func (s *Server) deviceDetails(w http.ResponseWriter, r *http.Request, id string) {
	reports, events, err := s.deviceData(accessFrom(r))
	if err != nil {
		fail(w, 500, err)
		return
	}
	var selected *Profile
	for _, p := range profiles(reports, events, "") {
		if p.ID == id {
			copy := p
			selected = &copy
			break
		}
	}
	if selected == nil {
		fail(w, 404, errors.New("device not found"))
		return
	}
	if err = s.keyStatus(selected); err != nil {
		fail(w, 500, err)
		return
	}
	history := []Report{}
	related := []Event{}
	accountMap := map[string]*deviceAccount{}
	for _, report := range reports {
		if report.AppID == selected.AppID && report.InstallationID == selected.InstallationID {
			history = append(history, report)
		}
	}
	for _, event := range events {
		if event.AppID == selected.AppID && event.InstallationID == selected.InstallationID {
			related = append(related, event)
			a := accountMap[event.AccountID]
			if a == nil {
				a = &deviceAccount{AccountID: event.AccountID}
				accountMap[event.AccountID] = a
			}
			a.Events++
			if event.ReceivedAt.After(a.LastSeen) {
				a.LastSeen = event.ReceivedAt
			}
		}
	}
	sort.Slice(history, func(i, j int) bool { return history[i].ReceivedAt.After(history[j].ReceivedAt) })
	sort.Slice(related, func(i, j int) bool { return related[i].ReceivedAt.After(related[j].ReceivedAt) })
	accounts := []deviceAccount{}
	for _, a := range accountMap {
		accounts = append(accounts, *a)
	}
	sort.Slice(accounts, func(i, j int) bool { return accounts[i].LastSeen.After(accounts[j].LastSeen) })
	// Daily risk trend over 30 days for the device: peak and average event score.
	type day struct {
		Start   time.Time `json:"start"`
		Events  int       `json:"events"`
		Max     int       `json:"maxScore"`
		Average int       `json:"averageScore"`
		total   int
	}
	y, m, d := time.Now().In(time.Local).Date()
	end := time.Date(y, m, d, 0, 0, 0, 0, time.Local).AddDate(0, 0, 1)
	trend := make([]*day, 30)
	for i := range trend {
		trend[i] = &day{Start: end.AddDate(0, 0, i-30).UTC()}
	}
	type ipUse struct {
		IP       string    `json:"ip"`
		Count    int       `json:"count"`
		Source   string    `json:"source"`
		LastSeen time.Time `json:"lastSeen"`
		Region   *IPRegion `json:"region,omitempty"`
	}
	ips := map[string]*ipUse{}
	use := func(ip, source string, at time.Time, region *IPRegion) {
		if ip == "" {
			return
		}
		u := ips[source+ip]
		if u == nil {
			u = &ipUse{IP: ip, Source: source}
			ips[source+ip] = u
		}
		u.Count++
		if at.After(u.LastSeen) {
			u.LastSeen = at
		}
		if region != nil {
			u.Region = region
		}
	}
	for _, e := range related {
		// Local days are not all 24 hours long (DST), so find the day by its bounds.
		i := sort.Search(len(trend), func(i int) bool { return trend[i].Start.After(e.ReceivedAt) }) - 1
		if i >= 0 && e.ReceivedAt.Before(end) {
			t := trend[i]
			t.Events++
			t.total += e.Score
			t.Max = max(t.Max, e.Score)
			t.Average = t.total / t.Events
		}
		var region *IPRegion
		if e.Resources != nil {
			region = e.Resources.IP
		}
		use(e.ClientIP, "business", e.ReceivedAt, region)
	}
	for _, report := range history {
		var region *IPRegion
		if report.Resources != nil {
			region = report.Resources.IP
		}
		use(report.ObservedIP, "device", report.ReceivedAt, region)
	}
	ipList := []*ipUse{}
	for _, u := range ips {
		ipList = append(ipList, u)
	}
	sort.Slice(ipList, func(i, j int) bool { return ipList[i].LastSeen.After(ipList[j].LastSeen) })
	if len(history) > 20 {
		history = history[:20]
	}
	if len(related) > 50 {
		related = related[:50]
	}
	control, err := s.controlState()
	if err != nil {
		fail(w, 500, err)
		return
	}
	rows := []eventRow{}
	for _, e := range related {
		rows = append(rows, control.row(e))
	}
	links, err := s.deviceLinks()
	if err != nil {
		fail(w, 500, err)
		return
	}
	apps, err := records[App](s.Store, "apps")
	if err != nil {
		fail(w, 500, err)
		return
	}
	names := map[string]string{}
	for _, a := range apps {
		names[a.ID] = a.Name
	}
	linked := linkedProfiles(profiles(reports, events, ""), links, names, selected.LinkID, selected.ID)
	lan := lanPeerRows(lanPeers(selected.LatestReport, reports, lanClusterWindow))
	keys := []DeviceRegistration{}
	if selected.DeviceID != "" {
		if keys, err = s.deviceKeys(selected.AppID, selected.DeviceID); err != nil {
			fail(w, 500, err)
			return
		}
	}
	var shared SharedRisk
	if selected.LinkID != "" {
		teams, err := s.teamOf()
		if err != nil {
			fail(w, 500, err)
			return
		}
		allReports, allEvents, err := s.deviceData(nil)
		if err != nil {
			fail(w, 500, err)
			return
		}
		shared = sharedRisk(selected.AppID, selected.LinkID, teams, allReports, allEvents)
	}
	behavior, err := s.behaviorSummary(selected.AppID, selected.DeviceID)
	if err != nil {
		fail(w, 500, err)
		return
	}
	reply(w, 200, map[string]any{"shared": shared, "profile": selected, "reports": history, "events": rows, "accounts": accounts, "trend": trend, "ips": ipList, "linked": linked, "lan": lan, "keys": keys, "behavior": behavior})
}
