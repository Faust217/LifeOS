# LifeOS Web Application

This directory is the boundary for the future responsive React/TypeScript Web/PWA client. It may present and validate user input, but authoritative business rules remain in the Go application services.

The Foundation shell uses pinned React, TypeScript, and Vite dependencies without a component library or product screen. API calls stay behind `src/api`; components do not own server or domain rules.

Run commands from the repository root with the inspected npm toolchain:

```text
npm install
npm run web:dev
npm run web:typecheck
npm run web:test
npm run web:build
```

`npm install` uses the committed root `package-lock.json`. Dependencies are repository-local under ignored `node_modules/`, and production output is generated under ignored `apps/web/dist/`.

The development shell uses `VITE_API_BASE_URL` from the root environment, defaulting to `http://127.0.0.1:8080`. Browser-visible `VITE_*` values must never contain secrets.

Foundation API types are generated from the canonical OpenAPI document and committed under `src/api/generated/`. Regenerate and verify them from the repository root:

```text
npm run openapi:generate
npm run openapi:check
```

Do not edit generated files by hand. Future domain endpoint types follow the same OpenAPI-to-generated-output-to-API-boundary path.
