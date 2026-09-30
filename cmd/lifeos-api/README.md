# LifeOS API Command

This directory is the composition root for the single LifeOS Go server. Composition and process startup belong here; domain business rules belong in their owning `internal/` modules.

The Foundation server validates configuration before listening and exposes only operational endpoints:

- `GET /healthz` reports process liveness.
- `GET /readyz` verifies PostgreSQL connectivity and requires the clean current migration version.

Run from the repository root after PostgreSQL is healthy and migrations are current:

```text
go run ./cmd/lifeos-api
```

Responses use stable JSON status/error fields. Logs are structured JSON with a request identifier and never include the database URL or password. Product endpoints, authentication, and domain behavior remain out of scope.
