import { afterEach, describe, expect, it, vi } from "vitest";

import {
  addGoalItem, archiveGoal, createGoal, deleteGoal, GoalApiError, listGoals, removeGoalItem,
  renameGoal, renameGoalItem, restoreGoal, setGoalCompletion, setGoalItemCompletion,
} from "./goals";

afterEach(() => { vi.unstubAllGlobals(); vi.useRealTimers(); });

describe("Goals API boundary", () => {
  it("accepts only empty 204 for Goal Delete and never parses it or sends financial keys", async () => {
    const json=vi.fn(); const request=vi.fn().mockResolvedValue({ok:true,status:204,json});vi.stubGlobal("fetch",request);
    await expect(deleteGoal("csrf", "same-goal")).resolves.toBeUndefined();
    expect(json).not.toHaveBeenCalled();
    expect(request.mock.calls[0][0]).toBe("/api/v1/goals/same-goal");
    expect(request.mock.calls[0][1]).toMatchObject({method:"DELETE",body:undefined,credentials:"same-origin",cache:"no-store",headers:{"X-CSRF-Token":"csrf"}});
    expect(JSON.stringify(request.mock.calls[0])).not.toContain("Idempotency-Key");
    request.mockResolvedValue({ok:true,status:200,json:async()=>({})});
    await expect(deleteGoal("csrf","same-goal")).rejects.toMatchObject({status:200,code:"internal_error"});
  });
  it("bounds an uncertain Delete without claiming its server transaction rolled back", async () => {
    vi.useFakeTimers();
    vi.stubGlobal("fetch",vi.fn((_path,options)=>new Promise((_resolve,reject)=>options.signal.addEventListener("abort",()=>reject(new Error("aborted"))))));
    const result=expect(deleteGoal("csrf","same-goal")).rejects.toMatchObject({status:0,code:"network_error"});
    await vi.advanceTimersByTimeAsync(15_000); await result;
  });
  it("reads the signed-in owner's Goals with same-origin cookies", async () => {
    const request = vi.fn().mockResolvedValue({ ok: true, status: 200, json: async () => ({ items: [] }) });
    vi.stubGlobal("fetch", request);
    expect(await listGoals()).toEqual({ items: [] });
    expect(request).toHaveBeenCalledWith("/api/v1/goals", expect.objectContaining({ method: "GET", credentials: "same-origin", cache: "no-store" }));
    expect(JSON.stringify(request.mock.calls[0])).not.toContain("owner_id");
  });

  it("sends CSRF on each mutation and only the requested Goal fields", async () => {
    const request = vi.fn().mockResolvedValue({ ok: true, status: 200, json: async () => ({ id: "goal-test" }) });
    vi.stubGlobal("fetch", request);
    await createGoal("csrf-test", "Walk daily");
    await renameGoal("csrf-test", "goal-test", "Walk weekly");
    await addGoalItem("csrf-test", "goal-test", "Put on shoes");
    await renameGoalItem("csrf-test", "goal-test", "item-test", "Walk 10 minutes");
    await setGoalItemCompletion("csrf-test", "goal-test", "item-test", true);
    await setGoalCompletion("csrf-test", "goal-test", true);
    await removeGoalItem("csrf-test", "goal-test", "item-test");
    await archiveGoal("csrf-test", "goal-test");
    await restoreGoal("csrf-test", "goal-test");
    for (const [path, options] of request.mock.calls) {
      expect(path).toMatch(/^\/api\/v1\/goals/);
      expect(options.credentials).toBe("same-origin");
      expect(options.headers["X-CSRF-Token"]).toBe("csrf-test");
      expect(JSON.stringify(options)).not.toContain("owner_id");
    }
    expect(JSON.parse(request.mock.calls[0][1].body)).toEqual({ title: "Walk daily" });
    expect(JSON.parse(request.mock.calls[4][1].body)).toEqual({ completed: true });
    expect(request.mock.calls[6][1]).toMatchObject({ method: "DELETE", body: undefined });
    expect(request.mock.calls[7][1]).toMatchObject({ method: "POST", body: undefined });
    expect(request.mock.calls[8][0]).toBe("/api/v1/goals/goal-test/restore");
  });

  it("maps non-leaking API, expired-session, network, and malformed-response errors", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ ok: false, status: 404, json: async () => ({ error: { code: "not_found", message: "private detail" } }) }));
    await expect(listGoals()).rejects.toMatchObject({ status: 404, code: "not_found" });
    await expect(listGoals()).rejects.not.toThrow("private detail");
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ ok: false, status: 401, json: async () => ({ error: { code: "authentication_required" } }) }));
    await expect(listGoals()).rejects.toMatchObject({ status: 401 });
    vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("secret network detail")));
    await expect(listGoals()).rejects.toEqual(new GoalApiError(0, "network_error"));
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ ok: true, status: 200, json: async () => { throw new Error("response bytes"); } }));
    await expect(listGoals()).rejects.toEqual(new GoalApiError(200, "internal_error"));
  });
});
