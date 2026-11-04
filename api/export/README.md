# LifeOS export v1 producer contract

[`lifeos-export-v1.schema.json`](lifeos-export-v1.schema.json) is the exact current JSON producer schema, independent of API and database versions. [`testdata/lifeos-export-v1.json`](testdata/lifeos-export-v1.json) is a synthetic fixture captured from the real guarded Go/PostgreSQL exporter. It contains no real user data or authentication secrets.

The approved JSON record fields, CSV header order, ZIP manifest, text guards and privacy/snapshot rules are in [`docs/EXPORT_DESIGN.md`](../../docs/EXPORT_DESIGN.md) and accepted ADR-017. JSON is authoritative; all money values are signed int64 integer strings. CSV decimal fields are exact readable representations derived from the same integers and currency scale.

The producer schema forbids unexpected keys to catch accidental secrets/contract drift in tests. A future v1 **reader** should tolerate additive optional fields as required by ADR-017; breaking semantics require a new version. This schema does not authorize an import or backup/restore implementation.

Run `npm run web:test` for schema/golden verification and `npm run openapi:check` for generated contracts. Real integration tests require a new empty `LIFEOS_EXPORT_TEST_DATABASE_URL` ending `_test`; see the Development Guide. CSV safeguards are tested with parsers; no universal spreadsheet or save/reopen guarantee is made.

## Accepted Goal visibility exception

[ADR-021](../../docs/adr/ADR-021-goal-soft-delete.md) explicitly permits normal JSON v1/v2 to omit the newly introduced soft-deleted Goal state and all its checklist rows without changing either schema or existing golden fixture. Visible active/completed/archived Goals remain included. This exception is limited to this new state; ADR-017's remaining compatibility/version rules and historical text stay unchanged. Finance CSV v1/v2 is unchanged.

The two Planning collections share the export snapshot and eligible-parent set, so no child is orphaned. Normal JSON is not a complete database backup: deleted Goals remain internally/backed up, and restoring an older backup can resurface them. Preserve the Personal Data Ready restore-safety gate.
