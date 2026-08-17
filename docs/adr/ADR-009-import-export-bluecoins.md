# ADR-009: First-Class Import/Export with Bluecoins First

**Status:** Accepted

**Date:** 2026-08-17

## Context

LifeOS must become useful with the owner's real history rather than remain a fake-data demo. The owner has existing Bluecoins data that must be migrated and reconciled safely. Long-term self-hosting also requires a reliable way to leave LifeOS or move it to another machine.

A one-off script or direct SQL import would bypass domain rules, be difficult to preview, and make duplicate or partial imports hard to diagnose. A product without complete export would create data lock-in.

## Decision

Import and export are first-class product capabilities.

- LifeOS will define a canonical import input independent of any source format.
- Bluecoins CSV is the first source adapter and a V0.1 real-world acceptance path.
- Source adapters parse and normalize data but never insert directly into core tables.
- Imports use validation, explicit account/category mapping, and a dry-run preview before confirmation.
- Confirmed imports use an `ImportBatch` with source, timestamps, counts, status, detailed errors, and idempotency support such as source IDs or row fingerprints.
- Import commit goes through Finance application services and their invariants.
- Import mistakes are handled through controlled batch rollback/void behavior, not manual SQL repair.
- LifeOS provides complete structured JSON export of core data and useful CSV finance export.
- An anonymized representative Bluecoins sample is used for fixtures and analysis before importer implementation.

## Alternatives Considered

### Manual re-entry

Ask the owner to recreate history by hand. This is slow, error-prone, and prevents V0.1 from proving real replacement value.

### One-off direct database script

Insert Bluecoins rows directly into LifeOS tables. This bypasses validation and couples an external format to internal storage.

### Import without dry-run or batch identity

Commit rows immediately. This reduces interface work but makes mapping errors, partial failures, duplicates, and rollback unsafe.

### Proprietary or incomplete export

Offer only backup files or selected reports. This would weaken portability and the user's ability to leave.

## Consequences

### Positive

- V0.1 is tested against real personal history and reconciliation.
- Additional source adapters can reuse the canonical pipeline.
- Dry-run, mapping, idempotency, and errors make imports safer and explainable.
- Complete export reduces lock-in and supports migration/self-host portability.

### Costs and Trade-Offs

- Import is a workflow with parsing, mapping, validation, preview, commit, audit, and error handling rather than a simple file upload.
- Source-format edge cases require anonymized fixtures and reconciliation tests.
- Export compatibility must be maintained as the domain grows.

## Data Migration Impact

The schema needs traceable import-batch identity and a way to associate imported records or fingerprints with their source. Bluecoins data must be converted into canonical owner-scoped money, currency, account, category, event, movement, and allocation semantics.

Future schema changes must keep exports complete and migrate imported history without requiring re-import.

## Security and AI Impact

Import and export files contain sensitive personal data. Samples used in the repository must be anonymized. Exports require owner authorization and safe handling. AI is not required for import mapping or correctness; any future AI assistance remains a draft aid behind the Tool Gateway.

## Rollback or Exit Path

A source adapter can be retired without changing canonical records after successful import. A confirmed batch uses controlled rollback/void rules and backup where necessary. The complete JSON/CSV export path is the user's long-term exit from LifeOS and must remain available even if import implementations change.
