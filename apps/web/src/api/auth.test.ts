import { afterEach, describe, expect, it, vi } from "vitest";

import { fetchCurrentUser, signOut } from "./auth";

afterEach(() => vi.unstubAllGlobals());

describe("auth API", () => {
  it("treats an unauthenticated response as signed out", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ status: 401 }));
    await expect(fetchCurrentUser()).resolves.toBeNull();
  });

  it("does not treat a missing Auth API as a signed-out session", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ status: 404, ok: false }));
    await expect(fetchCurrentUser()).rejects.toThrow("Unable to check sign-in status");
  });

  it("sends the session CSRF token on sign-out", async () => {
    const request = vi.fn().mockResolvedValue({ ok: true });
    vi.stubGlobal("fetch", request);
    await signOut("test-csrf-token");
    expect(request).toHaveBeenCalledWith("/api/v1/auth/logout", {
      method: "POST",
      credentials: "same-origin",
      headers: { "X-CSRF-Token": "test-csrf-token" },
    });
  });

  it("treats an already expired session as signed out", async () => {
    vi.stubGlobal("fetch", vi.fn().mockResolvedValue({ status: 401, ok: false }));
    await expect(signOut("expired-session-csrf")).resolves.toBeUndefined();
  });
});
