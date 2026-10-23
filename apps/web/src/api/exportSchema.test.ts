import Ajv from "@redocly/ajv/dist/2020";
import schema from "../../../../api/export/lifeos-export-v1.schema.json";
import fixture from "../../../../api/export/testdata/lifeos-export-v1.json";
import schemaV2 from "../../../../api/export/lifeos-export-v2.schema.json";
import fixtureV2 from "../../../../api/export/testdata/lifeos-export-v2.json";
import { describe, expect, it } from "vitest";

// Reuse the already-pinned OpenAPI validator; no additional package is installed.
const validator = new Ajv({ allErrors: true, strict: false });
validator.addFormat("uuid", /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i);
validator.addFormat("email", (value: string) => /^[^@\s]+@[^@\s]+$/.test(value));
validator.addFormat("date-time", (value: string) => /^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,6})?Z$/.test(value) && Number.isFinite(Date.parse(value)));
const validate = validator.compile(schema);
const validateV2 = validator.compile(schemaV2);

describe("independent lifeos-export-v2 dated balances", () => {
  it("validates the real v2 golden without redefining v1", () => {
    expect(validateV2(fixtureV2),JSON.stringify(validateV2.errors)).toBe(true);
    expect(validate(fixtureV2)).toBe(false);
    expect(validateV2(fixture)).toBe(false);
    expect(fixtureV2.finance.accounts.some(a=>a.opening_balance_minor==="89999")).toBe(true);
  });
  it("rejects inconsistent account basis, non-UTC dates and internal revision evidence", () => {
    const wrong=structuredClone(fixtureV2); wrong.finance.accounts[0].opening_balance_basis="legacy_all_movements";
    const missing=structuredClone(fixtureV2) as Record<string,any>; missing.finance.accounts[0].opening_balance_at=null;
    const zone=structuredClone(fixtureV2); zone.finance.accounts[0].opening_balance_at="2026-10-01T08:00:00+08:00";
    const internal=structuredClone(fixtureV2) as Record<string,any>;internal.finance.accounts[0].balance_revision="5";
    for(const invalid of [wrong,missing,zone,internal]) expect(validateV2(invalid)).toBe(false);
  });
});

describe("independent lifeos-export-v1 contract", () => {
  it("validates the real synthetic downloaded golden artifact", () => {
    expect(validate(fixture), JSON.stringify(validate.errors)).toBe(true);
    expect(fixture.finance.accounts.some((a: { opening_balance_minor: string }) => a.opening_balance_minor === "9223372036854775807")).toBe(true);
  });
  it("rejects secret/internal fields, numerical money and non-UTC timestamps", () => {
    const secret = structuredClone(fixture) as Record<string, any>; secret.owner.csrf_token = "must-never-export";
    const deleted = structuredClone(fixture) as Record<string, any>; deleted.finance.events[0].deleted_at = "2026-10-20T00:00:00Z";
    const numeric = structuredClone(fixture) as Record<string, any>; numeric.finance.events[0].amount_minor = 100;
    const local = structuredClone(fixture); local.created_at = "2026-10-20T08:00:00+08:00";
    for (const invalid of [secret, deleted, numeric, local]) expect(validate(invalid)).toBe(false);
  });
});
