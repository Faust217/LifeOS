package identity

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrInvalidInput          = errors.New("invalid identity input")
	ErrNotAuthorized         = errors.New("owner administrator required")
	ErrConflict              = errors.New("identity or invitation already exists")
	ErrInvitationUnavailable = errors.New("invitation unavailable")
	ErrBootstrapUnavailable  = errors.New("owner bootstrap already provisioned")
	ErrOperatorRequired      = errors.New("trusted operator authorization required")
)

// User is a LifeOS owner boundary; its ID is never a Google identifier or email.
type User struct {
	ID           string
	Email        string
	Status       string
	IsOwnerAdmin bool
	TimeZone     *string
}

// InvitationState is derived from durable timestamps, so expiry needs no background job.
type InvitationState string

const (
	InvitationPending  InvitationState = "pending"
	InvitationAccepted InvitationState = "accepted"
	InvitationExpired  InvitationState = "expired"
	InvitationRevoked  InvitationState = "revoked"
)

// verifiedGoogleIdentity cannot be constructed by a handler in another package.
// Auth.callback creates it only after server-side ID-token and nonce verification.
type verifiedGoogleIdentity struct {
	sub           string
	email         string
	emailVerified bool
}

// operatorAuthorization is deliberately private. The local-only operator command
// requires explicit confirmation and loopback database access before approval.
type operatorAuthorization struct{ approved bool }

// repository owns identity persistence. Only a future identity service in this package can invoke it.
type repository struct{ pool *pgxpool.Pool }

func newRepository(pool *pgxpool.Pool) *repository { return &repository{pool: pool} }

// bootstrapOwnerInvitation is for a trusted local operator only. Before any user exists,
// the operator may replace an expired, revoked, or mistaken bootstrap invitation in place.
// It has no network route. The unique database index is the final concurrent-use guard.
func (r *repository) bootstrapOwnerInvitation(ctx context.Context, operator operatorAuthorization, email string, expiresAt time.Time) (string, error) {
	if !operator.approved {
		return "", ErrOperatorRequired
	}
	email, err := normalizeEmail(email)
	if err != nil || !expiresAt.After(time.Now()) {
		return "", ErrInvalidInput
	}
	id, err := newUUID()
	if err != nil {
		return "", err
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", fmt.Errorf("begin owner bootstrap: %w", err)
	}
	defer tx.Rollback(ctx)
	// Serialize first-owner attempts without introducing an application-wide lock service.
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(180018)"); err != nil {
		return "", fmt.Errorf("lock owner bootstrap: %w", err)
	}
	var usersExist bool
	err = tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM lifeos_users)").Scan(&usersExist)
	if err != nil {
		return "", fmt.Errorf("check owner bootstrap: %w", err)
	}
	if usersExist {
		return "", ErrBootstrapUnavailable
	}
	var existingID string
	err = tx.QueryRow(ctx, "SELECT id FROM invitations WHERE owner_bootstrap FOR UPDATE").Scan(&existingID)
	if err == nil {
		_, err = tx.Exec(ctx, `UPDATE invitations SET email = $1, expires_at = $2,
			created_at = now(), revoked_at = NULL WHERE id = $3 AND accepted_at IS NULL`, email, expiresAt.UTC(), existingID)
		if err != nil {
			return "", persistenceError("replace owner bootstrap invitation", err)
		}
		if err = tx.Commit(ctx); err != nil {
			return "", fmt.Errorf("commit owner bootstrap replacement: %w", err)
		}
		return existingID, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("read owner bootstrap: %w", err)
	}
	var invitationsExist bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM invitations)").Scan(&invitationsExist); err != nil {
		return "", fmt.Errorf("check other invitations: %w", err)
	}
	if invitationsExist {
		return "", ErrBootstrapUnavailable
	}
	_, err = tx.Exec(ctx, "INSERT INTO invitations (id, email, owner_bootstrap, expires_at) VALUES ($1, $2, true, $3)", id, email, expiresAt.UTC())
	if err != nil {
		return "", persistenceError("create owner bootstrap invitation", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return "", fmt.Errorf("commit owner bootstrap: %w", err)
	}
	return id, nil
}

// revokeBootstrapInvitation is a local operator recovery action before any owner is admitted.
func (r *repository) revokeBootstrapInvitation(ctx context.Context, operator operatorAuthorization) error {
	if !operator.approved {
		return ErrOperatorRequired
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin owner bootstrap revocation: %w", err)
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(180018)"); err != nil {
		return fmt.Errorf("lock owner bootstrap: %w", err)
	}
	var id string
	err = tx.QueryRow(ctx, `UPDATE invitations SET revoked_at = now()
		WHERE owner_bootstrap AND accepted_at IS NULL AND revoked_at IS NULL
		AND NOT EXISTS (SELECT 1 FROM lifeos_users) RETURNING id`).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInvitationUnavailable
	}
	if err != nil {
		return persistenceError("revoke owner bootstrap invitation", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit owner bootstrap revocation: %w", err)
	}
	return nil
}

// createInvitation permits only an active owner administrator to invite a tester.
// The local operator supplies adminID after confirming trusted database access.
func (r *repository) createInvitation(ctx context.Context, adminID, email string, expiresAt time.Time) (string, error) {
	email, err := normalizeEmail(email)
	if err != nil || !expiresAt.After(time.Now()) || adminID == "" {
		return "", ErrInvalidInput
	}
	id, err := newUUID()
	if err != nil {
		return "", err
	}
	err = r.pool.QueryRow(ctx, `
		INSERT INTO invitations (id, email, issued_by, expires_at)
		SELECT $1, $2, id, $3 FROM lifeos_users
		WHERE id = $4 AND status = 'active' AND is_owner_admin
		RETURNING id`, id, email, expiresAt.UTC(), adminID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotAuthorized
	}
	if err != nil {
		return "", persistenceError("create invitation", err)
	}
	return id, nil
}

// revokeInvitation withdraws an unaccepted invitation; accepted access is a later user/session concern.
func (r *repository) revokeInvitation(ctx context.Context, adminID, invitationID string) error {
	if adminID == "" || invitationID == "" {
		return ErrInvalidInput
	}
	var id string
	err := r.pool.QueryRow(ctx, `
		UPDATE invitations SET revoked_at = now()
		WHERE id = $1 AND accepted_at IS NULL AND revoked_at IS NULL
		AND EXISTS (SELECT 1 FROM lifeos_users WHERE id = $2 AND status = 'active' AND is_owner_admin)
		RETURNING id`, invitationID, adminID).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrInvitationUnavailable
	}
	if err != nil {
		return persistenceError("revoke invitation", err)
	}
	return nil
}

// acceptInvitation atomically consumes one invitation and binds one stable Google sub.
// No outside package can pass client claims as a verified identity; Auth.callback
// validates the server-side ID token before calling this method.
func (r *repository) acceptInvitation(ctx context.Context, invitationID string, identity verifiedGoogleIdentity) (User, error) {
	email, err := normalizeEmail(identity.email)
	if err != nil || !identity.emailVerified || strings.TrimSpace(identity.sub) != identity.sub || len(identity.sub) == 0 || len(identity.sub) > 255 || invitationID == "" {
		return User{}, ErrInvalidInput
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return User{}, fmt.Errorf("begin invitation acceptance: %w", err)
	}
	defer tx.Rollback(ctx)
	var invitedEmail string
	var expiresAt, databaseNow time.Time
	var acceptedAt, revokedAt *time.Time
	var bootstrap bool
	err = tx.QueryRow(ctx, `SELECT email, expires_at, accepted_at, revoked_at, owner_bootstrap, now()
		FROM invitations WHERE id = $1 FOR UPDATE`, invitationID).
		Scan(&invitedEmail, &expiresAt, &acceptedAt, &revokedAt, &bootstrap, &databaseNow)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrInvitationUnavailable
	}
	if err != nil {
		return User{}, persistenceError("read invitation", err)
	}
	if invitedEmail != email || acceptedAt != nil || revokedAt != nil || !databaseNow.Before(expiresAt) {
		return User{}, ErrInvitationUnavailable
	}
	userID, err := newUUID()
	if err != nil {
		return User{}, err
	}
	user := User{ID: userID, Email: email, Status: "active", IsOwnerAdmin: bootstrap}
	_, err = tx.Exec(ctx, "INSERT INTO lifeos_users (id, email, is_owner_admin) VALUES ($1, $2, $3)", user.ID, user.Email, user.IsOwnerAdmin)
	if err != nil {
		return User{}, persistenceError("create LifeOS user", err)
	}
	_, err = tx.Exec(ctx, "INSERT INTO google_identities (user_id, google_sub) VALUES ($1, $2)", user.ID, identity.sub)
	if err != nil {
		return User{}, persistenceError("bind Google identity", err)
	}
	var consumedID string
	err = tx.QueryRow(ctx, `UPDATE invitations SET accepted_by = $1, accepted_at = now()
		WHERE id = $2 AND accepted_at IS NULL AND revoked_at IS NULL AND expires_at > now()
		RETURNING id`, user.ID, invitationID).Scan(&consumedID)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrInvitationUnavailable
	}
	if err != nil {
		return User{}, persistenceError("consume invitation", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return User{}, fmt.Errorf("commit invitation acceptance: %w", err)
	}
	return user, nil
}

// findByGoogleSub resolves the internal owner ID without using email as a key.
// Auth.admitVerifiedIdentity supplies a sub from a server-verified token.
func (r *repository) findByGoogleSub(ctx context.Context, sub string) (User, error) {
	if sub == "" {
		return User{}, ErrInvalidInput
	}
	var user User
	err := r.pool.QueryRow(ctx, `SELECT u.id, u.email, u.status, u.is_owner_admin, to_jsonb(u)->>'time_zone'
		FROM google_identities g JOIN lifeos_users u ON u.id = g.user_id
		WHERE g.google_sub = $1`, sub).Scan(&user.ID, &user.Email, &user.Status, &user.IsOwnerAdmin, &user.TimeZone)
	if err != nil {
		return User{}, err
	}
	return user, nil
}

func (r *repository) updateTimeZone(ctx context.Context, owner OwnerID, zone string) error {
	tag, err := r.pool.Exec(ctx, `UPDATE lifeos_users SET time_zone = $2, updated_at = now()
		WHERE id = $1 AND status = 'active'`, owner.String(), zone)
	if err != nil {
		return fmt.Errorf("update owner timezone: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return ErrNotAuthorized
	}
	return nil
}

func (r *repository) findPendingInvitationByEmail(ctx context.Context, email string) (string, error) {
	email, err := normalizeEmail(email)
	if err != nil {
		return "", err
	}
	var id string
	err = r.pool.QueryRow(ctx, `SELECT id FROM invitations WHERE email = $1
		AND accepted_at IS NULL AND revoked_at IS NULL AND expires_at > now()`, email).Scan(&id)
	return id, err
}

// State derives expiry at read time; accepted and revoked outcomes remain durable.
func State(acceptedAt, revokedAt *time.Time, expiresAt, now time.Time) InvitationState {
	if acceptedAt != nil {
		return InvitationAccepted
	}
	if revokedAt != nil {
		return InvitationRevoked
	}
	if !now.Before(expiresAt) {
		return InvitationExpired
	}
	return InvitationPending
}

func normalizeEmail(email string) (string, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if len(email) < 3 || len(email) > 320 || strings.IndexByte(email, '@') < 1 || strings.ContainsAny(email, " \t\r\n") {
		return "", ErrInvalidInput
	}
	return email, nil
}

func newUUID() (string, error) {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "", fmt.Errorf("generate user identity: %w", err)
	}
	id[6] = (id[6] & 0x0f) | 0x40
	id[8] = (id[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", id[:4], id[4:6], id[6:8], id[8:10], id[10:]), nil
}

func persistenceError(action string, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrConflict
	}
	return fmt.Errorf("%s: %w", action, err)
}
