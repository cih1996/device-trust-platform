package platform

import "time"

type App struct {
	ID        string       `json:"id"`
	Name      string       `json:"name"`
	Package   string       `json:"package"`
	KeyHash   string       `json:"keyHash,omitempty"`
	CreatedAt time.Time    `json:"createdAt"`
	Policy    *Policy      `json:"policy,omitempty"`
	Security  *AppSecurity `json:"security,omitempty"`
}
type Signal struct {
	Status string `json:"status"`
	Value  any    `json:"value,omitempty"`
	Reason string `json:"reason,omitempty"`
}
type Report struct {
	Purpose        string              `json:"purpose,omitempty"`
	Resources      *ResourceAssessment `json:"resources,omitempty"`
	DeviceID       string              `json:"deviceId,omitempty"`
	ID             string              `json:"id"`
	AppID          string              `json:"appId"`
	InstallationID string              `json:"installationId"`
	Package        string              `json:"package"`
	Signals        map[string]Signal   `json:"signals"`
	ObservedIP     string              `json:"observedIp"`
	ReceivedAt     time.Time           `json:"receivedAt"`
	Proof          *ReportProof        `json:"proof,omitempty"`
}
type Event struct {
	Resources      *ResourceAssessment `json:"resources,omitempty"`
	DeviceID       string              `json:"deviceId,omitempty"`
	ID             string              `json:"id"`
	BusinessID     string              `json:"eventId,omitempty"`
	AppID          string              `json:"appId"`
	ReportID       string              `json:"reportId"`
	AccountID      string              `json:"accountId"`
	Type           string              `json:"type"`
	ClientIP       string              `json:"clientIp"`
	InstallationID string              `json:"installationId"`
	Score          int                 `json:"score"`
	Decision       string              `json:"decision"`
	Reasons        []string            `json:"reasons"`
	ModelVersion   string              `json:"modelVersion"`
	ReceivedAt     time.Time           `json:"receivedAt"`
	Context        BusinessContext     `json:"context"`
	Scene          string              `json:"scene"`
	Action         string              `json:"action"`
	Mode           string              `json:"mode"`
	Hits           []RuleHit           `json:"hits"`
	MissingFields  []string            `json:"missingFields"`
	PolicySnapshot Policy              `json:"policy"`
	ReportProof    *ReportProof        `json:"reportProof,omitempty"`
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
	runtimeIntegrity(r, add)
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

func signalMap(r Report, key string) map[string]any {
	if s := r.Signals[key]; s.Status == "available" {
		if v, ok := s.Value.(map[string]any); ok {
			return v
		}
	}
	return nil
}

func signalTrue(r Report, key string) bool {
	s := r.Signals[key]
	return s.Status == "available" && s.Value == true
}

// runtimeIntegrity scores traces of debugging, hooking, emulation and app
// cloning. Each is an observation an attacker can hide, never proof of fraud.
func runtimeIntegrity(r Report, add func(int, string)) {
	if native := signalMap(r, "integrityNative"); native != nil {
		if pid, _ := native["tracerPid"].(float64); pid > 0 {
			add(25, "native_debugger")
		}
		if hooks, _ := native["hooks"].([]any); len(hooks) > 0 {
			add(40, "hook_framework")
		} else if threads, _ := native["hookThreads"].([]any); len(threads) > 0 {
			add(30, "hook_threads")
		}
		if traits, _ := native["emulator"].([]any); len(traits) > 0 {
			add(35, "emulator_traits")
		}
	}
	if machine, _ := signalMap(r, "resourceNative")["machine"].(string); machine == "x86_64" || machine == "i686" {
		add(15, "x86_runtime")
	}
	if s := r.Signals["sensors"]; s.Status == "available" {
		sensors, _ := s.Value.([]any)
		accelerometer := false
		for _, item := range sensors {
			if sensor, ok := item.(map[string]any); ok && sensor["type"] == float64(1) {
				accelerometer = true
			}
		}
		if !accelerometer {
			add(15, "missing_motion_sensor")
		}
	}
	if signalTrue(r, "appDebuggable") {
		add(30, "app_debuggable")
	}
	if _, ok := signalMap(r, "httpProxy")["host"]; ok {
		add(10, "http_proxy")
	}
	if sandbox := signalMap(r, "appSandbox"); sandbox != nil {
		if sandbox["ownDataDir"] == false {
			add(30, "virtual_container")
		} else if user, _ := sandbox["userId"].(float64); user != 0 {
			add(5, "secondary_user")
		}
	}
	if signalTrue(r, "adbEnabled") {
		add(5, "adb_enabled")
	}
}
