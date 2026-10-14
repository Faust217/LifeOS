# ADR-016: Owner-Scoped Soft Delete of Financial Events

**Status:** Accepted — Product Owner approved on 2026-10-14 for Issue #29

**Date:** 2026-10-13

## Draft revision history

The [initial, unaccepted ADR-016 draft](history/ADR-016-initial-physical-deletion-draft.md) proposed physical deletion of only already-voided events and their dependent rows. On 2026-10-13, the Product Owner clarified that **disappearance from the normal LifeOS experience**, rather than physical erasure, is the requirement, and that posted events should be deletable in one confirmed operation. This accepted Soft Delete decision replaced that draft. The historical draft is retained for review; it has never been an accepted decision.

## Context

Accepted ADR-004 defines the `FinancialEvent` / `AccountMovement` / `CategoryAllocation` lightweight ledger. Void is an audited correction that leaves an event visible but removes its balance and spending effects. The conceptual Domain Model already allows soft-delete behavior with audit metadata. The current v4 Finance schema has `posted`/`voided` status and no deletion marker; its restrictive foreign keys preserve linked ledger rows. Existing history reads all owner events, while balance and category-spending queries count posted events. Today consumes Money history and balances. A user also needs a distinct Delete action that removes a transaction from the ordinary application experience.

## Decision

1. Add a nullable UTC `deleted_at` marker to `financial_events` in the next numbered **forward** migration (currently version 6). `NULL` means visible; a timestamp means deleted. Keep the existing `posted`/`voided` status and Void metadata as a separate correction dimension. Preserve the event, movements, allocations, and any future approved references internally. Never edit the published v4 migration.
2. Permit an owner to delete either a posted or an already-voided income, expense, or transfer. One explicit, confirmed server-side operation locks the owned event and performs the transition in a database transaction. A posted event stops contributing to balances and spending in the same atomic commit that sets `deleted_at`. An already-voided event receives the marker without any second financial reversal. Direct deletion of a posted event does **not** fabricate a Void reason or mark it voided.
3. Define ordinary visible Finance data as owner-scoped events with `deleted_at IS NULL`. History, Today, balances, category spending, and later normal exports, charts, search, reports, and linked presentation must consistently apply this rule. Balance and spending calculations also retain their existing `status = 'posted'` rule. Transfers stay one event, with both account effects excluded together; no mixed-currency aggregation or client-page-derived totals.
4. Keep **Void** as the separate audited action: it leaves the original event visible as Voided and excludes its effects. Delete hides the event from ordinary application views while retaining its ledger records internally. Do not add a per-transaction tombstone or separate deletion receipt solely for this UX requirement; the marker is the retained deletion state.
5. Require a clear, accessible confirmation explaining that Delete removes the transaction from LifeOS views but does not necessarily erase internal storage, backups, or copies. Proposed wording: “Delete this transaction? It will disappear from LifeOS. If posted, it will stop affecting balances and spending. Its data may remain in internal records and backups. You cannot undo this in the app.” The normal UI provides no undelete action in Issue #29. Any exceptional recovery must be separately reviewed and owner-scoped.

## Transaction, concurrency, and API rules for Issue #29

- Derive owner identity from the verified session. Apply the existing Origin and CSRF mutation guards. Scope the event lock and all related reads/updates to that owner; foreign and unknown IDs must be indistinguishable.
- Serialize Delete against another Delete or Void with an event row lock. For a posted event, lock affected accounts in a stable order and validate resulting integer balances/overflow using the same Finance rules as Void before setting the marker. If any step fails, roll back without a partial financial or visibility change. A Void racing with Delete must resolve to one coherent final state: Delete may follow a completed Void, but Void must reject an event already marked deleted.
- Define repeat deletion of the same owned event as an idempotent success without another balance change; retain not-found behavior for unknown or foreign events. The HTTP contract should state the exact success/error responses before implementation. A repeated request still passes authentication, Origin, and CSRF checks.
- Filter deleted events **before** history `ORDER BY occurred_at DESC, id DESC`, `LIMIT`, and `OFFSET`; refresh affected UI pages after mutation. Existing offset pages can shift between requests when rows are inserted or deleted; do not promise a cross-request snapshot. Future reports/exports must query the complete server-side eligible set, not a paginated UI slice.
- Review any future `FinancialEvent` foreign-key references and dependent module semantics before implementation. Soft Delete preserves current linked rows, but linked records must not expose deleted transaction content through normal views or silently count its effects.

## Retention, backup, and recovery

The event and linked rows remain in the authoritative database. Delete is therefore **not physical erasure**. PostgreSQL storage/WAL, replicas, retained backups, exported files, and logs can also hold data. Application access/error logs should avoid transaction contents. A backup taken before a deletion can restore a state where `deleted_at` is null, so an old-backup restore can reintroduce the transaction into the UI. Document that limit and verify restore/reconciliation in the existing **Personal Data Ready** backup/restore gate before real-data use. Do not create a separate 45-day deletion inventory or special retention service solely for Issue #29. Do not claim a complete erasure lifecycle.

## Relationship to accepted decisions

ADR-004's ledger structure and deterministic calculations remain intact because the event and dependent rows are retained. ADR-003's numbered forward migrations and data-preserving upgrades, and ADR-014's server-derived owner isolation, remain authoritative. This decision specifies a soft-delete transition already contemplated in `docs/DOMAIN_MODEL.md`; it does **not** supersede an accepted ADR or authorize general physical deletion. ADR-015 remains a separate Proposed import-boundary decision.

## Alternatives considered

- **Physical deletion after Void:** the initial ADR-016 draft; it exceeds the clarified UX requirement and discards live ledger history.
- **Require Void before Delete:** adds an unwanted extra action and audit reason for posted transactions; one guarded atomic Delete can remove financial effects directly.
- **Reuse `status = 'voided'` as deletion:** cannot distinguish an ordinary visible Void from a hidden Delete and loses the original posted/voided state.
- **Hide only in React:** leaves other clients, exports, reports, and balance calculations inconsistent.
- **Separate tombstone or 45-day deletion inventory:** unnecessary for the present UX requirement; revisit only if a separately approved erasure or recovery policy needs it.

## Consequences and migration impact

Soft Delete preserves internal ledger evidence and referential integrity, while ordinary users no longer see deleted events. It requires a new nullable column, a numbered up/down migration and upgrade-path verification, coordinated predicates in every existing Finance read/aggregate, guarded API and UI changes, OpenAPI/generated contract updates, and real PostgreSQL tests for posted/voided, transfer, Uncategorized, concurrency, idempotency, owner isolation, CSRF, pagination, and rollback. The down migration should refuse to drop the marker while any deleted rows exist, because doing so would make them visible again. It is **not** an ordinary safe production rollback. Backups and forward correction are the recovery boundary.

## Security and AI impact

Delete is a sensitive mutation and must use existing session, Origin, CSRF, owner-scope, and privacy-safe logging rules. No AI/MCP deletion capability is authorized by this ADR or Issue #29.

## Implementation boundary

The Product Owner accepted this ADR and authorized local Issue #29 implementation on 2026-10-14, then approved the verified implementation for commit, push, and Issue #29 closure after visual review. Personal-database access or migration, deployment, and Issue #30 work remain outside that authorization.
