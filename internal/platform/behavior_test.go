package platform

import (
	"testing"
	"time"
)

func TestBehaviorProfileAccumulates(t *testing.T) {
	store, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{Store: store}
	window := func(sessions, taps int, cv float64) map[string]any {
		hours := make([]any, 24)
		for i := range hours {
			hours[i] = 0
		}
		hours[21] = sessions
		return map[string]any{"sessions": sessions, "foregroundMs": 60000.0 * float64(sessions), "screens": 3 * sessions, "hours": hours,
			"taps": map[string]any{"count": taps, "intervals": max(taps-1, 0), "intervalMeanMs": 800.0, "intervalCv": cv, "topCellShare": 0.2, "exactRepeat": 0.0, "sizeCv": 0.3},
			"swipes": map[string]any{"count": 2 * sessions, "straightness": 0.9, "speedCv": 0.5}}
	}
	day := time.Date(2026, 11, 1, 12, 0, 0, 0, beijing)
	for i, w := range []map[string]any{window(1, 11, 0.6), window(2, 21, 0.3), window(0, 0, 0)} {
		r := Report{AppID: "a", DeviceID: "d", ReceivedAt: day.AddDate(0, 0, i), Signals: map[string]Signal{"behavior": {Status: "available", Value: w}}}
		if err := s.updateBehavior(r); err != nil {
			t.Fatal(err)
		}
	}
	// No usage data for this device: no profile.
	if err := s.updateBehavior(Report{AppID: "a", DeviceID: "x", Signals: map[string]Signal{}}); err != nil {
		t.Fatal(err)
	}
	if b, _ := s.behaviorSummary("a", "x"); b != nil {
		t.Fatal(b)
	}
	b, err := s.behaviorSummary("a", "d")
	if err != nil {
		t.Fatal(err)
	}
	// The empty third window does not count as an active day.
	if b.Sessions != 3 || b.ActiveDays != 2 || b.AvgSessionSeconds != 60 || b.ScreensPerSession != 3 || b.Hours[21] != 3 || b.Taps != 32 || b.Swipes != 6 {
		t.Fatalf("%+v", b)
	}
	// Interval CV weighted by intervals: (0.6*10 + 0.3*20) / 30.
	if b.TapIntervalCv != 0.4 || b.Straightness != 0.9 {
		t.Fatalf("%+v", b)
	}
}
