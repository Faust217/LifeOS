import { useEffect, useRef, useState } from "react";
import { confirmOpeningBalance, listMoneyAccounts, MoneyApiError, previewOpeningBalance, type MoneyAccount, type OpeningPreview } from "./api/money";
import { formatLocalDate, formatLocalTimeWithZone, formatMinorUnits, localDateTimeValue, matchingInstants, parseAmountToMinor, toOccurredAt } from "./moneyAmount";

export type OpeningBalanceDraft = { now: string; historical: boolean; when: string; instant: string; preserve: boolean; amount: string };

export function OpeningBalanceSheet({ account, csrfToken, timeZone, draft, onDraft, onClose, onSaved, onSessionExpired }: { account: MoneyAccount; csrfToken: string; timeZone: string; draft?: OpeningBalanceDraft; onDraft: (draft: OpeningBalanceDraft) => void; onClose: () => void; onSaved: () => void; onSessionExpired: () => void }) {
  const dialog = useRef<HTMLDialogElement>(null);
  const trigger = useRef<HTMLElement | null>(null);
  const reviewHeading = useRef<HTMLParagraphElement>(null);
  const [now] = useState(() => draft?.now ?? new Date().toISOString());
  const [historical, setHistorical] = useState(draft?.historical ?? false);
  const [when, setWhen] = useState(() => draft?.when ?? localDateTimeValue(new Date(now), timeZone));
  const [instant, setInstant] = useState(draft?.instant ?? "");
  const [preserve, setPreserve] = useState(draft?.preserve ?? true);
  const [amount, setAmount] = useState(draft?.amount ?? "");
  const [preview, setPreview] = useState<OpeningPreview | null>(null);
  const [ack, setAck] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const pending = useRef(false);
  useEffect(() => {
    trigger.current = document.activeElement as HTMLElement;
    dialog.current?.showModal();
    dialog.current?.querySelector<HTMLElement>("button")?.focus();
    return () => { if (trigger.current?.isConnected) trigger.current.focus(); };
  }, []);
  useEffect(() => { if (preview) reviewHeading.current?.focus(); }, [preview]);
  useEffect(() => { onDraft({ now, historical, when, instant, preserve, amount }); }, [now, historical, when, instant, preserve, amount, onDraft]);
  const matches = (() => { try { return historical ? matchingInstants(when, timeZone) : []; } catch { return []; } })();
  async function review() {
    if (pending.current) return;
    pending.current = true; setBusy(true); setError("");
    try {
      const at = historical ? toOccurredAt(when, timeZone, instant || undefined) : now;
      const chosen = preserve ? undefined : parseAmountToMinor(amount, account.currency, true);
      const result = await previewOpeningBalance(csrfToken, account.id, { opening_balance_at: at, ...(chosen === undefined ? {} : { opening_balance_minor: chosen }) });
      setPreview(result); setAck(false);
    } catch (cause) { report(cause); }
    finally { pending.current = false; setBusy(false); }
  }
  function report(cause: unknown) {
    if (cause instanceof MoneyApiError && cause.status === 401) onSessionExpired();
    else setError(cause instanceof MoneyApiError && cause.status === 409 ? "This account changed. Review the balance again before confirming." : cause instanceof Error ? cause.message : "Unable to review this balance. Please retry.");
  }
  async function confirm() {
    if (!preview?.chosen_opening_balance_minor || pending.current) return;
    pending.current = true; setBusy(true); setError("");
    try {
      await confirmOpeningBalance(csrfToken, account.id, { opening_balance_at: preview.opening_balance_at, opening_balance_minor: preview.chosen_opening_balance_minor, balance_revision: preview.balance_revision, acknowledge_change: ack });
      onSaved();
    } catch (cause) {
      // Resolve an uncertain response by reading current state, never by blindly converting again.
      if (cause instanceof MoneyApiError && (cause.status === 0 || cause.status === 200 || cause.status === 409 || cause.status >= 500)) {
        try {
          const current = (await listMoneyAccounts()).items.find((item) => item.id === account.id);
          if (current?.opening_balance_at === preview.opening_balance_at && current.opening_balance_minor === preview.chosen_opening_balance_minor) { onSaved(); return; }
        } catch { /* Keep the original draft/error and allow a safe fresh review. */ }
      }
      report(cause);
      if (cause instanceof MoneyApiError && cause.status === 409) setPreview(null);
    } finally { pending.current = false; setBusy(false); }
  }
  const changed = preview?.balance_change_minor != null && preview.balance_change_minor !== "0";
  return <dialog ref={dialog} className="money-transaction-dialog" aria-labelledby="opening-title" onCancel={(event) => { if (busy) event.preventDefault(); }} onClose={onClose} onKeyDown={(event) => {
    if (event.key !== "Tab") return;
    const controls = Array.from(event.currentTarget.querySelectorAll<HTMLElement>('button:not(:disabled), input:not(:disabled), select:not(:disabled), a[href]')).filter((node) => node.getClientRects().length > 0);
    const first = controls[0], last = controls.at(-1);
    if (event.shiftKey && document.activeElement === first && last) { event.preventDefault(); last.focus(); }
    else if (!event.shiftKey && document.activeElement === last && first) { event.preventDefault(); first.focus(); }
  }}>
    <div className="money-dialog-content">
      <div className="money-dialog-head"><h2 id="opening-title">Confirm balance date</h2><button className="money-dialog-close" type="button" aria-label="Close balance review" disabled={busy} onClick={onClose}>×</button></div>
      <p><strong>{account.name}</strong> · {account.currency}</p>
      <p className="money-help">Choose a balance and date so older entries are not counted twice. Nothing changes until you confirm.</p>
      {!preview ? <form className="money-form" onSubmit={(event) => { event.preventDefault(); void review(); }}>
        <fieldset disabled={busy}>
          <p>Balance date: {formatLocalDate(now, timeZone)} · {formatLocalTimeWithZone(now, timeZone)}</p>
          <label className="money-choice"><input type="checkbox" checked={historical} onChange={(event) => setHistorical(event.target.checked)} />Use an earlier date</label>
          {historical && <><label>Balance date and time<input type="datetime-local" required value={when} onChange={(event) => { setWhen(event.target.value); setInstant(""); }} /></label>{matches.length > 1 && <label>This time occurs twice<select required value={instant} onChange={(event) => setInstant(event.target.value)}><option value="">Choose which occurrence</option>{matches.map((value) => <option key={value} value={value}>{formatLocalTimeWithZone(value, timeZone)} · {value}</option>)}</select></label>}</>}
          <label className="money-choice"><input type="checkbox" checked={preserve} onChange={(event) => setPreserve(event.target.checked)} />Keep my recorded balance unchanged</label>
          {!preserve && <label>Known balance ({account.currency})<input inputMode="decimal" required value={amount} onChange={(event) => setAmount(event.target.value)} /></label>}
          <button type="submit" className="button button-primary">{busy ? "Reviewing…" : "Review balance"}</button>
        </fieldset>
      </form> : <div className="money-form">
        <p ref={reviewHeading} tabIndex={-1}>Balance date: <strong>{formatLocalDate(preview.opening_balance_at, timeZone)} · {formatLocalTimeWithZone(preview.opening_balance_at, timeZone)}</strong></p>
        <p className="money-help">{timeZone} · {preview.opening_balance_at}</p>
        <dl className="money-opening-summary"><div><dt>Balance at this date</dt><dd>{preview.chosen_opening_balance_minor == null ? "Enter a known balance within the supported range." : formatMinorUnits(preview.chosen_opening_balance_minor, account.currency)}</dd></div><div><dt>Recorded balance now</dt><dd>{formatMinorUnits(preview.balance_before_minor, account.currency)}</dd></div><div><dt>After confirmation</dt><dd>{preview.balance_after_minor == null ? "Not available" : formatMinorUnits(preview.balance_after_minor, account.currency)}</dd></div><div><dt>Change</dt><dd>{preview.balance_change_minor == null ? "Not available" : formatMinorUnits(preview.balance_change_minor, account.currency)}</dd></div></dl>
        {preserve && preview.suggested_representable && <p className="money-help">Calculated to keep your recorded balance unchanged. Please check the amount and date; this is not a bank verification.</p>}
        {!preview.suggested_representable && preserve && <p role="alert">A balance that preserves the current total cannot be suggested within the supported range. Change details and enter a known balance.</p>}
        <p className="money-help">{preview.before_count} earlier recorded transaction(s) stay in history and spending. They will not change this dated balance again.</p>
        {changed && <label className="money-choice"><input type="checkbox" checked={ack} disabled={busy} onChange={(event) => setAck(event.target.checked)} />I understand my recorded balance will change by {formatMinorUnits(preview.balance_change_minor!, account.currency)}.</label>}
        <div className="money-confirm-actions"><button className="button button-outline" type="button" disabled={busy} onClick={() => setPreview(null)}>Change details</button><button className="button button-primary" type="button" disabled={busy || preview.chosen_opening_balance_minor == null || (changed && !ack)} onClick={() => void confirm()}>{busy ? "Confirming…" : "Confirm balance"}</button></div>
      </div>}
      {error && <p role="alert" className="money-field-error">{error}</p>}
      <button className="money-text-action" type="button" disabled={busy} onClick={onClose}>Cancel</button>
    </div>
  </dialog>;
}
