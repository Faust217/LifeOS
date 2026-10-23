# ADR-018: Known-Balance Anchors and Backdated Financial Events

**Status:** Accepted — Product Owner approved ADR-018 and authorized local Issue #33 implementation only on 2026-10-20; final visual/export approval and commit/push/closure authorization followed on 2026-10-23.

**Date:** 2026-10-20

**Issue:** [#33 — Opening Balance Effective Date and Backdated Transactions](https://github.com/Faust217/LifeOS/issues/33).

**Approved order:** [#33 Opening Balance](https://github.com/Faust217/LifeOS/issues/33) → [#34 Idempotency](https://github.com/Faust217/LifeOS/issues/34) → [#35 Atomic Edit](https://github.com/Faust217/LifeOS/issues/35) → [#36 Running Balances](https://github.com/Faust217/LifeOS/issues/36). Issue #33 alone is now authorized for local implementation; #34–#36 remain separately gated.

## Context

Before Issue #33, published migration v4 stored `finance_accounts.opening_balance_minor` without an effective date. `internal/finance/repository.go` `balanceSQL` adds every posted, nondeleted movement to that value, regardless of occurrence date. Create, Void and Delete validate their account effects using that same formula. There is no reliable way to infer whether an older movement was already included in an owner's starting balance. An account's `created_at` is a storage timestamp, not evidence of the balance's effective date.

ADR-003 requires integer money and safe forward migrations; ADR-004 retains one FinancialEvent with signed account movements and optional category allocations; ADR-014 requires session-derived owner scope; ADR-016 governs visible Void versus hidden Soft Delete. ADR-017's v1 export reconstructs balances from opening balance plus all eligible movements. New anchor semantics therefore require an explicit compatibility boundary rather than a silent reinterpretation of v1 exports.

The Product Owner prefers a known balance at a precise instant, before transactions at that instant. Existing v7 accounts must retain their values and calculations until an explicit, safe conversion.

## Decision

### 1. Two explicit account calculation bases

- **Legacy:** `opening_balance_at` is null. Preserve the stored opening amount and existing all-movements calculation exactly. Do not backfill from `created_at`, earliest transaction, browser date or migration execution time.
- **Anchored:** `opening_balance_at = A` and `opening_balance_minor = O`. O is the owner's declared known account balance immediately **before every event whose occurrence instant equals A**. It is a balance assertion, not an income, expense, transfer or fabricated FinancialEvent.

The stored account basis can be derived from the nullable anchor. API/export DTOs explicitly name it `legacy_all_movements` or `known_balance_before_instant`. Current eligibility means `status = posted` and `deleted_at IS NULL`; after the separately approved Edit issue, only the event's current effective revision qualifies.

For each account, in its own currency:

| Basis | Current recorded ledger balance |
| --- | --- |
| Legacy | `L + sum(all eligible signed movements)` |
| Anchored | `O + sum(eligible signed movements whose occurred_at >= A)` |

An anchor may be at or before the server's confirmation time. This issue does not introduce scheduled posting or change the existing acceptance of future-dated FinancialEvents: any already-posted movement on/after A still contributes to the current **recorded ledger position**, even if its date is later than the wall clock. Do not describe that position as a live bank balance or silently add an `occurred_at <= now()` filter. A future-date policy change needs separate review; recurring/scheduled transactions remain out of scope.

Negative opening values, liabilities and negative resulting balances remain valid within the existing signed int64 range. Account currency stays immutable; no FX conversion or mixed-currency total is introduced.

### 2. Backdating, Void, Delete and transfers

For an anchored account, a movement before A has zero contribution to its current balance, but remains normal visible history when not deleted. A posted pre-anchor expense remains spending on its occurrence date; the anchor is not a spending/report filter. Exact category-allocation totals and Uncategorized rules remain unchanged.

Every financial operation must compare the old and new **effective contribution per account**, rather than blindly adding/reversing its raw amount. This applies to posting, Void, Delete and the later Edit implementation. Voiding/deleting a pre-anchor event does not change current balance; it still changes visible history/spending according to existing status/visibility rules. Deleting an already-voided event never reverses effects twice. Void audit text and metadata are not fabricated or changed by anchoring.

A transfer remains one event with two signed movements. Apply each account's own legacy/anchor rule independently; accounts may have different anchors or one may remain legacy. A pre-anchor leg is already represented by that account's known balance, while a post-anchor leg contributes normally. The incremental effect on the two displayed balances need not sum to zero when their starting assertions cover different periods; the underlying transfer movements must still offset exactly and never become spending. Lock and validate both owned accounts atomically.

### 3. Ordering and historical interpretation

Use ascending `(occurred_at UTC, event_id)` for reconstruction and its reverse for history display. The virtual anchor sorts before all events at A. UUID order is only a deterministic tie-breaker for identical instants, not a claim about their real-world sequence.

For a future running-balance implementation, let `S(e)` be the complete eligible movement prefix through event e and `S_before(A)` the complete eligible movement sum strictly before A:

`reconstructed_balance_after(e) = O + S(e) - S_before(A)`

This works on either side of the anchor. It is derived from recorded data and the owner's assertion, not independent proof of historical bank balances. Incomplete earlier history must carry a concise reconstruction warning; confirming the anchor must not set a fabricated history-complete/bank-verified flag. Legacy accounts retain current balances but require anchor confirmation before the planned dated running-balance display; do not invent an effective date for them. Actual running-balance UI/API delivery belongs to the fourth issue.

All arithmetic uses exact wide accumulators (PostgreSQL numeric / Go integer arithmetic), then validates required output values against signed int64 bounds. Do not fail or wrap because an intermediate `current - old + new` overflows int64 when its exact final result is valid. No floats, rounding, cross-currency sums or page-local reconstruction.

### 4. Saved timezone and precise boundaries

Use the existing confirmed owner IANA timezone to enter/display an anchor, with the existing DST-gap rejection and repeated-time choice. Require timezone confirmation through Me before new anchor entry; do not fall back to the device zone. Save the resolved UTC instant with PostgreSQL microsecond precision. Preview shows the local time, IANA identifier and corresponding UTC value.

Changing the timezone preference changes presentation/grouping only. It must not move the stored anchor or rewrite event instants. Events exactly at A contribute; events one microsecond before A do not contribute to current balance. Conversion does not change any existing event occurrence/creation timestamp, amount, ID, relationship or archive/Void/Delete state.

### 5. Explicit legacy conversion preview and confirmation

Conversion is account-specific. Provide an accessible, compact preview before any write. Define these sums over the same complete owner-scoped snapshot:

- `B_old = L + S_all`: existing current balance.
- `S_before(A)`: eligible movements strictly before the chosen anchor.
- `S_after(A)`: eligible movements at/after it.
- `O_preserve = B_old - S_after(A) = L + S_before(A)`: suggested anchor amount that preserves the current balance.

The default preview uses O_preserve when representable as int64. Label it **calculated to preserve LifeOS's current balance**, not a bank-verified amount. Require the owner's explicit confirmation of both amount and instant. The owner may instead enter a genuinely known amount O; preview `B_new = O + S_after(A)` and `delta = B_new - B_old`, with clear old/new currency-labelled balances. A nonzero change requires explicit acknowledgement. Do not silently clamp an unrepresentable suggested anchor or silently accept stale preview totals.

Preview includes the before/after basis, chosen instant, old/suggested/chosen amounts, old/new current balance, signed delta, and counts/net effects before versus at/after A. Use exact decimal strings and no paginated API input. Changing amount/date regenerates the preview. Cancel/error leaves the legacy account untouched and preserves the draft.

Preview returns the account's balance revision. Confirmation uses the same revision and chosen values, locks the owner-scoped account, recomputes from current data and fails with a refreshable conflict if anything affecting the preview changed. The persisted `balance_revision` increments on each relevant account creation/conversion and event post/Void/Delete, including pre-anchor changes; later Edit uses it too. Lock all affected accounts in stable ID order. Thus a concurrent mutation either precedes a refreshed preview or follows the confirmed conversion, with no mixed formula.

Record conversion evidence atomically with the anchor: owner/account, previous basis/opening value, selected anchor/value, calculated preserve value, previous/resulting ledger balance, assertion choice and confirmation UTC time. Keep this evidence owner-scoped and internal; do not create a financial event, claim bank verification or log financial contents. Account archive state is preserved.

This first scope permits new-account anchor confirmation and a one-time legacy conversion. General anchor rebasing/editing, multiple reconciliation checkpoints and bank reconciliation tools are not authorized here. After an uncertain conversion response, re-read the owned account; an already converted account is not converted again. Revision checks and the one-way transition protect this first issue without pre-implementing the separate general idempotency mechanism. Later idempotency extends to this mutation.

### 6. Synthetic examples

| Recorded state/action | Bank MYR | Cash MYR |
| --- | ---: | ---: |
| Both known balances before 10/1 00:00 | 1,000 | 200 |
| 10/2 expense 50 from Bank | 950 | 200 |
| 10/3 transfer 100 Bank to Cash | 850 | 300 |
| Later Edit changes expense 50 to 80 | 820 | 300 |
| Void or Delete the edited expense | 900 | 300 |

Edit/running-balance behavior in this table is an acceptance dependency for later issues, not implementation authorization in this issue. A 9/30 expense 20 added to the anchored Bank account does not reduce the current balance again; immediately before/after that lone pre-anchor event, reconstruction gives 1,020/1,000, with an incomplete-history warning rather than a verified-bank claim.

Legacy conversion example: L=1,000; pre-anchor expense=-100; post-anchor expense=-50. Existing current balance is 850. O_preserve=900, so confirmation gives `900 - 50 = 850`. If the owner instead confirms a known anchor balance of 1,000, preview must show new current balance 950 and an explicit +100 change. Merely assigning A while retaining L would silently change meaning and is prohibited.

## Forward migration and upgrade strategy

The accepted implementation uses new forward migration v8; published v1–v7 remain unchanged. Migration execution is limited to disposable test targets:

1. Add forward migration v8 (after the immutable v1–v7 chain) for nullable UTC `opening_balance_at`, nullable UTC `opening_balance_confirmed_at`, a nonnegative `balance_revision`, and owner-scoped opening-confirmation evidence. Anchor and confirmation timestamps are both null for legacy or both present for anchored accounts. Do not edit v1–v7.
2. Leave every existing account opening amount, event/child row and legacy formula unchanged. Setting concurrency metadata is not balance conversion. Never populate anchors from account creation timestamps.
3. Ship dual-basis calculations across Finance writes/current balances and therefore Today, with guarded preview/confirmation and updated OpenAPI/generated contracts. New-account UI/API requires an explicit amount/instant confirmation; older clients missing the new required input receive a clear contract error rather than silently creating an assumed anchor. Existing-account legacy reads continue to work.
4. Use a populated-v7 synthetic upgrade rehearsal proving exact pre-confirmation equality, then explicit conversion and full rollback-on-error/concurrency checks. Personal upgrades require separately approved backups and restore verification; this issue grants no personal database access.
5. Down migration/binary rollback must refuse when anchors or retained confirmation evidence would be lost or an older all-movements reader would miscalculate. Use an approved corrective forward migration or rehearsed restore; never clear anchors or overwrite amounts as an automatic rollback.

## Export compatibility

Preserve ADR-017, the `lifeos-export-v1` / Finance CSV v1 contracts and their golden fixtures unchanged. They remain historical version contracts, not extensible labels for a new calculation rule. Accepted anchor support introduces **lifeos-export-v2** and **lifeos-finance-csv-v2** before any anchored account is enabled:

- JSON v2 starts from the retained v1 fields, changes the explicit format/version to v2 and adds these required account fields: `opening_balance_basis` (the two names above), `opening_balance_at` (nullable UTC), `opening_balance_confirmed_at` (nullable UTC). Legacy accounts retain original opening values/null timestamps. Events and their stable relationships remain unchanged in this first scope. Internal concurrency/conversion evidence is not mixed into normal financial event collections.
- CSV v2 keeps the same seven fixed ZIP filenames. Its accounts header becomes `account_id,name,account_type,currency,opening_balance_minor,opening_balance_decimal,opening_balance_basis,opening_balance_at_utc,opening_balance_confirmed_at_utc,archived_at_utc,created_at_utc,updated_at_utc`. Other CSV headers remain as v1 in this first scope. Manifest format/version become v2; README defines both formulas and pre-anchor effects. Preserve exact integer/decimal text, BOM, guards, limits, privacy and one-snapshot/session-revalidation rules.
- Keep an explicit format-version request choice. The current request with version omitted remains a v1 request. New UI requests v2 explicitly. A v1 request for an owner with any anchored account fails clearly with a version-incompatible error directing the client to v2; do not omit accounts, rewrite opening amounts, discard pre-anchor events or emit new semantics under a v1 filename. The exact additive request/error contract and complete v2 schema/golden fixtures must be documented and reviewed within the first implementation scope before enabling conversion.
- Previously downloaded v1 files keep their original interpretation. A later Edit revision model requires its own compatibility review; this decision does not pre-approve exporting superseded ledger rows or decide that a future incompatible change may reuse v2.

No import/restore implementation is authorized by either export version.

## Verification requirements

- Clean-chain and populated-v7 forward upgrade; legacy field/balance/export equality until confirmation; no inferred anchor; safe down refusal.
- Exact-at-anchor, microsecond-before/after, negative/zero/int64-boundary amounts, wide intermediate arithmetic, mixed currencies and negative liabilities.
- Full preview sums, preserve-current versus deliberate-change examples, cancellation, stale preview, concurrent post/Void/Delete, repeat confirmation and lost-response recovery; account archiving must not be changed as a side effect.
- Pre-anchor versus post-anchor Void/Delete; already-voided Delete; transfers across different and mixed legacy/anchored bases; allocation/Uncategorized and spending semantics unchanged.
- Saved-zone versus device-zone input, timezone changes, DST gaps/overlaps, future-dated posted records under the documented recorded-position rule.
- Owner/session/Origin/CSRF isolation for preview, confirmation and ordinary reads/writes; private errors/no sensitive logs; no partial writes on validation/overflow/failure.
- Exact v1 fixtures unchanged; v1 incompatibility failure when necessary; complete v2 JSON/CSV relationships/counts/formulas/guards and consistent snapshots.
- Full Go/frontend/contracts and disposable PostgreSQL/browser verification when implementation is authorized, including mobile conversion preview and draft/focus recovery. Record actual function-level flow and English learning notes then.

## Alternatives considered

- **Add all backdated events to an undated opening amount:** preserves the current ambiguity and can double-count an already-known position.
- **Use account created_at as the anchor:** invents a financial fact and can silently change legacy balances.
- **Reject every pre-anchor event:** simpler, but does not meet the approved visible backdated-history direction.
- **Automatically alter a confirmed anchor when old events change:** destroys the owner's known-balance assertion; anchor correction requires explicit separate review.
- **Create an opening income/event:** confuses a balance assertion with actual money received and risks spending/transfer misclassification.
- **Rewrite historical exports or silently normalize v1 opening values:** violates the approved stable format meanings.

## Consequences, decision relationships and exit path

The two calculation bases add deliberate complexity during a safe legacy transition. Current balance changes only after explicit conversion; pre-anchor history remains useful without being presented as complete banking evidence. Transfers can have different incremental current effects against different balance assertions, while remaining one offsetting event. Dedicated projections, tests and v2 exports must share the same rule.

This is an accepted refinement of Account/opening-balance semantics and the application of ADR-016's financial-effect removal. It does not supersede ADR-003/004/014/016, change Void visibility or introduce physical deletion. It follows ADR-017's breaking-version policy without changing the v1 contract. Accepted historical ADR files remain unchanged; ADR-015 stays Proposed. If final review identifies a conflicting accepted clause, record the exact proposed supersession before implementation.

Owner isolation, exact money, immutable published migrations and Personal Data Ready backup/restore/deletion-state reconciliation remain permanent gates. No new AI/MCP permissions, queue, Redis, scheduler or runtime provider is authorized. Disable unaccepted conversion UI/endpoints until the ADR and implementation are separately approved; after actual conversions, disabling presentation alone does not make an old binary safe. Preserve anchored data and use a reviewed forward correction/restore strategy.
