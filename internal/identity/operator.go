package identity

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// LocalOperator is a narrow adapter for the explicitly confirmed, localhost-only
// command. It is never mounted in the HTTP API or included in the API image.
type LocalOperator struct{ repo *repository }

// NewLocalOperator requires the command's explicit human approval. Database
// credentials and localhost access are the other operator boundaries.
func NewLocalOperator(pool *pgxpool.Pool, approved bool) (*LocalOperator, error) {
	if !approved {
		return nil, ErrOperatorRequired
	}
	return &LocalOperator{repo: newRepository(pool)}, nil
}

func (o *LocalOperator) Bootstrap(ctx context.Context, email string, expiresAt time.Time) (string, error) {
	return o.repo.bootstrapOwnerInvitation(ctx, operatorAuthorization{approved: true}, email, expiresAt)
}

func (o *LocalOperator) RevokeBootstrap(ctx context.Context) error {
	return o.repo.revokeBootstrapInvitation(ctx, operatorAuthorization{approved: true})
}

func (o *LocalOperator) Invite(ctx context.Context, adminID, email string, expiresAt time.Time) (string, error) {
	return o.repo.createInvitation(ctx, adminID, email, expiresAt)
}

func (o *LocalOperator) Revoke(ctx context.Context, adminID, invitationID string) error {
	return o.repo.revokeInvitation(ctx, adminID, invitationID)
}
