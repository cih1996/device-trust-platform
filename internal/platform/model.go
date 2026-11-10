package platform

import "time"

type App struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	Package   string    `json:"package"`
	KeyHash   string    `json:"keyHash,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	Policy    *Policy   `json:"policy,omitempty"`
}
type Signal struct {
	Status string `json:"status"`
	Value  any    `json:"value,omitempty"`
	Reason string `json:"reason,omitempty"`
}
type Report struct {
	ID             string            `json:"id"`
	AppID          string            `json:"appId"`
	InstallationID string            `json:"installationId"`
	Package        string            `json:"package"`
	Signals        map[string]Signal `json:"signals"`
	ObservedIP     string            `json:"observedIp"`
	ReceivedAt     time.Time         `json:"receivedAt"`
}
type Event struct {
	ID             string          `json:"id"`
	BusinessID     string          `json:"eventId,omitempty"`
	AppID          string          `json:"appId"`
	ReportID       string          `json:"reportId"`
	AccountID      string          `json:"accountId"`
	Type           string          `json:"type"`
	ClientIP       string          `json:"clientIp"`
	InstallationID string          `json:"installationId"`
	Score          int             `json:"score"`
	Decision       string          `json:"decision"`
	Reasons        []string        `json:"reasons"`
	ModelVersion   string          `json:"modelVersion"`
	ReceivedAt     time.Time       `json:"receivedAt"`
	Context        BusinessContext `json:"context"`
	Scene          string          `json:"scene"`
	Action         string          `json:"action"`
	Mode           string          `json:"mode"`
	Hits           []RuleHit       `json:"hits"`
	MissingFields  []string        `json:"missingFields"`
	PolicySnapshot Policy          `json:"policy"`
}

// These are explainable trial rules, not a calibrated fraud probability.
func evaluate(e *Event, r Report, history []Event) {
	evaluateBusiness(e, r, history, nil, DefaultPolicy())
}

func evaluateDevice(e *Event, r Report, history []Event, add func(int, string)) {
	if s := r.Signals["debugger"]; s.Status == "available" && s.Value == true {
		add(15, "debugger")
	}
	if s := r.Signals["emulatorHint"]; s.Status == "available" && s.Value == true {
		add(20, "emulator_hint")
	}
	if s := r.Signals["vpn"]; s.Status == "available" && s.Value == true {
		add(5, "vpn")
	}
	if r.Package != "" && r.Signals["expectedPackage"].Value == false {
		add(30, "package_mismatch")
	}
	accounts := map[string]bool{e.AccountID: true}
	count := 0
	for _, old := range history {
		if old.AppID != e.AppID || old.InstallationID != e.InstallationID {
			continue
		}
		age := e.ReceivedAt.Sub(old.ReceivedAt)
		if age >= 0 && age < 24*time.Hour {
			accounts[old.AccountID] = true
		}
		if age >= 0 && age < time.Minute {
			count++
		}
	}
	if len(accounts) >= 4 {
		add(35, "multiple_accounts_24h")
	}
	if count >= 10 {
		add(35, "frequent_events_1m")
	}
}
