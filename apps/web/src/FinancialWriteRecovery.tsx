import { useEffect, useRef, useState } from "react";
import { MoneyApiError, retryFinancialWrite } from "./api/money";
import { pendingFinancialWrite } from "./pendingFinancialWrite";

export function usePendingFinancialWrite() {
  const [, refresh] = useState(0);
  useEffect(() => pendingFinancialWrite.subscribe(() => refresh((value) => value + 1)), []);
  return pendingFinancialWrite.status();
}

export function FinancialWriteRecovery({ csrfToken, onRecovered, onSessionExpired }: { csrfToken: string; onRecovered: () => void; onSessionExpired: () => void }) {
  const status = usePendingFinancialWrite();
  const [acknowledged, setAcknowledged] = useState(false);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  const retryButton = useRef<HTMLButtonElement>(null);
  useEffect(() => {
    if (status.kind !== "pending" || status.sending) return;
    const button = retryButton.current;
    const modal = document.querySelector("dialog[open]");
    if (button?.getClientRects().length && (!modal || modal.contains(button))) button.focus();
  }, [status.kind, status.kind === "pending" ? status.sending : false]);
  if (status.kind === "empty" || status.kind === "hidden" || (status.kind === "pending" && status.sending)) return null;
  async function retry() {
    if (busy) return;
    setBusy(true); setError("");
    try { await retryFinancialWrite(csrfToken); onRecovered(); }
    catch (cause) {
      setError(cause instanceof Error ? cause.message : "Save is still unresolved. Keep this recovery record.");
      if (cause instanceof MoneyApiError && cause.status === 401) onSessionExpired();
    } finally { setBusy(false); }
  }
  function clear() {
    if (!acknowledged || busy) return;
    try { pendingFinancialWrite.clearAcknowledged(); setAcknowledged(false); setError(""); }
    catch (cause) { setError(cause instanceof Error ? cause.message : "Recovery storage could not be cleared."); }
  }
  return <section className="money-feedback money-recovery" aria-label="Save recovery">
    <h2>{status.kind === "pending" ? "Check your last Save" : status.kind === "other_owner" ? "Another account has an unresolved Save" : "Recovery storage needs attention"}</h2>
    <p>{status.kind === "pending" ? "Your last Save may have succeeded. Retry safely to check without creating a duplicate."
      : status.kind === "other_owner" ? "This tab has a Save from a different account. Its details are hidden. Sign in to that account to retry, or explicitly clear the local record if you are abandoning its retry key."
      : status.kind === "corrupted" ? "The recovery record is unreadable. A lost request key cannot be recovered. Check your accounts and activity before clearing it."
      : "Recovery storage is unavailable. New financial Saves are blocked. If a previous Save was uncertain and its key is lost, LifeOS cannot recover that key."}</p>
    {status.kind === "pending" && <button ref={retryButton} type="button" className="button button-primary" disabled={busy} onClick={() => void retry()}>{busy ? "Checking Save…" : "Retry Save"}</button>}
    <details><summary>More recovery options</summary>
      <p>{status.kind === "pending" ? "Refreshing activity does not confirm or discard this Save." : status.kind === "other_owner" ? "Only the original account can retry this Save." : "Refreshing does not repair recovery storage."} {status.kind === "other_owner" ? "Review the original account's activity first if possible." : status.kind === "unavailable" ? "If a Save was uncertain, check activity before clearing local recovery." : "Check your accounts and activity before clearing this record."} Clearing cannot undo a Save and loses this tab's retry key.</p>
      <label className="money-choice"><input type="checkbox" checked={acknowledged} disabled={busy} onChange={(event) => setAcknowledged(event.target.checked)} />{status.kind === "other_owner" ? "I understand the other account's retry key will be lost." : "I checked my activity and understand the retry key will be lost."}</label>
      <button type="button" className="button button-outline" disabled={busy || !acknowledged} onClick={clear}>Clear recovery record</button>
    </details>
    {error && <p role="alert">{error}</p>}
  </section>;
}
