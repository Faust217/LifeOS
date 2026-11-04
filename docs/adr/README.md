# Architecture Decision Records

This directory contains normative Architecture Decision Records (ADRs) for important LifeOS domain and technical decisions. ADRs preserve why a decision was made, its consequences, and how the project can migrate or exit later.

The Master Project Blueprint v0.2 established the initial decisions below as the project baseline. The Product Owner reviewed and accepted the documentation baseline, and the dedicated ADR files were recorded on 2026-08-17.

The Product Owner approved the new product direction in [Issue #16](https://github.com/Faust217/LifeOS/issues/16) and accepted ADR-013 and ADR-014 during Issue #17. Historical ADR-008 and ADR-009 files remain unchanged. Apply their still-valid principles together with the precise supersession boundaries below.

## When an ADR Is Required

Create or update the ADR set before implementing a change that affects:

- a non-negotiable product or architecture principle;
- domain meaning or a cross-domain relationship;
- module ownership or allowed dependencies;
- persistent data shape, migration strategy, or compatibility policy;
- public API or AI tool contract strategy;
- authentication, privacy, secrets, or AI permission policy;
- core runtime, database, deployment, or licensing direction;
- a previously accepted decision.

Small implementation details that do not change these boundaries do not require an ADR.

## Initial ADR Set

| ADR | Baseline decision | Status and current applicability |
| --- | --- | --- |
| [ADR-001](ADR-001-server-first.md) | Server-first, client-agnostic core; V0.x Web/PWA first | Accepted |
| [ADR-002](ADR-002-modular-monolith.md) | Go modular monolith rather than microservices | Accepted |
| [ADR-003](ADR-003-postgresql-migrations-and-money.md) | PostgreSQL with migrations; money stored as minor units plus currency | Accepted |
| [ADR-004](ADR-004-lightweight-ledger.md) | `FinancialEvent` + `AccountMovement` + `CategoryAllocation` lightweight ledger | Accepted |
| [ADR-005](ADR-005-ai-optional-tool-gateway.md) | AI is optional; Tool Gateway; no raw database access | Accepted |
| [ADR-006](ADR-006-ai-provider-abstraction.md) | AI provider abstraction; Ollama is the first experimental provider; cloud providers optional | Accepted |
| [ADR-007](ADR-007-mcp-external-adapter.md) | MCP is an external adapter, not a core dependency | Accepted |
| [ADR-008](ADR-008-owner-scoped-single-user.md) | Historical single-user V0.x direction and owner-scoped data | Accepted in part: single-user V0.x scope superseded by ADR-013; owner scoping and no client-selected owner remain in force |
| [ADR-009](ADR-009-import-export-bluecoins.md) | First-class import/export with Bluecoins as first source adapter | Accepted in part: Bluecoins-in-V0.1 timing superseded by ADR-013; first-class export and safe import/Bluecoins-first-adapter principles remain in force |
| [ADR-010](ADR-010-open-source-license-deferred.md) | Open-source license selection is deferred until before public release | Accepted deferral |

## Foundation ADRs

| ADR | Decision | Status |
| --- | --- | --- |
| [ADR-011](ADR-011-golang-migrate.md) | Use `golang-migrate` v4 through an explicitly pinned, project-local invocation path | Accepted |
| [ADR-012](ADR-012-chi.md) | Use `chi` v5 as a transport-only router with thin standard `net/http` handlers | Accepted |

## Accepted Rebaseline ADRs

| ADR | Decision | Status |
| --- | --- | --- |
| [ADR-013](ADR-013-life-first-multi-user-rebaseline.md) | Life-first, invite-only multi-user V0.1; explicit ADR-008 single-user and ADR-009 timing supersession | Accepted |
| [ADR-014](ADR-014-google-identity-invitations-owner-isolation.md) | Google identity plus invitation-gated access, revocable sessions, owner isolation, and owner-only admin boundary | Accepted |

Offline transaction synchronization is V0.2 work. It needs a separate **accepted** ADR before implementation. Issue #17 records that gate without choosing a protocol prematurely.

## Proposed decisions

| ADR | Proposed change | Status |
| --- | --- | --- |
| [ADR-015](ADR-015-personal-import-boundary.md) | Remove the Bluecoins-first product adapter/milestone commitment while retaining safe import and export rules; allow separately scoped personal conversion | Proposed; accepted ADR-009/013 portions remain authoritative until Product Owner approval |

## Accepted Finance correction decision

| ADR | Decision | Status |
| --- | --- | --- |
| [ADR-016](ADR-016-financial-event-soft-delete.md) | Owner-scoped Soft Delete for posted or voided Finance events, preserving linked ledger rows and excluding deleted events from normal views and calculations | Accepted 2026-10-14; [initial physical-deletion draft](history/ADR-016-initial-physical-deletion-draft.md) was never accepted; ADR-004 remains authoritative |

## Accepted export decision

| ADR | Decision | Status |
| --- | --- | --- |
| [ADR-017](ADR-017-owner-data-export.md) | Versioned owner JSON/Finance CSV, shared read snapshot and bounded guarded downloads | Accepted 2026-10-20 for local [Issue #32](https://github.com/Faust217/LifeOS/issues/32) implementation; Product Owner visual/export-file review passed and final delivery authorized |

## Accepted opening-balance decision

| ADR | Decision | Status |
| --- | --- | --- |
| [ADR-018](ADR-018-opening-balance-effective-instant.md) | Known-balance instant, explicit legacy conversion and versioned export compatibility | Accepted 2026-10-20; #33 visual/export review passed and final delivery authorized 2026-10-23 |

## Accepted financial-write retry decision

| ADR | Decision | Status |
| --- | --- | --- |
| [ADR-019](ADR-019-owner-scoped-financial-write-idempotency.md) | Owner-scoped request keys, atomic PostgreSQL success markers, content-free replay, capacity warning and frozen draft recovery | Accepted 2026-10-26; Issue #34 delivered and closed at `884b013d98d65f52d8f0e2dab424cfb03916539f`; deployment remains separate |

## Accepted atomic editing decision

| ADR | Decision | Status |
| --- | --- | --- |
| [ADR-020](ADR-020-atomic-financial-event-edit-revisions.md) | Stable-ID atomic Edit, current-only ledger, immutable internal snapshots and revision-guarded idempotent writes | Accepted 2026-10-27; Issue #35 final UI/financial evidence accepted and delivery authorized 2026-11-02 |

## Accepted Goal deletion and narrow export exception

| ADR | Decision | Status |
| --- | --- | --- |
| [ADR-021](ADR-021-goal-soft-delete.md) | Owner-scoped Goal Soft Delete; normal JSON v1/v2 excludes deleted Goals/checklists as an explicit exception to ADR-017 for this new state only | Accepted 2026-11-04; Issue #38 implemented, successful deletion manually verified by the Product Owner, final delivery approved; full responsive/focus/mobile coverage remains incomplete |

ADR-017's historical file remains unchanged. Its other versioning/visibility requirements stay in force; this exception does not authorize future unversioned changes. ADR-015 remains Proposed.

## Naming

Use the `ADR-` prefix, a zero-padded three-digit number, and a concise slug:

```text
ADR-001-server-first.md
ADR-002-modular-monolith.md
```

Use status values `Proposed`, `Accepted`, or `Superseded`. Do not erase an accepted record when direction changes. If a new ADR supersedes only part of an older decision, keep the older file and identify both the superseded and retained portions in this index.

## Template

```markdown
# ADR-NNN: Title

Status: Proposed | Accepted | Superseded
Date: YYYY-MM-DD

## Context

## Decision

## Alternatives Considered

## Consequences

## Data Migration Impact

## Security and AI Impact

## Rollback or Exit Path
```

Learning notes and chat history may provide context, but they are not substitutes for an ADR.
