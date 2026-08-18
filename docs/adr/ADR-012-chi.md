# ADR-012: chi v5 for HTTP Routing

**Status:** Accepted

**Date:** 2026-08-18

## Context

LifeOS is a server-first Go modular monolith whose external application contract is REST/OpenAPI. The HTTP layer needs routing and middleware composition without becoming a second home for business rules.

Modern Go `net/http.ServeMux` and `chi` are both capable REST routers. Both use the standard Go HTTP model, so handlers can use `http.Handler`, `http.Request`, `http.ResponseWriter`, and normal `httptest` tooling. `ServeMux` has no third-party dependency cost. `chi` adds a small dependency while providing clearer composition for route groups, nested resources, and middleware.

LifeOS is expected to grow into multiple API route groups and explicit middleware layers. The Product Owner selected `chi` v5 for that transport-level composition while preserving standard-library HTTP concepts and application-service ownership of behavior.

In beginner-friendly terms, the router decides which handler receives an HTTP request. It does not decide what a financial event means, whether an action is allowed, or how a use case works.

## Decision

LifeOS will use the `chi` v5 major line as the initial HTTP router for the Go server.

- `chi` is a transport/router concern only.
- HTTP handlers remain thin. They may handle authentication context, transport validation, request/response DTO mapping, and HTTP status/error translation.
- Use-case orchestration, authorization decisions, and business rules belong in application services and domain modules.
- Router-specific APIs, route contexts, and middleware types must not leak into domain or application packages. Handlers translate transport values into ordinary application inputs.
- OpenAPI remains the external API contract and source of truth. Route declarations must implement that contract; they do not replace or redefine it.
- Middleware ordering must remain explicit and reviewable, especially around authentication, owner context, authorization, recovery, and logging.
- Handler and middleware tests should continue to use standard `net/http` and `httptest` concepts where practical.

The architecture accepts the `chi` v5 major line, not an unpinned moving dependency. The exact `github.com/go-chi/chi/v5` version will be added and pinned project-locally in `go.mod` only when API implementation is explicitly authorized. No global installation or PATH change is required.

## Alternatives Considered

### Modern `net/http.ServeMux`

Modern `ServeMux` supports method-aware and parameterized route patterns, uses only the standard library, and is capable enough for a REST API. It would avoid a third-party dependency and remain fully compatible with normal `net/http` testing.

It was not selected because LifeOS is expected to gain several module-oriented route groups and middleware layers. Those needs can be implemented with `ServeMux`, but `chi` provides clearer grouping, mounting, and middleware composition with a small dependency cost.

### `chi` v5

`chi` preserves the standard `net/http` model while adding focused routing and middleware composition. It is selected because that balance fits a growing modular monolith without adopting a larger web framework.

## Consequences

### Positive

- Route groups can mirror API/module boundaries without turning them into separate services.
- Middleware can be composed explicitly at global, group, or route scope.
- Standard `net/http` handler interfaces and `httptest` remain usable.
- Thin transport adapters preserve one set of application and domain rules for all clients.

### Costs and Trade-Offs

- `chi` adds a third-party dependency that must be pinned and deliberately upgraded.
- Review is still required to prevent router contexts or middleware from accumulating business logic.
- Middleware order can affect security and behavior, so composition must be tested when implementation begins.
- Choosing `chi` does not generate or validate the OpenAPI contract automatically.

## Data Migration Impact

There is no database, schema, persistent-data, or migration impact. Selecting or replacing an HTTP router must not require domain-data migration.

## Security and AI Impact

Selecting `chi` does not provide authentication, authorization, owner isolation, input validation, or secure defaults by itself.

- Authentication and owner context must be established before protected handlers execute.
- Authorization and domain invariants remain enforced by application services, even when middleware performs an earlier transport-level check.
- Middleware ordering and route-group scope must make security boundaries explicit and testable.
- Logs and error handling must not expose secrets or sensitive personal data.

There is no direct AI-permission change. Runtime AI and MCP continue to call controlled application services and cannot bypass authorization through the HTTP router.

## Rollback or Exit Path

Handlers will use standard `net/http` concepts and keep business logic outside router-specific code. Replacing `chi` with modern `ServeMux` or another compatible router should therefore primarily change route registration, parameter extraction, and middleware composition in the transport layer.

A replacement must preserve OpenAPI behavior, authentication and authorization boundaries, error contracts, and handler tests. If a future change would move business authority into a router or framework, it requires a superseding ADR.
