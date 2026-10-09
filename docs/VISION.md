# LifeOS Vision

**Status:** Issue #16 product direction approved; ADR-013 and ADR-014 accepted

**Baseline date:** 2026-08-17

**Sources:** Master Project Blueprint v0.2 (historical baseline); Product Owner-approved [Issue #16](https://github.com/Faust217/LifeOS/issues/16) (current direction)

## North Star

LifeOS is a Life-first, mobile-first personal life management platform. It starts with useful everyday Money and Plans workflows and can grow into other life domains without losing stable meanings or user data.

If later introduced, AI may help users understand, query, and act through controlled tools. It is never the source of truth and is not part of the V0.1 Alpha.

> AI is optional. Data is durable. The core remains useful without a model.

LifeOS is not trying to become an everything app in one release. It is building a stable personal core that can grow for years. Every release must be independently useful, preserve existing data, and retain a clear path for later modules.

The first measure of success is whether the Product Owner and invited Alpha testers can use the initial workflows safely and repeatedly. A later Bluecoins import remains important, but it is no longer a V0.1 release gate.

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

## Intended Users and Modes

V0.1 serves the Product Owner and a small group of invited testers. Each person has independent data. Google sign-in verifies identity but does not grant access without a valid invitation. This is a limited Alpha, not public signup or a commitment to organizations, billing, subscriptions, or collaboration.

| Mode | Direction |
| --- | --- |
| Invite-only online Alpha | V0.1: mobile-first React Web/PWA, Go API, and PostgreSQL for the Product Owner and a few testers |
| Local/self-hosted | Keep Foundation Compose and a portable path; production-grade self-host packaging is later work |
| Offline capture | V0.2: reliable transaction capture/synchronization after an accepted sync ADR |
| External AI client | Optional later adapter through controlled tools such as MCP |
| Native mobile | Future candidate; Capacitor is not yet an architectural commitment |

## What LifeOS Is Not

- It is not an early bank aggregator and will not initially collect banking credentials or use Open Banking.
- It is not an investment execution platform and will not place trades.
- It is not a full accounting ERP; finance semantics are reliable, but the experience remains personal-life oriented.
- It is not an AI chat wrapper.
- It is not an immediate health, career, documents, journaling, or general super-app.
- It is not a public-signup SaaS, subscription, organization, or collaboration product in V0.1.
- It is not an AI-dependent product; AI runtime is excluded from the initial Alpha.

New life domains require separate evaluation after the core is stable. They must own their data model, migrations, permissions, and UI and connect through explicit relationships instead of expanding an everything-shaped table.

## Long-Term Success Criteria

| Dimension | Success criterion |
| --- | --- |
| Personal utility | Invited users can manage real money and checklist Goals repeatedly; later modules remain coherent |
| Privacy | Users cannot read or change each other's data; administrative metrics avoid unnecessary private financial detail |
| Stability | Normal upgrades use migrations and never require deleting the database or re-importing all history |
| Extensibility | New modules enter through explicit domain boundaries and contracts |
| AI independence | Disabling or replacing an AI provider does not break implemented core modules or alter their schemas |
| Portability | Users can export structured data and move their data between suitable deployments |
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

## Longer-Term Success

Future release criteria will be set after V0.1 use. Long-term success includes reliable Money and Plans workflows, correct user isolation, portable exports, tested upgrades and restores, and later modules added without duplicating financial facts. AI and external integrations remain optional. No normal workflow should require manual database surgery.

See [ROADMAP.md](ROADMAP.md) for the sequence of capabilities and [V0.1_SCOPE.md](V0.1_SCOPE.md) for the current product milestone.
