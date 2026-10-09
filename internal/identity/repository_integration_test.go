package identity

import (
	"context"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Run against an empty disposable database named *_test. This test never resets an existing database.
func TestIdentityMigrationAndRepository(t *testing.T) {
	dsn := os.Getenv("LIFEOS_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set LIFEOS_TEST_DATABASE_URL to an empty disposable *_test PostgreSQL database")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.HasSuffix(strings.TrimPrefix(parsed.Path, "/"), "_test") {
		t.Fatal("LIFEOS_TEST_DATABASE_URL must name a disposable *_test database")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var existing bool
	if err := pool.QueryRow(ctx, "SELECT to_regclass('public.schema_migrations') IS NOT NULL").Scan(&existing); err != nil || existing {
		t.Fatalf("test database must be empty: existing=%t err=%v", existing, err)
	}
	migrationsPath, err := filepath.Abs("../../db/migrations")
	if err != nil {
		t.Fatal(err)
	}
	source := (&url.URL{Scheme: "file", Path: filepath.ToSlash(migrationsPath)}).String()
	runner, err := migrate.New(source, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer runner.Close()
	if err := runner.Steps(1); err != nil {
		t.Fatalf("apply Foundation version 1: %v", err)
	}
	if version, dirty, err := runner.Version(); err != nil || version != 1 || dirty {
		t.Fatalf("Foundation version = %d dirty=%t err=%v", version, dirty, err)
	}
	if err := runner.Steps(1); err != nil {
		t.Fatalf("upgrade version 1 to identity version 2: %v", err)
	}
	if version, dirty, err := runner.Version(); err != nil || version != 2 || dirty {
		t.Fatalf("identity version = %d dirty=%t err=%v", version, dirty, err)
	}

	repo := newRepository(pool)
	operator := operatorAuthorization{approved: true} // Test-only stand-in for the confirmed local operator command.
	expiry := time.Now().Add(2 * time.Hour)
	if _, err := repo.bootstrapOwnerInvitation(ctx, operatorAuthorization{}, "Owner@Example.com", expiry); !errors.Is(err, ErrOperatorRequired) {
		t.Fatalf("unauthorized bootstrap error = %v", err)
	}
	ownerInvite, err := repo.bootstrapOwnerInvitation(ctx, operator, "Owner@Example.com", expiry)
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.revokeBootstrapInvitation(ctx, operatorAuthorization{}); !errors.Is(err, ErrOperatorRequired) {
		t.Fatalf("unauthorized bootstrap revocation error = %v", err)
	}
	if err := repo.revokeBootstrapInvitation(ctx, operator); err != nil {
		t.Fatalf("revoke mistaken bootstrap: %v", err)
	}
	if _, err := repo.acceptInvitation(ctx, ownerInvite, verifiedGoogleIdentity{sub: "owner-sub", email: "owner@example.com", emailVerified: true}); !errors.Is(err, ErrInvitationUnavailable) {
		t.Fatalf("revoked bootstrap admission error = %v", err)
	}
	replacedID, err := repo.bootstrapOwnerInvitation(ctx, operator, "Owner@Example.com", expiry)
	if err != nil || replacedID != ownerInvite {
		t.Fatalf("replace bootstrap = %q, %v", replacedID, err)
	}
	if _, err := repo.createInvitation(ctx, ownerInvite, "tester@example.com", expiry); !errors.Is(err, ErrNotAuthorized) {
		t.Fatalf("non-admin invite error = %v", err)
	}
	if _, err := repo.acceptInvitation(ctx, ownerInvite, verifiedGoogleIdentity{sub: "owner-sub", email: "owner@example.com"}); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("unverified email error = %v", err)
	}
	owner, err := repo.acceptInvitation(ctx, ownerInvite, verifiedGoogleIdentity{sub: "owner-sub", email: "OWNER@example.com", emailVerified: true})
	if err != nil || !owner.IsOwnerAdmin {
		t.Fatalf("admit owner = %+v, %v", owner, err)
	}
	if _, err := repo.bootstrapOwnerInvitation(ctx, operator, "other@example.com", expiry); !errors.Is(err, ErrBootstrapUnavailable) {
		t.Fatalf("post-admission bootstrap error = %v", err)
	}
	bound, err := repo.findByGoogleSub(ctx, "owner-sub")
	if err != nil || bound.ID != owner.ID {
		t.Fatalf("owner binding = %+v, %v", bound, err)
	}
	if _, err := repo.acceptInvitation(ctx, ownerInvite, verifiedGoogleIdentity{sub: "other-sub", email: owner.Email, emailVerified: true}); !errors.Is(err, ErrInvitationUnavailable) {
		t.Fatalf("reused invitation error = %v", err)
	}

	revokedInvite, err := repo.createInvitation(ctx, owner.ID, "revoked@example.com", expiry)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.createInvitation(ctx, owner.ID, "REVOKED@example.com", expiry); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate pending invitation error = %v", err)
	}
	if err := repo.revokeInvitation(ctx, ownerInvite, revokedInvite); !errors.Is(err, ErrInvitationUnavailable) {
		t.Fatalf("non-admin revocation error = %v", err)
	}
	if err := repo.revokeInvitation(ctx, owner.ID, revokedInvite); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.acceptInvitation(ctx, revokedInvite, verifiedGoogleIdentity{sub: "revoked-sub", email: "revoked@example.com", emailVerified: true}); !errors.Is(err, ErrInvitationUnavailable) {
		t.Fatalf("revoked invitation error = %v", err)
	}

	expiredInvite, err := repo.createInvitation(ctx, owner.ID, "expired@example.com", expiry)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "UPDATE invitations SET expires_at = created_at + interval '1 second' WHERE id = $1", expiredInvite); err != nil {
		t.Fatal(err)
	}
	// The test moves its own disposable database clock-relative expiry into the past.
	if _, err := pool.Exec(ctx, "UPDATE invitations SET created_at = created_at - interval '2 hours', expires_at = expires_at - interval '2 hours' WHERE id = $1", expiredInvite); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.acceptInvitation(ctx, expiredInvite, verifiedGoogleIdentity{sub: "expired-sub", email: "expired@example.com", emailVerified: true}); !errors.Is(err, ErrInvitationUnavailable) {
		t.Fatalf("expired invitation error = %v", err)
	}

	duplicateInvite, err := repo.createInvitation(ctx, owner.ID, "duplicate@example.com", expiry)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.acceptInvitation(ctx, duplicateInvite, verifiedGoogleIdentity{sub: "new-sub", email: "wrong@example.com", emailVerified: true}); !errors.Is(err, ErrInvitationUnavailable) {
		t.Fatalf("mismatched verified email error = %v", err)
	}
	if _, err := repo.acceptInvitation(ctx, duplicateInvite, verifiedGoogleIdentity{sub: "owner-sub", email: "duplicate@example.com", emailVerified: true}); !errors.Is(err, ErrConflict) {
		t.Fatalf("duplicate sub error = %v", err)
	}
	var stillPending bool
	if err := pool.QueryRow(ctx, "SELECT accepted_at IS NULL FROM invitations WHERE id = $1", duplicateInvite).Scan(&stillPending); err != nil || !stillPending {
		t.Fatalf("duplicate-sub attempt consumed invitation: pending=%t err=%v", stillPending, err)
	}

	concurrentInvite, err := repo.createInvitation(ctx, owner.ID, "concurrent@example.com", expiry)
	if err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	type admissionResult struct {
		user User
		err  error
	}
	results := make(chan admissionResult, 2)
	for _, sub := range []string{"concurrent-sub-1", "concurrent-sub-2"} {
		group.Add(1)
		go func(sub string) {
			defer group.Done()
			user, err := repo.acceptInvitation(ctx, concurrentInvite, verifiedGoogleIdentity{sub: sub, email: "concurrent@example.com", emailVerified: true})
			results <- admissionResult{user: user, err: err}
		}(sub)
	}
	group.Wait()
	close(results)
	successes, unavailable := 0, 0
	for result := range results {
		switch {
		case result.err == nil:
			if result.user.IsOwnerAdmin {
				t.Fatal("ordinary invited user gained administrator privileges")
			}
			successes++
		case errors.Is(result.err, ErrInvitationUnavailable):
			unavailable++
		default:
			t.Fatalf("unexpected concurrent admission error: %v", result.err)
		}
	}
	if successes != 1 || unavailable != 1 {
		t.Fatalf("concurrent admission: success=%d unavailable=%d", successes, unavailable)
	}
	var adminCount int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM lifeos_users WHERE is_owner_admin").Scan(&adminCount); err != nil || adminCount != 1 {
		t.Fatalf("administrator count = %d, err=%v", adminCount, err)
	}

	// Database constraints reject impossible outcomes even if a future caller bypasses Go validation.
	badID, err := newUUID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO invitations (id, email, issued_by, expires_at, accepted_at)
		VALUES ($1, 'invalid@example.com', $2, now() + interval '1 day', now())`, badID, owner.ID); err == nil {
		t.Fatal("database accepted an invitation with accepted_at but no accepted_by")
	}
	badID, err = newUUID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO invitations (id, email, expires_at)
		VALUES ($1, 'no-issuer@example.com', now() + interval '1 day')`, badID); err == nil {
		t.Fatal("database accepted a regular invitation without an issuer")
	}
	badID, err = newUUID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO lifeos_users (id, email, status)
		VALUES ($1, 'invalid-state@example.com', 'elevated')`, badID); err == nil {
		t.Fatal("database accepted an invalid user status")
	}
	badID, err = newUUID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO lifeos_users (id, email, is_owner_admin)
		VALUES ($1, 'second-admin@example.com', true)`, badID); err == nil {
		t.Fatal("database accepted a second owner administrator")
	}

	// A data-bearing identity schema cannot be dropped by the down migration.
	// This disposable database may become dirty; production recovery must be manual.
	if err := runner.Steps(-1); err == nil {
		t.Fatal("down migration removed identity tables with user data")
	}
	var survivingUsers int
	if err := pool.QueryRow(ctx, "SELECT count(*) FROM lifeos_users").Scan(&survivingUsers); err != nil || survivingUsers < 2 {
		t.Fatalf("down migration lost users: count=%d err=%v", survivingUsers, err)
	}
}
