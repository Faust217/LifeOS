package finance

import (
	"encoding/json"
	"github.com/Faust217/LifeOS/internal/identity"
	"github.com/go-chi/chi/v5"
	"net/http"
	"strconv"
	"time"
)

type openingRequest struct {
	OpeningBalanceAt    *time.Time      `json:"opening_balance_at"`
	OpeningBalanceMinor json.RawMessage `json:"opening_balance_minor,omitempty"`
}
type openingConfirmationRequest struct {
	OpeningBalanceAt    *time.Time `json:"opening_balance_at"`
	OpeningBalanceMinor string     `json:"opening_balance_minor"`
	BalanceRevision     string     `json:"balance_revision"`
	AcknowledgeChange   *bool      `json:"acknowledge_change"`
}

func (h moneyHandler) previewOpeningBalance(w http.ResponseWriter, r *http.Request, owner identity.OwnerID) {
	if _, err := moneyQuery(r); err != nil {
		writeMoneyError(w, err)
		return
	}
	var in openingRequest
	if !decodeMoneyJSON(w, r, &in) {
		return
	}
	if in.OpeningBalanceAt == nil {
		writeMoneyError(w, ErrInvalidInput)
		return
	}
	var chosen *int64
	if in.OpeningBalanceMinor != nil {
		var amountText string
		if json.Unmarshal(in.OpeningBalanceMinor, &amountText) != nil {
			writeMoneyError(w, ErrInvalidInput)
			return
		}
		v, err := parseMinorUnit(amountText)
		if err != nil {
			writeMoneyError(w, err)
			return
		}
		chosen = &v
	}
	result, err := h.service.PreviewOpeningBalance(r.Context(), owner, chi.URLParam(r, "accountID"), *in.OpeningBalanceAt, chosen)
	if err != nil {
		writeMoneyError(w, err)
		return
	}
	writeMoneyJSON(w, http.StatusOK, result)
}

func (h moneyHandler) confirmOpeningBalance(w http.ResponseWriter, r *http.Request, owner identity.OwnerID) {
	if _, err := moneyQuery(r); err != nil {
		writeMoneyError(w, err)
		return
	}
	var in openingConfirmationRequest
	if !decodeMoneyJSON(w, r, &in) {
		return
	}
	if in.OpeningBalanceAt == nil || in.AcknowledgeChange == nil {
		writeMoneyError(w, ErrInvalidInput)
		return
	}
	amount, err := parseMinorUnit(in.OpeningBalanceMinor)
	if err != nil {
		writeMoneyError(w, err)
		return
	}
	revision, err := strconv.ParseInt(in.BalanceRevision, 10, 64)
	if err != nil || revision < 0 || strconv.FormatInt(revision, 10) != in.BalanceRevision {
		writeMoneyError(w, ErrInvalidInput)
		return
	}
	result, err := h.service.ConfirmOpeningBalance(r.Context(), owner, chi.URLParam(r, "accountID"), *in.OpeningBalanceAt, amount, revision, *in.AcknowledgeChange)
	if err != nil {
		writeMoneyError(w, err)
		return
	}
	writeMoneyJSON(w, http.StatusOK, accountDTO(result))
}
