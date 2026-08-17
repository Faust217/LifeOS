# ADR-003: PostgreSQL, Forward Migrations, and Integer Money

**Status:** Accepted

**Date:** 2026-08-17

## Context

LifeOS must preserve years of personal finance and life data across upgrades. Its data contains relationships, owner boundaries, controlled states, transfers, credit-card liabilities, imports, and multiple currencies. Financial correctness also requires an exact representation of money.

Resetting a database during normal upgrades would destroy the product's long-term value. Floating-point numbers can introduce rounding errors and do not carry currency meaning.

## Decision

LifeOS will use PostgreSQL as its authoritative database and will evolve persistent data through numbered forward migrations.

- Every schema change requires a new migration.
- A migration must not be rewritten or deleted after it has shipped; corrections use a later migration.
- CI will create an empty database from the full migration chain and test supported upgrade paths.
- Breaking data changes follow an expand, migrate-data, then contract sequence.
- Real/personal data is never upgraded by dropping and recreating the database.
- Monetary values are stored as integer minor units plus ISO 4217 currency, never as float or double.
- Cross-currency reports must state the conversion value and date used; currencies are never silently combined.
- The exact migration tool remains deferred until before the first real migration.

## Alternatives Considered

### SQLite as the main database

SQLite could simplify a single-device installation, but PostgreSQL better matches the accepted relational, self-hosted, concurrent-client, constraint, and migration baseline.

### Reset-and-reimport upgrades

Recreate the schema and import history again after significant changes. This is acceptable only for disposable developer databases, not for real user data.

### Floating-point money

Store amounts as `float` or `double`. This is convenient but risks rounding errors and does not enforce currency-aware behavior.

### Decimal money without minor-unit semantics

Exact decimals avoid binary floating-point errors, but the accepted baseline uses integer minor units with an explicit currency for predictable storage and arithmetic.

## Consequences

### Positive

- Relational constraints and transactions protect domain relationships.
- Upgrade history is explicit, testable, and repeatable.
- Money arithmetic is deterministic at the stored currency's minor-unit precision.
- Backup, restore, self-hosting, and data reconciliation have a mature database foundation.

### Costs and Trade-Offs

- PostgreSQL adds a service to local development and deployment.
- Migrations and upgrade tests require ongoing discipline.
- Currency-specific minor units and conversion rules must be handled explicitly.
- Changing historical data shapes may require multi-step migrations.

## Data Migration Impact

All future persistent models must follow this decision. Imports, including Bluecoins data, must convert money into exact minor units and preserve currency. Existing data changes must be expressed as forward migrations and backed up before risky upgrades.

The choice between `golang-migrate` and `goose` is not made here and requires a later focused ADR.

## Security and AI Impact

Database credentials remain server secrets. They must never be sent to clients, models, or external tools. Every query must be owner-scoped through repositories/application services.

AI does not perform authoritative money arithmetic or currency aggregation; deterministic Go/SQL code does.

## Rollback or Exit Path

Migration rollback must be planned per change, with backup/restore as the safety boundary for destructive or irreversible transformations. Moving away from PostgreSQL requires a superseding ADR, a complete structured export/import path, reconciliation checks, and proof that identifiers, relationships, currency values, and audit history survive exactly.
