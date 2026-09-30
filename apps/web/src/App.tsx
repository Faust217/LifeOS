import { useState } from "react";

import { fetchHealth } from "./api/client";

type CheckState = "idle" | "checking" | "healthy" | "unavailable";

export function App() {
  const [checkState, setCheckState] = useState<CheckState>("idle");

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
        <h1 id="foundation-title">Foundation is taking shape.</h1>
        <p className="summary">
          The durable server, database, and web boundaries are being established before any personal finance features are added.
        </p>
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
