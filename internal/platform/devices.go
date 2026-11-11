package platform

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

func (s *Server) deviceData() (reports []Report, events []Event, err error) {
	reports, err = records[Report](s.Store, "reports")
	if err != nil {
		return
	}
	events, err = records[Event](s.Store, "events")
	return
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
	reports, events, err := s.deviceData()
	if err != nil {
		fail(w, 500, err)
		return
	}
	all := profiles(reports, events, r.URL.Query().Get("appId"))
	for i := range all {
		if err = s.keyStatus(&all[i]); err != nil {
			fail(w, 500, err)
			return
		}
	}
	stats := map[string]int{"total": len(all), "active": 0, "conflict": 0, "incomplete": 0}
	for _, p := range all {
		if time.Since(p.LastSeen) <= 24*time.Hour {
			stats["active"]++
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
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	filtered := []Profile{}
	for _, p := range all {
		value := p.DeviceID + " " + p.InstallationID + " " + p.LatestReport.ObservedIP
		for _, key := range []string{"manufacturer", "model"} {
			if s := p.LatestReport.Signals[key]; s.Status == "available" {
				if v, ok := s.Value.(string); ok {
					value += " " + v
				}
			}
		}
		if q == "" || strings.Contains(strings.ToLower(value), q) {
			filtered = append(filtered, p)
		}
	}
	offset, limit := 0, 50
	for name, target := range map[string]*int{"offset": &offset, "limit": &limit} {
		if text := r.URL.Query().Get(name); text != "" {
			value, parseErr := strconv.Atoi(text)
			if parseErr != nil || value < 0 || (name == "limit" && (value < 1 || value > 100)) {
				fail(w, 400, errors.New("invalid device pagination"))
				return
			}
			*target = value
		}
	}
	total := len(filtered)
	start := offset
	if start > total {
		start = total
	}
	end := start + limit
	if end > total {
		end = total
	}
	reply(w, 200, map[string]any{"items": filtered[start:end], "total": total, "offset": offset, "limit": limit, "stats": stats})
}

type deviceAccount struct {
	AccountID string    `json:"accountId"`
	Events    int       `json:"events"`
	LastSeen  time.Time `json:"lastSeen"`
}

func (s *Server) deviceDetails(w http.ResponseWriter, r *http.Request, id string) {
	reports, events, err := s.deviceData()
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
	if len(history) > 20 {
		history = history[:20]
	}
	if len(related) > 30 {
		related = related[:30]
	}
	reply(w, 200, map[string]any{"profile": selected, "reports": history, "events": related, "accounts": accounts})
}
