package platform

import (
	"errors"
	"net/http"
	"os"
	"time"
)

func (s *Server) policy(appID string) (Policy, error) {
	var p Policy
	err := s.Store.get("policies", appID, &p)
	if errors.Is(err, os.ErrNotExist) {
		return DefaultPolicy(), nil
	}
	return p, err
}
func (s *Server) savePolicy(w http.ResponseWriter, r *http.Request, appID string) {
	if _, ok := s.app(appID, w); !ok {
		return
	}
	var p Policy
	if err := decode(w, r, &p); err != nil {
		fail(w, 400, err)
		return
	}
	if p.ReviewScore < 1 || p.HoldScore < p.ReviewScore || p.HoldScore > 100 {
		fail(w, 400, errors.New("reviewScore and holdScore must be between 1 and 100, holdScore >= reviewScore"))
		return
	}
	for _, threshold := range []int{p.RegistrationAccounts, p.IPRegistrations, p.CampaignAccounts, p.PaymentMethods, p.PaymentAccounts, p.PaymentFailures} {
		if threshold < 2 {
			fail(w, 400, errors.New("association thresholds must be at least 2"))
			return
		}
	}
	p.Version = token()
	if err := s.Store.put("policies", appID, p); err != nil {
		fail(w, 500, err)
		return
	}
	reply(w, 200, p)
}

type Review struct {
	ID         string    `json:"id"`
	AppID      string    `json:"appId"`
	EventID    string    `json:"eventId"`
	Label      string    `json:"label"`
	Note       string    `json:"note"`
	ReceivedAt time.Time `json:"receivedAt"`
}

func (s *Server) review(w http.ResponseWriter, r *http.Request, eventID string) {
	var e Event
	if err := s.Store.get("events", eventID, &e); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fail(w, 404, errors.New("event not found"))
		} else {
			fail(w, 500, err)
		}
		return
	}
	var input struct {
		Label string `json:"label"`
		Note  string `json:"note"`
	}
	if err := decode(w, r, &input); err != nil {
		fail(w, 400, err)
		return
	}
	switch input.Label {
	case "fraud", "legitimate", "unknown":
	default:
		fail(w, 400, errors.New("label must be fraud, legitimate or unknown"))
		return
	}
	review := Review{token(), e.AppID, e.ID, input.Label, input.Note, time.Now().UTC()}
	if err := s.Store.create("reviews", review.ID, review); err != nil {
		fail(w, 500, err)
		return
	}
	reply(w, 201, review)
}
func (s *Server) outcome(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ID         string    `json:"outcomeId"`
		AppID      string    `json:"appId"`
		EventID    string    `json:"eventId"`
		Status     string    `json:"status"`
		OccurredAt time.Time `json:"occurredAt"`
	}
	if err := decode(w, r, &input); err != nil {
		fail(w, 400, err)
		return
	}
	a, ok := s.app(input.AppID, w)
	if !ok {
		return
	}
	if !same(digest(bearer(r)), a.KeyHash) {
		fail(w, 401, errors.New("invalid application key"))
		return
	}
	if input.ID == "" || input.EventID == "" || input.OccurredAt.IsZero() {
		fail(w, 400, errors.New("outcomeId, eventId and occurredAt required"))
		return
	}
	switch input.Status {
	case "failed", "succeeded", "chargeback":
	default:
		fail(w, 400, errors.New("status must be failed, succeeded or chargeback"))
		return
	}
	var event Event
	if err := s.Store.get("events", digest(a.ID+":"+input.EventID), &event); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fail(w, 404, errors.New("payment event not found"))
		} else {
			fail(w, 500, err)
		}
		return
	}
	if event.AppID != a.ID || event.Type != "payment" {
		fail(w, 400, errors.New("outcome must reference a payment event in this application"))
		return
	}
	result := Outcome{input.ID, a.ID, input.EventID, input.Status, input.OccurredAt.UTC(), time.Now().UTC()}
	id := digest(a.ID + ":" + input.ID)
	if err := s.Store.create("outcomes", id, result); errors.Is(err, os.ErrExist) {
		var old Outcome
		if err = s.Store.get("outcomes", id, &old); err != nil {
			fail(w, 500, err)
			return
		}
		if old.EventID != result.EventID || old.Status != result.Status || !old.OccurredAt.Equal(result.OccurredAt) {
			fail(w, 409, errors.New("outcomeId already used with different data"))
			return
		}
		reply(w, 200, old)
	} else if err != nil {
		fail(w, 500, err)
	} else {
		reply(w, 201, result)
	}
}
