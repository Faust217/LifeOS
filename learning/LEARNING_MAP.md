# LifeOS Learning Map

**Last updated:** 2026-08-17

**Purpose:** Track engineering topics exposed by real LifeOS work without inventing proficiency claims.

## How to Use This Map

Update this file after meaningful implementation or study. Use evidence such as a completed feature, test, migration, review, or explanation. Do not treat planned work as mastered knowledge.

Suggested status values:

- **Not assessed** - no evidence recorded yet.
- **Exploring** - learning through a current task.
- **Applied** - used successfully in LifeOS with verification.
- **Can explain** - can describe the trade-offs and review another implementation.

## Current Map

| Area | LifeOS application | Status | Evidence / next checkpoint |
| --- | --- | --- | --- |
| Product and domain modeling | Preserve the difference between actual money, plans, wishlist, assets, and splits | Exploring | Documentation baseline extracted; validate during first finance implementation |
| Go project structure | Modular monolith, module-owned services, thin adapters | Applied | Issue #2 root Go module and 12 internal module packages pass package discovery and build checks; see `journal/2026-08-17-monorepo-skeleton.md` |
| Go domain/application services | Business invariants and deterministic calculations | Not assessed | First Account and FinancialEvent use cases |
| PostgreSQL relational modeling | Owner scoping, constraints, relations, indexes | Not assessed | First schema design and review |
| SQL with `pgx`/`sqlc` | Explicit queries and typed access | Not assessed | First account and financial-event queries |
| Database migrations | Forward migrations and upgrade tests | Not assessed | Choose migration tool, record ADR, run clean and upgrade paths |
| Money and currency | Minor units, sign conventions, transfers, credit cards, FX boundaries | Exploring | Domain baseline documented; verify with unit and integration tests |
| REST and OpenAPI | Stable DTOs, validation, errors, generated-client path | Not assessed | FND-005 and first finance endpoints |
| React/TypeScript/PWA | Replaceable responsive client | Not assessed | First production UI slice |
| Import pipelines | Adapters, canonical input, dry-run, mapping, idempotency, error reports | Not assessed | Anonymized Bluecoins field analysis and fixture |
| Testing strategy | Unit, PostgreSQL integration, contract, import fixture, and E2E tests | Not assessed | First clean migration and FinanceService integration test |
| Backup and restore | PostgreSQL dump, restore, and upgrade safety | Not assessed | Restore rehearsal before first real import |
| Security and privacy | Owner isolation, secrets, authentication, safe logging | Exploring | Architecture baseline documented; test owner isolation in first API slice |
| AI Tool Gateway | Provider abstraction, schemas, permission tiers, confirmation, audit | Not assessed | V0.3; no runtime implementation in V0.1 |
| MCP integration | External adapter over shared tools | Not assessed | V0.6; preserve boundary only |
| Docker and self-host operations | Repeatable local and later NAS/VPS deployment | Not assessed | FND-002, then V0.7 hardening |
| ADR and change management | Record context, alternatives, consequences, and exit paths | Applied | ADR-001 through ADR-010 recorded and accepted; apply the ADR gate to future decision changes |

## Recommended Reading Path

For the current foundation:

1. `README.md`
2. `docs/VISION.md`
3. `docs/DOMAIN_MODEL.md`
4. `docs/ARCHITECTURE.md`
5. `docs/V0.1_SCOPE.md`
6. `docs/ROADMAP.md`
7. `docs/adr/README.md`
8. `docs/PROJECT_STATE.md`
9. `AGENTS.md`

For a future implementation task, add the relevant ADR, service, migration, query, API, test, and client files to that task's journal entry in the order that best explains the behavior.

## Near-Term Learning Sequence

1. Use the accepted ADR set as the decision gate for Foundation and V0.1 work.
2. Inspect the existing Go, Docker, Git, and database environment without modifying system configuration.
3. Learn the shape and edge cases of an anonymized Bluecoins export.
4. Establish one migration and prove both clean creation and upgrade behavior.
5. Implement Account and money primitives with deterministic tests.
6. Implement FinancialEvent movements and category allocations against real PostgreSQL.
7. Add OpenAPI contracts and a thin vertical UI slice.
8. Prove import dry-run, reconciliation, export, backup, and restore before relying on real data.

## Open Questions to Resolve Through Work

- Which migration tool best fits the repository workflow: `golang-migrate` or `goose`?
- Which Go HTTP router keeps handlers simple without unnecessary framework behavior?
- What fields and edge cases actually appear in the owner's Bluecoins export?
- Which account and category reconciliation reports best prove import correctness?
- What authentication boundary is sufficient for local-only use before remote access is introduced?

Answers that change architecture or domain rules must be recorded in ADRs, not only here.
