# LifeOS Project State

**Last updated:** 2026-10-13

**Current milestone:** Issues #26 Today, #27 Money category/Uncategorized, and #28 optional void reason completed and Product Owner approved; permanent transaction deletion is a separate proposal before Me/export

**Product milestone:** Invite-only online V0.1 Alpha approved in Issue #16; identity, owner boundary, Money, and Goals workflows implemented

**Blueprint:** Master Project Blueprint v0.2 is the historical baseline; Issue #16 is the approved current product direction

## Current Status

The Product Owner approved the [Issue #16 Product & Architecture Rebaseline](https://github.com/Faust217/LifeOS/issues/16): LifeOS becomes a Life-first, mobile-first platform whose V0.1 is an invite-only, online Alpha for the owner and a few testers. The Product Owner also approved ADR-013 and ADR-014 in [Issue #17](https://github.com/Faust217/LifeOS/issues/17). ADR-013 supersedes ADR-008's single-user V0.x scope and ADR-009's Bluecoins-in-V0.1 timing; owner scoping and safe import/export remain in force. Historical ADR-008/009 files remain untouched. Issues #18 and #19 establish identity persistence and the local OIDC/session flow. The Product Owner manually verified the live Google flow on local Compose on 2026-10-09; Internet-facing Alpha readiness remains a separate gate.

Foundation #1–#15 remains the technical baseline. Issues #1–#7 were accepted and closed. Issues #8–#15 were implemented and verified locally in separate commits; their GitHub review and closure remain pending. The chain covers configuration, PostgreSQL, migrations, operational API health/readiness, OpenAPI-generated TypeScript types, the minimal web shell, and Compose startup. Issues #18–#20 supply identity, sign-in, and owner authorization. Issue #21 adds the Money domain. Issue #22 adds authenticated Money APIs. Issue #23 adds the four-area app shell. Issue #24 adds the approved Money UI and archive restoration. The Product Owner approved Issue #25's Goals implementation and visual review; final verification passed before closure.

## Completed

- Preserved both Master Project Blueprint v0.2 reference documents under `docs/reference/`.
- Created the root agent governance and local-machine safety policy in `AGENTS.md`.
- Replaced the placeholder README with project orientation and the documentation reading path.
- Extracted the product vision into `docs/VISION.md`.
- Extracted the architecture baseline into `docs/ARCHITECTURE.md`.
- Defined the conceptual domain vocabulary and invariants in `docs/DOMAIN_MODEL.md`.
- Originally defined the Bluecoins-first Finance Core milestone; Issue #16 intentionally changes its V0.1 timing and scope.
- Recorded the Foundation-to-V1.0 capability sequence in `docs/ROADMAP.md`.
- Established ADR policy, naming, template, and the initial decision-record index in `docs/adr/README.md`.
- Recorded and accepted ADR-001 through ADR-010 using the repository's standard filenames.
- Established the non-normative learning area, learning map, and journal guidance.
- Published the Foundation milestone and 15 issues with acceptance criteria, explicit non-goals, dependencies, and environment/ADR gates.
- Completed, accepted, and closed Issue #1 after resolving its approved Go and Docker verification blockers without starting product implementation.
- Completed, accepted, and closed Issue #2 after creating and verifying the dependency-free monorepo skeleton and accepted module boundaries.
- Completed, accepted, and closed Issue #3 after selecting `golang-migrate` v4 in ADR-011 with project-local pinning, ordered forward migrations, transaction guidance, and explicit dirty-state recovery.
- Completed, accepted, and closed Issue #4 after selecting `chi` v5 in ADR-012; at Issue #4 closure, `chi` had not yet been installed and API implementation had not started.
- Completed, accepted, and closed Issue #5 after defining the minimal Foundation configuration contract.
- Completed, accepted, and closed Issue #6 after implementing and verifying the local PostgreSQL 18 Compose service.
- Completed, accepted, and closed Issue #7 after implementing and verifying typed Foundation configuration loading and validation.
- Implemented and verified Issue #8 locally with `golang-migrate` v4.20.1, production and test-only migration boundaries, repeated-up safety, one-step rollback, version reporting, and documented recovery; remote review and closure remain pending.
- Implemented and verified Issue #9 locally with `chi` v5.3.0, `pgx` v5.11.0, liveness/readiness endpoints, current migration visibility, request IDs, structured logs, graceful shutdown, and focused tests; remote review and closure remain pending.
- Implemented and verified Issue #10 locally with pinned React 19.3.0, TypeScript 7.0.2, Vite 8.3.1, a minimal accessible shell, a replaceable API boundary, type checking, tests, and a production build; remote review and closure remain pending.
- Implemented and verified Issue #11 locally with reproducible API/migration/web images, explicit Compose health dependencies, same-origin operational proxying, localhost-only ports, and preserved PostgreSQL storage; remote review and closure remain pending.
- Implemented and verified Issue #12 locally with a focused GitHub Actions migration workflow, a shared chain verifier, clean PostgreSQL 18 evidence, and `actionlint`; a GitHub-hosted run remains pending until the later authorized push.
- Implemented and verified Issue #13 locally with a canonical OpenAPI 3.1 contract for `/healthz` and `/readyz`, stable status/error schemas, explicit versioning guidance, and clean pinned Redocly validation; remote review and closure remain pending.
- Implemented and verified Issue #14 locally with pinned `openapi-typescript` 7.13.0, committed generated Foundation types, deterministic stale-output checking, and web compilation against generated DTOs; remote review and closure remain pending.
- Implemented and verified Issue #15 locally with a clean project-local startup guide and an isolated end-to-end Foundation check covering dependency bootstrap, tests, contracts, generated types, Compose, migrations, API readiness, and the web shell; remote review and closure remain pending.
- Completed and pushed the Issue #18 identity persistence foundation: migration version 2, internal user IDs, unique Google `sub` bindings, invitations, package-private admission/operator boundaries, and PostgreSQL security and upgrade tests.
- Completed and pushed Issue #19: server-verified Google OIDC, invitation-gated admission, revocable sessions, CSRF-protected logout, minimal web sign-in, and Product Owner live local Google verification. Issue #19 is closed.
- Completed Issue #20: reusable read and CSRF-protected mutation guards pass a server-validated owner ID; PostgreSQL integration tests cover cross-user isolation and invalid sessions. No domain schema or public product route was added.
- Completed Issue #21 after Product Owner review: owner-scoped Finance migration version 4 and lightweight ledger services implement accounts, categories, events, movements, allocations, deterministic balances, history, archive, and audited void. Ordinary accounts may be negative because LifeOS records financial activity rather than authorizing bank overdrafts. PostgreSQL integration and full Go tests passed without touching personal data.
- Completed Issue #22 after Product Owner final review: authenticated Money REST routes reuse Finance services and server-derived owner IDs; strict decimal-string minor-unit DTOs, OpenAPI, and generated TypeScript types are covered by PostgreSQL integration tests. No migration, React Money screen, personal-data change, or deployment was included. Live browser-to-Money testing remains for the UI milestone.

## Issue #23 Completion

- Created [Issue #23](https://github.com/Faust217/LifeOS/issues/23) after checking for an equivalent issue. The local React shell presents Today, Money, Plans, and Me with mobile bottom navigation, desktop sidebar, direct URLs and native browser history. It confirms `/api/v1/me` before rendering private pages, keeps real Google login and CSRF-protected logout, and shows honest loading, signed-out, API-error, and feature-empty states.
- In the committed #23 shell, Money and Plans were layouts only. No Money endpoint was called; no balances, transactions, or Goal records were fabricated. No Go code, API contract, migration, database, or deployment changed. The Product Owner visually approved the Sage Green design.
- Issue #23 web tests, typecheck, build, and isolated browser checks passed at 320 px and 390 px, including mobile navigation, keyboard skip link, browser history, logout, protected history after logout, and API-error recovery. Live Google OIDC was previously verified for Issue #19; no new live OAuth test was claimed for the shell.
- Issue #23 was committed as `f219847e0f3c8534c09640728f2a138a315c32b7`, pushed to `main`, verified against remote HEAD with a clean working tree, and closed after the Product Owner's visual approval.

## Issue #24 Completion

- Created [Issue #24](https://github.com/Faust217/LifeOS/issues/24) after checking for an equivalent issue. The React work connects the authenticated Money API to real account balances, account/category creation, income/expense entry, same-currency transfers, paginated history with movement/allocation details, void with reason, and account archive. Its approved UX review adds a sticky quick-transaction dialog, explicit account/category selections, lightweight Account → Category → First Transaction guidance, readable history, correction review after void, and recoverable feedback. The Product Owner approved the final Sage Green Money design and workflows.
- Issue #24 adds an owner-scoped Restore Account service and authenticated/CSRF-protected API route using the existing `archived_at` field. Historical events and balances remain intact; there is no new migration. OpenAPI and generated TypeScript types include the restore operation. Archive confirmation warns about nonzero balances and credit-card negative positions are labeled as liabilities.
- Amount entry uses decimal text and `BigInt` to produce exact signed int64 minor-unit strings. Each account and transaction displays its own currency; different currencies are never combined. Mutations use the existing session CSRF token and refresh account balances and history. Today, Plans, Me, and logout remain part of the approved Sage Green shell.
- Web tests (26), typecheck/build, OpenAPI lint/generated-type checks, full Go tests, `go vet`, and `git diff --check` passed for #24. Explicit Finance/Money API PostgreSQL integration tests ran on fresh disposable databases `lifeos_issue24_close_finance_1010_test` and `lifeos_issue24_close_api_1010_test`. An isolated synthetic-browser check covered 320 px, 390 px, and desktop usability. A real Edge browser then used the production React bundle, guarded Go API, and disposable PostgreSQL v4 database `lifeos_issue24_browser_20261010_test` to verify account/category creation, expense, income, transfer, balances and history after reload, and archive/restore. Unauthenticated reads and a mutation without CSRF were denied. This test seeded a synthetic session in the disposable database; it did not repeat live Google sign-in. All disposable databases remain intact. Final checks are recorded in the #24 learning entry.
- Before the private Alpha relies on real personal finance data, review and resolve dated opening-balance/backdated-transaction correctness and owner-scoped financial-write idempotency. API-backed history search/filtering and accurate historical post-transaction balances are future Money enhancements. The present warning and retry guidance are interim safeguards.
- The existing personal LifeOS database was not inspected, migrated, reset, or used for Money testing. It may still be at schema v3. Its Money use requires separate Product Owner approval for a backup and non-destructive v4 migration plan. No deployment or #25 work was included.

## Issue #25 Goals Completion

- Created [Issue #25](https://github.com/Faust217/LifeOS/issues/25) after checking for an equivalent issue. The local implementation adds migration v5 for owner-scoped `goals` and `goal_checklist_items`, focused Go services and guarded `/api/v1/goals` endpoints, OpenAPI and generated TypeScript contracts, and real checklist workflows in Plans.
- Goal creation, rename, completion/reopen, archive/restore, checklist add/edit/complete/uncomplete/remove, and completed/total progress are non-financial. Completion requires at least one item with every item done; completed Goals must be reopened before checklist changes, and archived Goals retain their checklists. Restore uses the existing `archived_at` column, a server-derived owner ID, and the normal session/Origin/CSRF mutation guard. Goal item writes lock the parent Goal to serialize against completion. PostgreSQL ownership constraints and SQL predicates include both owner and related record IDs.
- Product Owner review found that the ignored visual fixture answered Goal POST with a list-shaped GET response, never persisted generated Goals, returned 404 for Goal item/mutation routes, and returned 404 for Money reads. This caused false success, missing new Goals, and false Money errors in the preview. The fixture now models method-specific Goal responses and mutable related items, and reuses the existing complete Issue #24 synthetic Money fixture behind the same preview origin. The production Go handler already returned correct Goal DTOs; the real browser/API/database lifecycle below verifies that distinction.
- Plans now uses compact Goal overview cards with status and progress, dedicated `/plans/goals/{id}` detail URLs for the full checklist and management actions, native browser history/reload, and a separate archived section. The Sage Green styling and four-area navigation remain intact.
- Verification uses only new disposable `*_test` databases and synthetic sessions. The existing personal LifeOS database and earlier disposable databases are outside this task. Product Owner visual review passed; no deployment or Issue #26 work is included.
- The v5 migration chain and populated v4→v5 upgrade passed in the initial implementation. After the review fix, the full Go suite ran with fresh disposable identity/auth/Finance/Money/Goals PostgreSQL databases; `go vet`, 30 web tests, typecheck/build, OpenAPI lint/generated-type check, and `git diff --check` passed. Redocly has only the two existing redirect-only OAuth 2XX warnings.
- A Chrome browser exercised create → detail/reload → add/edit/check/uncheck/remove → complete/reopen → archive/restore through the production React bundle, normal Go session/CSRF guards, and new disposable PostgreSQL v5 database `lifeos_issue25_browser_v5_test`. The archived Goal's checklist persisted and Money loaded its legitimate empty state. Unauthenticated Goals and Money requests returned 401. A synthetic session and local fake OIDC discovery enabled this isolated test; it does not claim another live Google sign-in. The visual fixture is for design preview only.
- The personal database was not inspected or changed. It may be below v5, so running this build against it requires separate backup and migration approval. Existing disposable databases were not deleted.
- [Codebase Map](CODEBASE_MAP.md) and [Development Guide](DEVELOPMENT_GUIDE.md) consolidate current source paths, safe local commands, integration-test setup, and personal-data boundaries without changing accepted ADRs.
- Final verification reran the complete Go suite on five new, empty disposable `*_test` databases. Identity, OIDC/session, Finance ledger/Money API, and Goals migration/service/HTTP integration tests all executed and passed. The Goals test exercises populated v4→v5 upgrade, full lifecycle, owner/CSRF denials, concurrency, and a Money balance before and after Goals work. All 30 web tests, typecheck, build, `go vet`, OpenAPI lint and generated-type check passed; the two known redirect-only OAuth warnings remain. The already running synthetic preview passed create/list/step/check/archive/restore/read and returned 200 for Money accounts. The prior real-browser-to-Go-to-v5 smoke remains the separate full-stack evidence; final closure did not repeat live Google sign-in.

## Current Handoff

[Issue #26](https://github.com/Faust217/LifeOS/issues/26) is Product Owner approved. Today reads the existing guarded Money and Goals endpoints: server-calculated per-account balances, a four-record recent activity list without a derived total, and active Goals with server-provided checklist progress. It adds no API, schema, migration, financial write, or new infrastructure. The dashboard preserves Sage Green and has independent loading, empty, error, and expired-session handling. The existing personal database and existing disposable databases remain outside this work.

The Product Owner approved the Today dashboard and a separate focused Money usability refinement. Money places transaction history directly after the account overview; `+ Account` opens the existing account form in a responsive native dialog, and `Categories` opens a compact panel with the existing categories and add form. `+ Add transaction` remains the prominent action. Dialogs have labels, Escape/close and focus return, keep drafts through validation/request errors, and prevent closing during a pending setup write. Existing account guidance, balance/archive/restore logic, transaction forms, and guarded API calls remain in use. The compact layout introduced no backend API or migration and was incorporated into Issue #27 rather than #26.

The Money refinement passed all 33 frontend tests and the production typecheck/build. An isolated Edge browser checked 320 px, 390 px, and 1280 px layouts, account-sheet guidance and focus, Categories Escape/focus, a preserved account draft after invalid amount and simulated request failure, and successful account, category, and expense creation through the guarded Go API against the disposable v5 PostgreSQL container. Today then displayed the new synthetic account, and the original preview owner's Today data remained isolated and unchanged. The new test records belong only to the synthetic empty test user; the existing preview owner and personal database were not modified. Live Google sign-in and public hosting were not retested for this UI-only change.

The new disposable PostgreSQL 18 container `lifeos-today-issue26-20261013` holds only synthetic Issue #26 data at migration version 5 (`dirty=false`); it remains available for review. The production React build at `http://127.0.0.1:5187/__test_login_owner`, normal Go API at port 18086, and a local fake OIDC discovery endpoint were used with synthetic sessions. Guarded API checks verified 3 owner accounts in MYR and USD, 8 owner events with a four-record recent page, server-calculated balances of MYR 68.66, MYR 75.00, and USD 80.00, a transfer as the latest event, Goal progress 1/3, anonymous 401 responses, and foreign account/Goal detail 404 responses. Another user has separate GBP/Goal data. The Edge browser passed Today layout and data checks at 320 px, 390 px, and 1280 px without horizontal overflow, followed the Today Goal-detail link, verified other-user isolation, and showed a third synthetic user's empty accounts, activity, and Goals through the real API. It also tested an injected empty Goals response and a Money-read error while Goals remained visible. The browser used synthetic sessions; no live Google login or public deployment was tested.

The frontend suite passed 33 tests after using a repository-local temporary directory to avoid a sandbox Temp-file failure. Typecheck/build, generated OpenAPI type check, OpenAPI lint, `go vet`, and `git diff --check` passed. Five additional new empty `*_test` databases in the Issue #26 container ran the full Go suite with `TestIdentityMigrationAndRepository`, `TestOIDCAndSessionIntegration`, `TestFinanceMigrationAndLedger`, `TestMoneyHTTPIntegration`, and `TestGoalsMigrationServiceAndHTTP` all executing and passing. The two existing redirect-only OAuth 2XX warnings remain in OpenAPI lint/generation. No new external API contract or migration was added.

### Issue #27 Money category management and Uncategorized

After the Product Owner approved the compact layout, a search found no equivalent GitHub issue, so [Issue #27](https://github.com/Faust217/LifeOS/issues/27) was opened separately from Today #26. The local uncommitted implementation adds owner-scoped category rename/archive/restore APIs and compact sheet actions. Rename preserves category ID, kind and historical allocation links; archive hides a category from new entries while keeping history, balances and category-specific spending intact; restore makes it selectable again. Category names are validated, case-insensitive conflicts within a kind are rejected, and `Uncategorized` is reserved as a display label. No category rows are deleted or reclassified.

Income and expense requests may now send an explicit empty `allocations` array. The service still requires exact sum equality when allocations are nonempty; the repository writes the event and account movement atomically with zero allocation rows for Uncategorized. Money history and Today display the label. Overall spending/reporting must include posted uncategorized expense events and exclude transfers/voids; summing named-category allocations alone is insufficient. OpenAPI and generated TypeScript contracts were updated. Existing v4 Finance tables support this behavior, so no schema migration or new infrastructure was added.

The full Go suite passed using five new, empty disposable integration databases. All 35 frontend tests, production typecheck/build, generated-contract check, OpenAPI lint, `go vet`, and diff checks passed; lint retains only the two existing OAuth redirect warnings. A real Edge browser used the production build, normal Go API/session/Origin/CSRF guards, and a newly migrated disposable PostgreSQL v5 preview database with synthetic Finance and Goals data. At 320/390 px it verified rename of a linked category and historical display, archive/restore, exclusion from new transaction choices, uncategorized expense/income with correct balances, Today, other-owner isolation, and Escape/focus without horizontal overflow. Guarded API checks returned 401 anonymously, 403 without CSRF or with wrong Origin, and 404 for foreign owner resources. The existing personal database and earlier disposable databases were not inspected, modified, reset, or deleted. No public deployment or new live Google sign-in occurred. See the [Issue #27 learning entry](../learning/journal/2026-10-13-money-categories-uncategorized.md) for exact function-level paths.

### Issue #28 optional Void reason

[Issue #28](https://github.com/Faust217/LifeOS/issues/28) separately tracks the Product Owner's change to the earlier #22/#24 user-entered void-reason requirement. The code allows a blank, omitted, or whitespace-only note and records the server-generated `No reason provided by owner` audit text. A supplied note is trimmed and retained. Money displays the generated default as `Void reason: N/A` and displays an explicit reason as entered. The original event remains voided with its movements, allocations and timestamp; balances and spending exclude it. The existing v4 database constraint is satisfied without editing a migration. OpenAPI and generated TypeScript types describe an optional reason property. Permanent deletion, icons and localization were not implemented.

The first optional-void browser attempt timed out because its test script assumed a fixed MYR 117.00 balance and that the first history row was a posted income. A prior synthetic expense had already been voided in the preview. The preview server, Go API and guarded synthetic session were healthy. A corrected browser test creates a uniquely identified synthetic event and derives expected balances from the real API; the final browser pass verified empty-note `Void reason: N/A`, explicit reason display, history persistence, Money and Today balances, and 320/390 px layouts. A separate real-API check on the second synthetic user passed omitted, empty, whitespace-only and explicit notes; before/after balances and category spending; repeat-void conflict; and anonymous, cross-owner, Origin and CSRF denials. The full Go suite passed again on five new empty disposable databases with Identity, OIDC/session, Finance/Money HTTP, and Goals integration tests executing. All 35 frontend tests, TypeScript typecheck/production build, OpenAPI lint and generated-contract check, `go vet`, and `git diff --check` passed. OpenAPI retains only its two pre-existing redirect-only OAuth 2XX warnings. Exact function flow is recorded in the [Issue #28 learning entry](../learning/journal/2026-10-13-money-optional-void-reason.md). The personal database and earlier disposable databases remain untouched; all newly created test databases are retained.

The Product Owner set the next V0.1 priorities: finish Today and Me/export; resolve opening-balance effective dates and backdated entries; add safe financial-write idempotency; then assess historical running balances; rehearse backup/restore and non-destructive upgrades; evaluate installable PWA, HTTPS hosting, and persistent storage; and pass a formal Personal Data Ready gate before real financial data. V0.2 prioritizes API-backed transaction search/filter and monthly/category charts while retaining Wishlist/offline sync for later order approval. V0.3 prioritizes Trips and richer Plans after separate scope review. Recurring schedules, broad icon/text refinements, AI, monetization, and advanced modules are deferred. Data preservation, owner isolation, backups, and safe forward migrations remain permanent invariants. The requested removal of a general-purpose Bluecoins product milestone conflicts with accepted ADR-009/013; [ADR-015](adr/ADR-015-personal-import-boundary.md) is a proposal pending Product Owner approval, not a silent historical rewrite.

[Issue #22](https://github.com/Faust217/LifeOS/issues/22) completed the authenticated `/api/v1` Money API for accounts, categories, income, expense, transfers, balances, spending, history, archive, and void. Mutations require Origin and CSRF checks. Request and response DTOs are separate from domain/persistence types; minor units travel as signed decimal strings so the TypeScript client does not lose int64 precision. The running Compose stack remains on its earlier image/schema until a separately authorized rollout.

Issues #24 and #25 passed isolated browser-to-Go-API tests using normal session/CSRF guards, synthetic identities, and disposable PostgreSQL v4/v5 respectively. The Goals test verified its full lifecycle and that Money still loads. Redocly retains two pre-existing redirect-only OAuth response warnings. All existing and new test databases remain intact. A previously running personal Compose stack may still use its earlier image/schema; use of this build with that database is gated on separate backup and migration approval.

All 15 Foundation issues are implemented locally. Issues #1–#7 are accepted and closed; Issues #8–#15 await Product Owner review and GitHub closure. The Foundation milestone remains administratively open even though its technical implementation and local verification are complete.

## Foundation Issue Plan

The recommended order is represented by the issue numbers. Issues #3 and #4 may proceed in parallel after #1. Issues #6, #7, and #10 may proceed in parallel after their shared prerequisites are complete.

| Order | Issue | Depends on | Implementation gate |
| --- | --- | --- | --- |
| 1 | [Inspect the local development environment (read-only)](https://github.com/Faust217/LifeOS/issues/1) | Accepted repository baseline | Completed, accepted, and closed |
| 2 | [Create the monorepo application skeleton](https://github.com/Faust217/LifeOS/issues/2) | #1 | Completed, accepted, and closed |
| 3 | [ADR-011: Choose the database migration tool](https://github.com/Faust217/LifeOS/issues/3) | #1 and ADR-003 | Completed, accepted, and closed; ADR-011 accepted |
| 4 | [ADR-012: Choose the Go HTTP router](https://github.com/Faust217/LifeOS/issues/4) | #1, ADR-001, and ADR-002 | Completed, accepted, and closed; ADR-012 accepted |
| 5 | [Define the project-local configuration contract](https://github.com/Faust217/LifeOS/issues/5) | #2 | Completed, accepted, and closed |
| 6 | [Add the PostgreSQL Docker Compose service](https://github.com/Faust217/LifeOS/issues/6) | #1 and #5 | Completed, accepted, and closed |
| 7 | [Implement Go configuration loading and validation](https://github.com/Faust217/LifeOS/issues/7) | #1, #2, and #5 | Completed, accepted, and closed |
| 8 | [Add project-local database migration commands](https://github.com/Faust217/LifeOS/issues/8) | #2, #3, #6, and #7 | Implemented and verified locally; GitHub review and closure pending |
| 9 | [Bootstrap the Go API with health and readiness endpoints](https://github.com/Faust217/LifeOS/issues/9) | #2, #4, #6, #7, and #8 | Implemented and verified locally; GitHub review and closure pending |
| 10 | [Bootstrap the React/TypeScript/Vite web shell](https://github.com/Faust217/LifeOS/issues/10) | #1, #2, and #5 | Implemented and verified locally; GitHub review and closure pending |
| 11 | [Compose the API and web services with PostgreSQL](https://github.com/Faust217/LifeOS/issues/11) | #6, #9, and #10 | Implemented and verified locally; GitHub review and closure pending |
| 12 | [Add CI migration-chain verification](https://github.com/Faust217/LifeOS/issues/12) | #3 and #8 | Implemented and verified locally; hosted run, GitHub review, and closure pending |
| 13 | [Define the OpenAPI Foundation contract](https://github.com/Faust217/LifeOS/issues/13) | #4 and #9 | Implemented and verified locally; GitHub review and closure pending |
| 14 | [Add the generated TypeScript client path](https://github.com/Faust217/LifeOS/issues/14) | #1, #10, and #13 | Implemented and verified locally; GitHub review and closure pending |
| 15 | [Document and verify the clean Foundation startup flow](https://github.com/Faust217/LifeOS/issues/15) | #5 through #14 | Implemented and verified locally; GitHub review and closure pending |

## Next Work Requiring Explicit Authorization

1. The Product Owner approved Issues #26–#28 and requested a separate scoped proposal for permanent deletion of already-voided transactions before Me/export. Do not implement deletion until the detailed issue, domain/audit semantics, and backup-retention limits are approved. Then scope Me/export, opening-balance/backdated correctness, financial-write idempotency, backup/restore, and the non-destructive upgrade path through separate authorization. Foundation #8–#15 GitHub review/closure remains separate.
2. Establish and explicitly accept the Personal Data Ready gate before real financial data: Finance correctness, idempotency, export, owner isolation, backups and verified restore, forward upgrades, and secure persistent operation. Do not deploy or migrate the personal database without separate approval.
3. Before any Internet-facing Alpha use, review HTTPS/TLS, secrets, database transport, operational limits, PWA installation, persistent storage, and tested recovery. Before V0.2 offline synchronization, accept a focused ADR for reliability, recovery, conflicts, and account/device boundaries. Product Owner acceptance of proposed ADR-015 is required to remove the accepted Bluecoins product-adapter commitment.

## Known Risks

| Risk | Current mitigation |
| --- | --- |
| Scope creep | Keep V0.1 to the approved invite-only online Alpha; defer offline sync, Bluecoins, and other modules |
| Over-engineering | Preserve boundaries but implement one database, provider path, and deployment path only when needed |
| Financial correctness | Issue #21 stores integer money, calculates balances from posted movements, keeps transfer allocations empty, rejects currency mismatch/overflow, and tests atomicity/concurrency; ordinary negative balances are explicitly allowed |
| Real-data Money readiness | Opening-balance effective dates/backdated-entry correctness and financial-write idempotency require resolution before the private Alpha relies on personal finance data; UI warnings are interim only |
| Database redesign and data loss | Stable meanings, forward migrations, upgrade tests, export, and restore-tested backups |
| AI vendor churn or unsafe writes | Provider abstraction, Tool Gateway, explicit capability tiers, confirmation, and audit |
| Identity or cross-user data exposure | Require both Google identity and invitation, revocable sessions, owner-scoped services/repositories, and negative isolation tests |
| Live Google OAuth configuration | Real Google sign-in, invite denial, owner access, session persistence, and logout passed on local Compose; repeat configuration-specific checks before any new public deployment |
| Internet exposure and recovery | Review TLS, secrets, database transport, operational endpoints, and restore-tested backups before Alpha testing |
| Offline data loss or duplication | Keep V0.1 online; require accepted sync ADR and idempotency/recovery tests before V0.2 |
| Hosting cost drift | Treat USD $10 as a test target with spending visibility and shutdown controls, not a production guarantee |
| Agent drift | Required reading, module rules, ADR gate, tests, and completion reporting in `AGENTS.md` |
| Hosted verification gap | Run the pinned migration workflow on GitHub after an authorized push; only local workflow validation exists today |

## Pending Decisions

These decisions are intentionally deferred to their real decision points:

| Decision | Latest decision point | Baseline direction |
| --- | --- | --- |
| Web UI component library | Revisit if feature complexity demonstrates a need | Issue #23 uses local CSS and SVG components without a new library; maintain accessibility and consistency |
| Invitation operator entry point and session details | Completed in Issue #19 | Local trusted operator command and 24-hour revocable sessions implemented; later user/session administration needs its own scope before Internet exposure |
| Offline sync architecture | Before V0.2 implementation | Accept a focused ADR; do not prebuild a protocol in V0.1 |
| Cloud test provider and spending controls | Before paid or Internet-facing testing | Review concrete cost, TLS, secrets, backup, and shutdown plan |
| First Ollama model | Only if a later AI milestone is authorized | Evaluate against actual hardware and tool needs |
| Open-source license | Before the first public release | Decide through a separate ADR |
| Native client | After real Web/PWA use | Capacitor is a candidate, not a commitment |
| Bluecoins product import boundary | Before changing ADR-009/013 commitments | Review proposed ADR-015; retain safe import/export principles and require separate approval for any personal conversion |
| Wishlist versus offline sync order | V0.2 planning | Both remain planned; Product Owner decides exact order after the first search/chart priorities |

## Deferred Product Ideas

- Optional color theme packs and personal visual customization can be evaluated after the core workflows are usable. The approved Sage Green design remains the only implemented theme; no theme store or payment feature is planned in the current work.
- Recurring income/expense scheduling and broad icon/text refinements remain deferred. API-backed transaction search/filter and monthly/category charts are V0.2 priorities. Historical running balances need correct Finance semantics before evaluation. The separate financial-correctness and idempotency gates are mandatory before real-data Alpha use; see [Roadmap](ROADMAP.md).

## Resume Checklist

When work resumes:

1. Read `AGENTS.md`.
2. Read `docs/VISION.md`, `docs/ARCHITECTURE.md`, and `docs/DOMAIN_MODEL.md`.
3. Read `docs/V0.1_SCOPE.md` and this file.
4. Read the relevant ADRs.
5. Confirm the task's acceptance criteria and non-goals.
6. Inspect the existing local environment before proposing setup or dependency changes.
7. Use `docs/FOUNDATION_STARTUP.md` for the verified local Foundation workflow.

## State History

### 2026-10-10

- The Product Owner approved the second Issue #24 Sage Green Money UI. Final Go, web, OpenAPI, generated-type, and PostgreSQL integration checks passed. A real Edge browser completed the Money workflow through the production React bundle, normal session/CSRF guards, Go API, and isolated PostgreSQL v4 with synthetic data. The personal database and existing disposable databases were left untouched; a new disposable browser-test database was retained. Issue #24 was authorized for commit, push, and closure after this verification.

### 2026-10-09

- The Product Owner visually approved the Sage Green Issue #23 shell. Final web and isolated browser regressions passed without redesign, backend/schema changes, or personal-data use. The commit was pushed and remote `main` verified; Issue #23 was closed. Color theme packs and customization were recorded only as a deferred idea.
- Checked for an equivalent #24 issue and created Issue #24. Implemented the Money UI locally against the existing generated API contracts, with exact minor-unit parsing, session/CSRF requests, real owner-scoped balances and history, mobile forms, confirmation flows, and refresh/error states. Disposable PostgreSQL integration tests and an isolated synthetic browser fixture avoided the personal database. At this handoff the work awaited Product Owner review; using the personal database still requires separately approved backup and v4 migration.
- Created Issue #23 after checking for an equivalent issue. Implemented the four-area mobile-first React shell with real navigation, guarded profile/session display, working sign-out, responsive layouts, and honest Today/Money/Plans states. No Finance UI connection, backend endpoint, migration, personal-data change, or deployment was included in #23.
- Created Issue #22 after checking for an equivalent issue, then completed it after Product Owner final review. Money REST endpoints reuse Issue #21 services behind the Issue #19–#20 session, Origin, and CSRF guards. Strict DTOs and stable JSON errors reject invalid input and owner overrides. Minor units are decimal strings in JSON to preserve signed int64 exactness in generated TypeScript clients. OpenAPI and generated types were updated without a new migration, UI, deployment, or personal-data change. PostgreSQL integration tests and contract checks passed; live browser-to-Money testing remains for the UI milestone.
- Created Issue #21 after checking for an equivalent issue. Migration 000004 and Finance service/repository work implement the ADR-004 lightweight ledger with owner-scoped composite foreign keys, currency and category-kind integrity, atomic writes, account locking, archival, and void metadata. The Product Owner approved negative ordinary-account balances in final review; regression tests cover expenses, transfers, concurrent posting, and income voids across zero. The full Go suite with PostgreSQL integration tests, populated version 3→4 upgrade, clean version-4 migration chain, `go vet`, `go mod tidy -diff`, formatting, and diff checks passed. No personal financial data, public Money route, UI, or deployment was changed.
- Issue #19 was pushed and closed after Product Owner approval and live local Google verification. Issue #20 was created after checking for an equivalent issue and completed after Product Owner final review. Its owner-isolation integration subtest and full Go suite passed against disposable PostgreSQL databases; `go vet`, the clean migration chain at version 3, and `git diff --check` passed. The disposable test databases remain intact pending separate cleanup authorization.
- On 2026-10-09 the Product Owner completed the local live Google OIDC gate: the invited owner signed in through Google, `/api/v1/admin/status` returned 204 without a redirect, a Google test user without a LifeOS invitation reached LifeOS and was denied, the owner session survived a page reload, and sign-out remained effective after reload. The API initially failed Google discovery because its BusyBox runtime lacked a CA bundle; the local Dockerfile now copies standard CA certificates from the build stage, and the rebuilt API became healthy without disabling TLS verification. Issue #19 was subsequently pushed and closed.
- Product Owner authorized final Issue #19 review and completion. The full Go suite ran with both PostgreSQL integration tests enabled; the populated v2→v3 upgrade, guarded down migration, and clean version-3 chain passed. Invalid OIDC proofs, state/replay, PKCE, invitation/concurrent admission, fixed redirect, Secure/HttpOnly/SameSite cookies, CSRF, disabled users, expired/revoked sessions, and absent operator HTTP routes were checked. OpenAPI's two redirect-only 2xx warnings are documented and non-blocking. The live Google gate was completed later as recorded above.
- Created Issue #19 after checking for an equivalent issue. Local work adds Google OIDC verification, anti-forgery/replay checks, invitation admission, revocable sessions, a protected current-user API, local operator command, OpenAPI contract, and web sign-in UI. Automated local-provider tests are distinct from a live Google test.
- Created Issue #18 after checking for an equivalent issue. Local work adds migration version 2 and minimal identity persistence under ADR-014. Package-private bootstrap/revocation methods require explicit internal operator authorization; no production issuer or network path exists yet. The designated bootstrap invitation alone grants the owner-admin flag, while ordinary invitations never do so, including under concurrent acceptance. Admission takes a package-private verified-identity value; a later OIDC service must construct it only after server-side validation. Later services must derive the admin ID from a verified session. An explicit PostgreSQL integration run passed the version 1→2 upgrade, security/integrity checks, and data-preserving down-migration refusal. The full Go suite, `go vet`, and clean migration-chain verifier also passed locally.
- Product Owner approved Issue #16's Life-first, mobile-first, invite-only multi-user rebaseline. The original single-user scope and Bluecoins-first V0.1 timing are intentionally replaced as product direction; ADR-008/009 historical files remain preserved.
- Created Issue #17 for documentation and ADR rebaseline. The Product Owner reviewed and accepted ADR-013/014, then authorized documentation-only commit, push, and Issue #17 closure after verification. No code, API, schema, migration, deployment, or Issue #18 work was authorized.
- V0.1 is an online Alpha with Today/Money/Plans/Me, invitation-gated Google identity, user isolation, suitable export, security tests, and tested backup/restore. V0.2 adds offline capture/sync and Wishlist after an accepted sync ADR; Bluecoins import is V0.3 or later.

### 2026-09-30

- Implemented and verified Issue #8 locally using pinned `golang-migrate` v4.20.1. A clean Compose PostgreSQL database reached production migration version 1 with no domain tables, repeated `up` reported no change, and the isolated smoke fixture proved clean apply/version/one-step rollback behavior. The GitHub issue remains open pending later review and closure.
- Implemented and verified Issue #9 locally using pinned `chi` v5.3.0 and `pgx` v5.11.0. Focused and full Go tests passed; live `/healthz` and `/readyz` checks returned HTTP 200 with request IDs, healthy PostgreSQL, and clean migration version 1. No product endpoint or domain behavior was added.
- Implemented and verified Issue #10 locally using pinned React 19.3.0, TypeScript 7.0.2, Vite 8.3.1, and Vitest 5.0.2. Type checking, focused tests, and the production build passed; the accessible shell contains no financial data, product screen, or component-library decision.
- Implemented and verified Issue #11 locally. Compose configuration and image builds passed; PostgreSQL, API, and web reached healthy state, the migration job exited successfully, direct and same-origin-proxied readiness reported clean version 1, and all host ports bound only to `127.0.0.1`. Normal shutdown removed containers/network and preserved the named volume.
- Implemented and verified Issue #12 locally. The shared script passed from an empty disposable PostgreSQL 18 database, proving production apply/version/repeated-up and isolated fixture up/version/down behavior. Pinned `actionlint` validated the workflow; no hosted run exists because the sprint forbids pushing.
- Implemented and verified Issue #13 locally. The canonical OpenAPI 3.1 contract describes only the implemented operational endpoints and validates cleanly with pinned Redocly CLI 2.56.0; licensing remains explicitly deferred under ADR-010.
- Implemented and verified Issue #14 locally. Pinned `openapi-typescript` 7.13.0 generated committed Foundation API types; drift checking, OpenAPI lint, web type checking, tests, and the production build all passed.
- Implemented and verified Issue #15 locally from an isolated committed snapshot. Project-local dependency bootstrap, full Go checks, OpenAPI validation and deterministic generation, web checks, Compose startup, migration status, direct and proxied health/readiness, and isolated cleanup passed. The documented flow requires no Finance/domain behavior or private data. The hosted GitHub Actions run remains pending because no push was authorized.

### 2026-08-19

- Confirmed Issue #4 as completed, accepted, and closed with ADR-012 remaining accepted.
- Completed, accepted, and closed Issue #5 after defining the minimal Foundation configuration contract and safe `.env.example`; no configuration loader was implemented.
- Completed, accepted, and closed Issue #6 after implementing and verifying the PostgreSQL 18 Compose service; the container and network remain stopped while the named volume is preserved.
- Completed, accepted, and closed Issue #7 after implementing and verifying the typed Foundation configuration package with process environment over the project-root `.env` over defaults, key-specific validation, and secret-safe errors. The focused package tests, full Go suite, and `go vet` pass.

### 2026-08-18

- Recorded and accepted ADR-011 with the Product Owner's `golang-migrate` v4 decision, pinned project-local invocation requirement, ordered forward-chain policy, explicit transaction preference, and manual dirty-state recovery model.
- Completed, accepted, and closed Issue #3 after review; no migration tool was installed or executed, and no migrations, schema, Docker, CI, application work, Issue #4, Issue #8, or later issue was started.
- Recorded ADR-012 with the Product Owner's accepted `chi` v5 decision, thin-handler boundary, OpenAPI authority, project-local version policy, security consequences, and transport-layer exit path.
- Completed, accepted, and closed Issue #4. `chi` was not installed, and API implementation did not begin.

### 2026-08-17

- Established the documentation, governance, ADR, project-state, and learning foundation from Master Project Blueprint v0.2.
- Recorded the Product Owner's acceptance of the documentation baseline.
- Created and accepted ADR-001 through ADR-010 from that baseline.
- Published the Foundation milestone and all 15 ordered issues to the private GitHub repository.
- Recorded issue dependencies, acceptance criteria, explicit non-goals, and environment/ADR gates without implementing any issue or changing the local environment.
- Completed the Issue #1 inspection and approved blocker resolution: verified Go 1.26.6, Docker Engine/CLI 29.2.1, Docker Desktop 4.61.0, Compose v5.0.2, Git, Node.js, and available package-manager tooling; closed Issue #1 after Product Owner acceptance.
- Completed the Issue #2 monorepo skeleton: created the accepted module boundaries and reserved repository areas, added dependency-free Go/web metadata, documented ownership, passed the minimal structural and Go build checks, and closed Issue #2 after Product Owner acceptance; did not start Issue #3.
- Kept all application implementation explicitly out of this task.
