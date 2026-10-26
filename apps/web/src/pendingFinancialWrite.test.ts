import { describe, expect, it } from "vitest";
import { FINANCIAL_PENDING_STORAGE, PendingFinancialWrites } from "./pendingFinancialWrite";

const key = "11111111-1111-4111-8111-111111111111";
const body = JSON.stringify({ kind: "expense", amount_minor: "1250", note: "Frozen note", allocations: [{ category_id: key, amount_minor: "1250" }] });
class MemoryStorage {
  data = new Map<string, string>();
  unavailable = false; full = false; locked = false;
  getItem(k: string) { if (this.unavailable) throw new Error("unavailable"); return this.data.get(k) ?? null; }
  setItem(k: string, value: string) { if (this.full) throw new Error("quota"); this.data.set(k, value); }
  removeItem(k: string) { if (this.locked) throw new Error("unavailable"); this.data.delete(k); }
}
function setup(storage = new MemoryStorage()) { const manager = new PendingFinancialWrites(() => storage, () => key); manager.bindOwner("owner-A"); return { manager, storage }; }

describe("Frozen financial command recovery", () => {
  it("never overwrites an unresolved payload, operation, target or key", async () => {
    const { manager, storage } = setup();
    const original = await manager.prepare("/api/v1/events", body);
    const saved = storage.getItem(FINANCIAL_PENDING_STORAGE);
    await expect(manager.prepare("/api/v1/accounts", "{}")).rejects.toMatchObject({ reason: "pending" });
    await expect(manager.prepare("/api/v1/events", body.replace("1250", "1500"))).rejects.toMatchObject({ reason: "pending" });
    expect(storage.getItem(FINANCIAL_PENDING_STORAGE)).toBe(saved);
    expect(manager.retry()).toEqual(original);
    expect(Object.isFrozen(original)).toBe(true);
  });

  it("reserves preparation across asynchronous digest calculation", async () => {
    const { manager } = setup();
    const first = manager.prepare("/api/v1/events", body);
    await expect(manager.prepare("/api/v1/accounts", "{}")).rejects.toMatchObject({ reason: "pending" });
    expect((await first).operation).toBe("event.create");
  });

  it("recovers exact bytes on same-tab reload without generating another key", async () => {
    const { manager, storage } = setup(); const original = await manager.prepare("/api/v1/events", body);
    await manager.start(original); manager.finish(original, false);
    const reloaded = setup(storage).manager;
    expect(reloaded.retry()).toEqual(original);
    await reloaded.start(original);
    await expect(reloaded.prepare("/api/v1/events", body)).rejects.toMatchObject({ reason: "pending" });
    expect(() => reloaded.clearAcknowledged()).toThrow();
    reloaded.finish(original, true);
    expect(reloaded.status().kind).toBe("empty");
  });

  it("detects valid-JSON corruption before any replay can start", async () => {
    const { manager, storage } = setup(); await manager.prepare("/api/v1/events", body);
    const raw = JSON.parse(storage.getItem(FINANCIAL_PENDING_STORAGE)!);
    raw.body = raw.body.replace("1250", "9999");
    storage.setItem(FINANCIAL_PENDING_STORAGE, JSON.stringify(raw));
    await expect(manager.start(manager.retry())).rejects.toMatchObject({ reason: "corrupted" });
    expect(manager.status().kind).toBe("corrupted");
    await expect(manager.prepare("/api/v1/events", body)).rejects.toMatchObject({ reason: "corrupted" });
  });

  it("blocks malformed, foreign-path and target-mismatched storage until explicit clearing", async () => {
    for (const corrupt of ["{", "null", '{"version":9}', " ".repeat(33000)]) {
      const { manager, storage } = setup(); storage.setItem(FINANCIAL_PENDING_STORAGE, corrupt);
      await expect(manager.prepare("/api/v1/events", body)).rejects.toMatchObject({ reason: "corrupted" });
      expect(storage.getItem(FINANCIAL_PENDING_STORAGE)).toBe(corrupt);
      manager.clearAcknowledged(); expect((await manager.prepare("/api/v1/events", body)).key).toBe(key);
    }
    const { manager, storage } = setup(); await manager.prepare(`/api/v1/events/${key}/void`, '{"reason":""}');
    const raw = JSON.parse(storage.getItem(FINANCIAL_PENDING_STORAGE)!); raw.path = "/api/v1/auth/logout";
    storage.setItem(FINANCIAL_PENDING_STORAGE, JSON.stringify(raw));
    expect(manager.status().kind).toBe("corrupted");
  });

  it("fails safely for unavailable or full storage and does not invent recovery", async () => {
    const { manager, storage } = setup(); storage.unavailable = true;
    await expect(manager.prepare("/api/v1/events", body)).rejects.toMatchObject({ reason: "storage" });
    expect(storage.data.size).toBe(0);
    storage.unavailable = false; storage.full = true;
    await expect(manager.prepare("/api/v1/events", body)).rejects.toMatchObject({ reason: "storage" });
    expect(storage.data.size).toBe(0);
    storage.full = false;
    const record = await manager.prepare("/api/v1/events", body); storage.locked = true;
    await manager.start(record); manager.finish(record, true);
    expect(manager.status().kind).toBe("pending");
    await expect(manager.prepare("/api/v1/events", body)).rejects.toMatchObject({ reason: "pending" });
    expect(() => manager.clearAcknowledged()).toThrow();
  });

  it("hides expired-session drafts and never exposes them to another owner", async () => {
    const { manager, storage } = setup(); const original = await manager.prepare("/api/v1/events", body);
    manager.bindOwner(null); expect(manager.status().kind).toBe("hidden");
    expect(storage.data.size).toBe(1);
    manager.bindOwner("owner-A"); expect(manager.retry()).toEqual(original);
    manager.bindOwner("owner-B"); expect(manager.status()).toEqual({ kind: "other_owner" });
    await expect(manager.prepare("/api/v1/events", JSON.stringify({ kind: "expense" }))).rejects.toMatchObject({ reason: "other_owner" });
    expect(storage.getItem(FINANCIAL_PENDING_STORAGE)).toContain(original.key);
    manager.bindOwner("owner-A"); expect(manager.retry()).toEqual(original);
  });

  it("an old completion cannot clear a newer owner's pending record", async () => {
    const { manager, storage } = setup(); const old = await manager.prepare("/api/v1/events", body); await manager.start(old);
    const newer = new PendingFinancialWrites(() => storage, () => key); newer.bindOwner("owner-B");
    expect(newer.status()).toEqual({ kind: "other_owner" });
    newer.clearAcknowledged(); // Represents the separate, explicit UI acknowledgement.
    const next = await newer.prepare("/api/v1/accounts", "{}");
    manager.finish(old, true);
    expect(newer.retry()).toEqual(next);
  });

  it("rejects oversized drafts and cannot clear during preparation", async () => {
    const { manager, storage } = setup(); const first = manager.prepare("/api/v1/events", body);
    expect(() => manager.clearAcknowledged()).toThrow(); await first;
    manager.clearAcknowledged();
    await expect(manager.prepare("/api/v1/events", JSON.stringify({ note: "x".repeat(17000) }))).rejects.toThrow();
    expect(storage.data.size).toBe(0);
  });
});
