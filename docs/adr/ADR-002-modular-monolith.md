# ADR-002: Go Modular Monolith

**Status:** Accepted

**Date:** 2026-08-17

## Context

LifeOS will eventually contain several domains, including identity, finance, planning, wishlist, assets, people, split expenses, analytics, importing, AI, and external integrations. These domains need clear ownership, but V0.x is a personal, self-hosted product rather than a high-concurrency distributed system.

Starting with microservices would add deployment, networking, observability, data-consistency, and operational complexity before there is a demonstrated need. A completely unstructured monolith, however, would allow domain rules and tables to become tangled.

## Decision

LifeOS will use one Go backend organized as a modular monolith.

- Domain modules own their rules and persistence behavior.
- Modules communicate through explicit services or interfaces.
- A module must not casually reach into another module's tables.
- Business validation belongs in domain/application code, not only in clients or database scripts.
- Modules may share one PostgreSQL database; modular boundaries do not require separate databases.
- Cross-cutting adapters such as database access, configuration, logging, and authentication belong in an explicit platform area.
- Microservices will not be introduced without a real operational need and a superseding ADR.

## Alternatives Considered

### Unstructured monolith

Keep one Go application without enforced module ownership. This minimizes initial ceremony but makes it easy for finance, planning, AI, and integrations to become tightly coupled.

### Microservices from the start

Give each domain a separately deployed service and possibly a separate database. This provides strong physical boundaries but creates unnecessary distributed-system and self-hosting complexity for V0.x.

## Consequences

### Positive

- Local development, testing, deployment, backup, and self-hosting stay comparatively simple.
- Explicit module boundaries preserve a path for long-term growth.
- Cross-domain behavior can be coordinated transactionally inside one application and database.
- The owner learns and operates one backend system rather than a service fleet.

### Costs and Trade-Offs

- Boundaries depend on code structure, review, tests, and agent discipline rather than network isolation.
- The application is deployed as one unit even if only one module changes.
- Care is required to prevent shared-database access from bypassing module ownership.

## Data Migration Impact

There is no current application schema to migrate. Future migrations may create tables in one PostgreSQL database, but table ownership must remain clear. Moving a module into a separate service later would require an explicit data-ownership and migration plan.

## Security and AI Impact

A modular monolith does not replace authorization. Every module must enforce owner scope and use controlled services.

AI and MCP code remain separate modules/adapters and may invoke domain behavior only through registered application services. Sharing one process is not permission to bypass the Tool Gateway or policy checks.

## Rollback or Exit Path

If measured scale, independent deployment, fault isolation, or team ownership later justifies service extraction, explicit module interfaces provide a starting seam. A superseding ADR must identify the service boundary, data owner, consistency model, deployment cost, and migration/rollback process.
