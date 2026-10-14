# LifeOS Codebase Map

**Current code:** Issues #26–#28 are complete. The Product Owner accepted ADR-016 and approved Issue #29 Soft Delete with migration v6 after local verification and visual review. This is a navigation guide; [Architecture](ARCHITECTURE.md), [Domain Model](DOMAIN_MODEL.md), and accepted [ADRs](adr/README.md) define the rules.

## Start here

| Area | Entry point and ownership |
| --- | --- |
| Web | [`apps/web/src/main.tsx`](../apps/web/src/main.tsx) mounts [`App`](../apps/web/src/LifeApp.tsx) through [`App.tsx`](../apps/web/src/App.tsx). `LifeApp.tsx` confirms the session before rendering [`AuthenticatedShell`](../apps/web/src/LifeApp.tsx). [`navigation.ts`](../apps/web/src/navigation.ts) resolves Today, Money, Plans, Me, and Goal detail URLs. [`lifeos.css`](../apps/web/src/lifeos.css), [`money.css`](../apps/web/src/money.css), and [`goals.css`](../apps/web/src/goals.css) carry the Sage Green UI. |
| API | [`cmd/lifeos-api/main.go`](../cmd/lifeos-api/main.go) `run` creates services and registers identity, Finance, and Planning on one `/api/v1` chi router. [`httpapi.NewRouter`](../internal/platform/httpapi/router.go) mounts it beside health/readiness and request middleware. |
| Identity | [`internal/identity/auth.go`](../internal/identity/auth.go) owns Google OIDC, sessions, `Auth.RequireRead`, and `Auth.RequireMutation`; [`owner.go`](../internal/identity/owner.go) defines the server-derived `OwnerID`. [`repository.go`](../internal/identity/repository.go) persists identity and sessions. [`cmd/lifeos-admin/main.go`](../cmd/lifeos-admin/main.go) is the trusted local invitation operator. |
| Persistence | [`database.OpenPostgreSQL`](../internal/platform/database/postgres.go) opens the pool. [`db/migrations/`](../db/migrations/) holds numbered SQL; [`cmd/lifeos-migrate/main.go`](../cmd/lifeos-migrate/main.go) applies or reports them. v2/v3 add identity/sessions; v4 adds Money; v5 adds Goals; v6 adds the nullable Finance Soft Delete marker. |
| Contracts and deployment | [`api/openapi/openapi.yaml`](../api/openapi/openapi.yaml) is the API contract; [`apps/web/src/api/generated/schema.ts`](../apps/web/src/api/generated/schema.ts) is generated from it. [`deploy/compose.yaml`](../deploy/compose.yaml) starts PostgreSQL, migration job, API, and web; the [`API`](../deploy/api.Dockerfile) and [`web`](../deploy/web.Dockerfile) Dockerfiles build their images. |

## Feature paths

**Status key:** Implemented = working persisted flow; partial = useful UI with a missing V0.1 capability; placeholder = navigation without its planned data; future = no product implementation.

| Feature | User-facing UI | Frontend API | HTTP Handler | Service | Repository | Database/Migration | Relevant Tests |
| --- | --- | --- | --- | --- | --- | --- | --- |
| Today **implemented; Issue #26 approved** | [`TodayPage`](../apps/web/src/TodayPage.tsx) projects account balances, recent events, and active Goal progress | Existing [`api/money.ts`](../apps/web/src/api/money.ts) and [`api/goals.ts`](../apps/web/src/api/goals.ts) reads | Existing [`finance/http.go`](../internal/finance/http.go) and [`planning/http.go`](../internal/planning/http.go) read handlers | Existing Finance/Planning `Service` methods | Existing owner-scoped Finance/Planning repositories | Existing v4 Finance and v5 Goals tables; no new migration | [`TodayPage.test.ts`](../apps/web/src/TodayPage.test.ts), Finance/Goals integration tests, isolated real-stack browser checks |
| Money **implemented; Issues #27/#28 approved** | [`MoneyPage`](../apps/web/src/MoneyPage.tsx) shows Accounts then History, compact setup, category rename/archive/restore, optional Uncategorized entry, and optional Void note; [`moneyAmount.ts`](../apps/web/src/moneyAmount.ts) exact input | [`api/money.ts`](../apps/web/src/api/money.ts) typed mutations | [`finance/http.go`](../internal/finance/http.go) `moneyHandler` | [`finance/service.go`](../internal/finance/service.go) `Service` | [`finance/repository.go`](../internal/finance/repository.go) `repository` | [`000004_money_ledger.up.sql`](../db/migrations/000004_money_ledger.up.sql); no new migration | [`finance_integration_test.go`](../internal/finance/finance_integration_test.go), [`http_integration_test.go`](../internal/finance/http_integration_test.go), [`LifeApp.test.tsx`](../apps/web/src/LifeApp.test.tsx), [`api/money.test.ts`](../apps/web/src/api/money.test.ts), isolated real-stack browser flow |
| Money Soft Delete **implemented; Issue #29 approved** | [`HistorySection.confirmDelete`](../apps/web/src/MoneyPage.tsx) confirms and refreshes | [`deleteMoneyEvent`](../apps/web/src/api/money.ts) guarded POST | [`moneyHandler.deleteEvent`](../internal/finance/http.go) behind `Auth.RequireMutation` | [`Service.DeleteEvent`](../internal/finance/service.go) validates owner and ID | [`repository.deleteEvent`](../internal/finance/repository.go) locks owned event and affected accounts; `balanceSQL`, `categorySpending`, and `history` exclude deleted events | [`000006_financial_event_soft_delete.up.sql`](../db/migrations/000006_financial_event_soft_delete.up.sql) adds nullable `deleted_at`; linked rows retained | [`soft_delete_integration_test.go`](../internal/finance/soft_delete_integration_test.go), [`http_integration_test.go`](../internal/finance/http_integration_test.go), [`api/money.test.ts`](../apps/web/src/api/money.test.ts), real-stack browser checks |
| Plans/Goals **implemented** | [`GoalsPage`](../apps/web/src/GoalsPage.tsx) overview and detail | [`api/goals.ts`](../apps/web/src/api/goals.ts) | [`planning/http.go`](../internal/planning/http.go) `goalHandler` | [`planning/service.go`](../internal/planning/service.go) `Service` | [`planning/repository.go`](../internal/planning/repository.go) `repository` | [`000005_goals.up.sql`](../db/migrations/000005_goals.up.sql) | [`planning_integration_test.go`](../internal/planning/planning_integration_test.go), [`LifeApp.test.tsx`](../apps/web/src/LifeApp.test.tsx), [`api/goals.test.ts`](../apps/web/src/api/goals.test.ts) |
| Me **partial** | [`MePage`](../apps/web/src/LifeApp.tsx) profile/sign-out; data tools pending | [`api/auth.ts`](../apps/web/src/api/auth.ts) `fetchCurrentUser`, `signOut` | [`identity/auth.go`](../internal/identity/auth.go) `Auth.me`, `Auth.logout` | Session logic in `Auth` | [`identity/repository.go`](../internal/identity/repository.go) | [`000003_auth_sessions.up.sql`](../db/migrations/000003_auth_sessions.up.sql) | [`auth_integration_test.go`](../internal/identity/auth_integration_test.go), [`LifeApp.test.tsx`](../apps/web/src/LifeApp.test.tsx) |
| Google sign-in/invitations **implemented locally** | [`SignedOutScreen`](../apps/web/src/LifeApp.tsx), [`App`](../apps/web/src/LifeApp.tsx) | [`api/auth.ts`](../apps/web/src/api/auth.ts) | [`identity/auth.go`](../internal/identity/auth.go) `Auth.start`, `Auth.callback`, `Auth.me` | Admission/session in `Auth` | [`identity/repository.go`](../internal/identity/repository.go) | [`000002_identity_foundation.up.sql`](../db/migrations/000002_identity_foundation.up.sql), [`000003_auth_sessions.up.sql`](../db/migrations/000003_auth_sessions.up.sql) | [`auth_integration_test.go`](../internal/identity/auth_integration_test.go), [`repository_integration_test.go`](../internal/identity/repository_integration_test.go) |
| Owner boundary **implemented** | Session-gated [`AuthenticatedShell`](../apps/web/src/LifeApp.tsx) | Same-origin calls in `api/auth.ts`, `api/money.ts`, `api/goals.ts` | [`Auth.RequireRead`/`RequireMutation`](../internal/identity/auth.go) wrap Money/Goals routes | Finance/Planning `Service` methods accept `OwnerID` | Finance/Planning SQL filters owner and related IDs | Owner columns and foreign keys in v2/v4/v5 | [`owner_scope_integration_test.go`](../internal/identity/owner_scope_integration_test.go), Finance/Planning integration tests |
| Export, backup UI, offline sync, Wishlist **future** | N/A | N/A | N/A | N/A | N/A | N/A | N/A |

The Google login was manually verified locally in Issue #19. The isolated Goals browser test used a synthetic session with normal guards and does not substitute for a new live Google or public deployment check.

## Representative call chains

```mermaid
flowchart LR
  A[GoalsPage.submitGoal] --> B[api/goals.ts createGoal / goalRequest]
  B --> C[Auth.RequireMutation]
  C --> D[goalHandler.createGoal]
  D --> E[Service.CreateGoal]
  E --> F[repository.createGoal]
  F --> G[(PostgreSQL goals)]
  G --> H[goalDTO response]
  H --> I[GoalsPage.mutate / refresh]
```

```mermaid
flowchart LR
  A[MoneyPage EntryForm] --> B[api/money.ts postMoneyEvent]
  B --> C[Auth.RequireMutation]
  C --> D[moneyHandler.postEvent]
  D --> E[Service.PostExpense or PostIncome]
  E --> F[repository.post]
  F --> G[(FinancialEvents / Movements / Allocations)]
  G --> H[eventDTO response]
  H --> I[MoneyPage refresh]
```

```mermaid
flowchart LR
  A[LifeApp.App] --> B[api/auth.ts fetchCurrentUser]
  B --> C[Auth.me]
  C --> D[Auth.currentUser]
  D --> E[(lifeos_sessions / lifeos_users)]
  E --> F[CurrentUser + CSRF token]
  F --> G[AuthenticatedShell]
```

For inputs, outputs, errors, and tests at each function, read the [Goals](../learning/journal/2026-10-12-goals-checklists.md), [Money UI](../learning/journal/2026-10-09-money-ui.md), [Money API](../learning/journal/2026-10-09-money-api.md), [OIDC/session](../learning/journal/2026-10-09-google-oidc-sessions.md), and [owner boundary](../learning/journal/2026-10-09-owner-authorization-boundary.md) learning entries.

Today reads the existing server-calculated balance for every account, four recent event records as a list only, and the Goal list's completed/total counts. It never computes financial totals from the paginated event list. See the [Issue #26 learning entry](../learning/journal/2026-10-13-today-dashboard.md) for its exact read flow.

Issue #27 adds `CategoryPanel` rename/archive/restore through `api/money.ts` and `moneyHandler`/`Service`/owner-scoped repository functions. `EntryForm.submit` sends explicit `allocations: []` for Uncategorized; `postSingleAccountEvent` accepts zero allocations and requires exact totals for nonempty allocations. `repository.post` still writes the event and account movement atomically, while history and Today label the empty allocation list. The actual function paths, errors, and tests are in the [Issue #27 learning entry](../learning/journal/2026-10-13-money-categories-uncategorized.md).

Issue #28 changes the existing void path only: `HistorySection.confirmVoid` sends a typed optional note through `voidMoneyEvent`; `moneyHandler.voidEvent` passes it to `Service.VoidEvent`, which trims it and substitutes a clearly generated audit message when blank. `repository.voidEvent` retains the original event and links while removing its effects from balances and spending. `HistorySection` displays that server default as `Void reason: N/A` and displays an explicit reason as entered. The [Issue #28 learning entry](../learning/journal/2026-10-13-money-optional-void-reason.md) gives the actual function chain, errors, tests, and browser evidence. No migration was added.

Issue #29 follows this actual write path:

```mermaid
flowchart LR
  A[HistorySection.confirmDelete] --> B[api/money.ts deleteMoneyEvent]
  B --> C[Auth.RequireMutation]
  C --> D[moneyHandler.deleteEvent]
  D --> E[Service.DeleteEvent]
  E --> F[repository.deleteEvent]
  F --> G[eventMovementSpecs / checkRemovalBalances]
  G --> H[(PostgreSQL financial_events.deleted_at)]
  H --> I[204 No Content]
  I --> J[MoneyPage.runMutation / loadMoney]
  J --> K[Money and Today filtered reads]
```

`repository.deleteEvent` locks an event by server-derived owner, accepts repeats without a second effect, validates posted-event reversal under affected account locks, and writes only the marker in the same transaction. `repository.voidEvent` rejects deleted events; both operations serialize on the event row. Normal `history` filters deleted events before limit/offset; `balanceSQL` and `categorySpending` require posted and not deleted. Today uses `listMoneyHistory(4, 0)` and server balances, so its next read also omits the deleted event. The [Issue #29 learning entry](../learning/journal/2026-10-14-financial-event-soft-delete.md) maps inputs, outputs, errors, and tests at each function.

The compact Money refinement changes presentation only: `AccountOverview` launches `MoneyPage.chooseSetup`, which opens the existing `AccountForm` or `CategoryPanel`/`CategoryForm` in a native dialog. `AccountForm.submit` and `CategoryForm.submit` still call `MoneyPage.runMutation`, the existing typed API functions, and `MoneyPage.loadMoney` for refreshed accounts, categories, balances, and history. See the [compact Money setup learning entry](../learning/journal/2026-10-13-money-compact-setup.md) for the actual function-level write flow. This work is tracked with category management in Issue #27, separate from the read-only Today Issue #26.

## Where do I change this?

| Task | Start with |
| --- | --- |
| Page layout or styling | [`LifeApp.tsx`](../apps/web/src/LifeApp.tsx) for shell/Me, [`TodayPage.tsx`](../apps/web/src/TodayPage.tsx), [`MoneyPage.tsx`](../apps/web/src/MoneyPage.tsx), or [`GoalsPage.tsx`](../apps/web/src/GoalsPage.tsx) for features; their CSS files above. |
| Add a Goal field | [Goal service/model](../internal/planning/service.go), [repository SQL](../internal/planning/repository.go), [new migration](../db/migrations/), [HTTP DTO](../internal/planning/http.go), [OpenAPI](../api/openapi/openapi.yaml), generated types and UI; update integration tests. |
| Add a Money API field | [Finance service/model](../internal/finance/service.go), [HTTP DTO](../internal/finance/http.go), [OpenAPI](../api/openapi/openapi.yaml), regenerated [TypeScript](../apps/web/src/api/generated/schema.ts), [frontend API](../apps/web/src/api/money.ts), and tests. |
| Add an authenticated endpoint | Register in [module `RegisterRoutes`](../internal/planning/http.go) from [`cmd/lifeos-api/main.go`](../cmd/lifeos-api/main.go); wrap reads with `Auth.RequireRead` and mutations with `Auth.RequireMutation`; pass only server-derived `OwnerID`. |
| Add a database column | Create the next numbered [up/down migration](../db/migrations/), test a populated upgrade and safe rollback, then update repository/service/DTO/contract in that order. Never rewrite a published migration. |
| Fix login/session | [`identity/auth.go`](../internal/identity/auth.go), [`identity/repository.go`](../internal/identity/repository.go), [`api/auth.ts`](../apps/web/src/api/auth.ts), and [local setup](AUTH_LOCAL_SETUP.md). Never log secrets or callback codes. |
| Add tests | Frontend: colocated [`*.test.ts(x)`](../apps/web/src/LifeApp.test.tsx). Backend: package [`*_test.go`](../internal/planning/planning_integration_test.go) on a fresh disposable PostgreSQL database. See [Development Guide](DEVELOPMENT_GUIDE.md). |
