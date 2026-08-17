# LifeOS Vision

**Status:** Baseline v0.2 extraction

**Baseline date:** 2026-08-17

**Source:** Master Project Blueprint v0.2

## North Star

LifeOS is a self-hostable, AI-optional Personal Life Data & Decision System. It connects real money movements, future plans, desired purchases, owned assets, travel, shared expenses, and future life-data modules.

AI is an intelligent layer for understanding, querying, recommending, and initiating controlled actions. It is never the source of truth.

> AI is optional. Data is durable. The core remains useful without a model.

LifeOS is not trying to become an everything app in one release. It is building a stable personal core that can grow for years. Every release must be independently useful, preserve existing data, and retain a clear path for later modules.

The first measure of success is not user count. It is whether the owner uses LifeOS over time, can replace the existing expense tracker, and can extend the system without restarting it.

## Problems to Solve

- Personal data is fragmented across expense trackers, notes, e-commerce wishlists, memory, travel documents, and chat-based split calculations.
- General AI assistants lack durable structured personal context, forcing repeated explanations of savings, assets, upcoming plans, and recent spending.
- Traditional expense trackers explain where money went but rarely connect spending to what the user now owns or intends to do next.
- Hosted-only SaaS creates vendor lock-in, recurring AI cost, privacy exposure, and continuity risk.

## Product Promise

LifeOS will provide complete manual workflows for every implemented core module. AI may reduce friction or add interpretation, but it must never create the only path to core data.

| Domain | Complete without AI | Optional AI enhancement |
| --- | --- | --- |
| Finance | Accounts, income, expenses, transfers, categories, balances, reports, import, and export | Natural-language entry, explanations, anomaly and trend insights |
| Plans and travel | Plans, dates, budgets, planned items, checklists, and links to actual transactions | Budget drafts, suggestions, and overspend explanations |
| Wishlist | Desired items, estimated prices, priorities, dates, and plan/asset relationships | Purchase advice grounded in cash flow, assets, and plans |
| Assets | Acquisition, price, warranty, status, condition, and purchase-event links | Replacement and lifecycle reasoning |
| People and split expenses | Participants, shared costs, payers, rules, balances, and settlements | Parsing a conversation into a reviewable split draft |
| Analytics | Dashboards, trends, category comparisons, and actual-versus-plan views | Explanations and decision support based on deterministic summaries |

## Intended User and Modes

The real V0.x user is the project owner. The data model is owner-scoped from day one, but LifeOS will not pre-build organizations, billing, subscriptions, or complex role systems for hypothetical SaaS users.

| Mode | Direction |
| --- | --- |
| Local personal | V0.x baseline: Web/PWA, Go API, and PostgreSQL on the owner's computer; AI may be disabled or use a local provider |
| Self-hosted personal | Progressively hardened for a home server, NAS, or VPS and use from desktop and phone |
| External AI client | Later V0.x adapter through controlled tools such as MCP |
| Hosted SaaS | Explicitly deferred until real demand exists |
| Native mobile | Future option using the same server and business rules when native requirements are proven |

## What LifeOS Is Not

- It is not an early bank aggregator and will not initially collect banking credentials or use Open Banking.
- It is not an investment execution platform and will not place trades.
- It is not a full accounting ERP; finance semantics are reliable, but the experience remains personal-life oriented.
- It is not an AI chat wrapper.
- It is not an immediate health, career, documents, journaling, or general super-app.
- It is not committed to hosted SaaS, subscriptions, organizations, or collaboration in current releases.

New life domains require separate evaluation after the core is stable. They must own their data model, migrations, permissions, and UI and connect through explicit relationships instead of expanding an everything-shaped table.

## Long-Term Success Criteria

| Dimension | Success criterion |
| --- | --- |
| Personal utility | Real transactions, plans, assets, and split expenses are managed continuously |
| Stability | Normal upgrades use migrations and never require deleting the database or re-importing all history |
| Extensibility | New modules enter through explicit domain boundaries and contracts |
| AI independence | Disabling or replacing an AI provider does not break implemented core modules or alter their schemas |
| Portability | Users can export structured data and move a self-hosted installation to another machine |
| Maintainability | Important architecture changes require ADRs, and agents cannot silently bypass module boundaries |

## Product Principles

1. Core works without AI.
2. Structured data is the source of truth.
3. AI never owns or directly accesses the database.
4. Stable domain semantics matter more than a frozen schema.
5. Real data moves forward through migrations, never reset-based upgrades.
6. Import and export are first-class capabilities.
7. Start with a modular monolith, not microservices.
8. Keep the server and core client-agnostic.
9. Make AI permissions explicit, confirmed by risk, and audited.
10. Preserve extension points while keeping current implementation simple.

Changing one of these principles requires an accepted ADR before implementation.

## Definition of V1.0

V1.0 means LifeOS has become a reliable long-term personal system, not that every possible feature exists.

- Finance, Plans/Wishlist, Assets, and Split have stable daily-use core workflows.
- Imported Bluecoins history and later real data remain consistent across multiple migrations.
- AI can be disabled; at least one runtime provider works; write actions are confirmed and audited.
- At least one external integration has a clean boundary and can be disconnected without breaking the core.
- Self-host installation, upgrade, backup, and restore documentation has been rehearsed.
- Complete JSON/CSV export allows the user to leave LifeOS.
- Critical business rules, migrations, and imports are covered by automated tests.
- No normal user workflow requires manual database surgery.

See [ROADMAP.md](ROADMAP.md) for the sequence of capabilities and [V0.1_SCOPE.md](V0.1_SCOPE.md) for the current product milestone.
