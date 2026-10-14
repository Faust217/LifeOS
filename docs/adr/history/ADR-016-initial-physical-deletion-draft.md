# Historical ADR-016 draft: Permanent Deletion of Voided Financial Events

**Historical status:** Superseded unaccepted draft. Retained to show the 2026-10-13 change in Product Owner direction. The active [ADR-016 Soft Delete decision](../ADR-016-financial-event-soft-delete.md) was accepted on 2026-10-14. This file does not authorize physical deletion.

**Original draft status:** Proposed — never accepted

**Date:** 2026-10-13

## Context

Accepted ADR-004 defines one `FinancialEvent` with its `AccountMovement` and optional `CategoryAllocation` rows. The current Void operation is an audited correction: it preserves those rows and the reason while excluding the event from balances and spending. The Product Owner also wants a separate way to remove an unwanted transaction from the live application and authoritative database. A hidden row or permanently visible Voided row would not meet that need.

The current v4 schema links movements and allocations to events with owner-scoped, `ON DELETE RESTRICT` foreign keys. History orders events by `occurred_at` and ID. Balance and category-spending queries count only posted events. Future modules may link to a FinancialEvent, so deletion eligibility must be reviewed again when such links exist.

## Proposed decision

1. Keep **Void** as the existing auditable correction. Add a distinct, owner-confirmed **permanent deletion** operation. Only an already-voided event is eligible; a posted income, expense, or transfer must first be successfully voided. The server enforces this transition even if a client bypasses the UI.
2. In one owner-scoped PostgreSQL transaction, lock and verify the voided event, inspect current dependent references, delete its `CategoryAllocation` and `AccountMovement` rows, then delete the `FinancialEvent`. A failed or concurrent operation rolls back fully. Do not use unreviewed cascades or rewrite the published v4 migration.
3. After commit, the event and its dependent rows are absent from the live authoritative tables. Money, Today, and future normal exports and reports must not show it. Because a voided event already has zero balance and spending effect, deletion must not apply a second reversal. Mixed currencies and transfer semantics remain unchanged.
4. Do **not** retain a per-transaction tombstone or a separate live deletion receipt by default. Generic operational metrics may count deletions without event IDs, amounts, notes, category names, or other transaction contents. Application access/error logs must avoid transaction contents and should avoid raw event IDs in URLs or structured fields.
5. Require an explicit, accessible confirmation that distinguishes Void from permanent deletion and explains that the record disappears from the live app without in-app undo. Use server-derived owner identity and the existing session, Origin, and CSRF mutation guards. A foreign event must appear absent. Define stable responses for posted, already deleted, and concurrently changed events in the Issue #29 API contract before coding.
6. This is an **intentional user-requested exception** to the ordinary audit-history preservation described by ADR-004 and `docs/DOMAIN_MODEL.md`. ADR-004's ledger structure, deterministic money semantics, and audit-preserving Void remain accepted. ADR-003's data-preserving forward upgrades and ADR-014's owner isolation remain fully in force. Acceptance of this ADR would authorize only the Issue #29 scope, not bulk deletion or physical-erasure claims.

## Retention and restore policy proposal

Deletion removes rows from the live authoritative database after commit. It does not promise immediate physical erasure from PostgreSQL pages/MVCC, WAL, replicas, backups, exported files, or third-party copies. Backup retention duration and deletion-aware recovery procedures must be written and approved before calling the lifecycle complete.

A backup taken **before** deletion may reintroduce the transaction if restored. Therefore an old backup must first be restored into an isolated recovery environment, never promoted directly to the authoritative service. Compare its events and balances against a trusted post-deletion source or a separately controlled, minimal deletion inventory; reapply verified deletions and reconcile totals before promotion. This proposal does not create a live per-transaction receipt or select an external inventory system. If no trustworthy post-deletion source or inventory survives, the system cannot prove that an old backup preserves deletion decisions; the restore must be treated as incomplete, with owner review, rather than advertised as deletion-safe. A verified post-deletion backup reduces that gap but does not erase older retained copies.

The Product Owner must approve the concrete backup retention period, the source of deletion reconciliation evidence, isolation/promotion procedure, and user-facing limits before Issue #29 can be accepted as a complete deletion lifecycle.

## Alternatives considered

- **Hide or soft-delete the event:** preserves audit but conflicts with the requirement to remove the event from live authoritative tables.
- **Permanent live tombstone or deletion receipt:** simplifies restore reconciliation but retains a per-transaction trace contrary to the default privacy preference; revisit only with explicit approval.
- **Delete a posted event directly:** skips the established balance and spending correction transition and makes concurrency harder to reason about.
- **Database cascade:** conceals dependent-record scope and future references; explicit ordered deletion is reviewable under the current restrictive foreign keys.
- **Claim that a SQL DELETE erases every copy:** technically false while WAL, backups, replicas, exports, or storage remnants may remain.

## Consequences

The owner can remove an already-voided transaction from normal live history. Voiding still provides an auditable correction for ordinary mistakes. Permanent deletion intentionally gives up the event-level live audit trail and in-app undo. Future reports and export implementations must read the current authoritative tables and must not reconstruct deleted events from cached history. Any new FinancialEvent reference must specify whether it blocks deletion or has an explicitly approved cleanup rule; no cross-module silent cascade is allowed.

Without a durable deletion inventory, restoring a pre-deletion backup cannot automatically guarantee that deleted records stay deleted. This is a deliberate limitation, not an implementation detail to hide.

## Data migration impact

The current v4 tables can support explicit child-row deletion in a transaction, so no schema migration is assumed. Implementation must inventory all current foreign keys and future module references; a new numbered forward migration and populated upgrade test are required if that review reveals a storage change. Published migrations remain immutable. No reset or restore of the personal database is authorized by this proposal.

## Security and AI impact

The server must derive the owner from the verified session, enforce Origin and CSRF, scope every row lock/delete by owner, and test foreign IDs as not found. Do not log request bodies, transaction notes, amounts, or other contents. Review whether router/access logs expose raw event IDs before implementation. Permanent deletion is a destructive operation; AI/MCP invocation is out of scope and remains disabled by default under the accepted Tool Gateway policy.

## Rollback or exit path

Before commit, a failed delete rolls back the full database transaction. After commit there is no in-app undo. Restoring an older backup can recover data but may reintroduce previously deleted transactions and roll back unrelated newer data, so it is not an ordinary undo path. Use the isolated restore and reconciliation policy above before promoting any recovered database. If this policy cannot be made reliable enough for personal-data use, keep Void and defer permanent deletion rather than weakening the data-preservation or backup gate.

## Approval questions

- What backup retention period and deletion-reconciliation evidence source are acceptable without a live per-transaction receipt?
- Should a future linked record block deletion by default, or may a separately approved domain rule remove that link?
- Which exact confirmation wording and repeat/concurrent HTTP responses should Issue #29 expose?

**Approval boundary:** This file is a proposal. Do not mark it Accepted, implement Issue #29, touch personal data, or run migrations until the Product Owner reviews the detailed design and resolves the restore-policy gate.

## References

- [PostgreSQL 18 VACUUM](https://www.postgresql.org/docs/18/sql-vacuum.html) explains that deleted tuples are not immediately removed from table storage.
- [PostgreSQL 18 Backup and Restore](https://www.postgresql.org/docs/18/backup.html) describes backups and point-in-time recovery.
- [PostgreSQL 18 Write-Ahead Log](https://www.postgresql.org/docs/18/runtime-config-wal.html) describes WAL support for recovery and replication.
