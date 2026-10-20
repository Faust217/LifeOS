# ADR-017: Versioned Owner Data Export and Consistent Read Snapshots

**Status:** Accepted — Product Owner approved for local Issue #32 implementation on 2026-10-20
**Date:** 2026-10-20

**Issue:** [#32 — Me / Owner Data Export](https://github.com/Faust217/LifeOS/issues/32), implementation and visual/export-file review approved; final delivery authorized. Deployment remains separately gated.

## Context

Accepted ADR-009/013 require complete structured JSON and useful Finance CSV exports. ADR-014 requires server-derived owner scope; accepted ADR-016 excludes Soft Deleted events and their dependents from normal exports. The current Me page exposes profile/sign-out and the persisted timezone API exists, but no export API is implemented. Existing Finance/Planning UI reads use independent pool calls/transactions and omit some persisted timestamps; combining them would not define one complete, consistent export.

## Decision

Adopt the exact [Me and Owner Data Export design](../EXPORT_DESIGN.md), including:

- `lifeos-export-v1` as an independently versioned normal owner-data JSON format. Preserve IDs, relationships, UTC timestamps, currency and int64 decimal strings. Include archived and voided retained data, Goals/checklist items and the actual timezone preference; exclude Soft Deleted Finance events/dependents and authentication/operational secrets. Optional additive keys can extend v1; breaking type/meaning/visibility changes require a new export version and review.
- A Finance ZIP with accounts/categories, one transactions row per event, separate exact movements and allocations, manifest and README. Preserve all existing integer minor-unit columns. Add `opening_balance_decimal` immediately after `opening_balance_minor` in accounts CSV, and `amount_decimal` immediately after `amount_minor` in transactions, movements and allocations CSV. Derive each decimal string from that row's integer and the currency's standard decimal scale using integer/string operations only; do not use floating point, rounding, cash increments or a two-digit fallback. Preserve signs and exactly the required fractional digits, including zero/subunit values and int64 boundaries. Apply the existing apostrophe text guard and CSV escaping to both monetary column forms; README documents logical and encoded examples for 0/2/3/4-decimal currencies. JSON and the CSV manifest schema remain unchanged and JSON remains authoritative. CSV does not promise universal safety after spreadsheet re-saving.
- One read-only `POST /api/v1/me/export` operation behind the existing session/Origin/CSRF guard. A small export coordinator calls explicit module-owned readers using one PostgreSQL `REPEATABLE READ READ ONLY` transaction; it does not bypass module ownership or combine paginated UI records.
- Complete generation into a bounded memory buffer before secure no-store attachment response, releasing the database snapshot before network delivery and revalidating the same active authenticated session/owner before bytes are released. Initial limits are 64 MiB raw/output, 120 seconds generation plus a separate 120-second response write deadline, one export per owner and two per API process. Fail visibly rather than truncate; cancellation/errors discard unfinished bytes, and a retry captures a new consistent snapshot. No export tables, server disk artifacts, queue, object store or new dependency.
- Me hosts the shared existing timezone editor; Money links to it. No new preference semantics, localization or profile mutation is introduced.

This refines the retained export capability and records its public-format compatibility and module composition boundary. It supersedes no accepted ADR. ADR-015 stays Proposed; approving this record must not implicitly approve its separate Bluecoins scope change.

## Alternatives considered

Client-side export of history pages risks missing records and inconsistent snapshots. Independent module read transactions do not guarantee one view. Raw database dumps expose internal/security records and do not provide a client-independent normal export. One CSV row per allocation duplicates transfers/event amounts and confuses spending. An asynchronous export platform or server-side file store is unnecessary for the bounded initial Alpha requirement. Unlimited buffering or an unbounded long-lived snapshot is unsafe.

## Consequences

Users gain a documented portable snapshot without changing current Finance/Planning meanings. Module-owned export projections and shared transaction interfaces are required because existing API DTOs are incomplete. The full artifact is held in bounded memory and browser Blob memory, so output above the stated limits must fail explicitly; a larger measured need requires follow-up design. Separate JSON and CSV requests can represent different snapshots. A file captured before a concurrent deletion may still contain that event, and already downloaded copies cannot be revoked. Export is not an implemented import/restore format or a substitute for backup.

Current Void audit text has no reliable stored author-origin marker; export preserves it without claiming user authorship. Opening-balance effective dates/backdated semantics remain unresolved and are not invented by the format.

Readable CSV decimals are derived representations of the same exact integer facts, not new ledger amounts. They do not duplicate transfer/event rows, alter kind/status or allocation relationships, round cash values, or add fields to `lifeos-export-v1` JSON. Standard currency scale comes from the existing pinned currency metadata; an unresolved scale fails the export rather than changing interpretation. Golden CSV expectations cover fixed-scale examples, both int64 boundaries, text guards and exact reconstruction of minor units.

## Data migration and compatibility impact

No schema change is expected for v7 data. Published migrations stay unchanged. Any discovered schema need requires a separately reviewed new forward migration and upgrade test. Additive OpenAPI/download contracts and generated TypeScript types are authorized for local implementation; export format versioning stays separate from API/database versions. Golden export fixtures and complete owner-isolation/snapshot tests become compatibility gates.

## Security and AI impact

Export derives owner identity from the authenticated session and rejects client owner overrides. Revalidate the session before response, keep private no-store headers, omit sensitive logs and guard CSV text. A downloaded file contains private data; the UI must say so. Cancellation and generation failure discard the buffer without a retained artifact. No AI/MCP tool or personal-data development access is authorized.

## Rollback or exit path

Disable the export endpoint/UI if a defect is found without changing or deleting domain data. Fix format defects with an explicitly documented compatible correction or a new version; never silently redefine v1 meaning. Retain format documentation/fixtures and existing valid downloaded JSON. Personal Data Ready still requires verified backup/restore, deletion-state reconciliation and safe forward upgrades. The Product Owner accepted this ADR and separately authorized local Issue #32 implementation on 2026-10-20. The Product Owner subsequently approved the UI and actual artifacts and authorized commit, push and closure after final verification. Deployment remains separately gated.
