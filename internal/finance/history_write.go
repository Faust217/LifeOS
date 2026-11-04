package finance

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"strings"
	"time"

	"github.com/Faust217/LifeOS/internal/identity"
	"github.com/jackc/pgx/v5"
)

var ErrHistoryRevisionExhausted = errors.New("finance history revision exhausted")

func lockFinanceOwner(ctx context.Context, tx pgx.Tx, owner identity.OwnerID) error {
	hash := sha256.Sum256([]byte("lifeos.finance.write-owner.v1|" + strings.ToLower(owner.String())))
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, int64(binary.BigEndian.Uint64(hash[:8])))
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `SET LOCAL lifeos.finance_writer_version='11'`)
	return err
}

func bumpHistoryRevision(ctx context.Context, tx pgx.Tx, owner identity.OwnerID) error {
	result, err := tx.Exec(ctx, `INSERT INTO finance_history_revisions(owner_id,epoch,revision)
        VALUES($1,gen_random_uuid(),1) ON CONFLICT(owner_id) DO UPDATE
        SET revision=finance_history_revisions.revision+1 WHERE finance_history_revisions.revision < 9223372036854775807`, owner.String())
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return ErrHistoryRevisionExhausted
	}
	return nil
}

// Management operations share the same owner lock as keyed ledger writes.
func (r *repository) withHistoryMutation(parent context.Context, owner identity.OwnerID, mutate func(context.Context, pgx.Tx) (bool, error)) error {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return financialWriteError(err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer stop()
		_ = tx.Rollback(cleanup)
	}()
	if _, err = tx.Exec(ctx, `SET LOCAL lock_timeout='2s'; SET LOCAL statement_timeout='8s'; SET LOCAL idle_in_transaction_session_timeout='10s'`); err != nil {
		return financialWriteError(err)
	}
	if err = lockFinanceOwner(ctx, tx, owner); err != nil {
		return financialWriteError(err)
	}
	changed, err := mutate(ctx, tx)
	if err != nil {
		return err
	}
	if changed {
		if err = bumpHistoryRevision(ctx, tx, owner); err != nil {
			return financialWriteError(err)
		}
	}
	return financialWriteError(tx.Commit(ctx))
}
