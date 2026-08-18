# LifeOS Foundation Decision Notes

**Purpose:** A non-normative revision guide for remembering why early Foundation choices were made. The accepted ADRs remain the source of truth.

## ADR-011 — Database migration tool

### Decision

- **Choice:** `golang-migrate` v4
- **Alternative considered:** `goose`

### What was I choosing?

A migration tool applies ordered database-change files and records which version the database has reached. This makes a new database reproducible and lets an existing database upgrade without deleting real data.

### Why this choice?

- `golang-migrate` is a focused file-based runner that fits LifeOS's simple sequential migration chain.
- Its explicit version and dirty-state model makes progress and failures visible.
- It supports a pinned project-local command and an official Docker execution path without requiring a global installation.
- `goose` is capable, but LifeOS does not currently need its Go-function migrations, embedded files, seeding helpers, or broader workflow options.

### Concepts to remember

- **Forward migration:** an `up` change that moves the schema to the next version. A correction is a new later migration; a shipped migration is not rewritten or deleted.
- **Migration version:** the identifier showing the last migration the database attempted or completed, used to determine what runs next.
- **Dirty state:** a fail-closed warning that a migration failed and the database may be partially changed. Inspect and repair or restore the database before deliberately correcting the version metadata; `force` is not a repair.
- **Project-local pinning:** the repository chooses an exact reviewed tool version through its own wrapper or Docker image tag instead of relying on a global command or `latest`.
- Transactions are desirable where PostgreSQL supports them, but the migration author must make transaction boundaries explicit.

### Interview / implementation checkpoint

- Be able to explain migrations as versioned, reviewable schema history rather than one-time setup scripts.
- Explain why immutable shipped migrations protect reproducible upgrades and why destructive rollback may require backup/restore or a corrective forward migration.
- During implementation, pin the runner, protect database credentials, and verify both clean-database creation and supported upgrade paths.

### When might this change?

A switch could be justified if LifeOS later has a demonstrated need for another tool's capabilities or operational model. Any replacement must preserve version identity, ordered history, transaction intent, and real-data upgrades without resetting the database.

### Current LifeOS status

The tool decision is accepted. `golang-migrate` has not been installed or executed, and no migration workflow or migration files have been implemented yet.

## ADR-012 — Go HTTP router

### Decision

- **Choice:** `chi` v5
- **Alternative considered:** modern `net/http.ServeMux`

### What was I choosing?

An HTTP router matches an incoming request's method and path to the handler that should process it. The choice was between Go's capable zero-dependency `ServeMux` and the small `chi` router.

### Why this choice?

- `chi` gives a growing modular monolith clear route grouping, mounting, nested routes, and middleware composition.
- It remains based on standard `net/http` interfaces, so normal handlers and `httptest` still apply.
- Its small dependency cost is justified by clearer transport organization; LifeOS is not adopting a large web framework.

### Concepts to remember

- **Standard HTTP model:** `chi` routes standard `http.Handler` values using `http.Request` and `http.ResponseWriter`; learning `chi` does not replace learning `net/http`.
- **Thin handlers:** handlers deal with transport concerns such as request parsing, DTO validation/mapping, authentication context, and HTTP responses. Use-case orchestration, authorization decisions, and business rules stay in application/domain services.
- Router-specific types should not leak into application or domain packages.
- **External contract:** OpenAPI defines what API clients can rely on. Router declarations implement that contract but do not replace it.
- Middleware order affects security and behavior, and `chi` does not provide authentication or authorization by itself.

### Interview / implementation checkpoint

- Be able to separate routing, transport adaptation, application use cases, and domain rules.
- Explain why thin handlers improve testing, client independence, and the ability to replace infrastructure.
- During implementation, pin `chi` v5 locally, test handlers with `httptest`, and review middleware order and OpenAPI behavior.

### When might this change?

Returning to `ServeMux` could make sense if the API stays small enough that `chi` adds no practical value. Another router could be justified by a concrete requirement. Keeping standard `net/http` boundaries and business logic outside the router makes either change mostly a transport-layer rewrite.

### Current LifeOS status

The router decision is accepted. `chi` has not been installed, and no API routes, handlers, middleware, or OpenAPI implementation have been created yet.
