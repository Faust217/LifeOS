# LifeOS Domain Model

**Status:** Conceptual model updated for Issue #16; not final SQL

**Baseline date:** 2026-08-17

**Sources:** Master Project Blueprint v0.2 (historical); Product Owner-approved [Issue #16](https://github.com/Faust217/LifeOS/issues/16) (current scope)

## Modeling Principle

Model the real world rather than UI shortcuts:

- real money is a `FinancialEvent`;
- future intent is a `Plan`;
- something owned is an `Asset`;
- something desired is a `WishlistItem`;
- a person is a `Person`;
- a shared cost is a `SplitExpense`.

Distinct meanings are linked, not collapsed into one generic transaction or everything-shaped record.

V0.1 adds independent invited users and simple checklist Goals. A LifeOS account holder is not the same concept as a `Person` in a later shared-expense record. Today, Money, Plans, and Me are navigation labels, not database entities.

## Domain Map

```text
LifeOS user / Owner (one independent data boundary per admitted user)
├─ Identity (user, invitation, Google identity binding in Issue #18; sessions in Issue #19)
├─ Finance
│  ├─ Account
│  ├─ FinancialEvent
│  ├─ AccountMovement
│  ├─ Category / CategoryAllocation
│  └─ ImportBatch
├─ Planning
│  ├─ Plan
│  ├─ Goal (V0.1 checklist-based future intent; migration 000005)
│  ├─ PlanItem (later)
│  └─ ChecklistItem
├─ Wishlist
│  └─ WishlistItem
├─ Assets
│  └─ Asset
├─ People and Split
│  ├─ Person
│  ├─ SplitExpense
│  ├─ SplitShare
│  └─ Settlement
└─ AI / Integration
   ├─ AIProviderConfig
   ├─ ToolInvocationAudit
   └─ external client/session metadata
```

## Shared Record Baseline

- Core records use UUID identities, preferably application-generated UUIDv7.
- Every user-owned core record is scoped by `owner_id` from day one. The owner is determined from the verified, invited LifeOS identity and protected session, not a client-provided ID.
- Records carry `created_at` and `updated_at`; audited entities also use void/delete metadata or an audit event.
- Timestamps are stored in UTC; the owner's timezone is stored separately.
- The owner confirms a validated IANA timezone preference before Money renders local dates or accepts local datetime input. Existing and future Finance events keep UTC instants; a later preference change can regroup historical date headers without rewriting events or amounts. DST gaps are rejected and repeated wall times require an explicit instant choice.
- Plan and travel calendar dates may use date-only types where time-of-day is not meaningful.
- Money uses integer minor units plus currency.
- State fields use controlled values, constraints, or lookup tables rather than arbitrary text.
- Identity, invitations, and sessions are access-control records, not financial or planning facts. Accepted ADR-014 governs their boundary. Issues #18–#20 add user identity, invitation, session, and owner-authorization foundations; Issue #21 introduces the separate Finance schema.

## Finance

Finance is the lowest-level domain and must have the most stable semantics.

### Account

An `Account` represents a place or instrument that holds an asset or liability position.

| Concept | Baseline meaning |
| --- | --- |
| `account_type` | `cash`, `bank`, `ewallet`, `credit_card`, `savings`, future optional `receivable`, or `other` |
| `currency` | Primary ISO 4217 currency for the account |
| `opening_balance` | Original undated legacy position, or the owner's known position immediately before the confirmed effective instant |
| `opening_balance_at` | Nullable UTC instant; null preserves legacy all-movements semantics. Confirmed dates are explicit owner assertions, never inferred from creation time. |
| sign convention | Positive is an asset position; credit-card debt is a negative liability position. An ordinary account can also be negative when recorded activity exceeds its balance; LifeOS records the fact rather than authorizing an overdraft. |
| archive behavior | Historical accounts are archived rather than hard-deleted |

Accepted [ADR-018](adr/ADR-018-opening-balance-effective-instant.md) distinguishes undated legacy accounts (original opening plus all eligible movements) from confirmed known-balance accounts (opening plus eligible movements occurring at or after the UTC instant). The instant is before all equal-time events. Pre-instant events remain visible and count in occurrence-date spending but do not change the confirmed current position. Legacy values/formulas remain unchanged until explicit revision-guarded conversion preview/confirmation. No anchor is inferred from account creation. Transfers apply the rule independently per account; Void/Delete remove only effective current contributions. Saved IANA presentation never rewrites UTC facts. Historical pre-instant reconstruction is not bank-verified. Export v1 meanings remain unchanged; native v2 carries the dated basis. General editing and historical-balance UI remain separately authorized work.

### FinancialEvent

A `FinancialEvent` is the user-visible fact that money actually moved or an account position changed. Examples include an expense, income, transfer, credit-card purchase, or credit-card payment.

Planned spending is not a `FinancialEvent` until it happens.

### AccountMovement

An `AccountMovement` is the signed effect of a `FinancialEvent` on one account. An event may contain multiple movements.

| Event | Movements | Spending classification |
| --- | --- | --- |
| Lunch RM15 from Maybank | Maybank `-1500 MYR` | Food `+1500 MYR` |
| Salary RM3,646 | Maybank `+364600 MYR` | Salary `+364600 MYR` |
| Transfer RM500 to Savings | Maybank `-50000 MYR`; Savings `+50000 MYR` | No expense category |
| Credit-card purchase RM100 | CreditCard `-10000 MYR` | Shopping `+10000 MYR` |
| Pay RM100 credit card from bank | Bank `-10000 MYR`; CreditCard `+10000 MYR` | Not new spending |

### Category and CategoryAllocation

A `Category` classifies economic purpose. A `CategoryAllocation` assigns part or all of a `FinancialEvent` to a category without assuming one event always has one category.

An income or expense may have zero allocations and is then displayed as **Uncategorized** without creating a category row. If allocations are present, their amounts must sum exactly to the event amount. Renaming a category keeps its ID and historical links; archiving removes it from new-entry choices while retaining those links. Neither operation reclassifies past events. Overall spending must include every posted expense `FinancialEvent`, including those with zero allocations, and must exclude transfers and voided events. A sum over named-category allocations alone is not an overall spending total. Keep totals separated by currency unless an explicit conversion rule is defined.

Transfers and liability payments must not be reported as new spending merely because cash moved.

### Money and Currency

- Never store money as floating-point values. RM15.90 is `1590` with currency `MYR`.
- The owner has a base currency; each account and movement retains its own currency.
- A travel purchase may retain both its original amount/currency and the actual charged amount/currency.
- Automatic FX rates are not required in V0.1; the actual charged amount is more authoritative than a theoretical rate.
- Cross-currency reports must state the conversion value and date used. Different currencies are never silently added.

### Corrections

Important financial events support edit/void or soft-delete behavior with audit metadata. Bulk deletion, import rollback, and AI-initiated destructive changes require explicit confirmation. A stricter future ledger may introduce reversal events rather than rewriting history.

Voiding an event requires explicit confirmation but the owner may leave the reason blank. In that case the server records a clearly labeled generated audit message rather than attributing an explanation to the owner. The original event and its links remain visible as voided; its balance and spending effects no longer count. This does not authorize permanent deletion.

Accepted ADR-016 adds a separate, confirmed **Soft Delete** for posted or already-voided financial events. A deleted event disappears from normal Money/Today views and future normal exports/reports while its event, movements, allocations, and any Void metadata remain internally. A posted Delete atomically removes the event's effects from balances and spending; deleting an already-voided event does not reverse them again. Deletion is not physical erasure or a guarantee about retained backups. Owner scope, forward migrations, and the Personal Data Ready backup/restore gate still apply.

## Planning and Travel

### Plan

A `Plan` is a generic container for future intent. V0.1 Goals are simple checklist-based intent with no balance effect. Issue #25 uses a focused `goals` record and owner-scoped `goal_checklist_items` rather than a generic Plan table. Later Plan types may include travel, purchase, moving, event, and custom; their storage is a separate decision.

Type-specific details should use explicit extensions rather than continuously expanding the base Plan table.

### PlanItem

A `PlanItem` is an estimated future item such as a hotel budget. It does not affect account balances. It may later link to zero, one, or multiple actual financial events.

### ChecklistItem

A `ChecklistItem` is a non-financial task within a Goal or later Plan and never affects account balances. Issue #25 tracks completion per item and derives progress as completed count over total count. A Goal can be marked complete only with at least one item and every item complete. Reopen a completed Goal before changing its checklist. Archived Goals retain their checklist and are read-only while archived; the owner can restore one without losing its steps.

### Planning Invariants

| Object | Meaning | Affects real balance? |
| --- | --- | --- |
| Plan | A future activity or project | No |
| Goal | V0.1 checklist-based future intent | No |
| PlanItem | An estimated future item | No |
| FinancialEvent linked to a Plan | Money that actually happened | Yes |
| ChecklistItem | A non-financial task | No |
| WishlistItem linked to a Plan | Something desired within the plan | No, until purchased through a real event |

Travel remains a later Plan type, not an independent financial universe. Wishlist is a separate later domain even if presented within Plans navigation.

## Wishlist

A `WishlistItem` expresses a desired purchase. Baseline fields are name, estimated price, currency, priority, status, target date, and notes.

- It may link to a Plan.
- It may identify an Asset it could replace.
- Marking it purchased should guide the user to create or link a real FinancialEvent; the status alone is not spending.
- URLs, images, and price tracking are future enhancements, not core V0.x requirements.

## Assets

An `Asset` represents something the owner owns and answers what it is, how long it has been used, what it cost, when its warranty ends, and whether replacement may be reasonable.

| Concern | Baseline |
| --- | --- |
| identity | ID, owner, name, category, optional brand/model |
| lifecycle | acquisition date; `active`, `sold`, `gifted`, `disposed`, or `lost`; optional retirement date |
| financial relationship | Optional purchase-event link; price may be derived or retained as an explicit snapshot |
| warranty | Warranty end, seller, and optional sensitive serial number |
| condition | Simple controlled state plus notes; no elaborate scoring system initially |
| replacement | A WishlistItem may reference the Asset it could replace |

An Asset is financially connected but is not itself a transaction and must not create a second expense.

## People and Split Expenses

The split domain separates cash flow from economic share. If the owner pays RM90 for three equal participants, RM90 leaves the account, the owner's economic share is RM30, and RM60 is recoverable.

### Person

A `Person` is a local participant or contact. That person does not need a LifeOS account and is not the authenticated user/owner boundary.

### SplitExpense

A `SplitExpense` describes a shared cost, its payer, its total, optional Plan relationship, and optional FinancialEvent relationship. It describes economic ownership of a real payment and must not debit the paying account again.

### SplitShare

A `SplitShare` records what each participant owes using equal, exact, percentage, or share-based rules.

### Settlement

A `Settlement` records repayment or offset between participants. It is not new income and not a second expense.

Netting and split arithmetic are deterministic application logic. AI may parse a description or explain a result but is never the authoritative balance calculator.

## Cross-Domain Relationships

| Relationship | Rule |
| --- | --- |
| Asset to FinancialEvent | A purchase event may be linked; the Asset does not duplicate the expense |
| Plan to FinancialEvent | Actual events may link to a Plan; budgeted intent remains separate from cash flow |
| PlanItem to FinancialEvent | A planned item may be unfulfilled or fulfilled by multiple actual events |
| WishlistItem to Plan | A desired item may belong to a travel or purchase plan |
| WishlistItem to Asset | A desired item may be a replacement candidate for an owned Asset |
| SplitExpense to FinancialEvent | The split describes economic ownership of the payment and does not double-debit the account |

## AI and Integration Support Records

`AIProviderConfig` stores optional runtime configuration without coupling provider-specific fields to finance, planning, or asset tables. Secrets must be handled safely and never exposed to models or clients.

`ToolInvocationAudit` records authorized AI/MCP tool execution metadata such as tool, permission, outcome, timing, and provider/model metadata when available. Full sensitive prompts are not retained by default.

External client/session metadata supports adapters such as MCP without making them core domain dependencies.

## Conceptual Storage Sketch

The following is a vocabulary and ownership sketch, not final SQL. Issues #18–#25 define implemented identity, Finance, and Goal persistence in numbered migrations; other modules remain conceptual:

```text
lifeos_users (one independent owner per admitted LifeOS user; migration 000002)
google_identities (unique Google sub binding; migration 000002)
invitations (admission record; migration 000002)
accounts
categories
financial_events
account_movements
category_allocations
import_batches
goals (migration 000005; owner-scoped)
goal_checklist_items (migration 000005; owner-scoped composite Goal reference)
plans (future)
plan_items
checklist_items
wishlist_items
assets
people
split_expenses
split_shares
settlements
ai_provider_configs
tool_invocation_audits
```

Final tables, constraints, and indexes require dedicated migrations and relevant ADRs. Do not treat this sketch as permission to implement an undifferentiated schema.


## Financial write request metadata (ADR-019)

Accepted [ADR-019](adr/ADR-019-owner-scoped-financial-write-idempotency.md) adds owner-scoped operational success markers in migration v9, separate from domain facts. A client key identifies one logical command; it is not an event/account ID or authorization. Marker and financial mutation commit together. Replays acknowledge prior application without returning a historical DTO or changing current ledger state. The table retains no raw payload, result ID, money or note and is excluded from normal exports. Void/Delete/anchor rules and all domain relationships remain unchanged. Capacity warnings, indefinite retention and safe browser recovery are described in [Financial Write Recovery](FINANCIAL_WRITE_RECOVERY.md).

## Current FinancialEvent revisions (ADR-020)

Accepted [ADR-020](adr/ADR-020-atomic-financial-event-edit-revisions.md) keeps one stable event root and one current movement/allocation set. Posted events can be atomically corrected within their immutable kind/currency. Existing relationship IDs survive corrections; a different account/category receives a new current child ID. The old complete state remains in owner-scoped append-only `financial_event_revisions` snapshots, including after Soft Delete. Snapshots are internal evidence, excluded from ordinary calculations, reads and exports. The v10 baseline honestly records the state at upgrade; it cannot reconstruct earlier unrecorded states.

`event_revision` changes on Edit, Void and first Soft Delete; `balance_revision` separately guards account conversion. New commands use a reviewed event precondition plus an owner-scoped idempotency key. Per-account net old/new contributions obey ADR-018 even when dates/accounts cross anchors. Voided/deleted events cannot be edited, and no operation silently recreates them. Normal export v1/v2 continues to represent current visible state only.
