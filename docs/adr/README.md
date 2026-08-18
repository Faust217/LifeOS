# Architecture Decision Records

This directory contains normative Architecture Decision Records (ADRs) for important LifeOS domain and technical decisions. ADRs preserve why a decision was made, its consequences, and how the project can migrate or exit later.

The Master Project Blueprint v0.2 established the initial decisions below as the project baseline. The Product Owner reviewed and accepted the documentation baseline, and the dedicated ADR files were recorded on 2026-08-17.

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

| ADR | Baseline decision | Status |
| --- | --- | --- |
| [ADR-001](ADR-001-server-first.md) | Server-first, client-agnostic core; V0.x Web/PWA first | Accepted |
| [ADR-002](ADR-002-modular-monolith.md) | Go modular monolith rather than microservices | Accepted |
| [ADR-003](ADR-003-postgresql-migrations-and-money.md) | PostgreSQL with migrations; money stored as minor units plus currency | Accepted |
| [ADR-004](ADR-004-lightweight-ledger.md) | `FinancialEvent` + `AccountMovement` + `CategoryAllocation` lightweight ledger | Accepted |
| [ADR-005](ADR-005-ai-optional-tool-gateway.md) | AI is optional; Tool Gateway; no raw database access | Accepted |
| [ADR-006](ADR-006-ai-provider-abstraction.md) | AI provider abstraction; Ollama is the first experimental provider; cloud providers optional | Accepted |
| [ADR-007](ADR-007-mcp-external-adapter.md) | MCP is an external adapter, not a core dependency | Accepted |
| [ADR-008](ADR-008-owner-scoped-single-user.md) | V0.x is single-user, but the schema is owner-scoped from day one | Accepted |
| [ADR-009](ADR-009-import-export-bluecoins.md) | Import/export is first-class; Bluecoins is the first importer | Accepted |
| [ADR-010](ADR-010-open-source-license-deferred.md) | Open-source license selection is deferred until before public release | Accepted deferral |

## Foundation ADRs

| ADR | Decision | Status |
| --- | --- | --- |
| [ADR-011](ADR-011-golang-migrate.md) | Use `golang-migrate` v4 through an explicitly pinned, project-local invocation path | Accepted; Issue #3 review pending |

## Naming

Use the `ADR-` prefix, a zero-padded three-digit number, and a concise slug:

```text
ADR-001-server-first.md
ADR-002-modular-monolith.md
```

Use status values `Proposed`, `Accepted`, or `Superseded`. Do not erase an accepted record when direction changes; add a new ADR and mark the old record as superseded.

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
