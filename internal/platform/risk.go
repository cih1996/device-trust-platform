package platform

import "time"

type BusinessContext struct {
	RequestHash      string     `json:"requestHash,omitempty"`
	CampaignID       string     `json:"campaignId,omitempty"`
	RecipientID      string     `json:"recipientId,omitempty"`
	OrderID          string     `json:"orderId,omitempty"`
	PaymentMethodID  string     `json:"paymentMethodId,omitempty"`
	AmountMinor      int64      `json:"amountMinor,omitempty"`
	Currency         string     `json:"currency,omitempty"`
	AccountCreatedAt *time.Time `json:"accountCreatedAt,omitempty"`
}
type Policy struct {
	Version              string `json:"version"`
	ReviewScore          int    `json:"reviewScore"`
	HoldScore            int    `json:"holdScore"`
	RegistrationAccounts int    `json:"registrationAccounts"`
	IPRegistrations      int    `json:"ipRegistrations"`
	CampaignAccounts     int    `json:"campaignAccounts"`
	PaymentMethods       int    `json:"paymentMethods"`
	PaymentAccounts      int    `json:"paymentAccounts"`
	PaymentFailures      int    `json:"paymentFailures"`
	Mode                 string `json:"mode"`
}

func DefaultPolicy() Policy {
	return Policy{Version: "builtin-0.4", ReviewScore: 30, HoldScore: 70, RegistrationAccounts: 3, IPRegistrations: 8, CampaignAccounts: 3, PaymentMethods: 3, PaymentAccounts: 3, PaymentFailures: 3, Mode: "shadow"}
}

type RuleHit struct {
	Code          string `json:"code"`
	Points        int    `json:"points"`
	Source        string `json:"source"`
	Dimension     string `json:"dimension"`
	Observed      int    `json:"observed,omitempty"`
	Threshold     int    `json:"threshold,omitempty"`
	WindowSeconds int    `json:"windowSeconds,omitempty"`
}

func sceneFor(kind string) string {
	switch kind {
	case "register":
		return "registration"
	case "reward", "coupon", "withdraw":
		return "marketing"
	case "payment":
		return "payment"
	default:
		return "general"
	}
}

// Scores are trial rules. Evidence is separated by source and retained with the policy.
func evaluateBusiness(e *Event, r Report, history []Event, outcomes []Outcome, p Policy) {
	e.Score = 0
	e.ModelVersion = "rules-0.5"
	e.PolicySnapshot = p
	e.Scene = sceneFor(e.Type)
	e.Mode = p.Mode
	if e.Mode == "" {
		e.Mode = "shadow"
	}
	e.Reasons = []string{}
	e.Hits = []RuleHit{}
	e.MissingFields = []string{}
	add := func(points int, code, source string, observed, threshold int, window time.Duration) {
		e.Score += points
		e.Reasons = append(e.Reasons, code)
		e.Hits = append(e.Hits, RuleHit{Code: code, Points: points, Source: source, Dimension: dimensionFor(code, source), Observed: observed, Threshold: threshold, WindowSeconds: int(window.Seconds())})
	}
	evaluateDevice(e, r, history, func(points int, code string) {
		source := "client"
		if code == "multiple_accounts_24h" || code == "frequent_events_1m" {
			source = "business"
		}
		add(points, code, source, 0, 0, 0)
	})
	roots := 0
	for _, name := range []string{"rootBeerSuBinary", "rootBeerMagiskBinary", "rootBeerNativeSu"} {
		if s := r.Signals[name]; s.Status == "available" && s.Value == true {
			roots++
		}
	}
	if roots >= 2 {
		add(20, "root_indicators", "client", roots, 2, 0)
	}
	regDevice := map[string]bool{}
	regIP := map[string]bool{}
	accountDevices := map[string]bool{e.InstallationID: true}
	campaignDevice := map[string]bool{}
	campaignRecipient := map[string]bool{}
	methods := map[string]bool{}
	paymentAccounts := map[string]bool{}
	accountPayments := 0
	campaignAttempts := 0
	if e.Scene == "registration" {
		regDevice[e.AccountID] = true
		if e.ClientIP != "" {
			regIP[e.AccountID] = true
		}
	}
	if e.Scene == "marketing" && e.Context.CampaignID != "" {
		campaignDevice[e.AccountID] = true
		if e.Context.RecipientID != "" {
			campaignRecipient[e.AccountID] = true
		}
		campaignAttempts = 1
	}
	if e.Scene == "payment" {
		accountPayments = 1
		if e.Context.PaymentMethodID != "" {
			methods[e.Context.PaymentMethodID] = true
			paymentAccounts[e.AccountID] = true
		}
	}
	eventByID := map[string]Event{}
	for _, old := range history {
		if old.AppID != e.AppID {
			continue
		}
		eventByID[old.BusinessID] = old
		age := e.ReceivedAt.Sub(old.ReceivedAt)
		if age < 0 || age >= 24*time.Hour {
			continue
		}
		if old.AccountID == e.AccountID && age < time.Hour {
			accountDevices[old.InstallationID] = true
		}
		if old.Type == "register" && age < 10*time.Minute {
			if old.InstallationID == e.InstallationID {
				regDevice[old.AccountID] = true
			}
			if e.ClientIP != "" && old.ClientIP == e.ClientIP {
				regIP[old.AccountID] = true
			}
		}
		if e.Context.CampaignID != "" && sceneFor(old.Type) == "marketing" && old.Context.CampaignID == e.Context.CampaignID {
			if old.InstallationID == e.InstallationID {
				campaignDevice[old.AccountID] = true
			}
			if e.Context.RecipientID != "" && old.Context.RecipientID == e.Context.RecipientID {
				campaignRecipient[old.AccountID] = true
			}
			if old.AccountID == e.AccountID && age < time.Minute {
				campaignAttempts++
			}
		}
		if old.Type == "payment" {
			if old.AccountID == e.AccountID {
				if age < 10*time.Minute {
					accountPayments++
				}
				if old.Context.PaymentMethodID != "" {
					methods[old.Context.PaymentMethodID] = true
				}
			}
			if e.Context.PaymentMethodID != "" && old.Context.PaymentMethodID == e.Context.PaymentMethodID {
				paymentAccounts[old.AccountID] = true
			}
		}
	}
	if len(accountDevices) >= 3 {
		add(25, "account_device_changes", "business", len(accountDevices), 3, time.Hour)
	}
	if e.ClientIP == "" {
		e.MissingFields = append(e.MissingFields, "clientIp")
	}
	switch e.Scene {
	case "registration":
		if len(regDevice) >= p.RegistrationAccounts {
			add(45, "registration_device_cluster", "business", len(regDevice), p.RegistrationAccounts, 10*time.Minute)
		}
		if len(regIP) >= p.IPRegistrations {
			add(15, "registration_ip_cluster", "business", len(regIP), p.IPRegistrations, 10*time.Minute)
		}
	case "marketing":
		if e.Context.CampaignID == "" {
			e.MissingFields = append(e.MissingFields, "context.campaignId")
		}
		if e.Context.RecipientID == "" {
			e.MissingFields = append(e.MissingFields, "context.recipientId")
		}
		if len(campaignDevice) >= p.CampaignAccounts {
			add(45, "campaign_device_accounts", "business", len(campaignDevice), p.CampaignAccounts, 24*time.Hour)
		}
		if len(campaignRecipient) >= p.CampaignAccounts {
			add(40, "campaign_shared_recipient", "business", len(campaignRecipient), p.CampaignAccounts, 24*time.Hour)
		}
		if campaignAttempts >= 4 {
			add(25, "campaign_repeated_claims", "business", campaignAttempts, 4, time.Minute)
		}
		if e.Context.AccountCreatedAt != nil {
			age := e.ReceivedAt.Sub(*e.Context.AccountCreatedAt)
			if age >= 0 && age < time.Hour && len(campaignDevice) >= 2 {
				add(15, "new_account_campaign_cluster", "business", len(campaignDevice), 2, time.Hour)
			}
		}
	case "payment":
		if e.Context.PaymentMethodID == "" {
			e.MissingFields = append(e.MissingFields, "context.paymentMethodId")
		}
		if e.Context.OrderID == "" {
			e.MissingFields = append(e.MissingFields, "context.orderId")
		}
		if accountPayments >= 5 {
			add(25, "payment_attempt_velocity", "business", accountPayments, 5, 10*time.Minute)
		}
		if len(methods) >= p.PaymentMethods {
			add(25, "payment_method_rotation", "business", len(methods), p.PaymentMethods, 24*time.Hour)
		}
		if len(paymentAccounts) >= p.PaymentAccounts {
			add(40, "payment_shared_method", "business", len(paymentAccounts), p.PaymentAccounts, 24*time.Hour)
		}
		failures, chargebacks := recentPaymentHistory(e, eventByID, outcomes)
		if failures >= p.PaymentFailures {
			add(35, "payment_failure_history", "outcome", failures, p.PaymentFailures, time.Hour)
		}
		if chargebacks > 0 {
			add(30, "payment_chargeback_history", "outcome", chargebacks, 1, 30*24*time.Hour)
		}
	}
	resourcePoints := 0
	if r.Resources != nil {
		for _, check := range r.Resources.Checks {
			if check.Points > 0 && resourcePoints < 50 {
				points := check.Points
				if points > 50-resourcePoints {
					points = 50 - resourcePoints
				}
				resourcePoints += points
				e.Score += points
				e.Reasons = append(e.Reasons, check.Code)
				e.Hits = append(e.Hits, RuleHit{Code: check.Code, Points: points, Source: "resource", Dimension: check.Dimension})
			}
		}
	}
	if e.Score > 100 {
		e.Score = 100
	}
	e.Decision = "observe"
	e.Action = "observe"
	if e.Score >= p.ReviewScore {
		e.Decision = "review"
		e.Action = "verify"
	}
	businessDimensions := map[string]bool{}
	for _, hit := range e.Hits {
		if hit.Source != "client" && hit.Source != "resource" {
			businessDimensions[hit.Dimension] = true
		}
	}
	if e.Score >= p.HoldScore && len(businessDimensions) >= 2 && (e.Scene == "marketing" || e.Scene == "payment") {
		e.Action = "hold"
	}
}

type Outcome struct {
	ID         string    `json:"outcomeId"`
	AppID      string    `json:"appId"`
	EventID    string    `json:"eventId"`
	Status     string    `json:"status"`
	OccurredAt time.Time `json:"occurredAt"`
	ReceivedAt time.Time `json:"receivedAt"`
}

func recentPaymentHistory(e *Event, events map[string]Event, outcomes []Outcome) (int, int) {
	latest := map[string]Outcome{}
	for _, o := range outcomes {
		old, ok := events[o.EventID]
		if o.AppID != e.AppID || !ok || old.Type != "payment" || old.AccountID != e.AccountID || o.OccurredAt.After(e.ReceivedAt) {
			continue
		}
		prior, exists := latest[o.EventID]
		if !exists || o.OccurredAt.After(prior.OccurredAt) || (o.OccurredAt.Equal(prior.OccurredAt) && o.ReceivedAt.After(prior.ReceivedAt)) {
			latest[o.EventID] = o
		}
	}
	count := 0
	chargebacks := 0
	for _, o := range latest {
		if o.Status == "failed" && e.ReceivedAt.Sub(o.OccurredAt) < time.Hour {
			count++
		}
		if o.Status == "chargeback" && e.ReceivedAt.Sub(o.OccurredAt) < 30*24*time.Hour {
			chargebacks++
		}
	}
	return count, chargebacks
}

func dimensionFor(code, source string) string {
	if source == "client" {
		return "client_environment"
	}
	switch code {
	case "multiple_accounts_24h", "frequent_events_1m", "registration_device_cluster", "campaign_device_accounts", "new_account_campaign_cluster":
		return "device"
	case "registration_ip_cluster":
		return "network"
	case "account_device_changes":
		return "account_devices"
	case "campaign_shared_recipient":
		return "recipient"
	case "campaign_repeated_claims", "payment_attempt_velocity":
		return "account_velocity"
	case "payment_method_rotation", "payment_shared_method":
		return "payment_method"
	default:
		return "payment_outcome"
	}
}
