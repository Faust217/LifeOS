import Ajv from "@redocly/ajv/dist/2020";
import schema from "../../../../api/export/lifeos-export-v1.schema.json";
import fixture from "../../../../api/export/testdata/lifeos-export-v1.json";
import { describe, expect, it } from "vitest";

// Reuse the already-pinned OpenAPI validator; no additional package is installed.
const validator = new Ajv({ allErrors: true, strict: false });
validator.addFormat("uuid", /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i);
validator.addFormat("email", (value: string) => /^[^@\s]+@[^@\s]+$/.test(value));
validator.addFormat("date-time", (value: string) => /^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d{1,6})?Z$/.test(value) && Number.isFinite(Date.parse(value)));
const validate = validator.compile(schema);

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
