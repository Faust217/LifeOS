package finance

import (
	"bytes"
	"encoding/json"
	"github.com/Faust217/LifeOS/internal/identity"
	"github.com/go-chi/chi/v5"
	"net/http"
	"regexp"
	"strconv"
	"time"
)

var eventTagPattern = regexp.MustCompile(`^"([1-9][0-9]*)"$`)

func eventPrecondition(r *http.Request) (int64, error) {
	values := r.Header.Values("If-Match")
	if len(values) == 0 {
		return 0, nil
	}
	if len(values) != 1 {
		return 0, ErrInvalidInput
	}
	parts := eventTagPattern.FindStringSubmatch(values[0])
	if parts == nil {
		return 0, ErrInvalidInput
	}
	revision, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, ErrInvalidInput
	}
	return revision, nil
}

func (h moneyHandler) getEvent(w http.ResponseWriter, r *http.Request, owner identity.OwnerID) {
	if _, err := moneyQuery(r); err != nil {
		writeMoneyError(w, err)
		return
	}
	event, err := h.service.GetEvent(r.Context(), owner, chi.URLParam(r, "eventID"))
	if err != nil {
		writeMoneyError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store, private")
	w.Header().Set("ETag", strconv.Quote(strconv.FormatInt(event.Revision, 10)))
	writeMoneyJSON(w, 200, eventDTO(event))
}

type editRequest struct {
	Kind          string          `json:"kind"`
	Currency      string          `json:"currency"`
	AmountMinor   string          `json:"amount_minor"`
	OccurredAt    time.Time       `json:"occurred_at"`
	Note          *string         `json:"note"`
	AccountID     json.RawMessage `json:"account_id"`
	FromAccountID json.RawMessage `json:"from_account_id"`
	ToAccountID   json.RawMessage `json:"to_account_id"`
	Allocations   json.RawMessage `json:"allocations"`
}

func (h moneyHandler) editEvent(w http.ResponseWriter, r *http.Request, owner identity.OwnerID) {
	if _, err := moneyQuery(r); err != nil {
		writeMoneyError(w, err)
		return
	}
	revision, err := eventPrecondition(r)
	if err != nil {
		writeMoneyError(w, err)
		return
	}
	if revision == 0 {
		writeMoneyError(w, ErrRevisionRequired)
		return
	}
	var body editRequest
	if !decodeMoneyJSON(w, r, &body) {
		return
	}
	amount, err := parseMinorUnit(body.AmountMinor)
	if err != nil {
		writeMoneyError(w, err)
		return
	}
	if body.Note == nil {
		writeMoneyError(w, ErrInvalidInput)
		return
	}
	input := EditInput{Kind: body.Kind, EventInput: EventInput{AmountMinor: amount, Currency: body.Currency, OccurredAt: body.OccurredAt, Note: *body.Note}}
	if body.Kind == "transfer" {
		if body.AccountID != nil || body.Allocations != nil || body.FromAccountID == nil || body.ToAccountID == nil {
			writeMoneyError(w, ErrInvalidInput)
			return
		}
		if json.Unmarshal(body.FromAccountID, &input.FromAccountID) != nil || json.Unmarshal(body.ToAccountID, &input.ToAccountID) != nil {
			writeMoneyError(w, ErrInvalidInput)
			return
		}
	} else {
		if body.FromAccountID != nil || body.ToAccountID != nil || body.AccountID == nil || body.Allocations == nil {
			writeMoneyError(w, ErrInvalidInput)
			return
		}
		if json.Unmarshal(body.AccountID, &input.AccountID) != nil || len(bytes.TrimSpace(body.Allocations)) == 0 || bytes.TrimSpace(body.Allocations)[0] != '[' {
			writeMoneyError(w, ErrInvalidInput)
			return
		}
		var allocations []allocationRequest
		decoder := json.NewDecoder(bytes.NewReader(body.Allocations))
		decoder.DisallowUnknownFields()
		if decoder.Decode(&allocations) != nil {
			writeMoneyError(w, ErrInvalidInput)
			return
		}
		for _, a := range allocations {
			part, err := parseMinorUnit(a.AmountMinor)
			if err != nil {
				writeMoneyError(w, err)
				return
			}
			input.Allocations = append(input.Allocations, AllocationInput{a.CategoryID, part})
		}
	}
	result, err := h.service.EditEvent(r.Context(), owner, chi.URLParam(r, "eventID"), revision, input, r.Header.Get("Idempotency-Key"))
	h.finishFinancialWrite(w, r, owner, 200, eventDTO(result), err)
}
