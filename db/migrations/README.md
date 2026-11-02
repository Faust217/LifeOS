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

The initial Foundation baseline intentionally creates no LifeOS domain table. Migration 000004 introduces the first owner-scoped Finance tables. Its down migration refuses to remove populated financial history; use a reviewed forward correction or verified backup instead of dropping data.

Migration 000007 adds a nullable owner IANA timezone without rewriting UTC financial events. Its down migration refuses to discard any saved timezone preference.

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

## Migration-Chain Verification

After confirming a disposable PostgreSQL database is available and setting the normal `LIFEOS_DB_*` process values, run the same verification used by CI:

```text
pwsh -File scripts/verify-migrations.ps1
```

The script rejects missing production migrations, invalid or duplicate six-digit versions, and missing paired down files. It applies the complete production chain, checks the latest clean version, proves repeated `up` is a no-op, and exercises the isolated smoke fixture through up/version/down.

GitHub Actions runs this script against a clean pinned PostgreSQL 18 service. Its credential is an isolated CI-only value and is never printed by the script. The workflow does not access a development or production database.


## v9 financial write request markers

Issue #34 / accepted ADR-019 adds only `finance_write_requests`, with owner/key uniqueness and a versioned 32-byte fingerprint. Upgrade v8→v9 does not rewrite any ledger, anchor, Goal, preference or export. Empty-table v9 down is supported; populated v9 down explicitly refuses to lose successful-key protection. Do not clear markers to roll back, edit migrations v1–v8, or run an old unprotected writer against populated v9. Use reviewed forward corrections and consistent backups. Personal database access/upgrade still requires separate authorization and restore verification.

## v10 FinancialEvent revisions

Issue #35 / accepted ADR-020 adds positive event revisions, one append-only snapshot table and the `event.edit` request-marker operation. A single transaction backfills every existing v9 event, including Voided/Soft Deleted, as a truthful `legacy_baseline` captured at upgrade with complete child IDs/values. It does not invent earlier audit states or alter balances/anchors. Current-row uniqueness constraints remain appropriate because audit copies are outside the ledger tables. Populated down refuses to discard events/audit or v2 retry evidence; use a reviewed forward correction. Preserve published v1–v9. Drain old writers and coordinate client/server activation before any approved upgrade; v9 binaries must not write to v10. Personal upgrades and deployment remain separately gated.
