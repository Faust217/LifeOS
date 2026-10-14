import { afterEach, describe, expect, it, vi } from "vitest";

import { archiveMoneyAccount, archiveMoneyCategory, createMoneyAccount, deleteMoneyEvent, listMoneyAccounts, listMoneyHistory, MoneyApiError, postMoneyEvent, renameMoneyCategory, restoreMoneyAccount, restoreMoneyCategory, voidMoneyEvent } from "./money";

afterEach(() => vi.unstubAllGlobals());

describe("Money API boundary", () => {
  it("uses same-origin cookies and no owner override for reads", async () => {
    const request = vi.fn().mockResolvedValue({ ok: true, status: 200, json: async () => ({ items: [] }) });
    vi.stubGlobal("fetch", request);
    await listMoneyAccounts();
    await listMoneyHistory(11, 20);
    expect(request).toHaveBeenNthCalledWith(1, "/api/v1/accounts", expect.objectContaining({ method: "GET", credentials: "same-origin", cache: "no-store" }));
    expect(request).toHaveBeenNthCalledWith(2, "/api/v1/events?limit=11&offset=20", expect.anything());
  });

  it("sends exact decimal-string amounts and CSRF on mutations", async () => {
    const request = vi.fn().mockResolvedValue({ ok: true, status: 201, json: async () => ({ id: "test" }) });
    vi.stubGlobal("fetch", request);
    await createMoneyAccount("csrf-test", { name: "Cash", type: "cash", currency: "MYR", opening_balance_minor: "-25" });
    await postMoneyEvent("csrf-test", { kind: "expense", account_id: "account-test", amount_minor: "29", currency: "MYR", occurred_at: "2020-01-01T00:00:00Z", allocations: [{ category_id: "category-test", amount_minor: "29" }] });
    const accountCall = request.mock.calls[0];
    const eventCall = request.mock.calls[1];
    expect(accountCall[1]).toMatchObject({ method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json", "X-CSRF-Token": "csrf-test" } });
    expect(JSON.parse(accountCall[1].body)).toEqual({ name: "Cash", type: "cash", currency: "MYR", opening_balance_minor: "-25" });
    expect(JSON.parse(eventCall[1].body).allocations).toEqual([{ category_id: "category-test", amount_minor: "29" }]);
    expect(accountCall[1].body).not.toContain("owner_id");
  });

  it("uses empty-body archive/restore/delete and an optional void note", async () => {
    const request = vi.fn().mockResolvedValue({ ok: true, status: 204 });
    vi.stubGlobal("fetch", request);
    await archiveMoneyAccount("csrf", "account-1");
    await restoreMoneyAccount("csrf", "account-1");
    await voidMoneyEvent("csrf", "event-1", "Entered twice");
    await voidMoneyEvent("csrf", "event-2");
    await deleteMoneyEvent("csrf", "event-3");
    expect(request.mock.calls[0][0]).toBe("/api/v1/accounts/account-1/archive");
    expect(request.mock.calls[0][1].body).toBeUndefined();
    expect(request.mock.calls[1][0]).toBe("/api/v1/accounts/account-1/restore");
    expect(request.mock.calls[1][1]).toMatchObject({ method: "POST", headers: { "X-CSRF-Token": "csrf" } });
    expect(request.mock.calls[1][1].body).toBeUndefined();
    expect(JSON.parse(request.mock.calls[2][1].body)).toEqual({ reason: "Entered twice" });
    expect(JSON.parse(request.mock.calls[3][1].body)).toEqual({ reason: "" });
    expect(request.mock.calls[4][0]).toBe("/api/v1/events/event-3/delete");
    expect(request.mock.calls[4][1]).toMatchObject({ method: "POST", credentials: "same-origin", headers: { "X-CSRF-Token": "csrf" } });
    expect(request.mock.calls[4][1].body).toBeUndefined();
  });

  it("uses guarded category management routes and explicit empty allocations", async () => {
    const request = vi.fn().mockResolvedValue({ ok: true, status: 200, json: async () => ({ id: "category-1", name: "Medical", kind: "expense" }) });
    vi.stubGlobal("fetch", request);
    await renameMoneyCategory("csrf", "category-1", { name: "Medical" });
    request.mockResolvedValue({ ok: true, status: 204 });
    await archiveMoneyCategory("csrf", "category-1");
    await restoreMoneyCategory("csrf", "category-1");
    request.mockResolvedValue({ ok: true, status: 201, json: async () => ({ id: "event-1" }) });
    await postMoneyEvent("csrf", { kind: "expense", account_id: "account-1", amount_minor: "100", currency: "MYR", occurred_at: "2026-10-13T00:00:00Z", allocations: [] });
    expect(request.mock.calls.map((call) => call[0])).toEqual([
      "/api/v1/categories/category-1/rename", "/api/v1/categories/category-1/archive",
      "/api/v1/categories/category-1/restore", "/api/v1/events",
    ]);
    for (const call of request.mock.calls) expect(call[1]).toMatchObject({ method: "POST", credentials: "same-origin", headers: { "X-CSRF-Token": "csrf" } });
    expect(JSON.parse(request.mock.calls[0][1].body)).toEqual({ name: "Medical" });
    expect(request.mock.calls[1][1].body).toBeUndefined();
    expect(request.mock.calls[2][1].body).toBeUndefined();
    expect(JSON.parse(request.mock.calls[3][1].body).allocations).toEqual([]);
  });

  it("maps stable server errors without exposing private server messages", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ ok: false, status: 404, json: async () => ({ error: { code: "not_found", message: "private detail" } }) }));
    await expect(listMoneyAccounts()).rejects.toMatchObject({ status: 404, code: "not_found" });
    await expect(listMoneyAccounts()).rejects.not.toThrow("private detail");
  });

  it("distinguishes expired sessions and network failures", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ ok: false, status: 401, json: async () => ({ error: { code: "authentication_required" } }) }));
    await expect(listMoneyAccounts()).rejects.toMatchObject({ status: 401 });
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("raw network detail")));
    await expect(listMoneyAccounts()).rejects.toEqual(new MoneyApiError(0, "network_error"));
  });

  it("does not surface malformed success bodies as raw parser errors", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ ok: true, status: 200, json: async () => { throw new SyntaxError("private response bytes"); } }));
    await expect(listMoneyAccounts()).rejects.toEqual(new MoneyApiError(200, "internal_error"));
  });
});
