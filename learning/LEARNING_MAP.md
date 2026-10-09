# LifeOS Learning Map

**Last updated:** 2026-10-09

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
| Database migrations | Forward migrations and upgrade tests | Applied | Foundation #8/#12 used pinned `golang-migrate` v4 and local chain checks; next: first user-data migration and upgrade test |
| Money and currency | Minor units, sign conventions, transfers, credit cards, FX boundaries | Exploring | Domain baseline documented; verify with unit and integration tests |
| REST and OpenAPI | Stable DTOs, validation, errors, generated-client path | Applied | Foundation operational endpoints and generated types exist; next: first protected product contract |
| React/TypeScript/PWA | Replaceable responsive client | Exploring | Foundation shell exists; next: mobile-first Alpha workflows |
| Import pipelines | Adapters, canonical input, dry-run, mapping, idempotency, error reports | Not assessed | Bluecoins import is V0.3 or later; preserve ADR-009 safety rules |
| Testing strategy | Unit, PostgreSQL integration, contract, import fixture, and E2E tests | Not assessed | First clean migration and FinanceService integration test |
| Backup and restore | PostgreSQL dump, restore, and upgrade safety | Not assessed | Restore rehearsal before invited Alpha users rely on data |
| Security and privacy | Invitation gate, Google identity, sessions, owner isolation, safe logging | Exploring | Issue #16 direction and ADR-014 accepted; cross-user tests needed before Alpha |
| AI Tool Gateway | Provider abstraction, schemas, permission tiers, confirmation, audit | Not assessed | Deferred beyond initial Alpha; preserve optional boundary |
| MCP integration | External adapter over shared tools | Not assessed | Deferred; preserve boundary only |
| Docker and self-host operations | Repeatable local and later hosted testing | Applied | Foundation Compose stack verified locally; private Alpha deployment needs separate review |
| ADR and change management | Record context, alternatives, consequences, and exit paths | Applied | ADR-001 through ADR-012 recorded and accepted; ADR-011 and ADR-012 document Foundation alternatives, trade-offs, and exit paths |

## Recommended Reading Path

For the current documentation rebaseline:

1. `README.md`
2. `docs/VISION.md`
3. `docs/DOMAIN_MODEL.md`
4. `docs/ARCHITECTURE.md`
5. `docs/V0.1_SCOPE.md`
6. `docs/ROADMAP.md`
7. `docs/adr/README.md`
8. `learning/FOUNDATION_DECISIONS.md`
9. `docs/PROJECT_STATE.md`
10. `AGENTS.md`

For a future implementation task, add the relevant ADR, service, migration, query, API, test, and client files to that task's journal entry in the order that best explains the behavior. Include the Function-level Data Flow required by `AGENTS.md`.

## Near-Term Learning Sequence

1. Read accepted ADR-013 and ADR-014 before separately authorized dependent implementation.
2. Inspect the current local environment without modifying system configuration.
3. Implement invitation-gated identity and user isolation with negative security tests.
4. Add user-data migrations and test clean creation and upgrades.
5. Implement Money services and transfer invariants with deterministic tests.
6. Add protected OpenAPI contracts and mobile-first Today/Money/Plans/Me slices.
7. Verify user export and backup/restore before Alpha users rely on real data.
8. Design offline synchronization through an accepted ADR before V0.2 implementation; study Bluecoins import for V0.3 or later.

## Open Questions to Resolve Through Work

- Which invitation provisioning, session storage, and owner-admin bootstrapping details should implement accepted ADR-014?
- What protected route groups and middleware order should the first product `chi` API slice use?
- What fields and edge cases actually appear in the owner's Bluecoins export?
- Which account and category reconciliation reports best prove import correctness?
- Which offline failure and conflict cases need the V0.2 sync ADR after the transaction API exists?

Answers that change architecture or domain rules must be recorded in ADRs, not only here.
