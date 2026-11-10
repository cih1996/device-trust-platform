package platform

import (
	"errors"
	"net/http"
	"os"
	"time"
)

type Resolution struct {
	ID         string    `json:"resolutionId"`
	EventID    string    `json:"eventId"`
	Status     string    `json:"status"`
	Note       string    `json:"note"`
	Source     string    `json:"source"`
	ReceivedAt time.Time `json:"receivedAt"`
}
type Claim struct {
	RequestHash string    `json:"requestHash,omitempty"`
	AppID       string    `json:"appId"`
	EventID     string    `json:"eventId"`
	OperationID string    `json:"operationId"`
	Token       string    `json:"token"`
	Mode        string    `json:"mode"`
	ReceivedAt  time.Time `json:"receivedAt"`
	ExpiresAt   time.Time `json:"expiresAt"`
}
type Execution struct {
	ReceiptID   string    `json:"receiptId"`
	AppID       string    `json:"appId"`
	EventID     string    `json:"eventId"`
	OperationID string    `json:"operationId"`
	Status      string    `json:"status"`
	Reference   string    `json:"reference"`
	ReceivedAt  time.Time `json:"receivedAt"`
}

func (s *Server) businessEvent(w http.ResponseWriter, r *http.Request, appID, eventID string) (Event, bool) {
	var e Event
	app, ok := s.app(appID, w)
	if !ok {
		return e, false
	}
	if !same(digest(bearer(r)), app.KeyHash) {
		fail(w, 401, errors.New("invalid application key"))
		return e, false
	}
	if err := s.Store.get("events", digest(app.ID+":"+eventID), &e); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fail(w, 404, errors.New("event not found"))
		} else {
			fail(w, 500, err)
		}
		return e, false
	}
	return e, true
}
func (s *Server) claim(w http.ResponseWriter, r *http.Request) {
	var input struct {
		AppID       string `json:"appId"`
		EventID     string `json:"eventId"`
		OperationID string `json:"operationId"`
		RequestHash string `json:"requestHash"`
	}
	if err := decode(w, r, &input); err != nil {
		fail(w, 400, err)
		return
	}
	if input.OperationID == "" {
		fail(w, 400, errors.New("operationId required"))
		return
	}
	event, ok := s.businessEvent(w, r, input.AppID, input.EventID)
	if !ok {
		return
	}
	var old Claim
	security, err := s.security(event.AppID)
	if err != nil {
		fail(w, 500, err)
		return
	}
	if security.RequireRequestBinding || event.Context.RequestHash != "" {
		if !validRequestHash(event.Context.RequestHash) || input.RequestHash != event.Context.RequestHash {
			fail(w, 403, errRequestBinding)
			return
		}
	}
	if err := s.Store.get("claims", event.ID, &old); err == nil {
		if old.OperationID != input.OperationID {
			fail(w, 409, errors.New("decision already claimed for another operation"))
			return
		}
		reply(w, 200, old)
		return
	} else if !errors.Is(err, os.ErrNotExist) {
		fail(w, 500, err)
		return
	}
	validFrom := event.ReceivedAt
	if event.Mode == "enforce" && event.Action != "observe" {
		var resolution Resolution
		if err := s.Store.get("resolutions", event.ID, &resolution); errors.Is(err, os.ErrNotExist) {
			reply(w, 403, map[string]string{"error": "business action is pending approval", "state": event.Action})
			return
		} else if err != nil {
			fail(w, 500, err)
			return
		}
		if resolution.Status != "approved" {
			fail(w, 403, errors.New("business action rejected"))
			return
		}
		validFrom = resolution.ReceivedAt
	}
	if time.Since(validFrom) > 5*time.Minute {
		fail(w, 410, errors.New("decision or approval expired before execution claim"))
		return
	}
	if event.Mode == "enforce" {
		var report Report
		if err := s.Store.get("reports", event.ReportID, &report); err != nil {
			fail(w, 500, err)
			return
		}
		binding := eventInput{BusinessID: event.BusinessID, AppID: event.AppID, AccountID: event.AccountID, Type: event.Type, ClientIP: event.ClientIP, Context: event.Context}
		if err := s.validateReportBinding(report, binding); err != nil {
			fail(w, 403, err)
			return
		}
		if report.Proof != nil {
			app, ok := s.app(event.AppID, w)
			if !ok {
				return
			}
			security, err := s.security(app.ID)
			if err != nil {
				fail(w, 500, err)
				return
			}
			if _, _, err = s.activeKey(app, report.Proof.KeyID, security); err != nil {
				fail(w, 403, err)
				return
			}
		}
	}
	claim := Claim{RequestHash: event.Context.RequestHash, AppID: event.AppID, EventID: event.ID, OperationID: input.OperationID, Token: token(), Mode: event.Mode, ReceivedAt: time.Now().UTC(), ExpiresAt: validFrom.Add(5 * time.Minute)}
	if err := s.Store.create("claims", event.ID, claim); errors.Is(err, os.ErrExist) {
		if err = s.Store.get("claims", event.ID, &old); err != nil {
			fail(w, 500, err)
			return
		}
		if old.OperationID != input.OperationID {
			fail(w, 409, errors.New("decision already claimed for another operation"))
			return
		}
		reply(w, 200, old)
	} else if err != nil {
		fail(w, 500, err)
	} else {
		reply(w, 201, claim)
	}
}
func (s *Server) resolve(w http.ResponseWriter, r *http.Request) {
	var input struct {
		AppID        string `json:"appId"`
		EventID      string `json:"eventId"`
		ResolutionID string `json:"resolutionId"`
		Status       string `json:"status"`
		Note         string `json:"note"`
	}
	if err := decode(w, r, &input); err != nil {
		fail(w, 400, err)
		return
	}
	event, ok := s.businessEvent(w, r, input.AppID, input.EventID)
	if !ok {
		return
	}
	s.saveResolution(w, event, Resolution{ID: input.ResolutionID, EventID: event.ID, Status: input.Status, Note: input.Note, Source: "business", ReceivedAt: time.Now().UTC()})
}
func (s *Server) adminResolve(w http.ResponseWriter, r *http.Request, id string) {
	var event Event
	if err := s.Store.get("events", id, &event); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fail(w, 404, errors.New("event not found"))
		} else {
			fail(w, 500, err)
		}
		return
	}
	var input struct {
		ResolutionID string `json:"resolutionId"`
		Status       string `json:"status"`
		Note         string `json:"note"`
	}
	if err := decode(w, r, &input); err != nil {
		fail(w, 400, err)
		return
	}
	s.saveResolution(w, event, Resolution{ID: input.ResolutionID, EventID: event.ID, Status: input.Status, Note: input.Note, Source: "operator", ReceivedAt: time.Now().UTC()})
}
func (s *Server) saveResolution(w http.ResponseWriter, event Event, resolution Resolution) {
	if event.Mode != "enforce" || event.Action == "observe" {
		fail(w, 400, errors.New("event does not require a resolution"))
		return
	}
	if resolution.ID == "" || (resolution.Status != "approved" && resolution.Status != "rejected") {
		fail(w, 400, errors.New("resolutionId and approved/rejected status required"))
		return
	}
	if err := s.Store.create("resolutions", event.ID, resolution); errors.Is(err, os.ErrExist) {
		var old Resolution
		if err = s.Store.get("resolutions", event.ID, &old); err != nil {
			fail(w, 500, err)
			return
		}
		if old.ID != resolution.ID || old.Status != resolution.Status || old.Note != resolution.Note || old.Source != resolution.Source {
			fail(w, 409, errors.New("decision already has a final resolution"))
			return
		}
		reply(w, 200, old)
	} else if err != nil {
		fail(w, 500, err)
	} else {
		reply(w, 201, resolution)
	}
}
func (s *Server) execution(w http.ResponseWriter, r *http.Request) {
	var input struct {
		AppID       string `json:"appId"`
		EventID     string `json:"eventId"`
		OperationID string `json:"operationId"`
		Token       string `json:"token"`
		ReceiptID   string `json:"receiptId"`
		Status      string `json:"status"`
		Reference   string `json:"reference"`
	}
	if err := decode(w, r, &input); err != nil {
		fail(w, 400, err)
		return
	}
	event, ok := s.businessEvent(w, r, input.AppID, input.EventID)
	if !ok {
		return
	}
	if input.ReceiptID == "" || (input.Status != "succeeded" && input.Status != "failed") {
		fail(w, 400, errors.New("receiptId and succeeded/failed status required"))
		return
	}
	var claim Claim
	if err := s.Store.get("claims", event.ID, &claim); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fail(w, 403, errors.New("execution claim required"))
		} else {
			fail(w, 500, err)
		}
		return
	}
	if claim.OperationID != input.OperationID || !same(claim.Token, input.Token) {
		fail(w, 403, errors.New("execution claim mismatch"))
		return
	}
	receipt := Execution{input.ReceiptID, event.AppID, event.ID, input.OperationID, input.Status, input.Reference, time.Now().UTC()}
	id := event.AppID + ":" + input.ReceiptID
	if err := s.Store.create("executions", id, receipt); errors.Is(err, os.ErrExist) {
		var old Execution
		if err = s.Store.get("executions", id, &old); err != nil {
			fail(w, 500, err)
			return
		}
		if old.EventID != receipt.EventID || old.OperationID != receipt.OperationID || old.Status != receipt.Status || old.Reference != receipt.Reference {
			fail(w, 409, errors.New("receiptId already used with different data"))
			return
		}
		reply(w, 200, old)
	} else if err != nil {
		fail(w, 500, err)
	} else {
		reply(w, 201, receipt)
	}
}
