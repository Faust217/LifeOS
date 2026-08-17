# LifeOS Roadmap

**Status:** Baseline v0.2

**Baseline date:** 2026-08-17

## Roadmap Rule

Versions describe usable capability, not date promises. Every stage must be valuable in daily use, preserve earlier data, and grow through migrations rather than restart the project.

Unplanned ideas go to a backlog unless they change core semantics. They should not interrupt the current milestone.

## Capability Sequence

| Release | Goal | Major capability |
| --- | --- | --- |
| Foundation | Build the base | Repository, governance, Docker, PostgreSQL, migrations, Go module boundaries, OpenAPI, CI, and ADRs |
| V0.1 Finance Core | Become a real starting replacement for Bluecoins | Accounts, categories, financial events, transfers, dashboard, Bluecoins CSV import, CSV/JSON export, and backup |
| V0.2 Plans and Wishlist | Connect current money to future intent | Plans, plan items, budgets, transaction links, and wishlist |
| V0.3 AI Foundation | Make optional AI real | AI Gateway, Ollama adapter, structured extraction, read analytics, confirmation, and audit |
| V0.4 Assets | Know what is owned | Asset lifecycle, purchase-event links, and replacement wishlist relationships |
| V0.5 People and Split | Support travel and daily shared expenses | People, split expenses, shares, settlements, and plan participants |
| V0.6 MCP Integration | Use LifeOS from compatible AI clients | Remote MCP, read tools, controlled writes, and authentication |
| V0.7 Self-host Hardening | Let another person follow a reliable tutorial | Compose/release packaging, upgrades, backups, and security guidance |
| V1.0 Personal Stable | Become a reliable long-term personal system | Real data survives upgrades; critical modules have backup, restore, tests, and documentation; SaaS remains optional |

## Foundation

Foundation establishes durable project memory and the technical base. The documentation/governance portion is now present; implementation portions require separate authorization.

Expected capability:

- repository documents and agent rules;
- accepted ADRs for the initial architecture baseline;
- application and deployment skeleton;
- PostgreSQL migration discipline;
- OpenAPI and CI baselines.

## V0.1 Finance Core

The current product milestone is defined in [V0.1_SCOPE.md](V0.1_SCOPE.md). Its real-world acceptance path is safe Bluecoins import, reconciliation, correct finance behavior, export, and restored backup.

No AI runtime, plans, wishlist, assets, split expenses, native client, or SaaS work belongs in V0.1.

## Later Milestone Boundaries

### V0.2 - Plans and Wishlist

Introduce future intent without changing the meaning of real money. Plans and estimated items never affect balances until linked to actual FinancialEvents.

### V0.3 - AI Foundation

Add an optional provider-neutral AI Gateway. `None` remains a complete mode. Start with an Ollama experiment, task-specific context, deterministic calculations, permission tiers, confirmation, and auditing.

### V0.4 - Assets

Track ownership and lifecycle, link assets to purchase events, and connect replacement candidates through Wishlist relationships without duplicating expenses.

### V0.5 - People and Split

Separate cash flow from economic share through People, SplitExpenses, SplitShares, and Settlements. Arithmetic remains deterministic.

### V0.6 - MCP Integration

Expose the shared Tool Registry through a separately authenticated external adapter. External AI remains optional and destructive capabilities remain disabled by default.

### V0.7 - Self-host Hardening

Rehearse repeatable install, upgrade, backup, restore, and security paths for local, NAS/home-server, and VPS use.

## V1.0 Outcome

V1.0 is reached when:

- core Finance, Plans/Wishlist, Assets, and Split workflows are stable for daily use;
- real history survives multiple migrations;
- AI can be disabled and at least one provider works safely when enabled;
- an external integration can disconnect without breaking the core;
- self-host operations have been rehearsed;
- complete export preserves the user's exit path;
- critical rules, migrations, and imports are tested;
- normal workflows never require manual database surgery.

## Future Domains

Health/Wellness may be evaluated only after the core is stable and proven useful. If pursued, it must be an independent module with its own model, migrations, permissions, and UI. Health data is more sensitive and requires explicit AI opt-in and separate privacy policy. LifeOS must not present itself as a diagnostic or medical-advice system without dedicated medical and compliance design.

Other future life-data domains follow the same rule: prove the user need, define the boundary, and avoid an everything-shaped schema.
