# Monorepo Application Skeleton

Date: 2026-08-17

Related task/issue: [Foundation Issue #2](https://github.com/Faust217/LifeOS/issues/2)

Related ADRs: ADR-001, ADR-002, ADR-003, ADR-005, ADR-007, and ADR-008

## What Changed

- Added one root Go module with no third-party dependencies.
- Added a buildable but behavior-free `lifeos-api` command as the future server composition root.
- Added Go package placeholders for the accepted domain, support, integration, and platform boundaries.
- Added a private root web workspace and an empty private `@lifeos/web` package with no dependencies or scripts.
- Reserved the migration, query, OpenAPI, and deployment areas with concise boundary documentation.
- Documented which module owns business rules and why shared storage will not permit casual cross-module table access.

No domain behavior, schema, migration, endpoint, UI, Docker service, or runtime configuration was added.

## Why This Design

A modular monolith needs boundaries that are visible to both Go tooling and reviewers before it needs product code. Small `doc.go` files make each intended Go package real and buildable without inventing services, models, or interfaces prematurely.

One root Go module keeps the backend a single deployable application. The `internal/` location also prevents code outside the parent module tree from importing these packages as a public library. Module ownership still depends on explicit service interfaces, review, and tests; directory structure alone cannot prevent every improper dependency.

The private web workspace records the future client boundary without installing React, TypeScript, Vite, or any package-manager dependency. The reserved database, API-contract, and deployment directories are tracked with explanatory files instead of fake migrations, schemas, or configurations.

## Concepts Learned

- A monorepo can contain several application areas without turning them into separate services.
- A Go module defines a dependency/import unit; a domain module here is an architectural ownership boundary inside that Go module.
- `internal/` provides a Go import-visibility rule, while services/interfaces and table ownership provide the finer domain boundaries.
- The command directory is a composition root: later work may wire adapters and modules there, while business rules stay in the owning packages.
- An npm workspace can declare a future web package without installing dependencies or generating a lockfile.
- Build caches can be redirected with process-only environment values so verification stays repository-local and does not alter machine configuration.

## Recommended Reading Order

1. `go.mod`
2. `package.json` and `apps/web/package.json`
3. `internal/README.md`
4. The `internal/*/doc.go` package placeholders
5. `cmd/lifeos-api/README.md` and `cmd/lifeos-api/main.go`
6. `apps/web/README.md`
7. `db/migrations/README.md`, `db/queries/README.md`, `api/openapi/README.md`, and `deploy/README.md`
8. `docs/PROJECT_STATE.md`

## Checks Performed

- Verified all 18 directories required by Issue #2 exist.
- Listed the root Go module packages and confirmed the API command plus all 12 accepted internal package boundaries are discoverable.
- Built every Go package successfully with repository-local build/module caches.
- Parsed and asserted the root/web package metadata with Node.js; no dependencies are declared.

No behavior tests, database checks, Docker checks, web build, or dependency installation was appropriate because Issue #2 intentionally implements none of those concerns.

## Questions and Follow-Up

- Foundation Issue #3 must choose the migration tool and record ADR-011 before real migration work.
- Foundation Issue #4 must choose the HTTP router and record ADR-012 before API bootstrap work that depends on it.
- Concrete service interfaces should be introduced only with the first authorized use cases, not as speculative empty abstractions.
- The React/TypeScript/Vite toolchain belongs to the later authorized web-shell issue.
