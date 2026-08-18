# ADR-011: golang-migrate for Database Migrations

**Status:** Accepted

**Date:** 2026-08-18

## Context

ADR-003 requires PostgreSQL, numbered forward migrations, tested upgrade paths, and preservation of real data. It also requires shipped migrations to remain immutable: a released migration is never rewritten or deleted, and a correction is made through a later migration.

LifeOS now needs one migration runner before Foundation Issues #8 and #12 can implement project-local migration commands and migration-chain verification. The runner must fit a Go modular monolith, work with PostgreSQL, operate predictably in local and CI environments, expose the current version, support deliberate rollback where safe, and avoid requiring a global machine installation.

The Product Owner evaluated `golang-migrate` and `goose` and selected `golang-migrate`. This ADR records that accepted tool decision; it does not install or execute the tool and does not authorize any schema or migration implementation.

In beginner-friendly terms, a migration is a numbered instruction for changing the database. The runner remembers which instruction was last applied, executes later instructions in order, and stops when one fails. `golang-migrate` matches LifeOS's preference for a small, explicit, ordered mechanism rather than a broader migration framework.

## Decision

LifeOS will use the supported v4 line of [`golang-migrate`](https://github.com/golang-migrate/migrate) as its database migration runner.

### Migration Chain

- Migrations use a simple, monotonically increasing sequential version order.
- The primary upgrade path is forward: apply pending `up` migrations in order.
- Migration files will use the runner's explicit up/down filename convention when implementation is authorized. A down migration is included only when it is safe and meaningful; backup/restore or a later corrective forward migration remains the safety boundary for destructive or irreversible changes.
- Shipped migrations are never rewritten, renumbered, or deleted. Corrections use a later numbered migration, consistent with ADR-003.
- Out-of-order application, reset-and-reimport, and drop/recreate are not normal upgrade strategies.

### Transactions

Transactional migrations remain desirable because PostgreSQL can roll back many schema changes when an error occurs.

- Migration authors should explicitly wrap compatible SQL work in a transaction when the PostgreSQL operations support it; the project must not assume the runner automatically makes every migration atomic.
- Operations that PostgreSQL does not allow inside a transaction must be isolated in a focused migration, documented, backed up where risk requires it, and tested through the upgrade path.
- A migration must not mix unrelated transactional and non-transactional work merely for convenience.

### Project-Local, Pinned Invocation

- LifeOS will not depend on a globally installed `migrate` command or an incidental executable found on `PATH`.
- Foundation Issue #8 must provide a repository-local wrapper or command that selects an explicit reviewed v4 release. It must never use an unpinned `latest` reference.
- The exact version is release/tooling metadata and may be updated deliberately after review; changing it must run the migration-chain checks before adoption.
- Where Docker execution is appropriate, the wrapper will use the official image path `migrate/migrate:<pinned-version>` and mount the repository migration directory read-only. Network and secret wiring belong to the later authorized implementation issue.
- A non-Docker path may use a pinned binary resolved into an ignored project-local tool location. It must not require a global install or persistent PATH change.
- Both paths must expose the selected tool version in diagnostics so local and CI runs can be reproduced.

The intended Docker shape is:

```text
docker run --rm \
  --network <approved-project-network> \
  -v <repository-migration-directory>:/migrations:ro \
  migrate/migrate:<pinned-version> \
  -path=/migrations -database <runtime-database-url> up
```

This is an invocation contract, not Docker configuration. No image is pulled and no container is created by this ADR.

### Dirty State and Manual Recovery

`golang-migrate` marks a target version dirty before running it. If the migration fails, later migration commands stop and the dirty state remains. This is a deliberate fail-closed signal that the database may have been changed only partially.

Recovery is explicitly manual and evidence-led:

1. Stop automated migration attempts. Automation must not automatically clear or force a dirty version.
2. Record the failing version and error, inspect the actual database state, and compare it with the migration SQL and the latest verified backup.
3. Determine whether the migration applied none, some, or all of its intended changes.
4. Repair or reverse partial effects through reviewed SQL or restore from backup as appropriate. For a shipped migration, keep the original file unchanged and create a later corrective migration when required.
5. Only after the database state is reconciled may an authorized operator run the pinned tool's `force <version>` command to set the version metadata and clear the dirty flag. `force` does not repair schema changes and must never be treated as the repair itself.
6. Re-run the intended forward path and verify version, schema state, and application/integration checks.

Dirty-state recovery, `force`, unrestricted `down`, and `drop` are privileged operational actions. Future wrappers must keep them explicit and must not hide them behind an automatic startup path.

## Alternatives Considered

### goose

[`goose`](https://github.com/pressly/goose) is capable and well maintained. It supports PostgreSQL, SQL and Go migrations, embedded migrations, transaction-by-default SQL execution, status/version commands, and sequential numbering.

It was not selected because LifeOS does not currently need its broader feature set: Go-function migrations, embedded migration files, seeding helpers, environment substitution, out-of-order options, or hybrid timestamp/sequential workflows. Those capabilities are useful in other projects, but they create additional policy choices that do not improve LifeOS's intentionally simple ordered forward chain.

`goose`'s default transaction behavior is an advantage, not a defect. LifeOS preserves the same safety goal by requiring explicit transactions where PostgreSQL supports them. The Product Owner preferred `golang-migrate`'s smaller file-runner model, official Docker image, explicit version/dirty-state behavior, and straightforward fit with an external project-local command.

Rejecting `goose` does not mean it is unsafe or unsuitable in general. It means `golang-migrate` better matches the accepted LifeOS workflow.

### Custom migration runner

A custom SQL runner could enforce exactly the desired conventions, but it would require LifeOS to implement version storage, ordering, locking, error handling, dirty-state behavior, and operational tooling. That adds risk without a demonstrated product benefit.

### Manual SQL or reset-and-reimport

Applying SQL by hand or recreating the database would make upgrades difficult to reproduce and audit. Reset-and-reimport directly conflicts with ADR-003 and is unacceptable for real personal data.

## Consequences

### Positive

- PostgreSQL is directly supported, including database-level locking for concurrent migration attempts.
- Ordered file migrations make the schema history easy to inspect and reproduce.
- Version and dirty-state reporting make failed upgrades visible instead of silently continuing.
- The same pinned runner can be used locally and later in CI without a system-wide install.
- The official Docker image provides a consistent execution path while a pinned local binary remains possible when Docker is inappropriate.
- The choice stays separate from application startup and domain behavior.

### Costs and Trade-Offs

- PostgreSQL migration files are not automatically transactional merely because the runner is used; authors must make transaction boundaries explicit where supported.
- A failed non-transactional or partially applied migration can require careful manual recovery before `force` is safe.
- The project must maintain a reviewed tool pin and deliberately upgrade it.
- Down migrations cannot guarantee safe rollback for data-destructive changes; backup/restore and corrective forward migrations remain necessary.
- Wrapper commands, secret handling, migration files, and CI verification still require separately authorized implementation work.

## Data Migration Impact

This ADR creates no database, schema, table, owner record, or migration file, and it changes no existing data.

When first executed in a later issue, `golang-migrate` will maintain its own schema-version metadata, including the current version and dirty state. That support metadata is operational state, not a LifeOS domain record.

Every future schema change must remain a new numbered migration. Clean-database creation, supported-version upgrades, dirty-state behavior, and backup/restore boundaries must be tested before real data relies on the chain. Breaking changes continue to use expand, migrate data, then contract.

## Security and AI Impact

- Database URLs and credentials are runtime secrets. They must not be committed, embedded in an image reference, printed in logs, returned to clients, or sent to an AI model.
- Migration execution requires only the database authority necessary for the reviewed schema change and should not use broader credentials than required by the deployment design.
- The Docker path must receive secrets through an approved runtime mechanism; migration files may be mounted read-only.
- `force`, broad `down`, and `drop` can misrepresent or destroy database state. They require explicit authorized human action and must not be exposed as ordinary application, AI, or MCP tools.
- AI may help review migration SQL, but it cannot authorize execution, clear dirty state, choose a force version, or bypass backup and verification requirements.

This tool choice does not change owner scoping, AI permissions, or the rule that models never receive database credentials or unrestricted SQL access.

## Rollback or Exit Path

The normal rollback strategy is migration-specific:

- use a reviewed down migration only when it is demonstrably safe;
- otherwise restore a verified backup or apply a later corrective forward migration;
- never rewrite a shipped migration to make history appear clean.

If LifeOS later replaces `golang-migrate`, a superseding ADR must explain why, map the existing version and dirty-state metadata to the new tool, preserve the ordered migration history, and prove both clean creation and supported upgrades against representative backups. The transition must not reset real data or require re-importing history.

Because migrations are explicit SQL files rather than tool-specific Go functions, another runner can be adopted later if it can preserve version identity, ordering, transaction intent, and upgrade behavior.

## References

- [`golang-migrate` project and official Docker usage](https://github.com/golang-migrate/migrate)
- [`golang-migrate` dirty-state FAQ](https://github.com/golang-migrate/migrate/blob/master/FAQ.md)
- [`golang-migrate` force/recovery guidance](https://github.com/golang-migrate/migrate/blob/master/GETTING_STARTED.md)
- [`goose` project and migration behavior](https://github.com/pressly/goose)
