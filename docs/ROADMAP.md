# LifeOS Roadmap

**Status:** Issue #16 direction approved; ADR-013 and ADR-014 accepted

**Updated:** 2026-10-12

## Roadmap Rule

Versions describe usable capability, not date promises. Every stage must be valuable in daily use, preserve earlier data, and grow through migrations rather than restart the project.

Unplanned ideas go to a backlog unless they change core semantics. They should not interrupt the current milestone.

## Capability Sequence

| Release | Goal | Major capability |
| --- | --- | --- |
| Foundation | Build the base | Repository, governance, Docker, PostgreSQL, migrations, Go module boundaries, OpenAPI, CI, and ADRs |
| V0.1 Invite-only online Alpha | Prove a useful, private mobile-first life workflow | Google sign-in plus invitations; Today, Money, Plans checklist Goals, Me; user isolation, export, tested backup/restore |
| V0.2 Offline capture and Wishlist | Enter transactions during disconnection and connect desired purchases to plans | Reliable transaction outbox/sync after an accepted ADR; Wishlist without double-counted spending |
| V0.3 or later | Extend real-data continuity and Plans | Bluecoins import with the accepted safe pipeline; additional Plans functionality |
| Later milestones | Expand only after use proves the need | Trips, Assets, People/Split, privacy-conscious Admin Dashboard, optional AI/MCP, self-host hardening, and possible native client |
| V1.0 | Define from sustained use | Reliability, privacy, migrations, export, backup/restore, and complete workflows across the modules actually shipped |

## Foundation

Foundation #1–#15 establishes the documented and locally verified technical base. Issue #16 changes product direction without discarding compatible Foundation work. The GitHub review/closure state remains separate from this documentation rebaseline.

Expected capability:

- repository documents and agent rules;
- accepted ADRs for the initial architecture baseline;
- application and deployment skeleton;
- PostgreSQL migration discipline;
- OpenAPI and CI baselines.

## V0.1 Invite-only Online Alpha

The current product milestone is defined in [V0.1_SCOPE.md](V0.1_SCOPE.md). Its acceptance path is invited Google access, strict independent user data, usable Today/Money/Plans/Me workflows, correct transfer and balance behavior, suitable export, security tests, and a restored backup.

Plans includes only simple checklist Goals. Offline sync, Wishlist, Bluecoins import, Trips, Assets, Split, AI, native apps, public signup, and a full Admin Dashboard are outside V0.1.

## Later Milestone Boundaries

### V0.2 - Offline Transaction Capture and Wishlist

Add a reliable offline create-transaction workflow and Wishlist. An accepted offline-sync ADR must define idempotency, pending/error states, recovery, conflict handling, and account/device boundaries before implementation. Wishlist remains desired intent and does not affect balances until linked to an actual FinancialEvent.

### V0.3 or Later - Bluecoins and Additional Plans

Import Bluecoins through the accepted canonical, previewed, mapped, traceable, idempotent, and reconcilable pipeline in ADR-009. Add further Plans functionality only through its own scoped issue. The former Bluecoins-first V0.1 schedule is intentionally replaced by Issue #16; its import safety rules remain.

### Later Capabilities

Trips, Assets, People/Split, AI, MCP, self-host hardening, and native mobile applications remain deferred until separately scoped and approved. Optional AI retains the accepted Tool Gateway and no-AI core. Capacitor is a candidate to evaluate after Web/PWA use, not a current commitment. A future owner-only Admin Dashboard should use aggregate operational metrics without unnecessary private financial data.

## V1.0 Outcome

V1.0 criteria will be set from observed use rather than assuming every deferred module is required. Data isolation, correct financial behavior, tested upgrades and restores, complete export for implemented domains, and no-AI operation remain durable quality gates. No normal workflow should require manual database surgery.

## Future Money Improvements and Alpha Gates

Product improvements to evaluate after the core V0.1 workflows: gradually reduce excessive explanatory UI text with appropriate accessible icons; recurring scheduled income and expense entries; monthly spending and category charts; API-backed transaction search and filtering. These are backlog ideas, not current implementation commitments.

Before the private Alpha relies on real personal finance data, review and resolve opening-balance effective dates with correct backdated-entry semantics, and owner-scoped idempotency for financial writes. Current warning text and uncertain-result retry guidance do not close these correctness gates. Historical post-transaction account balances are another deferred Money enhancement.

## Future Domains

Health/Wellness may be evaluated only after the core is stable and proven useful. If pursued, it must be an independent module with its own model, migrations, permissions, and UI. Health data is more sensitive and requires explicit AI opt-in and separate privacy policy. LifeOS must not present itself as a diagnostic or medical-advice system without dedicated medical and compliance design.

Other future life-data domains follow the same rule: prove the user need, define the boundary, and avoid an everything-shaped schema.
