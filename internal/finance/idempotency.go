package finance

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/Faust217/LifeOS/internal/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Raising this reviewed application limit never expires or deletes old keys.
const FinancialWriteKeyLimit int64 = 100_000
const financialWriteTimeout = 10 * time.Second

var (
	// ErrAlreadyApplied is a successful, content-free outcome, not a domain failure.
	ErrAlreadyApplied     = errors.New("financial command already applied")
	ErrKeyRequired        = errors.New("idempotency key required")
	ErrInvalidKey         = errors.New("invalid idempotency key")
	ErrKeyConflict        = errors.New("idempotency key conflict")
	ErrKeyCapacity        = errors.New("idempotency capacity exceeded")
	ErrWriteBusy          = errors.New("financial write busy")
	ErrWriteUnknown       = errors.New("financial write outcome unknown")
	ErrFingerprintVersion = errors.New("unsupported idempotency fingerprint version")
)

type financialCommand struct {
	version          int16
	revisionRequired bool
	operation        string
	target           any
	parameters       []any
}

type financialAdmissionKey struct{}

func validateRequestKey(key string) error {
	if key == "" {
		return ErrKeyRequired
	}
	if !validID(key) || key != strings.ToLower(key) || key[14] != '4' || !strings.ContainsRune("89ab", rune(key[19])) {
		return ErrInvalidKey
	}
	return nil
}

// This v1 encoding is immutable. New versions must retain this normalizer for old keys.
func canonicalFinancialCommand(owner identity.OwnerID, cmd financialCommand) ([]byte, [32]byte, error) {
	return canonicalFinancialBytes(strings.ToLower(owner.String()), cmd)
}

func canonicalFinancialBytes(owner string, cmd financialCommand) ([]byte, [32]byte, error) {
	tag := "lifeos-finance-command-v1"
	if cmd.version == 2 {
		tag = "lifeos-finance-command-v2"
	}
	data, err := json.Marshal([]any{tag, owner, cmd.operation, cmd.target, cmd.parameters})
	return data, sha256.Sum256(data), err
}

func canonicalInstant(at time.Time) string {
	return at.UTC().Truncate(time.Microsecond).Format("2006-01-02T15:04:05.000000Z")
}

type writeQuery interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func committedCommand(ctx context.Context, db writeQuery, owner identity.OwnerID, key string, requestedVersion int16, digest [32]byte) (bool, error) {
	var version int16
	var stored []byte
	err := db.QueryRow(ctx, `SELECT fingerprint_version, request_fingerprint FROM finance_write_requests WHERE owner_id=$1 AND request_key=$2`, owner.String(), key).Scan(&version, &stored)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, financialWriteError(err)
	}
	if version != 1 && version != 2 {
		return false, ErrFingerprintVersion
	}
	if version != requestedVersion || !bytes.Equal(stored, digest[:]) {
		return false, ErrKeyConflict
	}
	return true, nil
}

func capacityBand(count, limit int64) string {
	if count >= limit {
		return "full"
	}
	if count >= (limit*9+9)/10 {
		return "approaching"
	}
	return "available"
}

// Never log owner IDs, keys, request bodies, resource IDs, or financial contents.
func observeCapacity(count, limit int64) {
	if band := capacityBand(count, limit); band != "available" {
		slog.Warn("Financial request key capacity", "event", "finance_idempotency_capacity", "band", band, "limit", limit)
	}
}

func executeFinancialWrite[T any](parent context.Context, r *repository, owner identity.OwnerID, key string, cmd financialCommand, mutate func(context.Context, pgx.Tx) (T, error)) (zero T, err error) {
	if !owner.Valid() {
		return zero, ErrUnauthorized
	}
	if err = validateRequestKey(key); err != nil {
		return zero, err
	}
	if cmd.version == 0 {
		cmd.version = 1
	}
	_, digest, err := canonicalFinancialCommand(owner, cmd)
	if err != nil {
		return zero, ErrInvalidInput
	}
	ctx, cancel := context.WithTimeout(parent, financialWriteTimeout)
	defer cancel()
	if ctx.Value(financialAdmissionKey{}) != r {
		select {
		case r.writeSlots <- struct{}{}:
			defer func() { <-r.writeSlots }()
		default:
			return zero, ErrWriteBusy
		}
	}
	if found, err := committedCommand(ctx, r.pool, owner, key, cmd.version, digest); err != nil {
		return zero, err
	} else if found {
		return zero, ErrAlreadyApplied
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return zero, financialWriteError(err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer stop()
		_ = tx.Rollback(cleanup)
	}()
	if _, err = tx.Exec(ctx, `SET LOCAL lock_timeout='2s'; SET LOCAL statement_timeout='8s'; SET LOCAL idle_in_transaction_session_timeout='10s'`); err != nil {
		return zero, financialWriteError(err)
	}
	if err = lockFinanceOwner(ctx, tx, owner); err != nil {
		return zero, financialWriteError(err)
	}
	// A new READ COMMITTED statement after waiting sees the previous writer's commit.
	if found, err := committedCommand(ctx, tx, owner, key, cmd.version, digest); err != nil {
		return zero, err
	} else if found {
		return zero, ErrAlreadyApplied
	}
	if cmd.revisionRequired {
		return zero, ErrRevisionRequired
	}
	var count int64
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM finance_write_requests WHERE owner_id=$1`, owner.String()).Scan(&count); err != nil {
		return zero, financialWriteError(err)
	}
	if count >= r.keyLimit {
		r.capacityObserver(count, r.keyLimit)
		return zero, ErrKeyCapacity
	}
	inserted, err := tx.Exec(ctx, `INSERT INTO finance_write_requests(owner_id,request_key,operation,fingerprint_version,request_fingerprint) VALUES($1,$2,$3,$4,$5) ON CONFLICT(owner_id,request_key) DO NOTHING`, owner.String(), key, cmd.operation, cmd.version, digest[:])
	if err != nil {
		return zero, financialWriteError(err)
	}
	if inserted.RowsAffected() == 0 {
		if found, err := committedCommand(ctx, tx, owner, key, cmd.version, digest); err != nil {
			return zero, err
		} else if found {
			return zero, ErrAlreadyApplied
		}
		return zero, ErrWriteBusy
	}
	result, err := mutate(ctx, tx)
	if err != nil {
		return zero, financialWriteError(err)
	}
	if err = tx.Commit(ctx); err != nil {
		var pgError *pgconn.PgError
		if errors.Is(err, pgx.ErrTxCommitRollback) || errors.As(err, &pgError) {
			return zero, financialWriteError(err)
		}
		return zero, ErrWriteUnknown
	}
	r.capacityObserver(count+1, r.keyLimit)
	return result, nil
}

func financialWriteError(err error) error {
	var pgError *pgconn.PgError
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) || errors.Is(err, pgx.ErrTxCommitRollback) {
		return ErrWriteBusy
	}
	if errors.As(err, &pgError) {
		switch pgError.Code {
		case "55P03", "40P01", "40001", "57014":
			return ErrWriteBusy
		}
	}
	return err
}
