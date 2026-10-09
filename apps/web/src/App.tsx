import { useEffect, useState } from "react";

import { fetchCurrentUser, signOut, type CurrentUser } from "./api/auth";
import { fetchHealth } from "./api/client";

type CheckState = "idle" | "checking" | "healthy" | "unavailable";

export function App() {
  const [checkState, setCheckState] = useState<CheckState>("idle");
  const [user, setUser] = useState<CurrentUser | null>(null);
  const [authState, setAuthState] = useState("Checking sign-in status…");

  useEffect(() => {
    void fetchCurrentUser()
      .then((current) => { setUser(current); setAuthState(current ? "Signed in" : "Sign in with an invited Google account."); })
      .catch(() => setAuthState("Sign-in status is unavailable."));
  }, []);

  async function handleSignOut() {
    if (!user) return;
    try {
      await signOut(user.csrf_token);
      setUser(null);
      setAuthState("Signed out.");
    } catch {
      setAuthState("Sign-out failed. Please try again.");
    }
  }

  async function checkAPI() {
    setCheckState("checking");
    try {
      const result = await fetchHealth();
      setCheckState(result.status === "ok" ? "healthy" : "unavailable");
    } catch {
      setCheckState("unavailable");
    }
  }

  return (
    <main className="shell">
      <section className="foundation" aria-labelledby="foundation-title">
        <p className="eyebrow">LifeOS</p>
        <h1 id="foundation-title">Your life, in one place.</h1>
        <p className="summary">
          Invite-only Alpha sign-in is ready for local testing. Life features will follow.
        </p>
        <div className="status-panel">
          <h2>Account</h2>
          <p aria-live="polite">{authState}</p>
          {user ? (
            <>
              <p>{user.email}</p>
              <button type="button" onClick={() => void handleSignOut()}>Sign out</button>
            </>
          ) : (
            <a href="/api/v1/auth/start">Sign in with Google</a>
          )}
        </div>
        <div className="status-panel">
          <h2>API connection</h2>
          <p aria-live="polite">{statusMessage(checkState)}</p>
          <button type="button" onClick={checkAPI} disabled={checkState === "checking"}>
            {checkState === "checking" ? "Checking…" : "Check API health"}
          </button>
        </div>
      </section>
    </main>
  );
}

function statusMessage(state: CheckState) {
  switch (state) {
    case "checking":
      return "Contacting the local Foundation API.";
    case "healthy":
      return "The local Foundation API is healthy.";
    case "unavailable":
      return "The local Foundation API is unavailable.";
    default:
      return "Run the local API, then check its health from this shell.";
  }
}
