# ADR-021: Owner-Scoped Goal Soft Delete and Normal Export Visibility

**Status:** Accepted - Product Owner approved the complete design, including the narrow ADR-017 exception, and separately authorized local Issue #38 implementation on 2026-11-04. The Product Owner subsequently manually verified successful Goal deletion and approved final Issue #38 delivery on 2026-11-04. Deployment and personal-data operations remain separately gated.

**Date:** 2026-11-04

**Issue:** [#38 Safe Goal Delete](https://github.com/Faust217/LifeOS/issues/38). Exact API/UI/tests/function flow: [Goal Delete Design](../GOAL_DELETE_DESIGN.md).

## Context and compatibility checkpoint

Schema v11 has owner-scoped Goals and checklist items but no parent deletion marker. Archive is reversible and remains visible; completion records achievement. The Product Owner requests disappearance from normal Plans, Today and owner exports while retaining current checklist relationships internally. Goals are non-financial intent and have no account/Finance History effect.

Existing checklist and completion operations lock the parent; rename/archive/restore take implicit parent UPDATE locks. Composite checklist foreign keys are restrictive, not cascades. Normal list/detail and export reads already use consistent snapshots. ADR-016 governs FinancialEvents only; it does not automatically authorize a new Goal lifecycle or change export contracts.

There is a specific policy conflict to resolve explicitly: accepted ADR-017 and Export Design currently include every retained Goal and require a new version for visibility changes. Omitting newly soft-deleted Goals from unchanged v1/v2 JSON therefore needs an approved narrow exception, even though schemas and existing fixtures need not change. No existing accepted ADR is edited or silently reinterpreted here. ADR-015 remains Proposed.

## Decision

1. Add one nullable UTC `goals.deleted_at` in the forward migration v12. A marked Goal is unavailable in ordinary application reads. Keep its ID/title, completion/archive state and every checklist row present at deletion. Previously removed checklist items are not reconstructed. Do not add child markers, audit snapshots, receipts, Trash/Undo, hard cascades or retention infrastructure.
2. Permit explicit confirmed Delete on active, completed and archived Goals, including empty Goals. This is a narrow exception to the ordinary read-only treatment of archived Goals; it does not make their other fields editable. Deleted Goals cannot be completed, reopened, renamed, archived, restored or have checklist mutations through normal routes.
3. Use guarded owner-scoped `DELETE /api/v1/goals/{goalID}`, empty body/query, committed 204 with no content. Lock the parent in one bounded database transaction. First Delete sets marker/updated timestamp once; repeat Delete of the same owned retained Goal returns 204 without changing either timestamp. Foreign/missing IDs and otherwise valid ordinary operations on deleted parents return the same 404. Every retry rechecks session, Origin, CSRF and server-derived ownership. No financial request key, new Goal revision or replay infrastructure.
4. Serialize competing mutations on the existing parent row. A valid mutation that commits first is retained before Delete; one waiting behind Delete fails not-found. Confirmed Delete applies to the same Goal ID's latest committed checklist, not a frozen checklist revision. Existing post-mutation GetGoal can race Delete and return 404 after that mutation committed; never claim this proves rollback.
5. Filter normal parents and children in module-owned SQL, including archived lists and Today through their existing Goals API. Keep each read's current REPEATABLE READ consistency. No client-only deletion state or realtime/cross-tab synchronization system.
6. **Explicit accepted ADR-017 exception:** normal JSON export v1/v2 excludes newly soft-deleted Goals and all their checklist rows. This ADR narrowly supersedes the export visibility/version requirement **only for introducing this deleted-Goal state**. All remaining ADR-017 requirements stay in force. Do not remove/reclassify a visible active/completed/archived Goal, remove required fields, change field types/order/meaning, add deletion fields/receipts or change existing golden fixtures. Existing v11 data with null markers exports the same logical record sets. Future unrelated visibility/schema changes still require the existing compatibility/version review.
7. Reuse `planning.WriteExport` in the export coordinator's one read-only REPEATABLE READ snapshot, filtering parent and child sets together with exact counts/resolvable links. Preserve final session revalidation and bounded privacy-safe downloads. Finance CSV v1/v2 contains no Goals and is unchanged; no v3 or audit export is proposed. An earlier snapshot/file can still contain subsequently deleted data.
8. Use a compact secondary action and accessible inline confirmation, preserving drafts on cancellation/errors. A dedicated frontend delete path accepts 204 without passing undefined into the existing Goal-upsert workflow. Keep an uncertain attempt bound to the same Goal ID; Check/Retry cannot claim a timed-out request rolled back. A 204 confirms desired state, not which attempt performed it. No persistent browser recovery is promised or new storage introduced.

## Alternatives and consequences

- Archive-only leaves the Goal in Archived/exports and does not satisfy deletion.
- Parent hard delete/cascade loses retained relationships and is unnecessary for this UX requirement.
- Hiding only in React leaves direct links, APIs and exports inconsistent.
- A new revision/financial-idempotency/audit framework is unnecessary for a terminal, fixed-ID command with one durable marker.
- A new JSON export version is the stricter alternative considered before the Product Owner accepted the narrow ADR-017 exception. Do not silently keep deleted Goals in normal exports or change accepted semantics.

The design adds one column and coordinated predicates to existing paths. It retains private data internally and offers no ordinary undelete. SQL filters must be covered on every Goal/checklist path; restrictive FKs alone do not enforce soft-deleted visibility. Logs must omit titles/checklist contents. A direct database operator is outside these application guards.

## Migration, upgrade and exit

Only forward v12 adds the nullable marker; existing rows remain null. Preserve migrations v1-v11 and all current parent/child/Finance/History facts. Keep current indexes absent measured need for another. The down migration must lock before checking and refuse while any deleted Goal exists, preventing accidental resurrection. After use, prefer a reviewed forward repair.

Drain old Planning/API readers/writers and exports before activating the new API/UI or allowing any deletion. Old binaries do not filter the marker; Finance's v11 writer fence is not a Planning fence. No rolling mixed-version compatibility is promised by the one-column design. The authorized implementation advances readiness to v12; production/personal migration remains separately gated.

Backups, earlier exports and storage/WAL may retain contents; an old-backup restore can resurface a Goal. Preserve the general Personal Data Ready restore/reconciliation gate, with no special deletion inventory or recovery implementation. Stopping the Delete UI is an exit for a defect; do not clear markers or downgrade in a way that silently restores visibility.

## Security, Finance and AI boundaries

All authorization is server-session derived and owner-scoped; query/header/body owner selectors cannot grant access. Keep generic unknown/foreign/deleted errors and no-store handling. Same-ID retries after response loss/process restart must be safe. Read/export snapshots can finish with their consistent earlier state; already downloaded content cannot be revoked.

No FinancialEvent, account movement/allocation, account balance, spending total, Finance history epoch/revision, financial pending command/key or export money representation changes. No AI/MCP deletion capability is added.

## Acceptance and authorization boundary

The detailed design's acceptance gates include fresh populated-v11/full-chain/guarded-down tests; all lifecycle states and preserved children; parent-lock races in both orders; duplicate/lost-response/restart/rollback/timeout handling; session/Origin/CSRF/owner isolation; full deleted-parent mutation rejection; coherent Planning/JSON snapshots, unchanged v1/v2 schemas/goldens and Finance CSV; unchanged Finance History tokens/balances; frontend 204 handling, drafts/uncertainty/focus and 320px/390px/desktop visual checks. Record actual tested platforms and unavailable checks accurately.

The Product Owner accepted this ADR and separately authorized local Issue #38 implementation on 2026-11-04. The exact narrow exception is in force; ADR-017 historical text remains unchanged. The Product Owner manually confirmed successful Goal deletion and authorized final verification, commit, push and closure on 2026-11-04. Full rendered 320px/390px, keyboard/focus and physical-mobile verification remain incomplete; approval does not convert those checks into passes. Personal-data operations, deployment, Backup/Restore and unrelated features remain unauthorized.
