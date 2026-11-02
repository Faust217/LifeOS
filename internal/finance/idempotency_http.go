package finance

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/Faust217/LifeOS/internal/identity"
	"github.com/jackc/pgx/v5"
)

type financialWriteContextKey struct{}

// Bound the complete guarded request, including session/pool waits and release checks.
func (h moneyHandler) financialMutation(next func(http.ResponseWriter, *http.Request, identity.OwnerID)) http.HandlerFunc {
	guarded := h.auth.RequireMutation(func(w http.ResponseWriter, r *http.Request, owner identity.OwnerID) {
		// Admission covers the guarded handler through final session revalidation.
		select {
		case h.service.repo.writeSlots <- struct{}{}:
			defer func() { <-h.service.repo.writeSlots }()
		default:
			writeMoneyError(w, ErrWriteBusy)
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), financialAdmissionKey{}, h.service.repo))
		keys := r.Header.Values("Idempotency-Key")
		if len(keys) > 1 {
			writeMoneyError(w, ErrInvalidKey)
			return
		}
		if err := validateRequestKey(r.Header.Get("Idempotency-Key")); err != nil {
			writeMoneyError(w, err)
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), financialWriteContextKey{}, true)), owner)
	})
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), financialWriteTimeout)
		defer cancel()
		w.Header().Set("Cache-Control", "no-store, private")
		guarded(w, r.WithContext(ctx))
	}
}

func (h moneyHandler) finishFinancialWrite(w http.ResponseWriter, r *http.Request, owner identity.OwnerID, status int, value any, err error) {
	replayed := errors.Is(err, ErrAlreadyApplied)
	if err != nil && !replayed {
		writeMoneyError(w, err)
		return
	}
	// The coordinator has released its transaction/connection before this query.
	if err := h.auth.RevalidateOwner(r, owner); err != nil {
		if errors.Is(err, identity.ErrNotAuthorized) || errors.Is(err, pgx.ErrNoRows) {
			writeMoneyErrorStatus(w, http.StatusUnauthorized, "authentication_required", "Authentication required")
		} else {
			writeMoneyErrorStatus(w, http.StatusServiceUnavailable, "session_unavailable", "Session could not be verified; retry the same Save")
		}
		return
	}
	w.Header().Set("Cache-Control", "no-store, private")
	w.Header().Set("Idempotency-Replayed", "false")
	if replayed {
		w.Header().Set("Idempotency-Replayed", "true")
		writeMoneyJSON(w, http.StatusOK, struct {
			Outcome string `json:"outcome"`
		}{"already_applied"})
		return
	}
	if value, ok := value.(eventResponse); ok && status == 200 {
		w.Header().Set("ETag", strconv.Quote(value.EventRevision))
	}
	if status == http.StatusNoContent {
		w.WriteHeader(status)
		return
	}
	writeMoneyJSON(w, status, value)
}

// Token walking preserves duplicate-member detection before struct decoding loses it.
func uniqueJSONObject(data []byte) bool {
	if trimmed := bytes.TrimSpace(data); len(trimmed) == 0 || trimmed[0] != '{' {
		return false
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var value func() bool
	value = func() bool {
		token, err := decoder.Token()
		if err != nil {
			return false
		}
		switch token {
		case json.Delim('{'):
			seen := map[string]bool{}
			for decoder.More() {
				key, err := decoder.Token()
				if err != nil {
					return false
				}
				name, ok := key.(string)
				if !ok || seen[name] || name != strings.ToLower(name) {
					return false
				}
				seen[name] = true
				if !value() {
					return false
				}
			}
			end, err := decoder.Token()
			return err == nil && end == json.Delim('}')
		case json.Delim('['):
			for decoder.More() {
				if !value() {
					return false
				}
			}
			end, err := decoder.Token()
			return err == nil && end == json.Delim(']')
		default:
			_, delim := token.(json.Delim)
			return !delim
		}
	}
	if !value() {
		return false
	}
	_, err := decoder.Token()
	return err == io.EOF
}
