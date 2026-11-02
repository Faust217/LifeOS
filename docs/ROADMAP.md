# LifeOS Roadmap

**Status:** Issue #16 direction approved; ADR-013, ADR-014 and ADR-016–020 accepted. Issue #35 Atomic Edit is completed and Product Owner accepted for delivery. Bluecoins import scope change awaits proposed ADR-015 acceptance.

**Updated:** 2026-11-02

## Roadmap Rule

Versions describe usable capability, not date promises. Every stage must be valuable in daily use, preserve earlier data, and grow through migrations rather than restart the project.

Unplanned ideas go to a backlog unless they change core semantics. They should not interrupt the current milestone.

## Capability Sequence

| Release | Goal | Major capability |
| --- | --- | --- |
| Foundation | Build the base | Repository, governance, Docker, PostgreSQL, migrations, Go module boundaries, OpenAPI, CI, and ADRs |
| V0.1 Invite-only online Alpha | Prove a useful, durable private mobile-first life workflow | Me/export complete; Opening Balance → Idempotency → Atomic Edit → Historical Running Balances under separate reviews; verify backup/restore and non-destructive upgrades; establish Personal Data Ready gate |
| V0.2 Insight and capture | Make Money history easier to use and assess offline needs | API-backed transaction search/filter and monthly/category spending charts first; Wishlist and offline sync remain planned, with their order subject to Product Owner approval |
| V0.3 | Extend Plans after daily-use evidence | Trips and richer Plans capabilities, each requiring separate scope review |
| Later milestones | Expand only after use proves the need | Assets, People/Split, privacy-conscious Admin Dashboard, optional AI/MCP, self-host hardening, and possible native client |
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

Issue #31 is completed, pushed and closed. The Product Owner accepted [ADR-017](adr/ADR-017-owner-data-export.md) and authorized [Issue #32](https://github.com/Faust217/LifeOS/issues/32) **Me / Owner Data Export** on 2026-10-20. The [approved design](EXPORT_DESIGN.md) is implemented, verified and Product Owner approved: compact profile/sign-out/timezone management, complete versioned normal owner JSON and guarded Finance CSV ZIP with exact minor-unit/readable decimal values. The Product Owner approved visual and actual export-file review and authorized final delivery after verification; Issue #32 records its commit/closure. Export does not pass Personal Data Ready or introduce import/restore. ADR-015 remains a separate Proposed decision.

The Product Owner approved the next Finance correctness/daily-use order. #33 and #34 are delivered; #35 is implemented locally pending Product Owner review, while #36 remains unauthorized:

1. [#33 Opening Balance Effective Date and Backdated Transactions](https://github.com/Faust217/LifeOS/issues/33): known-balance instant, explicit safe legacy conversion; [ADR-018](adr/ADR-018-opening-balance-effective-instant.md) Accepted. Completed, pushed and closed at [`09d365563dc4cc08c3ae11b5a162a78c727af86a`](https://github.com/Faust217/LifeOS/commit/09d365563dc4cc08c3ae11b5a162a78c727af86a).
2. [#34 Owner-scoped Financial Write Idempotency](https://github.com/Faust217/LifeOS/issues/34): [ADR-019](adr/ADR-019-owner-scoped-financial-write-idempotency.md) Accepted 2026-10-26. Six keyed writes, atomic v9 markers/ledger, content-free replay, 90% capacity warning and frozen per-tab recovery are delivered and closed at [`884b013d98d65f52d8f0e2dab424cfb03916539f`](https://github.com/Faust217/LifeOS/commit/884b013d98d65f52d8f0e2dab424cfb03916539f). See [Recovery and capacity operations](FINANCIAL_WRITE_RECOVERY.md).
3. [#35 Atomic Edit Transaction with Revision Audit](https://github.com/Faust217/LifeOS/issues/35): stable event ID, narrow one-save edit, complete splits and non-counting prior revision evidence. [ADR-020](adr/ADR-020-atomic-financial-event-edit-revisions.md) Accepted 2026-10-27; v10 snapshots, guarded atomic Edit and draft-safe UI are completed. Final UI/financial evidence accepted and delivery authorized 2026-11-02; normal actions are Edit and confirmed Delete, with Void compatibility retained.
4. [#36 Historical Per-Account Running Balances](https://github.com/Faust217/LifeOS/issues/36): complete account data, deterministic order, compact Sage Green rows and snapshot-aware pages.

Each depends on the previous capability and explicit design/implementation authorization. Do not infer legacy anchors, present incomplete pre-anchor reconstruction as bank-verified, count old edit revisions, or redefine export v1/v2. Issue #33 implements the accepted v2 boundary and forward migration v8 in disposable tests; #34 adds v9 keyed writes; #35 adds v10 internal snapshots and atomic editing. #36 implementation has not begun and needs separate authorization. Personal-data access, deployment and automatic progression remain unauthorized.

The current product milestone is defined in [V0.1_SCOPE.md](V0.1_SCOPE.md). Today and the focused Money category/Void refinements are complete. The Product Owner accepted [ADR-016](adr/ADR-016-financial-event-soft-delete.md) and closed [Issue #29](https://github.com/Faust217/LifeOS/issues/29) after verification and visual review. Linked ledger records remain internally. The Product Owner approved [Issue #30](https://github.com/Faust217/LifeOS/issues/30) compact date-grouped Money history and persisted owner timezone; implementation was delivered and Issue #30 was closed. [Issue #31](https://github.com/Faust217/LifeOS/issues/31) is a focused Today/Plans UI polish after Money; its implementation passed Product Owner visual review. Me/export is complete in Issue #32. The next four Finance capabilities follow the approved order above, then separately scoped database backup/restore and non-destructive upgrade rehearsal remain mandatory before real-data use. Evaluate an installable PWA, secure HTTPS hosting, and reliable persistent storage before inviting real-data use. Owner isolation and safe forward migrations remain permanent requirements.

**Personal Data Ready gate:** Do not use real financial data until opening-balance/backdated correctness, idempotent writes, owner isolation, suitable export, verified backups and restore, non-destructive upgrade, and an appropriately secured persistent deployment have evidence and Product Owner acceptance. A working dashboard or browser preview alone does not pass this gate.

Plans includes only simple checklist Goals. Offline sync, Wishlist, Bluecoins import, Trips, Assets, Split, AI, native apps, public signup, and a full Admin Dashboard are outside V0.1.

## Later Milestone Boundaries

### V0.2 - Money Insight, Wishlist, and Offline Capture

Prioritize API-backed transaction searching/filtering and monthly/category spending charts, using complete server-side Finance semantics rather than a client page of history. Wishlist and reliable offline create-transaction synchronization remain planned; their exact order needs Product Owner approval. An accepted offline-sync ADR must define idempotency, pending/error states, recovery, conflict handling, and account/device boundaries before implementation. Wishlist remains desired intent and does not affect balances until linked to an actual FinancialEvent. Consider Android packaging only if actual PWA use shows it is insufficient.

### V0.3 - Trips and Richer Plans

Prioritize Trips and richer Plans, subject to separate scope and domain review. The Product Owner no longer requires a general-purpose Bluecoins importer as a product milestone. A one-off personal conversion may be considered separately through a validated LifeOS import format, with a backup, dry run, mapping, reconciliation, and traceability. This changes the retained Bluecoins-first-adapter commitment in ADR-009 and the V0.3 import statement in ADR-013; [proposed ADR-015](adr/ADR-015-personal-import-boundary.md) records the exact supersession for Product Owner approval. Until acceptance, the accepted ADRs remain authoritative where they conflict.

### Later Capabilities

Assets, People/Split, AI assistants, monetization, and advanced modules remain deferred until separately scoped and approved. Recurring income/expense scheduling and broader icon/text UX refinements are deferred. Optional AI retains the accepted Tool Gateway and no-AI core. Native/Android packaging is conditional on PWA experience; Capacitor is only a candidate. A future owner-only Admin Dashboard should use aggregate operational metrics without unnecessary private financial data.

## V1.0 Outcome

V1.0 criteria will be set from observed use rather than assuming every deferred module is required. Data isolation, correct financial behavior, tested upgrades and restores, complete export for implemented domains, and no-AI operation remain durable quality gates. No normal workflow should require manual database surgery.

## Future Money Improvements and Alpha Gates

The API-backed search/filter and monthly/category charts are now V0.2 priorities. Recurring scheduled income/expense and broad icon/text refinement remain deferred backlog ideas.

Opening-balance/backdated and owner-scoped idempotency decisions are delivered under Issues #33/#34; #35 Atomic Edit is completed and accepted. Before real personal finance data, rehearse backup/restore and safe upgrades, establish secure persistent operation, and obtain Product Owner acceptance of the Personal Data Ready gate. Historical post-transaction balances remain the fourth Finance scope above; #36 has not begun.

## Future Domains

Health/Wellness may be evaluated only after the core is stable and proven useful. If pursued, it must be an independent module with its own model, migrations, permissions, and UI. Health data is more sensitive and requires explicit AI opt-in and separate privacy policy. LifeOS must not present itself as a diagnostic or medical-advice system without dedicated medical and compliance design.

Other future life-data domains follow the same rule: prove the user need, define the boundary, and avoid an everything-shaped schema.
