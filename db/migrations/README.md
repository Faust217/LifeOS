# Database Migrations

This directory is the immutable production migration chain for LifeOS. The project uses `golang-migrate` v4.20.1, pinned in `go.mod`, through the repository-local Go command in `cmd/lifeos-migrate`.

Run commands from the repository root after PostgreSQL is healthy and the ignored root `.env` contains the local database password:

```text
go run ./cmd/lifeos-migrate up
go run ./cmd/lifeos-migrate version
go run ./cmd/lifeos-migrate down
```

`up` applies pending files in version order and exits successfully with `no change` when the database is already current. `version` reports the current version and dirty flag. `down` rolls back exactly one version and must be used only after reviewing that migration's data impact.

## Migration File Policy

- Use monotonically increasing six-digit versions and paired names such as `000002_description.up.sql` and `000002_description.down.sql`.
- Prefer explicit PostgreSQL transactions when every operation in the migration supports them.
- Keep each migration focused. Isolate operations that cannot run in a transaction and document their backup and recovery needs.
- Once a migration has shipped, never rewrite, renumber, or delete it. Correct it with a later migration.
- Production migrations belong only in this directory. Test-only fixtures belong under `internal/platform/migrations/testdata` and must use isolated metadata tables.
- Never use database reset or re-import as the normal upgrade path.

The initial Foundation baseline intentionally creates no LifeOS domain table. It establishes a real, testable production chain while Finance schema remains out of scope.

## Failure and Dirty-State Recovery

Before a destructive, long-running, or otherwise risky migration, create and verify a PostgreSQL backup. A safe down migration is not a substitute for a backup when data could be lost.

If a migration fails or `version` reports `dirty=true`:

1. Stop migration and application rollout attempts.
2. Record the failed version and error without exposing the database URL or password.
3. Inspect the actual database state against the unchanged migration SQL and the latest verified backup.
4. Restore the backup or apply reviewed repair SQL as appropriate. A shipped file remains immutable; lasting corrections use a later forward migration.
5. Only an authorized operator may use the pinned tool's `force` operation after the schema is reconciled. `force` changes version metadata; it does not repair the database.
6. Re-run `up`, then verify version, readiness, and application checks.

Do not automate `force`, unrestricted rollback, `drop`, or manual schema surgery. Database credentials are runtime secrets and must not be printed, committed, or sent to clients or AI systems.
