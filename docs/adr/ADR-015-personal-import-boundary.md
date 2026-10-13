# ADR-015: Personal Conversion Instead of a Bluecoins Product Milestone

**Status:** Proposed — Product Owner approval required

**Date:** 2026-10-13

## Context

ADR-009 originally selected Bluecoins CSV as the first product import adapter. ADR-013 moved that adapter from V0.1 to V0.3 or later while retaining its first-adapter commitment. The Product Owner's current roadmap direction does not require a general-purpose Bluecoins import system as a product milestone. It prioritizes durable daily Money/Goals use, export, backups, safe upgrades, and later Trips/Plans. This proposed record makes the conflict explicit without editing either historical accepted ADR.

## Proposed decision

- Remove the Bluecoins-first-adapter product commitment and Bluecoins import as a V0.3 milestone. This supersedes only those portions of ADR-009 and ADR-013 if accepted.
- Retain first-class suitable export and the safe-import principles of canonical validated input, dry-run preview, explicit mapping, traceable batches, idempotency, reconciliation, and controlled rollback for any product import capability that is later authorized.
- Consider a separately scoped one-off personal data conversion only after a validated LifeOS import format and a reviewed backup, dry run, mapping, reconciliation, and recovery path exist. This proposal does not authorize reading or changing personal data.
- V0.3 product planning may prioritize Trips and richer Plans through separate issues; it does not authorize their implementation.

## Alternatives considered

Keep a general-purpose Bluecoins adapter in V0.3: preserves the prior promise but consumes a product milestone for one source before daily-use needs are validated. Direct SQL conversion: bypasses application validation and traceability and is unsuitable for personal data.

## Consequences

The roadmap can focus on durable V0.1 and broader V0.2 Money utility. Import remains a deliberate, separately approved capability rather than disappearing silently. ADR-009 and ADR-013 remain accepted until this proposal is approved; their retained safety and export rules continue afterward.

## Data migration impact

No schema or data changes follow from this proposal. Any future import format or conversion needs its own compatibility, validation, backup, and recovery tests. Normal upgrades continue through forward migrations without resetting real data.

## Security and AI impact

Owner isolation applies to all future import/export and conversion paths. Personal conversion requires separate authorization and must not use real data in ordinary development. AI is not required for mapping or correctness.

## Rollback or exit path

If a reusable Bluecoins adapter later proves necessary, scope it as a new product issue and implement it through the accepted safe-import principles. Keep exports complete enough to leave LifeOS.
