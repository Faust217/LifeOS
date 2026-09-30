# Foundation Startup and Verification

**Status:** Foundation workflow verified locally on 2026-09-30

## Purpose and Boundary

This is the clean, project-local workflow for validating and starting the LifeOS Foundation stack. It covers configuration, PostgreSQL, migrations, the operational Go API, the OpenAPI-generated TypeScript boundary, and the minimal React/Vite web shell.

The workflow needs no LifeOS Finance data or private personal data. It does not create Owner, Account, Category, FinancialEvent, Bluecoins import, or other product behavior.

Run all commands from the repository root. The expected tool versions and read-only environment record are in `docs/DEVELOPMENT_ENVIRONMENT.md`.

## 1. Create Local Configuration

Create the ignored root `.env` only when one does not already exist:

```powershell
Copy-Item .env.example .env
```

Replace `LIFEOS_DB_PASSWORD=CHANGE_ME` in `.env` with a local development password. Do not commit or print the password. Keep the default `127.0.0.1` bindings unless a separately reviewed deployment design requires otherwise.

The complete accepted contract, precedence, and validation rules are in `docs/CONFIGURATION.md`. `.env.example` is only a template; the application reads process environment values first, then the project-root `.env`, then safe built-in defaults.

## 2. Bootstrap Project-Local Dependencies

```powershell
go mod download
npm ci --ignore-scripts --no-audit --no-fund
```

These commands use the committed Go module files and npm lockfile. JavaScript packages remain under the ignored repository-local `node_modules/`; no global package installation is required.

## 3. Verify Source, Contracts, and Generated Types

```powershell
go test -count=1 ./...
go vet ./...
npm run openapi:lint
npm run openapi:generate
npm run openapi:check
npm run web:typecheck
npm run web:test
npm run web:build
git diff --exit-code -- apps/web/src/api/generated/schema.ts
```

`openapi:generate` must reproduce the committed TypeScript output. The final Git command proves generation did not change it.

## 4. Validate and Start the Complete Stack

```powershell
docker compose --env-file .env -f deploy/compose.yaml config --quiet
docker compose --env-file .env -f deploy/compose.yaml build api migrate web
docker compose --env-file .env -f deploy/compose.yaml up -d --wait
docker compose --env-file .env -f deploy/compose.yaml ps -a
```

Compose starts PostgreSQL first, applies the ordered migration chain once, starts the API after migration success, and starts the web service after API readiness. Published ports stay on `127.0.0.1`.

## 5. Verify Migration and Runtime Readiness

```powershell
go run ./cmd/lifeos-migrate version
Invoke-RestMethod http://127.0.0.1:8080/healthz
Invoke-RestMethod http://127.0.0.1:8080/readyz
Invoke-RestMethod http://127.0.0.1:4173/healthz
```

The migration command should report version `1` with `dirty=false`. Liveness should report `ok`; readiness should report `ready`, a healthy database, and current clean migration version `1`. The web-origin health request proves the web server can reach the API through its same-origin proxy.

Open `http://127.0.0.1:4173` to inspect the Foundation shell. It intentionally contains no product screen or private data.

## 6. Stop Safely

```powershell
docker compose --env-file .env -f deploy/compose.yaml down
```

Normal `down` removes the containers and network but preserves the PostgreSQL named volume. Do not add `-v` unless deletion of that specific development database has been explicitly approved.

## Verification Record

On 2026-09-30, this workflow was run from an isolated snapshot of the committed repository using only the documented toolchain and a disposable Compose project. The following checks passed:

- clean dependency bootstrap from committed Go and npm metadata;
- full Go tests and `go vet`;
- OpenAPI lint, deterministic TypeScript generation, and stale-output check;
- web type checking, tests, and production build;
- Compose configuration, image builds, startup, and service health;
- clean migration version reporting;
- direct API liveness/readiness and web-origin readiness;
- clean Compose shutdown and removal of only the isolated disposable verification resources.

No Finance/domain feature or private user data was required. The GitHub-hosted migration workflow was not run because this sprint intentionally did not push; its workflow and shared verification script were validated locally, and the first hosted result remains a review follow-up.
