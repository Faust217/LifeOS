# ADR-013: Life-first Multi-user Product Rebaseline

**Status:** Accepted

**Date:** 2026-10-09

## Context

The Master Project Blueprint v0.2 and accepted ADR-008 established a single-user, self-hosted V0.x path. ADR-009 and the original V0.1 scope made Bluecoins import a V0.1 acceptance point. Foundation #1–#15 implemented infrastructure and an operational shell without creating product tables or workflows. The Product Owner approved a different product direction in [Issue #16](https://github.com/Faust217/LifeOS/issues/16): a Life-first, mobile-first platform beginning with an invite-only online Alpha for the owner and a few testers.

This is an intentional product rebaseline. Historical blueprint and ADR files remain preserved; their conflicting scope statements must not be silently treated as current implementation instructions.

## Decision

- V0.1 is an invite-only, online Alpha with independent user data. Google sign-in alone never grants access without an invitation.
- The V0.1 navigation is Today, Money, Plans, and Me. Today is a basic life overview; Money provides manual income/expense, accounts, categories, history, deterministic balances, and correct transfers; Plans begins with simple checklist Goals; Me provides profile, sign-out, and basic data management.
- V0.1 requires suitable export, owner-isolation/security tests, and tested backup/restore. AI is optional and excluded from the initial Alpha.
- V0.2 adds reliable offline transaction capture/synchronization and Wishlist. An accepted offline-sync ADR is required before sync implementation; this ADR does not select a sync protocol.
- V0.3 or later adds Bluecoins import and further Plans functionality. The Bluecoins-first **V0.1 schedule** in ADR-009 is replaced; its first-class import/export, dry-run, mapping, batch, idempotency, reconciliation, and rollback principles remain.
- Trips, Assets, Split, AI runtime, native mobile apps, and the full owner-only Admin Dashboard remain deferred. Admin metrics, when built, must be privacy-conscious.
- React Web/PWA comes first. Capacitor is a future candidate, not an architectural commitment.
- Preserve compatible Foundation #1–#15 work and accepted choices: Go modular monolith, React/TypeScript/Vite, `chi`, PostgreSQL, numbered migrations, REST/OpenAPI, Docker Compose, server-owned business rules, integer money, and the lightweight finance ledger.
- Approximate USD $10 is a cloud **testing target**, not a guaranteed 24/7 production budget.

**Supersession boundary:** This ADR supersedes ADR-008's *single-user V0.x product scope* and ADR-009's *Bluecoins-in-V0.1 timing*. It retains ADR-008's owner-scoping rule and ADR-009's first-class import/export, Bluecoins-first-adapter, and safe-import rules. ADR-008 and ADR-009 files remain unchanged as historical records. Accepted ADR-014 defines the identity and authorization boundary for the new scope.

## Alternatives Considered

- Keep the original single-owner, Bluecoins-first V0.1: preserves the prior plan but does not match the Product Owner's approved Life-first Alpha and tester use.
- Build public SaaS, organizations, billing, or all life domains immediately: exceeds the approved small Alpha and adds cost and security surface before real use.
- Replace Foundation architecture: no technical evidence justifies discarding the compatible Go, PostgreSQL, React, contract, and migration work.

## Consequences

The first product work must establish invitations, sessions, and enforced user isolation before user-owned records. A smaller V0.1 can validate daily use sooner, while safe Bluecoins migration remains later work. Offline capture is treated as a separate reliability problem in V0.2. The retained modular monolith and client-agnostic service rules support later clients without duplicating business behavior.

The V0.1 release gate changes, so README, vision, scope, roadmap, architecture, domain model, project state, governance, and issue plans must agree. Dependent implementation still requires a separately authorized issue.

## Data Migration Impact

This ADR changes no files outside documentation and creates no schema or user data. Foundation's production migration is an operational baseline with no LifeOS domain tables. Future user, invitation, session, Money, and Goal records need new numbered migrations and upgrade-path tests. No reset or re-import of real data is an acceptable upgrade strategy. Bluecoins import remains a later canonical, owner-scoped migration workflow.

## Security and AI Impact

The online Alpha needs verified identity plus invitation, protected sessions, strict user-data isolation, privacy-safe logging, TLS, appropriate database transport security, and tested backup/restore before Internet exposure. Owner-only administrative capabilities must be separate from ordinary user paths and avoid unnecessary access to private financial records. AI remains disabled in V0.1; accepted no-AI and Tool Gateway rules remain in force for any later AI work.

## Rollback or Exit Path

If the Alpha direction is reconsidered, a new ADR must define revised users, release gates, access-control and data-migration consequences. Retained Foundation modules and complete export preserve a path to local or self-hosted use. Do not remove user scoping or rewrite shipped migrations to revert a product decision.
