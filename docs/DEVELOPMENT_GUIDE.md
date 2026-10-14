# LifeOS Development Guide

Use this with the [Codebase Map](CODEBASE_MAP.md), [Project State](PROJECT_STATE.md), and [local auth setup](AUTH_LOCAL_SETUP.md). Commands below are present in this repository. Run them from the repository root in PowerShell.

## Prerequisites and safety

- Go 1.26 (see [`go.mod`](../go.mod)), Node.js 24 and npm 11 (see [`package.json`](../package.json) and the [recorded environment](DEVELOPMENT_ENVIRONMENT.md)), Docker Desktop/Compose and PostgreSQL 18 for integration tests or Compose.
- Project-local dependencies: `go mod download` and `npm ci`. They populate local Go caches and `node_modules`; they do not install system-wide tools. Use the checked-in lockfiles.
- Copy [`.env.example`](../.env.example) to ignored `.env` only for an isolated development database; keep credentials out of chat, logs, screenshots, Git, and `VITE_*` variables.
- **Never run migrations, Compose startup, test fixtures, reset, or destructive SQL against the existing personal LifeOS database without separate Product Owner approval, a verified backup, and a non-destructive upgrade plan.** The personal database may be below v5. Never delete existing disposable test databases as routine cleanup.

## Run locally

For a separately created, empty disposable database, configure `LIFEOS_DB_*` and OIDC values as described in [Auth Local Setup](AUTH_LOCAL_SETUP.md). Apply migrations to **that disposable database only** before starting the API:

```powershell
go run ./cmd/lifeos-migrate up
go run ./cmd/lifeos-migrate version
go run ./cmd/lifeos-api
```

In another terminal:

```powershell
npm run web:dev
```

Vite serves `http://127.0.0.1:5173` and proxies `/api` to Go at `127.0.0.1:8080` ([`vite.config.ts`](../apps/web/vite.config.ts)). Register that exact callback and set `LIFEOS_AUTH_PUBLIC_ORIGIN` to the same web origin for real Google sign-in. Without an API/session, private screens stay inaccessible; the web UI may show a connection or signed-out state. The trusted local invitation command and OAuth console steps are in [Auth Local Setup](AUTH_LOCAL_SETUP.md).

For an **isolated Compose database only**, the [Compose stack](../deploy/compose.yaml) starts PostgreSQL, runs migrations automatically, then starts API and web:

```powershell
docker compose --env-file .env -f deploy/compose.yaml config --quiet
docker compose --env-file .env -f deploy/compose.yaml up --build -d
docker compose --env-file .env -f deploy/compose.yaml ps -a
```

Its web origin is `http://127.0.0.1:4173` by default. `docker compose ... down` stops services while preserving the named PostgreSQL volume; do not add `-v` when retaining data. The old [Foundation Startup](FOUNDATION_STARTUP.md) explains the initial stack, but its recorded migration version and demo shell predate Money and Goals.

## Preview fixtures versus the real backend

The app's normal frontend API functions call `/api/v1` using same-origin cookies. A synthetic preview can intercept those requests and show UI states without Google or PostgreSQL. The Issue #25 visual fixture lives in ignored `.cache/` on the development machine; it is **not shipped source, a reliable test database, or proof of real persistence/security**. An apparent success in that preview must be checked against the real guarded Go API and disposable PostgreSQL. The Goals integration test and the isolated browser smoke in [Project State](PROJECT_STATE.md) provide that separate evidence.

## Verification

```powershell
npm run web:test
npm run web:typecheck
npm run web:build
npm run openapi:lint
npm run openapi:check
go test -count=1 ./...
go vet ./...
git diff --check
```

`openapi:lint` currently reports two known OAuth redirect-only 2XX warnings; new warnings need investigation. After changing [`openapi.yaml`](../api/openapi/openapi.yaml), run `npm run openapi:generate`, commit the generated [TypeScript schema](../apps/web/src/api/generated/schema.ts), and verify with `openapi:check`.

PostgreSQL integration tests **skip unless their DSNs are supplied**. Each must point to a distinct, freshly created, empty database whose name ends in `_test`: `LIFEOS_TEST_DATABASE_URL`, `LIFEOS_AUTH_TEST_DATABASE_URL`, `LIFEOS_FINANCE_TEST_DATABASE_URL`, `LIFEOS_MONEY_API_TEST_DATABASE_URL`, `LIFEOS_GOALS_TEST_DATABASE_URL`, and `LIFEOS_SOFT_DELETE_TEST_DATABASE_URL`. The test helpers reject non-`_test` or already-migrated databases; set these process environment variables for the test invocation, then confirm the integration tests actually ran rather than skipped. The [migration verifier](../scripts/verify-migrations.ps1) also changes its target database and belongs only on an isolated disposable database.

## Migrations, browser builds, and auth

Numbered SQL lives in [`db/migrations/`](../db/migrations/); the [migration command](../cmd/lifeos-migrate/README.md) supports `up`, one-step `down`, and `version`. Existing migration files are immutable. Test both a clean chain and an upgrade with existing rows. The v5 down migration intentionally refuses to drop populated Goals; v6 down refuses while any deleted Finance event exists because dropping its marker would reveal that record again; v7 down refuses while any owner has a saved timezone. Use a corrective forward migration or a separately approved restore plan. Never use `down` as a casual rollback on personal data.

If the browser appears stale, check which origin you opened (`5173` Vite versus `4173` Compose), restart the relevant dev/preview server or rebuild the web image, then hard-refresh. Vite development does not serve a previously built Compose image; Compose does not automatically reflect local TS edits. Direct `/plans/goals/{id}` URLs, back, and reload should resolve through the web fallback.

The web app first calls `/api/v1/me`; it displays private pages only after a valid session response. Google OIDC, invitation admission, cookies, expiry, and revocation live in [`identity/auth.go`](../internal/identity/auth.go). Mutations send the `/me` CSRF token in `X-CSRF-Token` with a same-origin cookie; Go checks Origin, session, and CSRF via `Auth.RequireMutation`. Reads use `Auth.RequireRead`. Keep OAuth secrets server-side and do not add a production authentication bypass for local previews.
