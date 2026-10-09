import { describe, expect, it } from "vitest";

import { currencyDigits, formatMinorUnits, minorUnitsToInput, parseAmountToMinor, toOccurredAt } from "./moneyAmount";

describe("exact Money amounts", () => {
  it("parses common currency amounts without floating-point arithmetic", () => {
    expect(parseAmountToMinor("0.29", "MYR")).toBe("29");
    expect(parseAmountToMinor("1234567890.01", "USD")).toBe("123456789001");
    expect(parseAmountToMinor("42", "JPY")).toBe("42");
    expect(currencyDigits("JPY")).toBe(0);
  });

  it("allows signed opening positions but requires positive transactions", () => {
    expect(parseAmountToMinor("-25.50", "MYR", true)).toBe("-2550");
    expect(parseAmountToMinor("0", "MYR", true)).toBe("0");
    expect(() => parseAmountToMinor("-1.00", "MYR")).toThrow();
    expect(() => parseAmountToMinor("0.00", "MYR")).toThrow();
  });

  it("rejects malformed, over-precise, and int64-overflowing input", () => {
    for (const input of ["1,000.00", "1e3", "1.234", "abc", "", ".50"]) {
      expect(() => parseAmountToMinor(input, "MYR")).toThrow();
    }
    expect(() => parseAmountToMinor("1.5", "JPY")).toThrow();
    expect(() => parseAmountToMinor("92233720368547758.08", "MYR")).toThrow();
    expect(parseAmountToMinor("-92233720368547758.08", "MYR", true)).toBe("-9223372036854775808");
    expect(() => parseAmountToMinor("1.00", "myr")).toThrow();
  });

  it("formats large and negative minor units without losing precision", () => {
    expect(formatMinorUnits("9223372036854775807", "MYR")).toBe("MYR 92,233,720,368,547,758.07");
    expect(formatMinorUnits("-2550", "MYR")).toBe("−MYR 25.50");
    expect(formatMinorUnits("125", "JPY")).toBe("JPY 125");
  });

  it("prefills a replacement with an exact editable decimal", () => {
    expect(minorUnitsToInput("9223372036854775807", "MYR")).toBe("92233720368547758.07");
    expect(minorUnitsToInput("-2550", "MYR")).toBe("-25.50");
    expect(minorUnitsToInput("125", "JPY")).toBe("125");
  });
});

describe("Money date input", () => {
  it("converts a valid local date and time to an API timestamp", () => {
    expect(new Date(toOccurredAt("2020-01-15T12:30")).getTime()).toBe(new Date("2020-01-15T12:30").getTime());
  });

  it("rejects impossible and malformed dates", () => {
    expect(() => toOccurredAt("2026-02-30T12:00")).toThrow();
    expect(() => toOccurredAt("2026-01-01")).toThrow();
  });
});
