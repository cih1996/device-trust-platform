package platform

import (
	"errors"
	"math"
	"os"
	"sort"
	"time"
)

// Recovery lets a device earn back soft (contextual) risk through clean use.
// Hard signals — a modified system, automation, a fake environment or a
// forged identity — are never discounted, and any of them, a business-rule
// hit, a fraud review or a chargeback restarts the count.
//
// State is updated when a report or event arrives (one read and one write per
// device), so idle devices cost nothing and no background sweep is needed.

const (
	recoveryFullDays = 7  // good days until network signals are fully discounted
	recoveryKeepDays = 30 // good days older than this no longer count
)

// softRisk maps contextual codes to the largest share of their points that
// good standing may remove. Codes not listed are hard and never decay.
// Clusters keep half: a studio's phones stay together however long they are farmed.
var softRisk = map[string]float64{
	"datacenter_egress":            1,
	"ip_outside_sim_country":       1,
	"http_proxy":                   1,
	"vpn":                          1,
	"ip_gps_region_mismatch":       1,
	"cell_ip_region_mismatch":      1,
	"sim_network_mismatch":         1,
	"cellular_ip_carrier_mismatch": 1,
	"lan_isolation":                1,
	"lan_cluster":                  0.5,
	"lan_device_cluster":           0.5,
	"lan_adb_peers":                0.5,
	"cell_device_cluster":          0.5,
}

// DeviceRecovery is the stored standing of one app installation.
type DeviceRecovery struct {
	AppID          string    `json:"appId"`
	InstallationID string    `json:"installationId"`
	GoodDays       []string  `json:"goodDays"` // distinct Beijing dates, oldest first
	ResetAt        time.Time `json:"resetAt,omitempty"`
	ResetReason    string    `json:"resetReason,omitempty"`
	UpdatedAt      time.Time `json:"updatedAt"`
}

// ReportRecovery is the discount granted to one report when it was received.
type ReportRecovery struct {
	GoodDays int `json:"goodDays"`
	Discount int `json:"discount"`
}

var beijing = time.FixedZone("CST", 8*3600)

func recoveryKey(appID, installationID string) string { return digest(appID + ":" + installationID) }

// hardHit reports whether a scored hit is outside the soft list.
func hardHit(hit RuleHit) bool {
	_, soft := softRisk[hit.Code]
	return hit.Points > 0 && !soft && hit.Source != "recovery"
}

// recoveryDiscount is the number of points removed from soft hits after
// goodDays clean days; zero when any hard hit is present.
func recoveryDiscount(hits []RuleHit, goodDays int) int {
	share := float64(min(goodDays, recoveryFullDays)) / recoveryFullDays
	cut := 0.0
	for _, hit := range hits {
		if hardHit(hit) {
			return 0
		}
		if limit, soft := softRisk[hit.Code]; soft && hit.Points > 0 {
			cut += float64(hit.Points) * limit * share
		}
	}
	return int(math.Round(cut))
}

// recoveryHit turns a report's stored discount into a negative hit.
func recoveryHit(r Report) []RuleHit {
	if r.Recovery == nil || r.Recovery.Discount <= 0 {
		return nil
	}
	return []RuleHit{{Code: "good_standing", Points: -r.Recovery.Discount, Source: "recovery", Dimension: "device", Observed: r.Recovery.GoodDays, Threshold: recoveryFullDays}}
}

// applyRecovery records this report's discount and advances the device's
// standing. Only days before today count, so one day of activity cannot farm it.
func (s *Server) applyRecovery(r *Report) error {
	key := recoveryKey(r.AppID, r.InstallationID)
	state := DeviceRecovery{AppID: r.AppID, InstallationID: r.InstallationID}
	if err := s.Store.get("recovery", key, &state); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	now := r.ReceivedAt
	today := now.In(beijing).Format(time.DateOnly)
	oldest := now.In(beijing).AddDate(0, 0, -recoveryKeepDays).Format(time.DateOnly)
	kept := []string{}
	for _, day := range state.GoodDays {
		if day >= oldest && day < today {
			kept = append(kept, day)
		}
	}
	hits := environmentHits(*r)
	discount := recoveryDiscount(hits, len(kept))
	r.Recovery = &ReportRecovery{GoodDays: len(kept), Discount: discount}
	hard := false
	for _, hit := range hits {
		hard = hard || hardHit(hit)
	}
	if hard {
		state.GoodDays, state.ResetAt, state.ResetReason = nil, now, "环境出现硬风险"
		r.Recovery = &ReportRecovery{}
	} else if state.ResetAt.In(beijing).Format(time.DateOnly) != today {
		state.GoodDays = append(kept, today) // a day with a reset is not a good day
	} else {
		state.GoodDays = kept
	}
	sort.Strings(state.GoodDays)
	state.UpdatedAt = now
	return s.Store.put("recovery", key, state)
}

// resetRecovery restarts a device's standing after bad behaviour.
func (s *Server) resetRecovery(appID, installationID, reason string) error {
	if installationID == "" {
		return nil
	}
	now := time.Now().UTC()
	return s.Store.put("recovery", recoveryKey(appID, installationID), DeviceRecovery{AppID: appID, InstallationID: installationID, ResetAt: now, ResetReason: reason, UpdatedAt: now})
}

func (s *Server) chargebackReset(e Event, o Outcome) error {
	if o.Status != "chargeback" {
		return nil
	}
	return s.resetRecovery(e.AppID, e.InstallationID, "支付拒付")
}
