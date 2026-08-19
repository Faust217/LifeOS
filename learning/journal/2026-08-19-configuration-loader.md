# Typed Foundation Configuration

Date: 2026-08-19

Related task/issue: Foundation Issue #7

Related contract: `docs/CONFIGURATION.md`

## What Changed

LifeOS now has a small typed configuration package at `internal/platform/config`. It resolves the ten accepted Foundation keys, validates the complete result, and returns one `Config` value for a future API entry point.

The tests use temporary `.env` files and an injected environment lookup, so they never read or change the developer's real environment.

## Why This Design

- Standard-library parsing keeps the package project-local and dependency-free.
- Nested API, database, logging, and web structures keep platform settings organized without adding domain settings.
- Values are merged first and validated second, so the same rules apply regardless of which source supplied a value.
- Errors name configuration keys but never echo the database password or malformed values.

## Concepts Learned

- Configuration precedence can be modeled as ordered overlays: defaults, then `.env`, then process environment.
- `os.LookupEnv` distinguishes an unset variable from a deliberately empty higher-precedence value.
- Dependency injection can be very small: passing a lookup function makes tests deterministic without changing global environment variables.
- Validation at the configuration boundary prevents an application from starting with unsafe partial state.
- `net/netip`, `net/url`, and integer range checks turn raw strings into safer typed values.

## Recommended Reading Order

1. `docs/CONFIGURATION.md`
2. `internal/platform/config/config.go`
3. `internal/platform/config/dotenv.go`
4. `internal/platform/config/config_test.go`

## Checks Performed

- `gofmt` on all configuration Go files
- `go test ./internal/platform/config`
- `go test ./...`
- `go vet ./...`

## Questions and Follow-Up

- Issue #9 can wire `config.Load` into the future API startup path only after its remaining dependencies are complete and it is explicitly authorized.
- Domain preferences, AI/MCP settings, remote authentication, TLS, and hosted secret management remain deferred to their own feature boundaries.
