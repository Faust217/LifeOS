import { describe, expect, it } from "vitest";

import { currencyDigits, formatLocalDate, formatLocalTime, formatLocalTimeWithZone, formatMinorUnits, localDateKey, localDateTimeValue, matchingInstants, minorUnitsToInput, parseAmountToMinor, timeZoneLabel, toOccurredAt } from "./moneyAmount";

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
    expect(toOccurredAt("2020-01-15T12:30", "Asia/Kuala_Lumpur")).toBe("2020-01-15T04:30:00.000Z");
  });

  it("rejects impossible and malformed dates", () => {
    expect(() => toOccurredAt("2026-02-30T12:00", "UTC")).toThrow();
    expect(() => toOccurredAt("2026-01-01", "UTC")).toThrow();
  });

  it("groups existing UTC events by the saved timezone, independent of the device", () => {
    expect(localDateKey("2026-10-13T17:30:00Z", "Asia/Kuala_Lumpur")).toBe("2026-10-14");
    expect(localDateKey("2026-10-13T17:30:00Z", "America/New_York")).toBe("2026-10-13");
    expect(localDateTimeValue(new Date("2026-10-13T17:30:00Z"), "Asia/Kuala_Lumpur")).toBe("2026-10-14T01:30");
    expect(formatLocalDate("2026-10-13T17:30:00Z", "Asia/Kuala_Lumpur")).toContain("October 14");
    expect(formatLocalTime("2026-10-13T17:30:00Z", "Asia/Kuala_Lumpur")).toBe("01:30");
    expect(formatLocalTime("2026-10-13T17:30:00Z", "Asia/Kuala_Lumpur")).not.toMatch(/UTC|GMT/);
    expect(timeZoneLabel("Asia/Kuala_Lumpur", new Date("2026-10-13T17:30:00Z"))).toBe("Kuala Lumpur (UTC+8)");
  });

  it("rejects DST gaps and requires an explicit choice for repeated wall times", () => {
    expect(() => matchingInstants("2024-03-10T02:30", "America/New_York")).toThrow(/does not exist/);
    const matches = matchingInstants("2024-11-03T01:30", "America/New_York");
    expect(matches).toEqual(["2024-11-03T05:30:00.000Z", "2024-11-03T06:30:00.000Z"]);
    expect(() => toOccurredAt("2024-11-03T01:30", "America/New_York")).toThrow(/occurs twice/);
    expect(toOccurredAt("2024-11-03T01:30", "America/New_York", matches[1])).toBe(matches[1]);
    expect(formatLocalTime(matches[0], "America/New_York")).toBe(formatLocalTime(matches[1], "America/New_York"));
    expect(formatLocalTimeWithZone(matches[0], "America/New_York")).not.toBe(formatLocalTimeWithZone(matches[1], "America/New_York"));
    expect(timeZoneLabel("America/New_York", new Date(matches[0]))).toBe("New York (UTC-4)");
    expect(timeZoneLabel("America/New_York", new Date(matches[1]))).toBe("New York (UTC-5)");
  });
});
