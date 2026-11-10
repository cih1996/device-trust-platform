package platform

import (
	"testing"
	"time"
)

func hit(e Event, code string) bool {
	for _, h := range e.Hits {
		if h.Code == code {
			return true
		}
	}
	return false
}
func TestRegistrationDeviceClusterAndSharedNetwork(t *testing.T) {
	now := time.Now().UTC()
	current := Event{AppID: "one", AccountID: "c", InstallationID: "device", Type: "register", ReceivedAt: now, ClientIP: "203.0.113.1"}
	history := []Event{{AppID: "one", AccountID: "a", InstallationID: "device", Type: "register", ReceivedAt: now.Add(-time.Minute)}, {AppID: "one", AccountID: "b", InstallationID: "device", Type: "register", ReceivedAt: now.Add(-2 * time.Minute)}}
	evaluateBusiness(&current, Report{}, history, nil, DefaultPolicy())
	if current.Action != "verify" || !hit(current, "registration_device_cluster") || current.Hits[0].Observed != 3 {
		t.Fatal(current)
	}
	// Distinct users on a shared NAT are not sufficient for a verification recommendation.
	history = nil
	for i := 0; i < 7; i++ {
		history = append(history, Event{AppID: "one", AccountID: string(rune('d' + i)), InstallationID: string(rune('x' + i)), Type: "register", ClientIP: current.ClientIP, ReceivedAt: now.Add(-time.Minute)})
	}
	evaluateBusiness(&current, Report{}, history, nil, DefaultPolicy())
	if current.Score != 15 || current.Action != "observe" || !hit(current, "registration_ip_cluster") {
		t.Fatal(current)
	}
	for i := range history {
		history[i].AppID = "other"
	}
	evaluateBusiness(&current, Report{}, history, nil, DefaultPolicy())
	if current.Score != 0 {
		t.Fatal(current)
	}
}
func TestCampaignAssociationsAndBoundaries(t *testing.T) {
	now := time.Now().UTC()
	current := Event{AppID: "one", AccountID: "c", InstallationID: "device", Type: "reward", ReceivedAt: now, Context: BusinessContext{CampaignID: "summer", RecipientID: "recipient"}}
	history := []Event{}
	for _, account := range []string{"a", "b"} {
		history = append(history, Event{AppID: "one", AccountID: account, InstallationID: "device", Type: "coupon", ReceivedAt: now.Add(-time.Hour), Context: current.Context})
	}
	evaluateBusiness(&current, Report{}, history, nil, DefaultPolicy())
	if current.Action != "hold" || current.Score != 85 || current.Mode != "shadow" {
		t.Fatal(current)
	}
	for i := range history {
		history[i].Context.CampaignID = "another"
	}
	evaluateBusiness(&current, Report{}, history, nil, DefaultPolicy())
	if hit(current, "campaign_device_accounts") || hit(current, "campaign_shared_recipient") {
		t.Fatal(current)
	}
	current.Type = "register"
	current.Context = BusinessContext{}
	for i := range history {
		history[i].Type = "register"
		history[i].ReceivedAt = now.Add(-10 * time.Minute)
	}
	evaluateBusiness(&current, Report{}, history, nil, DefaultPolicy())
	if hit(current, "registration_device_cluster") {
		t.Fatal(current)
	}
}
func TestPaymentFailuresUseLatestProviderTimeAndPolicy(t *testing.T) {
	now := time.Now().UTC()
	current := Event{AppID: "one", AccountID: "a", InstallationID: "device", Type: "payment", ReceivedAt: now, Context: BusinessContext{PaymentMethodID: "method-c", OrderID: "order-new"}}
	history := []Event{}
	outcomes := []Outcome{}
	for i := 0; i < 4; i++ {
		id := string(rune('a' + i))
		method := "method-a"
		if i == 1 {
			method = "method-b"
		}
		history = append(history, Event{AppID: "one", BusinessID: id, AccountID: "a", InstallationID: "device", Type: "payment", ReceivedAt: now.Add(-5 * time.Minute), Context: BusinessContext{PaymentMethodID: method}})
		if i < 3 {
			outcomes = append(outcomes, Outcome{AppID: "one", EventID: id, Status: "failed", OccurredAt: now.Add(-4 * time.Minute), ReceivedAt: now.Add(-3 * time.Minute)})
		}
	}
	evaluateBusiness(&current, Report{}, history, outcomes, DefaultPolicy())
	if current.Action != "hold" || !hit(current, "payment_failure_history") || current.Score != 85 {
		t.Fatal(current)
	}
	// A success happened after a decline, despite arriving before a delayed decline webhook.
	outcomes = append(outcomes, Outcome{AppID: "one", EventID: "a", Status: "succeeded", OccurredAt: now.Add(-2 * time.Minute), ReceivedAt: now.Add(-2 * time.Minute)})
	outcomes = append(outcomes, Outcome{AppID: "one", EventID: "a", Status: "failed", OccurredAt: now.Add(-3 * time.Minute), ReceivedAt: now.Add(-time.Minute)})
	evaluateBusiness(&current, Report{}, history, outcomes, DefaultPolicy())
	if hit(current, "payment_failure_history") || current.Action != "verify" || current.Score != 50 {
		t.Fatal(current)
	}
	p := DefaultPolicy()
	p.PaymentMethods = 10
	p.ReviewScore = 60
	p.HoldScore = 90
	p.Version = "custom"
	evaluateBusiness(&current, Report{}, history, outcomes, p)
	if current.Score != 25 || current.Action != "observe" || current.PolicySnapshot.Version != "custom" {
		t.Fatal(current)
	}
}
func TestMissingPaymentContextAndFailedClientCollector(t *testing.T) {
	current := Event{AppID: "one", AccountID: "a", InstallationID: "device", Type: "payment", ReceivedAt: time.Now().UTC()}
	report := Report{Signals: map[string]Signal{"rootBeerSuBinary": {Status: "error", Value: true}, "rootBeerNativeSu": {Status: "error", Value: true}}}
	evaluateBusiness(&current, report, nil, nil, DefaultPolicy())
	if current.Score != 0 || len(current.MissingFields) != 3 || hit(current, "root_indicators") {
		t.Fatal(current)
	}
}

func TestOverlappingDeviceRulesDoNotCountAsIndependentEvidence(t *testing.T) {
	now := time.Now().UTC()
	current := Event{AppID: "one", AccountID: "d", InstallationID: "device", Type: "reward", ReceivedAt: now, Context: BusinessContext{CampaignID: "activity"}}
	history := []Event{}
	for _, account := range []string{"a", "b", "c"} {
		history = append(history, Event{AppID: "one", AccountID: account, InstallationID: "device", Type: "reward", ReceivedAt: now.Add(-time.Minute), Context: current.Context})
	}
	evaluateBusiness(&current, Report{}, history, nil, DefaultPolicy())
	if current.Score != 80 || current.Action != "verify" {
		t.Fatal(current)
	}
}

func TestChargebackHistoryIsScopedAndNotAConfirmedFraudLabel(t *testing.T) {
	now := time.Now().UTC()
	current := Event{AppID: "one", AccountID: "a", InstallationID: "device", Type: "payment", ReceivedAt: now}
	history := []Event{{AppID: "one", BusinessID: "prior", AccountID: "a", Type: "payment", ReceivedAt: now.Add(-48 * time.Hour)}}
	outcomes := []Outcome{{AppID: "one", EventID: "prior", Status: "chargeback", OccurredAt: now.Add(-time.Hour)}}
	evaluateBusiness(&current, Report{}, history, outcomes, DefaultPolicy())
	if current.Score != 30 || current.Action != "verify" || !hit(current, "payment_chargeback_history") {
		t.Fatal(current)
	}
	outcomes[0].AppID = "other"
	evaluateBusiness(&current, Report{}, history, outcomes, DefaultPolicy())
	if current.Score != 0 {
		t.Fatal(current)
	}
}

func TestPaymentToolAssociationAcrossDifferentDevices(t *testing.T) {
	now := time.Now().UTC()
	current := Event{AppID: "one", AccountID: "c", InstallationID: "device-c", Type: "payment", ReceivedAt: now, Context: BusinessContext{PaymentMethodID: "shared-token"}}
	history := []Event{{AppID: "one", AccountID: "a", InstallationID: "device-a", Type: "payment", Context: current.Context, ReceivedAt: now.Add(-time.Hour)}, {AppID: "one", AccountID: "b", InstallationID: "device-b", Type: "payment", Context: current.Context, ReceivedAt: now.Add(-time.Hour)}}
	evaluateBusiness(&current, Report{}, history, nil, DefaultPolicy())
	if current.Score != 40 || current.Action != "verify" || !hit(current, "payment_shared_method") {
		t.Fatal(current)
	}
}
