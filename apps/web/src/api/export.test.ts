import { afterEach, describe, expect, it, vi } from "vitest";
import { downloadOwnerExport, MAX_EXPORT_BYTES } from "./export";
import { SessionExpiredError } from "./auth";

const bytes = new TextEncoder().encode('{"format":"lifeos-export-v1"}');
function response(data: Uint8Array<ArrayBuffer> = bytes, overrides: Record<string,string> = {}) {
  return new Response(data, { headers: { "Content-Type":"application/json; charset=utf-8", "Content-Length":String(data.byteLength), "Content-Disposition":'attachment; filename="lifeos-export-v1-20261020T040000Z.json"', ...overrides } });
}
afterEach(() => vi.unstubAllGlobals());
describe("private owner export download", () => {
  it("uses the guarded contract and retains exact downloaded bytes", async () => {
    const request=vi.fn().mockResolvedValue(response()); vi.stubGlobal("fetch",request);
    const signal=new AbortController().signal;
    const result=await downloadOwnerExport("csrf-only","json",signal);
    expect(new Uint8Array(await result.blob.arrayBuffer())).toEqual(bytes);
    expect(result.filename).toBe("lifeos-export-v1-20261020T040000Z.json");
    expect(request).toHaveBeenCalledWith("/api/v1/me/export",expect.objectContaining({method:"POST",credentials:"same-origin",cache:"no-store",signal,body:'{"format":"json"}',headers:{"Content-Type":"application/json","X-CSRF-Token":"csrf-only"}}));
  });
  it("does not turn errors or session expiry into downloadable files", async () => {
    for(const code of [403,422,429,500,503]){vi.stubGlobal("fetch",vi.fn().mockResolvedValue(new Response("private details",{status:code})));await expect(downloadOwnerExport("csrf","json",new AbortController().signal)).rejects.not.toThrow("private details");}
    vi.stubGlobal("fetch",vi.fn().mockResolvedValue(new Response("",{status:401})));
    await expect(downloadOwnerExport("csrf","json",new AbortController().signal)).rejects.toBeInstanceOf(SessionExpiredError);
  });
  it("requests v2 explicitly without accepting a differently versioned filename", async () => {
    const request=vi.fn().mockResolvedValue(response(bytes,{"Content-Disposition":'attachment; filename="lifeos-export-v2-20261020T040000Z.json"'}));
    vi.stubGlobal("fetch",request);
    const signal=new AbortController().signal;
    expect((await downloadOwnerExport("csrf","json",signal,2)).filename).toContain("-v2-");
    expect(JSON.parse(request.mock.calls[0][1].body)).toEqual({format:"json",export_version:2});
    request.mockResolvedValue(response());
    await expect(downloadOwnerExport("csrf","json",signal,2)).rejects.toThrow();
  });
  it("rejects size, MIME, unsafe filename and truncated responses", async () => {
    const invalidHeaders: Record<string,string>[] = [{"Content-Length":String(MAX_EXPORT_BYTES+1)},{"Content-Length":"999"},{"Content-Type":"text/html"},{"Content-Disposition":'attachment; filename="../private.json"'}];
    for(const headers of invalidHeaders){
      vi.stubGlobal("fetch",vi.fn().mockResolvedValue(response(bytes,headers)));
      await expect(downloadOwnerExport("csrf","json",new AbortController().signal)).rejects.toThrow();
    }
  });
  it("cancels an oversized stream and respects cancellation", async () => {
    const cancel=vi.fn();
    const body=new ReadableStream({start(controller){controller.enqueue(bytes);},cancel});
    const res=response();
    const headers=new Headers(res.headers);headers.set("Content-Length","1");
    vi.stubGlobal("fetch",vi.fn().mockResolvedValue(new Response(body,{headers})));
    await expect(downloadOwnerExport("csrf","json",new AbortController().signal)).rejects.toThrow("expected size");
    expect(cancel).toHaveBeenCalled();
    const controller=new AbortController();controller.abort();vi.stubGlobal("fetch",vi.fn().mockResolvedValue(response()));
    await expect(downloadOwnerExport("csrf","json",controller.signal)).rejects.toMatchObject({name:"AbortError"});
  });
});
