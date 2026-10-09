import { describe, expect, it } from "vitest";

import { primaryAreas, resolveArea } from "./navigation";

describe("LifeOS navigation", () => {
  it("resolves direct URLs and the Google callback's root landing", () => {
    expect(resolveArea("/")).toBe("today");
    for (const area of primaryAreas) {
      expect(resolveArea(area.href)).toBe(area.id);
      expect(resolveArea(`${area.href}/`)).toBe(area.id);
    }
    expect(resolveArea("/not-a-page")).toBeNull();
  });

  it("keeps four distinct destinations for native browser history", () => {
    expect(primaryAreas.map(({ href }) => href)).toEqual(["/today", "/money", "/plans", "/me"]);
    expect(new Set(primaryAreas.map(({ href }) => href)).size).toBe(4);
  });
});
