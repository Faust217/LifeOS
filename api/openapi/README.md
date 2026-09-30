# OpenAPI Contract

`openapi.yaml` is the canonical external HTTP contract for the endpoints that currently exist. API DTOs are explicit transport contracts and must not expose database rows directly.

Validate it from the repository root with the pinned project-local CLI:

```text
npm run openapi:lint
npm run openapi:generate
npm run openapi:check
```

Operational endpoints such as `/healthz` and `/readyz` remain unversioned because deployment tooling needs stable process checks. Future domain resources are added under `/api/v1` only when their implementation issue is authorized. Prefer additive compatible changes; a genuinely breaking external contract requires a new major API namespace and appropriate migration planning.

The OpenAPI schemas describe request and response DTOs, not PostgreSQL tables. Pinned `openapi-typescript` generates the committed web types under `apps/web/src/api/generated/`; `openapi:check` fails when that output is stale. Generated types do not make it acceptable to expose persistence rows or move business rules into a client.
