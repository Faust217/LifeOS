# LifeOS

LifeOS is a self-hostable, AI-optional Personal Life Data & Decision System. It is intended to connect real money movements, future plans, desired purchases, owned assets, travel, shared expenses, and later life-data modules without making an AI model the source of truth.

> AI is optional. Data is durable. The core remains useful without a model.

## Repository Status

This repository is currently in the **Foundation** milestone. It contains the product, architecture, domain, scope, governance, and learning baseline. No product features or application skeleton have been implemented yet.

See [Project State](docs/PROJECT_STATE.md) for the current handoff point.

## Start Here

1. [Vision](docs/VISION.md) - what LifeOS is, the problems it solves, and what it is not.
2. [Architecture](docs/ARCHITECTURE.md) - the system shape, module boundaries, invariants, and technical baseline.
3. [Domain Model](docs/DOMAIN_MODEL.md) - the meaning and relationships of the core LifeOS concepts.
4. [V0.1 Scope](docs/V0.1_SCOPE.md) - the Finance Core milestone, explicit exclusions, and exit criteria.
5. [Roadmap](docs/ROADMAP.md) - the capability sequence from Foundation to V1.0.
6. [Project State](docs/PROJECT_STATE.md) - completed work, next steps, risks, and open decisions.
7. [ADR Index](docs/adr/README.md) - architecture decision policy and the accepted initial ADR set.
8. [Learning Area](learning/README.md) - non-normative implementation and engineering notes.

Agents must also read [AGENTS.md](AGENTS.md) before making changes.

## Product Baseline

- Personal and self-hosted first; hosted SaaS is not a near-term goal.
- The complete implemented core must remain usable with AI disabled.
- Structured LifeOS data is authoritative; AI is an optional interpretation and controlled-action layer.
- The server is client-agnostic, with a responsive Web/PWA recommended for V0.x.
- The backend baseline is a Go modular monolith backed by PostgreSQL and forward migrations.
- Money is stored as integer minor units plus currency, never as floating-point values.
- Import/export, backup/restore, and safe upgrades are product capabilities rather than afterthoughts.
- Bluecoins history import and reconciliation is a real-world V0.1 acceptance point.

## Source Blueprint

The repository documents were extracted from the preserved Master Project Blueprint v0.2 dated 2026-08-17:

- [English blueprint](docs/reference/LifeOS_Master_Project_Blueprint_EN_v0.2.docx)
- [Chinese blueprint](docs/reference/LifeOS_Master_Project_Blueprint_ZH_v0.2.docx)

The Word files remain the original baseline. Maintained project state and future accepted changes belong in the repository documents and ADRs.

## Current Boundary

There are intentionally no setup, install, or run commands yet. Those belong to a later, explicitly authorized implementation task after the documentation foundation is reviewed.
