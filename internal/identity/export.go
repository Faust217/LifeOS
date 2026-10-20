package identity

import (
	"context"
	"net/http"
	"time"

	"github.com/Faust217/LifeOS/internal/platform/exportdata"
	"github.com/jackc/pgx/v5"
)

// ExportProfile reads only the normal portability projection on the shared snapshot.
func ExportProfile(ctx context.Context, tx pgx.Tx, owner OwnerID) (exportdata.Profile, error) {
	if !owner.Valid() {
		return exportdata.Profile{}, ErrNotAuthorized
	}
	var p exportdata.Profile
	var created, updated time.Time
	err := tx.QueryRow(ctx, `SELECT id::text,email,created_at,updated_at,time_zone FROM lifeos_users WHERE id=$1 AND status='active'`, owner.String()).Scan(&p.ID, &p.Email, &created, &updated, &p.TimeZone)
	if err != nil {
		return p, err
	}
	if p.TimeZone != nil && !validIANAZone(*p.TimeZone) {
		return p, ErrInvalidInput
	}
	p.CreatedAt = exportdata.UTC(created)
	p.UpdatedAt = exportdata.UTC(updated)
	return p, nil
}

// RevalidateOwner checks the same request cookie immediately before an artifact is sent.
func (a *Auth) RevalidateOwner(r *http.Request, owner OwnerID) error {
	if !owner.Valid() {
		return ErrNotAuthorized
	}
	user, _, err := a.currentUser(r)
	if err != nil {
		return err
	}
	if user.ID != owner.String() {
		return ErrNotAuthorized
	}
	return nil
}
