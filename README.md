# LifeOS

LifeOS is a Life-first, mobile-first personal life management platform. It connects everyday money and plans through durable structured data, while leaving room for later life domains. The core remains useful without AI.

> AI is optional. Data is durable. The core remains useful without a model.

## Repository Status

The **Foundation** implementation is complete and was verified locally. Issues #18–#22 added invitation-gated Google sign-in, secure sessions, owner-scoped authorization, the Money domain, and authenticated Money APIs. [Issue #23](https://github.com/Faust217/LifeOS/issues/23) added the mobile-first Today, Money, Plans, and Me shell. [Issue #24](https://github.com/Faust217/LifeOS/issues/24) adds usable Money forms and history, verified end to end with a disposable PostgreSQL database. [Issue #25](https://github.com/Faust217/LifeOS/issues/25) adds persisted checklist Goals with overview/detail and archive/restore; export and tested backup/restore are still V0.1 work. The Product Owner approved the new direction in [Issue #16](https://github.com/Faust217/LifeOS/issues/16) and accepted ADR-013 and ADR-014 for the [Issue #17](https://github.com/Faust217/LifeOS/issues/17) documentation rebaseline.

See [Foundation Startup](docs/FOUNDATION_STARTUP.md) for the clean local workflow and [Project State](docs/PROJECT_STATE.md) for the current review and authorization boundary.

## Start Here

1. [Vision](docs/VISION.md) - what LifeOS is, the problems it solves, and what it is not.
2. [Architecture](docs/ARCHITECTURE.md) - the system shape, module boundaries, invariants, and technical baseline.
3. [Domain Model](docs/DOMAIN_MODEL.md) - the meaning and relationships of the core LifeOS concepts.
4. [V0.1 Scope](docs/V0.1_SCOPE.md) - the invite-only online Alpha, explicit exclusions, and exit criteria.
5. [Roadmap](docs/ROADMAP.md) - the capability sequence from Foundation to V1.0.
6. [Project State](docs/PROJECT_STATE.md) - completed work, next steps, risks, and open decisions.
7. [ADR Index](docs/adr/README.md) - architecture decision policy and the accepted initial ADR set.
8. [Foundation Startup](docs/FOUNDATION_STARTUP.md) - clean configuration, verification, startup, and shutdown workflow.
9. [Learning Area](learning/README.md) - non-normative implementation and engineering notes.
10. [Codebase Map](docs/CODEBASE_MAP.md) - current entry points and feature call paths.
11. [Development Guide](docs/DEVELOPMENT_GUIDE.md) - safe local startup, tests, migrations, and troubleshooting.

Agents must also read [AGENTS.md](AGENTS.md) before making changes.

## Product Baseline

- V0.1 is an invite-only, online Alpha for the Product Owner and a small group of testers. Google sign-in alone does not grant access without an invitation.
- Each user's data is isolated. Organizations, billing, and public signup are not V0.1 work.
- The complete implemented core must remain usable with AI disabled.
- Structured LifeOS data is authoritative; AI is an optional interpretation and controlled-action layer.
- Today, Money, Plans, and Me are the V0.1 navigation. React Web/PWA comes first; Capacitor is only a future candidate.
- The server remains client-agnostic while the first client is mobile-first.
- The backend baseline is a Go modular monolith backed by PostgreSQL and forward migrations.
- Money is stored as integer minor units plus currency, never as floating-point values.
- Import/export, backup/restore, and safe upgrades are product capabilities rather than afterthoughts.
- V0.1 includes manual Money workflows, simple checklist Goals, suitable export, isolation tests, and tested backup/restore. Offline transaction capture and Wishlist follow in V0.2; Bluecoins import is V0.3 or later.

## Source Blueprint

The repository documents were extracted from the preserved Master Project Blueprint v0.2 dated 2026-08-17:

- [English blueprint](docs/reference/LifeOS_Master_Project_Blueprint_EN_v0.2.docx)
- [Chinese blueprint](docs/reference/LifeOS_Master_Project_Blueprint_ZH_v0.2.docx)

The Word files remain the historical source baseline. The Product Owner-approved Issue #16 rebaseline takes precedence for current product scope; maintained documents and accepted ADRs carry the durable implementation rules.

## Current Boundary

The current web shell presents four real destinations, confirms the authenticated session before revealing personal pages, and offers working sign-out. The Money screen calls authenticated APIs and shows only real owner-scoped data. Plans uses owner-scoped Goal APIs for checklists and progress. Export, backup/restore, Bluecoins import, AI, and MCP require separately authorized work. The personal database may still need a separately approved, backed-up migration through v5 before this build can run against it. See [web preview instructions](apps/web/README.md).
