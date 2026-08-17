# Architecture Decision Records

This directory contains normative Architecture Decision Records (ADRs) for important LifeOS domain and technical decisions. ADRs preserve why a decision was made, its consequences, and how the project can migrate or exit later.

The Master Project Blueprint v0.2 already establishes the initial decisions below as the accepted project baseline. Dedicated ADR files have not yet been authored in this documentation-foundation task; their recording is the next documentation step.

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

## Initial ADR Backlog

| ADR | Baseline decision | Record status |
| --- | --- | --- |
| ADR-001 | Server-first, client-agnostic core; V0.x Web/PWA first | Baseline accepted; dedicated file pending |
| ADR-002 | Go modular monolith rather than microservices | Baseline accepted; dedicated file pending |
| ADR-003 | PostgreSQL with migrations; money stored as minor units plus currency | Baseline accepted; dedicated file pending |
| ADR-004 | `FinancialEvent` + `AccountMovement` + `CategoryAllocation` lightweight ledger | Baseline accepted; dedicated file pending |
| ADR-005 | AI is optional; Tool Gateway; no raw database access | Baseline accepted; dedicated file pending |
| ADR-006 | AI provider abstraction; Ollama is the first experimental provider; cloud providers optional | Baseline accepted; dedicated file pending |
| ADR-007 | MCP is an external adapter, not a core dependency | Baseline accepted; dedicated file pending |
| ADR-008 | V0.x is single-user, but the schema is owner-scoped from day one | Baseline accepted; dedicated file pending |
| ADR-009 | Import/export is first-class; Bluecoins is the first importer | Baseline accepted; dedicated file pending |
| ADR-010 | Open-source license will be decided separately before public release | Decision to defer accepted; dedicated file pending |

## Naming

Use zero-padded files with a concise slug:

```text
0001-server-first.md
0002-modular-monolith.md
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
