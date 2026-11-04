import { afterEach, describe, expect, it, vi } from "vitest";
import { captureHistoryLocation, restoreHistoryLocation } from "./historyPosition";

afterEach(() => vi.unstubAllGlobals());
function environment() {
  const record = (id: string, top: number) => ({ id: `money-event-${id}`, getBoundingClientRect: () => ({ top, bottom: top + 80 }), focus: vi.fn(), closest: vi.fn(() => null) });
  const records = [record("first", -200), record("edited", 120), record("older", 240)];
  const scrollBy = vi.fn();
  const available = new Map(records.map((record) => [record.id, record]));
  vi.stubGlobal("document", { querySelectorAll: () => records, getElementById: (id: string) => available.get(id) });
  vi.stubGlobal("window", { scrollBy });
  return { records, available, scrollBy };
}
describe("History viewport recovery", () => {
  it("captures an edited record and adjacent stable IDs without an index cursor", () => {
    environment();
    expect(captureHistoryLocation("edited")).toEqual({ ids: ["money-event-edited", "money-event-older", "money-event-first"], top: 120 });
    expect(captureHistoryLocation().ids[0]).toBe("money-event-edited");
  });
  it("restores the record or its neighbor after deletion without an extra focus jump", () => {
    const e = environment();
    const location = captureHistoryLocation("edited");
    e.available.delete("money-event-edited");
    expect(restoreHistoryLocation(location)).toBe(true);
    expect(e.records[2].focus).toHaveBeenCalledWith({ preventScroll: true });
    expect(e.scrollBy).toHaveBeenCalledWith({ top: 120, behavior: "instant" });
  });
  it("falls back explicitly if old IDs are outside the rebuilt prefix or hidden", () => {
    const e = environment();
    e.records[1].closest.mockReturnValue({} as never);
    expect(restoreHistoryLocation({ ids: ["money-event-edited", "missing"], top: 120 })).toBe(false);
    expect(e.scrollBy).not.toHaveBeenCalled();
  });
});
