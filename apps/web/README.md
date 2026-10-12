# LifeOS Web Application

This directory contains the mobile-first React/TypeScript LifeOS application. It presents Today, Money, Plans, and Me while authoritative business rules remain in Go application services.

Issue #23 uses pinned React, TypeScript, and Vite without an added UI or routing library. The four navigation links use normal browser URLs and history. Vite and the existing nginx `try_files` fallback serve the same app for direct links. Styling stays in `src/lifeos.css`; the small SVG icons are local components. API calls stay behind `src/api`.

The shell confirms `/api/v1/me` before rendering personal pages. A missing session shows the invited-account sign-in link; an API error hides personal pages and offers a retry. Me displays the confirmed profile and signs out through the existing CSRF-protected endpoint. Issue #24 adds account/category setup, quick income/expense/transfer entry, history, void with optional reviewed replacement, archive, and restore workflows to Money. The local Issue #25 Plans overview shows compact Goal cards; `/plans/goals/{id}` shows the full owner-scoped checklist, management actions, and restore for archived Goals.

Run commands from the repository root with the inspected npm toolchain:

```text
npm install
npm run web:dev
npm run web:typecheck
npm run web:test
npm run web:build
```

`npm install` uses the committed root `package-lock.json`. Dependencies are repository-local under ignored `node_modules/`, and production output is generated under ignored `apps/web/dist/`.

To preview the layout without changing a personal database, run `npm run web:dev` from the repository root and open `http://127.0.0.1:5173/`. Direct URLs `/today`, `/money`, `/plans`, and `/me` work. Vite proxies `/api` to the existing local Go API at `http://127.0.0.1:8080`. If the API is unavailable, the app shows a connection error rather than private content. The personal database may still be at migration v3; Money needs v4 and Goals needs v5. Do not run migration or reset commands for this preview. Real browser testing against the personal database requires separate Product Owner approval for a backup and non-destructive upgrade plan.

For real local Google sign-in, use [AUTH_LOCAL_SETUP.md](../../docs/AUTH_LOCAL_SETUP.md). The Vite origin requires its exact Google callback URI and `LIFEOS_AUTH_PUBLIC_ORIGIN=http://127.0.0.1:5173` in the ignored local `.env`; the existing Compose origin is 4173. Keep OAuth secrets out of `VITE_*` values, Git, logs, and screenshots.

API types are generated from the canonical OpenAPI document and committed under `src/api/generated/`. Money calls in `src/api/money.ts` use these generated schemas. Regenerate and verify them from the repository root:

```text
npm run openapi:generate
npm run openapi:check
```

Do not edit generated files by hand. Future domain endpoint types follow the same OpenAPI-to-generated-output-to-API-boundary path.
