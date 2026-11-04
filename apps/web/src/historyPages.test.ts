import { describe, expect, it } from "vitest";
import { HistoryPages } from "./historyPages";
import type { RunningHistory } from "./api/money";
const item = (id: string): RunningHistory["items"][number] => ({ id, note: "", event_revision: "1", kind: "expense", currency: "MYR", amount_minor: "100", occurred_at: "2026-10-30T00:00:00Z", status: "posted", movements: [], allocations: [], account_balances: [] });
const page = (ids: string[], next: string | null = null): RunningHistory => ({ projection: "lifeos-running-history-v1", history_revision: "a".repeat(64), time_zone: "Asia/Kuala_Lumpur", limit: 25, has_more: !!next, next_cursor: next, items: ids.map(item), accounts: [], categories: [] });
describe("continuous snapshot-bound History", () => {
  it("appends bounded batches in order and retains earlier account context", () => {
    const pages = new HistoryPages(25);
    expect(pages.query()).toEqual({ limit: 25 });
    const account = { id: "bank", name: "Archived bank", currency: "MYR", archived_at: "2026-10-31T00:00:00Z", balance_basis: "known_balance_before_instant" as const, opening_balance_at: "2026-10-01T00:00:00Z" };
    const first = { ...page(Array.from({ length: 25 }, (_, i) => `first-${i}`), "cursor-1"), accounts: [account] };
    pages.accept(first);
    expect(pages.query()).toEqual({ limit: 25, history_revision: "a".repeat(64), cursor: "cursor-1" });
    const result = pages.accept(page(["older-1", "older-2"]));
    expect(result.items.map(item => item.id)).toEqual([...first.items.map(item => item.id), "older-1", "older-2"]);
    expect(result.accounts).toEqual([account]);
    expect(pages.count).toBe(2);
    expect(() => pages.query()).toThrow("All transactions");
  });
  it("keeps failed, duplicate and different-snapshot continuations out of visible records", () => {
    const pages = new HistoryPages(25);
    pages.accept(page(["current"], "cursor-1"));
    for (const bad of [page(["older", "current"]), { ...page(["older"]), history_revision: "b".repeat(64) }, { ...page(["older"]), time_zone: "UTC" }]) {
      expect(() => pages.accept(bad)).toThrow();
      expect(pages.current?.items.map(item => item.id)).toEqual(["current"]);
      expect(pages.query().cursor).toBe("cursor-1");
    }
    expect(pages.accept(page(["older"])).items).toHaveLength(2);
  });
  it("rejects non-advancing or empty continuations instead of spinning auto-load", () => {
    const pages = new HistoryPages(25);
    pages.accept(page(["first"], "cursor-1"));
    expect(() => pages.accept(page([], "cursor-2"))).toThrow();
    expect(() => pages.accept(page(["second"], "cursor-1"))).toThrow();
  });
  it("builds replacement state separately without clearing visible records or commands", () => {
    const visible = new HistoryPages(25);
    visible.accept(page(["kept"], "old"));
    const command = Object.freeze({ key: "original", target: "kept", body: '{"amount_minor":"1200"}' });
    const fresh = new HistoryPages(25);
    fresh.accept({ ...page(["updated"], "fresh"), history_revision: "b".repeat(64) });
    expect(visible.current?.items[0].id).toBe("kept");
    expect(command.body).toBe('{"amount_minor":"1200"}');
    expect(fresh.query()).toMatchObject({ history_revision: "b".repeat(64), cursor: "fresh" });
  });
});
