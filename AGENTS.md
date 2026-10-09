# LifeOS Agent Guide

This file is the durable entry point for coding agents working in this repository. The repository, not chat history, is the shared project memory.

## Authority and Scope

- The Master Project Blueprint v0.2 in `docs/reference/` is the original product and architecture baseline.
- The maintained repository documents under `docs/` are the working source of truth derived from that blueprint.
- The Product Owner approved the Life-first, invite-only multi-user rebaseline in GitHub Issue #16 and accepted ADR-013 and ADR-014. Preserve the historical ADRs and follow `docs/PROJECT_STATE.md` for the current boundary.
- If repository documents disagree with the blueprint or with each other, identify the conflict and follow an explicit Product Owner decision and accepted ADR when one exists; otherwise stop and ask. Issue #16 explicitly changes the single-user and Bluecoins-first V0.1 direction. Do not silently choose a new product meaning.
- The Product Owner decides real needs, priorities, and acceptance. Agents implement scoped work; they do not redefine LifeOS semantics on their own.

## Current Product Boundary

- Issue #16 approves a Life-first, mobile-first platform and an invite-only, online V0.1 Alpha for the Product Owner and a few testers.
- Google sign-in does not grant access without an invitation. Each admitted user has independent data.
- V0.1 navigation is Today, Money, Plans, and Me. Money includes manual income/expense, accounts, categories, history, correct transfers, and deterministic balances; Plans begins with checklist Goals.
- V0.1 includes suitable export, security and isolation tests, and tested backup/restore. Offline transaction capture and Wishlist are V0.2; Bluecoins import is V0.3 or later.
- Trips, Assets, Split, AI runtime, a native app, and a full Admin Dashboard are deferred. Capacitor is a candidate, not a commitment.
- ADR-013 and ADR-014 are accepted. Preserve historical ADR-008 and ADR-009 files; see `docs/adr/README.md` for the superseded and retained portions. Implement product behavior only in a separately authorized issue.

## Required Reading Before Work

Before changing code, data, APIs, infrastructure, or normative documentation:

1. Read `docs/VISION.md`.
2. Read `docs/ARCHITECTURE.md`.
3. Read `docs/DOMAIN_MODEL.md`.
4. Read `docs/V0.1_SCOPE.md` for work in the current milestone.
5. Read `docs/PROJECT_STATE.md`.
6. Read the relevant records in `docs/adr/`.
7. Restate the task's scope, acceptance criteria, and explicit non-goals before implementation.

For a documentation-only task, read the same sources in proportion to the decision being documented.

## Non-Negotiable Baseline

- Core features must work fully with AI disabled.
- Structured domain data is authoritative. Chat history, model memory, dashboard layout, and generated summaries are not domain facts.
- AI never receives database credentials, unrestricted SQL, arbitrary filesystem access, provider secrets, or an unregistered destructive tool.
- Preserve domain meanings: actual money is a `FinancialEvent`; intent is a `Plan`; an owned thing is an `Asset`; a desired thing is a `WishlistItem`; a shared cost is a `SplitExpense`.
- Use migrations for persistent schema change. Never reset real or production data as an upgrade strategy, and never rewrite or delete a published migration.
- Import and export are first-class. Imports use validation, mapping, dry-run, traceable batches, and controlled rollback.
- Keep a Go modular monolith with explicit domain boundaries. Do not introduce microservices without an accepted ADR.
- Keep the server and core client-agnostic. Web/PWA, future native clients, CLI, runtime AI, and MCP must use the same application rules.
- AI permissions are explicit, tool-scoped, confirmed according to risk, and audited. There is no god-mode agent.
- Preserve extension points without pre-building providers, databases, queues, synchronization, or modules that have no current requirement.

Any proposed change to these principles requires an ADR that explains the reason, data migration impact, consequences, and rollback or exit path before implementation starts.

## Module and Data Rules

- Modules communicate through explicit services or interfaces. Do not casually access another module's tables.
- Business rules live in domain/application code, not only in a client.
- API DTOs and AI tool schemas are contracts, not direct database row exposure.
- All owner data is owner-scoped from day one.
- Store money as integer minor units plus ISO 4217 currency; never use floating-point values for money.
- Store timestamps in UTC and retain the user's timezone; use date types for calendar-only plan/travel dates where appropriate.
- Real financial events, planned amounts, asset records, wishlist state, and split ownership must remain separate and be linked instead of duplicated.
- AI tools call application services through the Tool Gateway; they never bypass validation to write the database.

## Change Discipline

- Keep work limited to the agreed issue or task. Put unrelated ideas in a backlog rather than expanding the current scope.
- Every schema change requires a new numbered migration and an upgrade-path test.
- Update OpenAPI and contract tests when external API behavior changes.
- Test business invariants, error paths, owner isolation, imports, migrations, and AI permission/confirmation behavior where relevant.
- Run the appropriate tests and migration chain from a clean database before merge when implementation exists.
- Do not add dependencies or infrastructure without a demonstrated need and a clear ownership boundary.
- Update `docs/PROJECT_STATE.md` when a task changes milestone status, completed work, next steps, risks, or pending decisions.
- Record normative architecture or domain decisions in `docs/adr/`; learning notes and chat are not substitutes.

## Completion Report

At the end of a task, report:

- files changed;
- behavior or documentation changed;
- tests and checks run, including anything not run;
- migration, compatibility, security, or AI-permission impact;
- follow-up risks, deferred decisions, and the next useful action.

## Learning Journal

- `learning/` is non-normative and supports the owner's engineering learning.
- Record what was implemented, why the design was chosen, relevant Go/SQL/architecture concepts, a useful reading order, and unresolved questions.
- Keep durable product rules in `docs/`, not only in learning notes.
- Never put secrets, raw private financial data, or sensitive prompts in learning entries.

### Function-level Data Flow for Implementation Issues

Every future implementation issue must include a concise explanation of the implemented path:

`User Input → Frontend function → API request → Backend handler → Service → Repository → Database → Response → UI update`.

For each step, record the exact file path and function name, input and output, main processing logic, the next function called, error handling, and relevant tests. Label names as **proposed** before code exists. Mark a genuinely non-applicable step **N/A** with a reason. Update the explanation to actual implementation names before closing the issue. Keep it understandable to a developer learning the codebase; include it in the implementation issue's completion report and corresponding learning entry.

## Local Machine Safety

- Treat files outside the LifeOS repository as read-only by default.
- Never delete or modify files outside this repository without my explicit approval.
- Never modify PATH, system environment variables, registry, shell profiles, Go installation, Git global configuration, or other development tools without explicit approval.
- Never install or uninstall system-wide software without explicit approval.
- Before running unfamiliar install/setup commands, explain the purpose, affected files/directories, system impact, and reversibility.
- Prefer project-local dependencies.
- Destructive operations require explicit approval.
- Inspect my existing development environment before proposing changes.
- Use the least invasive solution.

These rules also prohibit broad cleanup or administrator-level changes outside the repository unless the Product Owner has explicitly approved the exact action.
