package platform

import (
	"errors"
	"net/http"
	"os"
	"time"
)

// A verdict is the server's assessment of one device for the business backend.
// It is read with the app key; the app itself never sees it.
type Verdict struct {
	ReportID      string          `json:"reportId"`
	DeviceID      string          `json:"deviceId"`
	LinkID        string          `json:"linkId,omitempty"`
	KeyStatus     string          `json:"keyStatus"`
	RiskScore     int             `json:"riskScore"`
	RiskLevel     string          `json:"riskLevel"`
	EnvScore      int             `json:"envScore"`
	BusinessScore int             `json:"businessScore"`
	Factors       []RuleHit       `json:"factors"`
	Recovery      *ReportRecovery `json:"recovery,omitempty"`
	App           AppIntegrity    `json:"app"`
	Linked        LinkedSummary   `json:"linked"`
	LAN           LANSummary      `json:"lan"`
	ObservedIP    string          `json:"observedIp"`
	Region        *IPRegion       `json:"region,omitempty"`
	ReportedAt    time.Time       `json:"reportedAt"`
}

// AppIntegrity says whether the reporting app is the one registered.
// signer: matched, mismatch, unknown (not reported) or unconfigured (no allowlist).
// attestation: tee or strongbox when the hardware vouched for package and signer, else none.
type AppIntegrity struct {
	Package     string `json:"package"`
	Signer      string `json:"signer"`
	Attestation string `json:"attestation"`
	Issue       string `json:"issue,omitempty"`
}

// LinkedSummary describes this handset as seen by the other linked profiles.
type LinkedSummary struct {
	Devices      int `json:"devices"`
	Apps         int `json:"apps"`
	MaxRiskScore int `json:"maxRiskScore"`
	HighRisk     int `json:"highRisk"`
}

// LANSummary counts other handsets seen on the same LAN in the last 24 hours,
// across all apps, and peers the SDK found with ADB over Wi-Fi open.
type LANSummary struct {
	Devices  int `json:"devices"`
	ADBPeers int `json:"adbPeers"`
}

const verdictReportAge = 15 * time.Minute

func (s *Server) appKeyAuth(w http.ResponseWriter, r *http.Request, appID string) (App, bool) {
	a, ok := s.app(appID, w)
	if !ok {
		return a, false
	}
	if !same(digest(bearer(r)), a.KeyHash) {
		fail(w, 401, errors.New("invalid application key"))
		return a, false
	}
	return a, true
}

// POST /v1/verdicts: assess a fresh signed report bound to this business request.
func (s *Server) reportVerdict(w http.ResponseWriter, r *http.Request) {
	var input struct {
		AppID     string `json:"appId"`
		ReportID  string `json:"reportId"`
		EventID   string `json:"eventId"`
		AccountID string `json:"accountId"`
	}
	if err := decode(w, r, &input); err != nil {
		fail(w, 400, err)
		return
	}
	a, ok := s.appKeyAuth(w, r, input.AppID)
	if !ok {
		return
	}
	if input.ReportID == "" || input.EventID == "" || input.AccountID == "" {
		fail(w, 400, errors.New("reportId, eventId and accountId required"))
		return
	}
	var report Report
	if err := s.Store.get("reports", input.ReportID, &report); errors.Is(err, os.ErrNotExist) {
		fail(w, 404, errors.New("report not found"))
		return
	} else if err != nil {
		fail(w, 500, err)
		return
	}
	if report.AppID != a.ID {
		fail(w, 403, errors.New("report belongs to another application"))
		return
	}
	if report.Proof == nil {
		fail(w, 403, errors.New("signed device report required"))
		return
	}
	var challenge storedChallenge
	if err := s.Store.get("challenges", report.Proof.ChallengeID, &challenge); err != nil {
		fail(w, 500, err)
		return
	}
	if challenge.EventID != input.EventID || challenge.AccountID != input.AccountID {
		fail(w, 403, errors.New("report is bound to a different business request"))
		return
	}
	if time.Since(report.ReceivedAt) > verdictReportAge {
		fail(w, 410, errors.New("report expired; collect a new report"))
		return
	}
	s.replyVerdict(w, a, report)
}

// POST /v1/devices/query: the latest known state of a registered device.
func (s *Server) deviceVerdict(w http.ResponseWriter, r *http.Request) {
	var input struct {
		AppID    string `json:"appId"`
		DeviceID string `json:"deviceId"`
	}
	if err := decode(w, r, &input); err != nil {
		fail(w, 400, err)
		return
	}
	a, ok := s.appKeyAuth(w, r, input.AppID)
	if !ok {
		return
	}
	if input.DeviceID == "" {
		fail(w, 400, errors.New("deviceId required"))
		return
	}
	reports, err := records[Report](s.Store, "reports")
	if err != nil {
		fail(w, 500, err)
		return
	}
	var latest *Report
	for i, report := range reports {
		if report.AppID == a.ID && report.DeviceID == input.DeviceID && (latest == nil || report.ReceivedAt.After(latest.ReceivedAt)) {
			latest = &reports[i]
		}
	}
	if latest == nil {
		fail(w, 404, errors.New("device not found"))
		return
	}
	s.replyVerdict(w, a, *latest)
}

func (s *Server) replyVerdict(w http.ResponseWriter, a App, report Report) {
	v, err := s.verdict(a, report)
	if err != nil {
		fail(w, 500, err)
		return
	}
	reply(w, 200, v)
}

func (s *Server) verdict(a App, report Report) (Verdict, error) {
	reports, events, err := s.deviceData(nil)
	if err != nil {
		return Verdict{}, err
	}
	all := profiles(reports, events, "")
	var self *Profile
	for i := range all {
		if all[i].AppID == a.ID && all[i].InstallationID == report.InstallationID {
			self = &all[i]
		}
	}
	if self == nil {
		return Verdict{}, errors.New("device profile missing for report")
	}
	if err = s.keyStatus(self); err != nil {
		return Verdict{}, err
	}
	v := Verdict{ReportID: report.ID, DeviceID: report.DeviceID, LinkID: report.LinkID, KeyStatus: self.KeyStatus, ObservedIP: report.ObservedIP, ReportedAt: report.ReceivedAt, Factors: environmentHits(report)}
	for _, hit := range v.Factors {
		v.EnvScore += hit.Points
	}
	v.EnvScore = max(min(v.EnvScore, 100), 0)
	v.Recovery = report.Recovery
	v.BusinessScore = self.BusinessScore
	if self.peakEvent != nil {
		for _, hit := range self.peakEvent.Hits {
			if hit.Source != "client" && hit.Source != "resource" && hit.Source != "recovery" {
				v.Factors = append(v.Factors, hit)
			}
		}
	}
	v.RiskScore = min(v.EnvScore+v.BusinessScore, 100)
	v.RiskLevel = riskLevel(v.RiskScore)
	if report.Resources != nil {
		v.Region = report.Resources.IP
	}
	v.App = appIntegrity(report)
	links, err := s.deviceLinks()
	if err != nil {
		return v, err
	}
	apps := map[string]bool{}
	for _, linked := range linkedProfiles(all, links, nil, report.LinkID, self.ID) {
		v.Linked.Devices++
		apps[linked.AppID] = true
		v.Linked.MaxRiskScore = max(v.Linked.MaxRiskScore, linked.RiskScore)
		if linked.RiskLevel == "high" {
			v.Linked.HighRisk++
		}
	}
	v.Linked.Apps = len(apps)
	v.LAN.Devices = len(lanPeers(report, reports, lanClusterWindow))
	var probe lanProbeResult
	if signalObject(report, "lanProbe", &probe) == nil {
		v.LAN.ADBPeers = len(probe.ADB)
	}
	return v, nil
}

func appIntegrity(r Report) AppIntegrity {
	result := AppIntegrity{Package: r.Package, Signer: "unconfigured", Attestation: "none"}
	if s, ok := r.Signals["expectedSigner"]; ok {
		switch {
		case s.Status != "available":
			result.Signer = "unknown"
		case s.Value == true:
			result.Signer = "matched"
		default:
			result.Signer = "mismatch"
		}
	}
	if r.Proof != nil {
		if r.Proof.Level == "tee" || r.Proof.Level == "strongbox" {
			result.Attestation = r.Proof.Level
		}
		result.Issue = r.Proof.Issue
	}
	return result
}
