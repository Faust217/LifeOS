package planning

import (
	"context"
	"errors"
	"time"

	"github.com/Faust217/LifeOS/internal/identity"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type repository struct{ pool *pgxpool.Pool }

func (r *repository) listGoals(ctx context.Context, owner identity.OwnerID, goalID string) ([]Goal, error) {
	// Both queries must observe the same snapshot so progress and Goal state
	// cannot disagree when a checklist mutation commits during a read.
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	query := `SELECT id, title, completed_at, archived_at, created_at, updated_at
        FROM goals WHERE owner_id = $1 AND deleted_at IS NULL`
	args := []any{owner.String()}
	if goalID != "" {
		query += ` AND id = $2`
		args = append(args, goalID)
	}
	query += ` ORDER BY created_at DESC, id DESC`
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	goals := make([]Goal, 0)
	byID := make(map[string]int)
	for rows.Next() {
		var goal Goal
		if err := rows.Scan(&goal.ID, &goal.Title, &goal.CompletedAt, &goal.ArchivedAt, &goal.CreatedAt, &goal.UpdatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		goal.Items = make([]ChecklistItem, 0)
		byID[goal.ID] = len(goals)
		goals = append(goals, goal)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(goals) == 0 {
		return goals, tx.Commit(ctx)
	}

	itemQuery := `SELECT goal_id, id, title, completed_at, created_at, updated_at
        FROM goal_checklist_items WHERE owner_id = $1 AND EXISTS
        (SELECT 1 FROM goals g WHERE g.owner_id=goal_checklist_items.owner_id
         AND g.id=goal_checklist_items.goal_id AND g.deleted_at IS NULL)`
	itemArgs := []any{owner.String()}
	if goalID != "" {
		itemQuery += ` AND goal_id = $2`
		itemArgs = append(itemArgs, goalID)
	}
	itemQuery += ` ORDER BY created_at, id`
	items, err := tx.Query(ctx, itemQuery, itemArgs...)
	if err != nil {
		return nil, err
	}
	for items.Next() {
		var id string
		var item ChecklistItem
		if err := items.Scan(&id, &item.ID, &item.Title, &item.CompletedAt, &item.CreatedAt, &item.UpdatedAt); err != nil {
			items.Close()
			return nil, err
		}
		if index, ok := byID[id]; ok {
			goals[index].Items = append(goals[index].Items, item)
			goals[index].TotalCount++
			if item.CompletedAt != nil {
				goals[index].CompletedCount++
			}
		}
	}
	items.Close()
	if err := items.Err(); err != nil {
		return nil, err
	}
	return goals, tx.Commit(ctx)
}

func (r *repository) createGoal(ctx context.Context, owner identity.OwnerID, id, title string) error {
	_, err := r.pool.Exec(ctx, `INSERT INTO goals (id, owner_id, title) VALUES ($1, $2, $3)`, id, owner.String(), title)
	return err
}

func (r *repository) renameGoal(ctx context.Context, owner identity.OwnerID, goalID, title string) error {
	var id string
	err := r.pool.QueryRow(ctx, `UPDATE goals SET title = $3, updated_at = now()
        WHERE owner_id = $1 AND id = $2 AND archived_at IS NULL AND deleted_at IS NULL RETURNING id`, owner.String(), goalID, title).Scan(&id)
	return notFoundOr(err)
}

func (r *repository) archiveGoal(ctx context.Context, owner identity.OwnerID, goalID string) error {
	var id string
	err := r.pool.QueryRow(ctx, `UPDATE goals SET archived_at = now(), updated_at = now()
        WHERE owner_id = $1 AND id = $2 AND archived_at IS NULL AND deleted_at IS NULL RETURNING id`, owner.String(), goalID).Scan(&id)
	return notFoundOr(err)
}

func (r *repository) restoreGoal(ctx context.Context, owner identity.OwnerID, goalID string) error {
	var id string
	err := r.pool.QueryRow(ctx, `UPDATE goals SET archived_at = NULL, updated_at = now()
        WHERE owner_id = $1 AND id = $2 AND archived_at IS NOT NULL AND deleted_at IS NULL RETURNING id`, owner.String(), goalID).Scan(&id)
	return notFoundOr(err)
}

func (r *repository) setGoalCompleted(ctx context.Context, owner identity.OwnerID, goalID string, completed bool) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	_, archived, err := lockGoal(ctx, tx, owner, goalID)
	if err != nil {
		return err
	}
	if archived != nil {
		return ErrConflict
	}
	if completed {
		var total, done int
		if err := tx.QueryRow(ctx, `SELECT count(*), count(*) FILTER (WHERE completed_at IS NOT NULL)
            FROM goal_checklist_items WHERE owner_id = $1 AND goal_id = $2`, owner.String(), goalID).Scan(&total, &done); err != nil {
			return err
		}
		if total == 0 || done != total {
			return ErrConflict
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE goals SET completed_at = CASE WHEN $3 THEN COALESCE(completed_at, now()) ELSE NULL END,
        updated_at = now() WHERE owner_id = $1 AND id = $2`, owner.String(), goalID, completed); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Lock the parent row before any checklist change. This serializes edits with
// completion and prevents a completed Goal from gaining an incomplete item.
func (r *repository) withEditableGoal(ctx context.Context, owner identity.OwnerID, goalID string, change func(pgx.Tx) error) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	completed, archived, err := lockGoal(ctx, tx, owner, goalID)
	if err != nil {
		return err
	}
	if completed != nil || archived != nil {
		return ErrConflict
	}
	if err := change(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func lockGoal(ctx context.Context, tx pgx.Tx, owner identity.OwnerID, goalID string) (*time.Time, *time.Time, error) {
	var completed, archived *time.Time
	err := tx.QueryRow(ctx, `SELECT completed_at, archived_at FROM goals
        WHERE owner_id = $1 AND id = $2 AND deleted_at IS NULL FOR UPDATE`, owner.String(), goalID).Scan(&completed, &archived)
	if err != nil {
		return nil, nil, notFoundOr(err)
	}
	return completed, archived, nil
}

func (r *repository) addItem(ctx context.Context, owner identity.OwnerID, goalID, itemID, title string) error {
	return r.withEditableGoal(ctx, owner, goalID, func(tx pgx.Tx) error {
		_, err := tx.Exec(ctx, `INSERT INTO goal_checklist_items (id, owner_id, goal_id, title) VALUES ($1, $2, $3, $4)`, itemID, owner.String(), goalID, title)
		return err
	})
}

func (r *repository) renameItem(ctx context.Context, owner identity.OwnerID, goalID, itemID, title string) error {
	return r.withEditableGoal(ctx, owner, goalID, func(tx pgx.Tx) error {
		var id string
		err := tx.QueryRow(ctx, `UPDATE goal_checklist_items SET title = $4, updated_at = now()
            WHERE owner_id = $1 AND goal_id = $2 AND id = $3 RETURNING id`, owner.String(), goalID, itemID, title).Scan(&id)
		return notFoundOr(err)
	})
}

func (r *repository) setItemCompleted(ctx context.Context, owner identity.OwnerID, goalID, itemID string, completed bool) error {
	return r.withEditableGoal(ctx, owner, goalID, func(tx pgx.Tx) error {
		var id string
		err := tx.QueryRow(ctx, `UPDATE goal_checklist_items
            SET completed_at = CASE WHEN $4 THEN COALESCE(completed_at, now()) ELSE NULL END, updated_at = now()
            WHERE owner_id = $1 AND goal_id = $2 AND id = $3 RETURNING id`, owner.String(), goalID, itemID, completed).Scan(&id)
		return notFoundOr(err)
	})
}

func (r *repository) removeItem(ctx context.Context, owner identity.OwnerID, goalID, itemID string) error {
	return r.withEditableGoal(ctx, owner, goalID, func(tx pgx.Tx) error {
		var id string
		err := tx.QueryRow(ctx, `DELETE FROM goal_checklist_items
            WHERE owner_id = $1 AND goal_id = $2 AND id = $3 RETURNING id`, owner.String(), goalID, itemID).Scan(&id)
		return notFoundOr(err)
	})
}

func notFoundOr(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return err
}

func (r *repository) deleteGoal(parent context.Context, owner identity.OwnerID, goalID string) error {
	ctx, cancel := context.WithTimeout(parent, 10*time.Second)
	defer cancel()
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		defer stop()
		_ = tx.Rollback(cleanup)
	}()
	if _, err = tx.Exec(ctx, `SET LOCAL lock_timeout='2s'; SET LOCAL statement_timeout='8s'; SET LOCAL idle_in_transaction_session_timeout='10s'`); err != nil {
		return err
	}
	var deleted *time.Time
	if err = tx.QueryRow(ctx, `SELECT deleted_at FROM goals WHERE owner_id=$1 AND id=$2 FOR UPDATE`, owner.String(), goalID).Scan(&deleted); err != nil {
		return notFoundOr(err)
	}
	if deleted == nil {
		if _, err = tx.Exec(ctx, `UPDATE goals SET deleted_at=now(),updated_at=now() WHERE owner_id=$1 AND id=$2 AND deleted_at IS NULL`, owner.String(), goalID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
