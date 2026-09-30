import { describe, expect, it, vi } from "vitest";

import { fetchHealth } from "./client";

describe("fetchHealth", () => {
  it("uses the API boundary and returns the Foundation status", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(
      new Response(JSON.stringify({ status: "ok" }), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      }),
    );

    await expect(fetchHealth(fetcher)).resolves.toEqual({ status: "ok" });
    expect(fetcher).toHaveBeenCalledOnce();
    expect(String(fetcher.mock.calls[0][0])).toBe("http://127.0.0.1:8080/healthz");
  });

  it("rejects unsuccessful responses without inventing product behavior", async () => {
    const fetcher = vi.fn<typeof fetch>().mockResolvedValue(new Response(null, { status: 503 }));
    await expect(fetchHealth(fetcher)).rejects.toThrow("status 503");
  });
});
