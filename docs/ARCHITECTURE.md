# LifeOS Architecture

**Status:** Architectural baseline v0.2

**Baseline date:** 2026-08-17

**Source:** Master Project Blueprint v0.2

## Architectural Goal

LifeOS must remain useful, portable, and understandable across years of personal data, additional modules, changing clients, and changing AI providers. The system therefore stabilizes the server, domain semantics, contracts, and data migration path before optimizing any one client.

The baseline is:

- server-first and client-agnostic;
- a Go modular monolith, not microservices;
- PostgreSQL with numbered forward migrations;
- REST and OpenAPI for external application contracts;
- a responsive React/TypeScript Web/PWA for V0.x;
- optional AI behind a controlled gateway;
- Docker Compose as the initial self-host packaging path.

## System Context

```text
Web/PWA       Future native/CLI       Runtime AI       External MCP client
    \                |                    |                    /
                 Stable API and Tool Contracts
                              |
                    Go Modular Monolith
                              |
        Domain/Application Services and Policy Checks
                              |
                  Repositories and PostgreSQL
```

Clients may present different layouts, but all business behavior flows through the same application services. The dashboard is a projection and interaction surface over domain data; its card order, visibility, size, and grouping are preferences rather than domain facts.

## Recommended V0.x Stack

| Concern | Baseline | Constraint |
| --- | --- | --- |
| Web client | React, TypeScript, Vite, responsive/PWA | Replaceable client; business rules remain server-side |
| Native client | Flutter, only as a future option | Start only after a proven native requirement |
| Backend | Go modular monolith | Explicit modules and services; no microservices by default |
| HTTP/API | REST and OpenAPI | Thin handlers and generated typed-client path; no initial GraphQL requirement |
| Database | PostgreSQL | Pin a supported major version and preserve upgradeability |
| SQL access | `pgx` and `sqlc` | Explicit SQL and compile-time types; avoid excessive ORM magic |
| Migrations | `golang-migrate` or `goose` | Deferred choice requiring an ADR before the first real migration |
| Runtime AI | Provider-neutral AI Gateway; Ollama is the first experimental path | `None` is always a valid provider |
| External AI | MCP adapter over the shared Tool Registry | Optional adapter, never a core dependency |
| Packaging | Docker Compose | Initial local, NAS, and VPS self-host path |

Specific versions, models, prices, and provider quotas are configuration and release concerns, not permanent architectural facts.

## Modular Monolith

The intended module boundaries are:

```text
identity
finance
planning
wishlist
assets
people
split
analytics
importing
ai
integrations/mcp
platform        # database, config, logging, and auth adapters
```

Likely repository areas, when implementation is authorized, are:

```text
apps/web/
cmd/lifeos-api/
internal/<module>/
db/migrations/
db/queries/
api/openapi/
deploy/
```

This layout is directional, not implemented in this foundation task.

### Boundary Rules

- A module owns its domain rules and persistence behavior.
- Modules communicate through explicit services or interfaces, not casual cross-module table access.
- Sharing one PostgreSQL database does not erase module boundaries and does not require separate databases.
- Business validation occurs in application services, not only in clients, SQL triggers, or AI prompts.
- Analytics may read stable domain projections but does not redefine the underlying facts.
- Optional modules and dashboard widgets must fail or disappear independently rather than breaking the core.

## Layer Responsibilities

| Layer | Responsibility |
| --- | --- |
| Client | Presentation, user interaction, and local layout preferences |
| API/adapter | Authentication, transport validation, DTO mapping, and stable contract behavior |
| Application service | Use-case orchestration, authorization, confirmation, and business invariants |
| Domain module | Durable meaning, calculations, state transitions, and module-owned rules |
| Repository | Explicit persistence operations scoped to an owner |
| PostgreSQL | Authoritative structured data, relations, constraints, and migration version |

Database rows are not API DTOs. Client contracts are defined through OpenAPI. AI and MCP tools use registered JSON schemas and route to the same application services.

## Data and Migration Invariants

- Use application-generated UUIDs, preferably UUIDv7, for externally visible identities.
- Include `owner_id` on all core records even while V0.x remains a single-user product.
- Store `created_at` and `updated_at`; audited entities also use void/delete metadata or audit events.
- Store timestamps in UTC and retain user timezone separately; use calendar dates for date-only plan/travel concepts.
- Store money as `amount_minor` integer plus ISO 4217 currency, never float or double.
- Use controlled state values rather than arbitrary free-form states.
- Give every schema change a numbered migration.
- Never modify a migration after it has shipped; correct it with a later migration.
- CI must build an empty database from the full chain and test upgrades from the previous stable schema.
- Use expand, migrate data, then contract for breaking data changes.
- Back up before deployment changes and make large migrations observable.

Stable architecture does not mean a frozen schema. It means domain meanings remain documented, history is preserved, and change is migratable.

## Finance Architecture

Finance is the lowest-level and most stability-sensitive domain. The baseline is a lightweight ledger:

```text
FinancialEvent
  -> one or more AccountMovements
  -> zero or more CategoryAllocations
```

This supports expenses, income, transfers, credit-card liabilities, and later split-expense links without assuming one visible event always affects exactly one account. It is not intended to become a full double-entry ERP.

Detailed meanings and invariants are in [DOMAIN_MODEL.md](DOMAIN_MODEL.md).

## API Contracts

The initial REST namespace is `/api/v1`. Intended resource areas include accounts, financial events, categories, plans, wishlist items, assets, people, splits, analytics, imports, and exports as their roadmap milestones arrive.

- OpenAPI is the external application contract.
- Prefer additive, backward-compatible changes.
- Use a new major API version only for a true breaking change.
- Return stable machine-readable error codes with user-readable messages.
- Never expose database table schemas directly as a client contract.

## AI Architecture

Development AI, runtime AI, and an external AI client are separate systems:

| Layer | Role | Core dependency? |
| --- | --- | --- |
| Development AI | Helps plan, code, test, and review | No |
| Runtime AI | Optional provider invoked by the LifeOS AI Gateway | No; `None` must remain complete |
| External AI client | Calls registered LifeOS tools through an adapter such as MCP | No |

### Tool Execution Chain

```text
User request
  -> AI client/model proposes a tool call
  -> Tool Registry validates registration and schema
  -> Policy/Context layer checks owner scope and permission
  -> explicit confirmation when required
  -> Application Service enforces domain rules
  -> Repository persists to PostgreSQL
  -> audit and structured result
```

### Capability Tiers

| Tier | Examples | Default policy |
| --- | --- | --- |
| R0 Read | Summaries, assets, and plans | May run automatically within owner scope |
| R1 Draft/Create | Prepare a transaction or create a wishlist item | Structured preview and confirmation before execution |
| R2 Modify | Update a financial event or asset lifecycle | Explicit confirmation and audit |
| R3 Destructive/Bulk | Delete records, bulk rewrite, import rollback | Second confirmation; disabled for external AI by default |

Models never receive database credentials, unrestricted SQL, another owner's data, provider secrets, arbitrary server filesystem access, or unregistered destructive capabilities.

The Context Builder sends only task-relevant facts and deterministic summaries. SQL/Go performs money arithmetic, split netting, aggregations, and budget deltas; models interpret and explain results.

## Import and Export

Import adapters map external data into canonical application inputs and never insert directly into core tables.

```text
Source file
  -> source adapter/parser
  -> normalized import record
  -> validation and explicit mapping
  -> dry-run preview
  -> confirmed ImportBatch with idempotency/fingerprints
  -> Finance Application Service
  -> PostgreSQL
```

Every import run records its source, time, counts, status, errors, and batch identity. Controlled batch rollback or voiding replaces manual SQL repair.

Exports must include complete structured JSON for core data and useful CSV finance exports. Portability is a core capability.

## Security and Privacy

- Scope every query to the authenticated owner; never trust an arbitrary client-supplied owner ID.
- Localhost mode may start simply, but any network-exposed installation requires secure authentication, HTTPS, and backup.
- Keep provider secrets in environment or secret files and never return them to clients or models.
- Remote MCP requires independent authentication and conservative write permissions.
- Never log API keys, database passwords, or full sensitive prompts by default.
- Future uploads must validate MIME type, size, and paths and must not trust filenames.
- A model's tool request is not authorization; policy and application services decide.

## Deployment and Operations

Deployment grows only as real needs appear:

1. Development: PostgreSQL, API, and web through Docker Compose; Ollama external or optional.
2. Local personal: one repeatable startup path on localhost or trusted LAN.
3. Home server/NAS: persistent volumes, backup, and secure access.
4. VPS self-host: reverse proxy, TLS, authentication, backup, and upgrade guide.
5. Hosted SaaS: deferred; no billing or AI-quota architecture now.

Backups must be documented, run before upgrades, and periodically restore-tested. Health endpoints should expose liveness, readiness, and migration version. Logs should be structured and privacy-safe.

## Reliability and Scale

The relevant V0.x scale is years of personal data and growing modules, not a million concurrent users.

- Start with indexes, pagination, SQL aggregation, and measurable query behavior.
- Add materialized views only after demonstrated slow queries.
- Add a queue only for proven large imports, OCR, notification, or background workloads.
- Add Redis only after measurement shows a cache is needed.
- Do not add a vector database by default; task-specific summaries and context come first.
- Abstract object storage only when attachments become a real capability.

## Change Gate and Deferred Decisions

Important domain or technical changes require an ADR. The initial ADR backlog is recorded in [adr/README.md](adr/README.md).

The following remain intentionally open until their decision point:

- Web UI component library: before the first production UI screen.
- `golang-migrate` versus `goose`: before the first real migration.
- Go HTTP router: during Foundation implementation; standard library or `chi` are acceptable if handlers stay thin.
- Remote authentication approach: before Internet exposure.
- First Ollama model: during V0.3 based on hardware and tool-calling evaluation.
- Open-source license: before the first public release.
- Flutter client: only after real Web/PWA usage proves a native need.
