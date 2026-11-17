package platform

import (
	"fmt"
	"math"
)

// Impossible travel: two fresh GPS fixes of the same handset (link ID, else
// device) farther apart than an airliner could fly in the time between them.
const (
	travelMinMeters = 100000
	travelMaxKmh    = 900
)

func (s *Server) locationJump(r *Report) error {
	if r.Resources == nil {
		return nil
	}
	current, err := gpsFix(*r)
	if err != nil || current.Mock || !freshFix(current) {
		return nil
	}
	reports, err := records[Report](s.Store, "reports")
	if err != nil {
		return err
	}
	var previous *Report
	var fix gpsObservation
	for i := range reports {
		old := &reports[i]
		same := (r.LinkID != "" && old.LinkID == r.LinkID) || (old.AppID == r.AppID && old.DeviceID != "" && old.DeviceID == r.DeviceID)
		if !same || old.ID == r.ID || !old.ReceivedAt.Before(r.ReceivedAt) || (previous != nil && !old.ReceivedAt.After(previous.ReceivedAt)) {
			continue
		}
		if gps, err := gpsFix(*old); err == nil && !gps.Mock && freshFix(gps) {
			previous, fix = old, gps
		}
	}
	if previous == nil {
		return nil
	}
	meters := distanceMeters(fix.Latitude, fix.Longitude, current.Latitude, current.Longitude)
	hours := math.Max(r.ReceivedAt.Sub(previous.ReceivedAt).Hours(), 1.0/60)
	if meters > travelMinMeters && meters/1000/hours > travelMaxKmh {
		r.Resources.Checks = append(r.Resources.Checks, ResourceCheck{Code: "location_jump", State: "conflict", Dimension: "network",
			Detail: fmt.Sprintf("%.1f 小时内移动 %.0f km", hours, meters/1000), Source: "client", Points: 25})
	}
	return nil
}
