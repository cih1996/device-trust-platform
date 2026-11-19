package platform

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"sort"
	"time"
)

// Behaviour profile. Each report carries the SDK's usage statistics since the
// previous report (sessions, screens, active hours, tap and swipe feel). They
// are summed per device as reports arrive — one read and one write per
// report, no background sweep. This stage only observes: nothing here scores.

const behaviorKeepDays = 60

// behaviorWindow is the "behavior" signal of one report.
type behaviorWindow struct {
	WindowMs     float64   `json:"windowMs"`
	Sessions     int       `json:"sessions"`
	ForegroundMs float64   `json:"foregroundMs"`
	Screens      int       `json:"screens"`
	Hours        []int     `json:"hours"`
	Taps         tapStats  `json:"taps"`
	Swipes       swipeStat `json:"swipes"`
}

type tapStats struct {
	Count          int     `json:"count"`
	Intervals      int     `json:"intervals"`
	IntervalMeanMs float64 `json:"intervalMeanMs"`
	IntervalCv     float64 `json:"intervalCv"`
	Cells          int     `json:"cells"`
	TopCellShare   float64 `json:"topCellShare"`
	ExactRepeat    float64 `json:"exactRepeat"`
	SizeCv         float64 `json:"sizeCv"`
	PressureCv     float64 `json:"pressureCv"`
}

type swipeStat struct {
	Count        int     `json:"count"`
	Straightness float64 `json:"straightness"`
	SpeedCv      float64 `json:"speedCv"`
}

// BehaviorProfile is the stored running total. Averages are kept as weighted
// sums so a later window adds without rereading history.
type BehaviorProfile struct {
	AppID        string    `json:"appId"`
	DeviceID     string    `json:"deviceId"`
	FirstSeen    time.Time `json:"firstSeen"`
	LastSeen     time.Time `json:"lastSeen"`
	Days         []string  `json:"days"` // Beijing dates with use, oldest first
	Windows      int       `json:"windows"`
	Sessions     int       `json:"sessions"`
	ForegroundMs float64   `json:"foregroundMs"`
	Screens      int       `json:"screens"`
	Hours        [24]int   `json:"hours"`
	Taps         int       `json:"taps"`
	Intervals    int       `json:"intervals"`
	Swipes       int       `json:"swipes"`
	// Weighted sums: by intervals for interval stats, by taps for the rest of
	// the tap stats, by swipes for swipe stats.
	SumIntervalMean, SumIntervalCv                       float64
	SumTopCell, SumExactRepeat, SumSizeCv, SumPressureCv float64
	SumStraightness, SumSpeedCv                          float64
}

// BehaviorSummary is what the console shows.
type BehaviorSummary struct {
	FirstSeen         time.Time `json:"firstSeen"`
	LastSeen          time.Time `json:"lastSeen"`
	ActiveDays        int       `json:"activeDays"`
	ActiveDays7       int       `json:"activeDays7"`
	Days              []string  `json:"days"`
	Sessions          int       `json:"sessions"`
	AvgSessionSeconds float64   `json:"avgSessionSeconds"`
	ScreensPerSession float64   `json:"screensPerSession"`
	Hours             [24]int   `json:"hours"`
	Taps              int       `json:"taps"`
	TapIntervalMs     float64   `json:"tapIntervalMs"`
	TapIntervalCv     float64   `json:"tapIntervalCv"`
	TopCellShare      float64   `json:"topCellShare"`
	ExactRepeat       float64   `json:"exactRepeat"`
	TouchSizeCv       float64   `json:"touchSizeCv"`
	Swipes            int       `json:"swipes"`
	Straightness      float64   `json:"straightness"`
	SwipeSpeedCv      float64   `json:"swipeSpeedCv"`
}

func behaviorKey(appID, deviceID string) string { return appID + ":" + deviceID }

// updateBehavior adds a report's usage window to its device profile.
func (s *Server) updateBehavior(r Report) error {
	signal := r.Signals["behavior"]
	if signal.Status != "available" || r.DeviceID == "" {
		return nil
	}
	raw, err := json.Marshal(signal.Value)
	if err != nil {
		return err
	}
	var w behaviorWindow
	if err = json.Unmarshal(raw, &w); err != nil {
		return err
	}
	key := behaviorKey(r.AppID, r.DeviceID)
	p := BehaviorProfile{AppID: r.AppID, DeviceID: r.DeviceID, FirstSeen: r.ReceivedAt}
	if err = s.Store.get("behavior", key, &p); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	p.add(w, r.ReceivedAt)
	return s.Store.put("behavior", key, p)
}

func (p *BehaviorProfile) add(w behaviorWindow, at time.Time) {
	p.LastSeen = at
	p.Windows++
	if w.Sessions > 0 || w.Taps.Count > 0 || w.Swipes.Count > 0 {
		today := at.In(beijing).Format(time.DateOnly)
		if len(p.Days) == 0 || p.Days[len(p.Days)-1] != today {
			p.Days = append(p.Days, today)
		}
		oldest := at.In(beijing).AddDate(0, 0, -behaviorKeepDays).Format(time.DateOnly)
		p.Days = p.Days[sort.SearchStrings(p.Days, oldest):]
	}
	p.Sessions += w.Sessions
	p.ForegroundMs += w.ForegroundMs
	p.Screens += w.Screens
	for i := 0; i < len(w.Hours) && i < 24; i++ {
		p.Hours[i] += w.Hours[i]
	}
	t, n, iv := w.Taps, float64(w.Taps.Count), float64(w.Taps.Intervals)
	p.Taps += t.Count
	p.Intervals += t.Intervals
	p.SumIntervalMean += t.IntervalMeanMs * iv
	p.SumIntervalCv += t.IntervalCv * iv
	p.SumTopCell += t.TopCellShare * n
	p.SumExactRepeat += t.ExactRepeat * n
	p.SumSizeCv += t.SizeCv * n
	p.SumPressureCv += t.PressureCv * n
	sw := float64(w.Swipes.Count)
	p.Swipes += w.Swipes.Count
	p.SumStraightness += w.Swipes.Straightness * sw
	p.SumSpeedCv += w.Swipes.SpeedCv * sw
}

func (p BehaviorProfile) summary(now time.Time) BehaviorSummary {
	div := func(a float64, b int) float64 {
		if b == 0 {
			return 0
		}
		return math.Round(a/float64(b)*1000) / 1000
	}
	week := now.In(beijing).AddDate(0, 0, -7).Format(time.DateOnly)
	recent := len(p.Days) - sort.SearchStrings(p.Days, week)
	return BehaviorSummary{
		FirstSeen: p.FirstSeen, LastSeen: p.LastSeen, ActiveDays: len(p.Days), ActiveDays7: recent, Days: p.Days,
		Sessions: p.Sessions, AvgSessionSeconds: div(p.ForegroundMs/1000, p.Sessions), ScreensPerSession: div(float64(p.Screens), p.Sessions),
		Hours: p.Hours, Taps: p.Taps, TapIntervalMs: div(p.SumIntervalMean, p.Intervals), TapIntervalCv: div(p.SumIntervalCv, p.Intervals),
		TopCellShare: div(p.SumTopCell, p.Taps), ExactRepeat: div(p.SumExactRepeat, p.Taps), TouchSizeCv: div(p.SumSizeCv, p.Taps),
		Swipes: p.Swipes, Straightness: div(p.SumStraightness, p.Swipes), SwipeSpeedCv: div(p.SumSpeedCv, p.Swipes),
	}
}

// behaviorSummary returns the device's profile, or nil before any usage data.
func (s *Server) behaviorSummary(appID, deviceID string) (*BehaviorSummary, error) {
	var p BehaviorProfile
	err := s.Store.get("behavior", behaviorKey(appID, deviceID), &p)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	summary := p.summary(time.Now())
	return &summary, nil
}
