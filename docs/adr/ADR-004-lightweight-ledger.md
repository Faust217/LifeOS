# ADR-004: FinancialEvent Lightweight Ledger

**Status:** Accepted

**Date:** 2026-08-17

## Context

A model where one transaction always belongs to one account is too limited for transfers, credit-card purchases, credit-card payments, multi-currency travel, and later split expenses. At the same time, LifeOS is a personal-life product rather than a full accounting ERP.

The system needs stable finance semantics that separate the user-visible event, its effects on accounts, and its economic category classification without duplicating spending.

## Decision

The Finance domain will use a lightweight ledger composed of:

- `FinancialEvent`: the user-visible fact that money actually moved or an account position changed;
- `AccountMovement`: a signed effect of the event on one account;
- `CategoryAllocation`: the economic classification of part or all of the event.

An event may contain one or more account movements and zero or more category allocations.

The following invariants apply:

- A transfer has offsetting account movements and is not new income or spending.
- A credit-card purchase increases the liability position and records spending once.
- Paying a credit card moves value between accounts and is not new spending.
- A Plan or PlanItem is future intent and does not affect balances.
- An Asset purchase links to the existing FinancialEvent and does not create another expense.
- A SplitExpense describes economic ownership of a real payment and does not debit the account again.
- Finance calculations are deterministic application/database behavior.

This is not a commitment to a full double-entry general ledger.

## Alternatives Considered

### One transaction row with one account and one category

This is simpler for basic expense entry but handles transfers, split categories, liabilities, and linked domains poorly and encourages duplicated records.

### Full double-entry accounting ledger

A complete accounting system offers stronger formalism, but it would introduce concepts and user experience beyond the accepted personal-life scope.

### Separate unrelated tables for each event type

Independent expense, income, transfer, and card-payment tables could model each case, but reporting and cross-domain links would become fragmented.

## Consequences

### Positive

- Transfers and credit-card behavior have explicit, testable semantics.
- One real payment remains one event even when it affects multiple accounts or categories.
- Plans, assets, and splits can link to finance without duplicating cash flow.
- The structure can grow toward stricter accounting behavior if real needs appear.

### Costs and Trade-Offs

- Creating and editing an event is more complex than inserting one transaction row.
- Balance and reporting queries must aggregate movements and allocations correctly.
- Service-level invariants and integration tests are essential.
- UI terminology may still use familiar words while mapping them to richer internal concepts.

## Data Migration Impact

The first finance schema must represent events, movements, and allocations separately. Bluecoins import must map source rows through canonical application inputs rather than inserting directly into these tables.

Any future move to reversal-only corrections or a stricter ledger must preserve event identity, account effects, category meaning, and audit history through migrations.

## Security and AI Impact

AI may prepare a draft or explain a result, but it is not the authoritative calculator. Tool calls go through Finance application services, which validate balanced transfer behavior, signs, currency, owner scope, and confirmation policy.

## Rollback or Exit Path

If the lightweight ledger proves insufficient, a superseding ADR may introduce stricter ledger semantics. The transition must provide deterministic conversion, reconciliation reports, backward-compatible APIs where practical, and a backup/restore path. Returning to a single undifferentiated transaction table would require explaining how transfers, liabilities, categories, and cross-domain links remain correct.
