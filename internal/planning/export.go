package planning

import (
	"context"
	"github.com/Faust217/LifeOS/internal/identity"
	"github.com/Faust217/LifeOS/internal/platform/exportdata"
	"github.com/jackc/pgx/v5"
)

// WriteExport includes visible Goals/items, including archived/completed Goals.
// ADR-021 excludes the newly introduced deleted state without changing v1/v2 shapes.
func WriteExport(ctx context.Context, tx pgx.Tx, owner identity.OwnerID, sink exportdata.Sink) error {
	if !owner.Valid() {
		return ErrUnauthorized
	}
	if err := exportdata.WriteRows(ctx, tx, owner.String(), "goals", `SELECT id::text,title,completed_at,archived_at,created_at,updated_at FROM goals WHERE owner_id=$1 AND deleted_at IS NULL ORDER BY created_at DESC,id DESC`, sink); err != nil {
		return err
	}
	return exportdata.WriteRows(ctx, tx, owner.String(), "checklist_items", `SELECT id::text,goal_id::text,title,completed_at,created_at,updated_at FROM goal_checklist_items WHERE owner_id=$1 AND EXISTS (SELECT 1 FROM goals g WHERE g.owner_id=goal_checklist_items.owner_id AND g.id=goal_checklist_items.goal_id AND g.deleted_at IS NULL) ORDER BY goal_id,created_at,id`, sink)
}
