import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { previewOpeningBalance, confirmOpeningBalance, retryFinancialWrite, editMoneyEvent, getMoneyEvent } from "./money";

import { archiveMoneyAccount, archiveMoneyCategory, createMoneyAccount, deleteMoneyEvent, listMoneyAccounts, listMoneyHistory, MoneyApiError, postMoneyEvent, renameMoneyCategory, restoreMoneyAccount, restoreMoneyCategory, voidMoneyEvent } from "./money";

import { pendingFinancialWrite } from "../pendingFinancialWrite";
beforeEach(() => {
 const storage=new Map<string,string>();
 vi.stubGlobal("window", {sessionStorage:{getItem:(key:string)=>storage.get(key)??null,setItem:(key:string,value:string)=>storage.set(key,value),removeItem:(key:string)=>storage.delete(key)}});
 pendingFinancialWrite.bindOwner("test-owner");
});
afterEach(() => { pendingFinancialWrite.bindOwner(null); vi.unstubAllGlobals(); });

describe("Money API boundary", () => {
  it("uses guarded exact balance preview and revision confirmation without owner input", async () => {
    const request=vi.fn().mockResolvedValue({ok:true,status:200,headers:new Headers({"Idempotency-Replayed":"false"}),json:async()=>({id:"account",currency:"MYR",opening_balance_minor:"0"})}); vi.stubGlobal("fetch",request);
    await previewOpeningBalance("csrf","11111111-1111-4111-8111-111111111111",{opening_balance_at:"2026-10-01T00:00:00Z"});
    await confirmOpeningBalance("csrf","11111111-1111-4111-8111-111111111111",{opening_balance_at:"2026-10-01T00:00:00Z",opening_balance_minor:"-9223372036854775808",balance_revision:"12",acknowledge_change:true});
    expect(request.mock.calls[0][0]).toBe("/api/v1/accounts/11111111-1111-4111-8111-111111111111/opening-balance/preview");
    expect(request.mock.calls[1][0]).toBe("/api/v1/accounts/11111111-1111-4111-8111-111111111111/opening-balance/confirm");
    expect(JSON.parse(request.mock.calls[0][1].body)).not.toHaveProperty("opening_balance_minor");
    expect(JSON.parse(request.mock.calls[1][1].body)).toMatchObject({opening_balance_minor:"-9223372036854775808",balance_revision:"12",acknowledge_change:true});
    for(const call of request.mock.calls) { expect(call[1]).toMatchObject({method:"POST",credentials:"same-origin",cache:"no-store",headers:{"X-CSRF-Token":"csrf"}}); expect(call[1].body).not.toContain("owner_id"); }
  });
  it("uses same-origin cookies and no owner override for reads", async () => {
    const request = vi.fn().mockResolvedValue({ ok: true, status: 200, json: async () => ({ items: [] }) });
    vi.stubGlobal("fetch", request);
    await listMoneyAccounts();
    await listMoneyHistory(11, 20);
    expect(request).toHaveBeenNthCalledWith(1, "/api/v1/accounts", expect.objectContaining({ method: "GET", credentials: "same-origin", cache: "no-store" }));
    expect(request).toHaveBeenNthCalledWith(2, "/api/v1/events?limit=11&offset=20", expect.anything());
  });

  it("sends exact decimal-string amounts and CSRF on mutations", async () => {
    const request = vi.fn().mockResolvedValue({ ok: true, status: 201, headers: new Headers({"Idempotency-Replayed":"false"}), json: async () => ({ id: "test",currency:"MYR",opening_balance_minor:"0",amount_minor:"0",movements:[] }) });
    vi.stubGlobal("fetch", request);
    await createMoneyAccount("csrf-test", { name: "Cash", type: "cash", currency: "MYR", opening_balance_minor: "-25", opening_balance_at: "2026-10-01T00:00:00Z" });
    await postMoneyEvent("csrf-test", { kind: "expense", account_id: "account-test", amount_minor: "29", currency: "MYR", occurred_at: "2020-01-01T00:00:00Z", allocations: [{ category_id: "category-test", amount_minor: "29" }] });
    const accountCall = request.mock.calls[0];
    const eventCall = request.mock.calls[1];
    expect(accountCall[1]).toMatchObject({ method: "POST", credentials: "same-origin", headers: { "Content-Type": "application/json", "X-CSRF-Token": "csrf-test" } });
    expect(JSON.parse(accountCall[1].body)).toEqual({ name: "Cash", type: "cash", currency: "MYR", opening_balance_minor: "-25", opening_balance_at: "2026-10-01T00:00:00Z" });
    expect(JSON.parse(eventCall[1].body).allocations).toEqual([{ category_id: "category-test", amount_minor: "29" }]);
    expect(accountCall[1].body).not.toContain("owner_id");
  });

  it("uses empty-body archive/restore/delete and an optional void note", async () => {
    const request = vi.fn().mockResolvedValue({ ok: true, status: 204, headers: new Headers({"Idempotency-Replayed":"false"}) });
    vi.stubGlobal("fetch", request);
    await archiveMoneyAccount("csrf", "11111111-1111-4111-8111-111111111111");
    await restoreMoneyAccount("csrf", "11111111-1111-4111-8111-111111111111");
    await voidMoneyEvent("csrf", "22222222-2222-4222-8222-222222222222", "1", "Entered twice");
    await voidMoneyEvent("csrf", "33333333-3333-4333-8333-333333333333", "1");
    await deleteMoneyEvent("csrf", "44444444-4444-4444-8444-444444444444", "1");
    expect(request.mock.calls[0][0]).toBe("/api/v1/accounts/11111111-1111-4111-8111-111111111111/archive");
    expect(request.mock.calls[0][1].body).toBeUndefined();
    expect(request.mock.calls[1][0]).toBe("/api/v1/accounts/11111111-1111-4111-8111-111111111111/restore");
    expect(request.mock.calls[1][1]).toMatchObject({ method: "POST", headers: { "X-CSRF-Token": "csrf" } });
    expect(request.mock.calls[1][1].body).toBeUndefined();
    expect(JSON.parse(request.mock.calls[2][1].body)).toEqual({ reason: "Entered twice" });
    expect(JSON.parse(request.mock.calls[3][1].body)).toEqual({ reason: "" });
    expect(request.mock.calls[4][0]).toBe("/api/v1/events/44444444-4444-4444-8444-444444444444/delete");
    expect(request.mock.calls[4][1]).toMatchObject({ method: "POST", credentials: "same-origin", headers: { "X-CSRF-Token": "csrf" } });
    expect(request.mock.calls[4][1].body).toBeUndefined();
  });

  it("uses guarded category management routes and explicit empty allocations", async () => {
    const request = vi.fn().mockResolvedValue({ ok: true, status: 200, json: async () => ({ id: "category-1", name: "Medical", kind: "expense" }) });
    vi.stubGlobal("fetch", request);
    await renameMoneyCategory("csrf", "category-1", { name: "Medical" });
    request.mockResolvedValue({ ok: true, status: 204, headers: new Headers({"Idempotency-Replayed":"false"}) });
    await archiveMoneyCategory("csrf", "category-1");
    await restoreMoneyCategory("csrf", "category-1");
    request.mockResolvedValue({ ok: true, status: 201, headers: new Headers({"Idempotency-Replayed":"false"}), json: async () => ({ id: "22222222-2222-4222-8222-222222222222",currency:"MYR",amount_minor:"100",movements:[] }) });
    await postMoneyEvent("csrf", { kind: "expense", account_id: "11111111-1111-4111-8111-111111111111", amount_minor: "100", currency: "MYR", occurred_at: "2026-10-13T00:00:00Z", allocations: [] });
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


describe("Financial Save recovery API", () => {
 const input = { kind: "expense" as const, account_id: "11111111-1111-4111-8111-111111111111", amount_minor: "1250", currency: "MYR", occurred_at: "2026-10-02T00:00:00.123456Z", note: "Frozen", allocations: [{category_id:"22222222-2222-4222-8222-222222222222",amount_minor:"1250"}] };
 const receipt = () => new Response(JSON.stringify({outcome:"already_applied"}),{status:200,headers:{"Idempotency-Replayed":"true"}});
 it("retries lost responses with identical frozen bytes/key and fresh CSRF", async () => {
  const request=vi.fn().mockRejectedValueOnce(new Error("response lost")).mockImplementationOnce(async()=>receipt());vi.stubGlobal("fetch",request);
  await expect(postMoneyEvent("csrf-first",input)).rejects.toMatchObject({status:0});
  const frozen=pendingFinancialWrite.retry();
  await expect(postMoneyEvent("csrf-new",{...input,amount_minor:"1500"})).rejects.toMatchObject({reason:"pending"});
  expect(request).toHaveBeenCalledTimes(1);
  await expect(retryFinancialWrite("csrf-renewed")).resolves.toEqual({outcome:"already_applied"});
  expect(request.mock.calls[1][1].body).toBe(request.mock.calls[0][1].body);
  expect(request.mock.calls[1][1].headers).toMatchObject({"Idempotency-Key":frozen.key,"X-CSRF-Token":"csrf-renewed"});
  expect(pendingFinancialWrite.status().kind).toBe("empty");
 });
 it("keeps a conflicting key until explicit recovery, never rotates automatically",async()=>{
  const request=vi.fn().mockImplementation(async()=>new Response(JSON.stringify({error:{code:"idempotency_key_conflict",message:"private payload"}}),{status:409}));vi.stubGlobal("fetch",request);
  await expect(postMoneyEvent("csrf",input)).rejects.toMatchObject({code:"idempotency_key_conflict"});
  const frozen=pendingFinancialWrite.retry();await expect(postMoneyEvent("csrf",input)).rejects.toMatchObject({reason:"pending"});
  expect(pendingFinancialWrite.retry()).toEqual(frozen);expect(request).toHaveBeenCalledTimes(1);
 });
 it("preserves pending state after expired sessions or a rejected retry",async()=>{
  const request=vi.fn().mockRejectedValueOnce(new Error("lost")).mockImplementationOnce(async()=>new Response(JSON.stringify({error:{code:"authentication_required"}}),{status:401})).mockImplementationOnce(async()=>new Response(JSON.stringify({error:{code:"conflict"}}),{status:409}));vi.stubGlobal("fetch",request);
  await expect(postMoneyEvent("csrf",input)).rejects.toThrow();const frozen=pendingFinancialWrite.retry();
  await expect(retryFinancialWrite("expired")).rejects.toMatchObject({status:401});
  await expect(retryFinancialWrite("renewed")).rejects.toMatchObject({status:409});expect(pendingFinancialWrite.retry()).toEqual(frozen);
 });
 it("releases a definitive initial validation failure while preserving caller input",async()=>{
  const original=JSON.stringify(input);const request=vi.fn().mockImplementation(async()=>new Response(JSON.stringify({error:{code:"invalid_input"}}),{status:400}));vi.stubGlobal("fetch",request);
  await expect(postMoneyEvent("csrf",input)).rejects.toMatchObject({status:400});expect(pendingFinancialWrite.status().kind).toBe("empty");expect(JSON.stringify(input)).toBe(original);
 });
 it("does not treat malformed success as resolved",async()=>{
  const request=vi.fn().mockImplementation(async()=>new Response("{}",{status:201,headers:{"Idempotency-Replayed":"false"}}));vi.stubGlobal("fetch",request);
  await expect(postMoneyEvent("csrf",input)).rejects.toMatchObject({status:201});expect(pendingFinancialWrite.status().kind).toBe("pending");
 });
 it("never sends a corrupted pending payload",async()=>{
  const record=await pendingFinancialWrite.prepare("/api/v1/events",JSON.stringify(input));
  const broken={...record,body:JSON.stringify({...input,amount_minor:"9999"})};window.sessionStorage.setItem("lifeos.financial-write.v1",JSON.stringify(broken));
  const request=vi.fn();vi.stubGlobal("fetch",request);
  await expect(retryFinancialWrite("csrf")).rejects.toMatchObject({reason:"corrupted"});expect(request).not.toHaveBeenCalled();
 });
});

describe("Atomic edit transport",()=>{
 const id="11111111-1111-4111-8111-111111111111";
 const input={kind:"expense" as const,currency:"MYR",amount_minor:"3500",occurred_at:"2026-10-20T00:00:00.123456Z",note:" exact note ",account_id:id,allocations:[{category_id:id,amount_minor:"2000"},{category_id:"22222222-2222-4222-8222-222222222222",amount_minor:"1500"}]};
 it("freezes complete splits, exact instant and If-Match across a lost response",async()=>{
  const request=vi.fn().mockRejectedValueOnce(new Error("lost")).mockImplementationOnce(async()=>new Response('{"outcome":"already_applied"}',{status:200,headers:{"Idempotency-Replayed":"true"}}));vi.stubGlobal("fetch",request);
  await expect(editMoneyEvent("csrf",id,"7",input)).rejects.toThrow();const frozen=pendingFinancialWrite.retry();
  await retryFinancialWrite("renewed");
  expect(request.mock.calls[1][1].body).toBe(frozen.body);expect(JSON.parse(frozen.body!)).toEqual(input);
  expect(request.mock.calls[1][1].headers).toMatchObject({"If-Match":'"7"',"Idempotency-Key":frozen.key,"X-CSRF-Token":"renewed"});
  expect(pendingFinancialWrite.status().kind).toBe("empty");
 });
 it("releases a definitive initial stale failure without changing the caller's draft",async()=>{
  const original=JSON.stringify(input);vi.stubGlobal("fetch",vi.fn().mockImplementation(async()=>new Response('{"error":{"code":"event_revision_conflict"}}',{status:412})));
  await expect(editMoneyEvent("csrf",id,"7",input)).rejects.toMatchObject({status:412});expect(JSON.stringify(input)).toBe(original);expect(pendingFinancialWrite.status().kind).toBe("empty");
 });
 it("reads the full event through the guarded owner-scoped route",async()=>{
  const request=vi.fn().mockImplementation(async()=>new Response('{}',{status:200}));vi.stubGlobal("fetch",request);await getMoneyEvent(id);
  expect(request).toHaveBeenCalledWith(`/api/v1/events/${id}`,expect.objectContaining({credentials:"same-origin",cache:"no-store"}));
 });
 it("keeps recovery when the Edit success omits its effective movements",async()=>{
  const result={id,currency:"MYR",amount_minor:"3500",movements:[],allocations:[],note:"",occurred_at:input.occurred_at,kind:"expense",status:"posted",event_revision:"8"};
  vi.stubGlobal("fetch",vi.fn().mockImplementation(async()=>new Response(JSON.stringify(result),{status:200,headers:{"Idempotency-Replayed":"false",ETag:'"8"'}})));
  await expect(editMoneyEvent("csrf",id,"7",input)).rejects.toMatchObject({status:200});expect(pendingFinancialWrite.status().kind).toBe("pending");
 });

});
