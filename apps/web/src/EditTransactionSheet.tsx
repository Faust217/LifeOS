import { useEffect, useRef, useState, type FormEvent } from "react";
import { editMoneyEvent, getMoneyEvent, MoneyApiError, type EventEdit, type MoneyAccount, type MoneyCategory, type MoneyEvent } from "./api/money";
import { FinancialWriteRecovery, usePendingFinancialWrite } from "./FinancialWriteRecovery";
import { pendingFinancialWrite } from "./pendingFinancialWrite";
import { openEditDialog } from "./editDialogLifecycle";
import { formatLocalTimeWithZone, formatMinorUnits, localDateTimeValue, matchingInstants, minorUnitsToInput, parseAmountToMinor, toOccurredAt } from "./moneyAmount";

type Props = { id: string; csrfToken: string; timeZone: string; accounts: MoneyAccount[]; categories: MoneyCategory[]; onClose: () => void; onSaved: () => void; onSessionExpired: () => void };

export function EditTransactionSheet(props: Props) {
  const dialog = useRef<HTMLDialogElement>(null);
  const returnFocus = useRef(document.activeElement as HTMLElement | null);
  const [event, setEvent] = useState<MoneyEvent | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [attempt, setAttempt] = useState(0);
  const dirty = useRef(false);
  const busy = useRef(false);
  const [saving, setSaving] = useState(false);
  const [confirmClose, setConfirmClose] = useState(false);
  const keepEditing = useRef<HTMLButtonElement>(null);
  const closeFrom = useRef<HTMLElement | null>(null);
  useEffect(() => { if (confirmClose) keepEditing.current?.focus(); }, [confirmClose]);
  useEffect(() => openEditDialog(dialog.current, returnFocus.current), []);
  useEffect(() => {
    let active = true; setLoading(true); setError("");
    void getMoneyEvent(props.id).then((value) => { if (active) setEvent(value); }).catch((cause: unknown) => {
      if (!active) return; setError(cause instanceof Error ? cause.message : "Transaction could not load.");
      if (cause instanceof MoneyApiError && cause.status === 401) props.onSessionExpired();
    }).finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [props.id, attempt]);
  function close() {
    if (busy.current) return;
    if (!dirty.current) { props.onClose(); return; }
    closeFrom.current = document.activeElement as HTMLElement | null;
    setConfirmClose(true);
  }
  return <dialog ref={dialog} className="money-transaction-dialog money-edit-dialog" aria-labelledby="money-edit-title" onCancel={(e) => { e.preventDefault(); close(); }} onClick={(e) => { if (e.target === dialog.current) close(); }}>
    <div className="money-dialog-content"><div className="money-dialog-head"><h2 id="money-edit-title">Edit transaction</h2><button type="button" className="money-dialog-close" onClick={close} disabled={saving} aria-label="Close edit transaction">×</button></div>
      {confirmClose && <section className="money-confirm" role="group" aria-label="Discard unsaved changes"><p>Discard your unsaved changes? Any unresolved Save will still be kept for safe retry.</p><div className="money-confirm-actions"><button ref={keepEditing} type="button" className="button button-outline" disabled={saving} onClick={() => { setConfirmClose(false); window.setTimeout(() => closeFrom.current?.focus(), 0); }}>Keep editing</button><button type="button" className="button button-danger" disabled={saving} onClick={props.onClose}>Discard changes</button></div></section>}
      <FinancialWriteRecovery csrfToken={props.csrfToken} onRecovered={props.onSaved} onSessionExpired={props.onSessionExpired} />
      {loading ? <p role="status">Loading transaction…</p> : error ? <div role="alert"><p>{error}</p><button type="button" className="button button-outline" onClick={() => setAttempt((v) => v + 1)}>Try again</button></div> : event && event.status === "posted" ? <EditForm {...props} initial={event} onDirty={() => { dirty.current = true; }} onBusy={(value) => { busy.current = value; setSaving(value); }} onCancel={close} /> : <p role="status">This transaction is no longer available to edit.</p>}
    </div>
  </dialog>;
}

function originalChoice(event: MoneyEvent, zone: string) {
  return matchingInstants(localDateTimeValue(new Date(event.occurred_at), zone), zone).find((v) => Math.abs(Date.parse(v) - Date.parse(event.occurred_at)) < 60_000) ?? "";
}

function EditForm({ initial, onDirty, onBusy, onCancel, ...props }: Props & { initial: MoneyEvent; onDirty: () => void; onBusy: (value: boolean) => void; onCancel: () => void }) {
  const [basis, setBasis] = useState(initial);
  const [occurrenceBase, setOccurrenceBase] = useState(initial);
  const reviewButton = useRef<HTMLButtonElement>(null);
  const [amount, setAmount] = useState(minorUnitsToInput(initial.amount_minor, initial.currency));
  const [account, setAccount] = useState(initial.movements[0]?.account_id ?? "");
  const [from, setFrom] = useState(initial.movements.find((m) => BigInt(m.amount_minor) < 0n)?.account_id ?? "");
  const [to, setTo] = useState(initial.movements.find((m) => BigInt(m.amount_minor) > 0n)?.account_id ?? "");
  const [when, setWhen] = useState(localDateTimeValue(new Date(initial.occurred_at), props.timeZone));
  const [choice, setChoice] = useState(originalChoice(initial, props.timeZone));
  const [note, setNote] = useState(initial.note);
  const [parts, setParts] = useState(initial.allocations.map((a) => ({ category: a.category_id, amount: minorUnitsToInput(a.amount_minor, initial.currency) })));
  const [pending, setPending] = useState(false);
  const [error, setError] = useState("");
  const [stale, setStale] = useState(false);
  const [latest, setLatest] = useState<MoneyEvent | null>(null);
  const [reviewing, setReviewing] = useState(false);
  useEffect(() => { if (stale) reviewButton.current?.focus(); }, [stale]);
  const recovery = usePendingFinancialWrite();
  const blocked = pending || (recovery.kind !== "empty" && recovery.kind !== "hidden");
  const allowedAccounts = props.accounts.filter((a) => a.currency === initial.currency && (!a.archived_at || basis.movements.some((m) => m.account_id === a.id) || [account, from, to].includes(a.id)));
  const allowedCategories = props.categories.filter((c) => c.kind === initial.kind && (!c.archived_at || basis.allocations.some((a) => a.category_id === c.id) || parts.some((p) => p.category === c.id)));
  let matches: string[] = []; try { matches = matchingInstants(when, props.timeZone); } catch { /* Validation on Save. */ }
  function changed() { onDirty(); setError(""); }
  async function reviewLatest() {
    setReviewing(true);
    try { setLatest(await getMoneyEvent(initial.id)); }
    catch (cause) { setError(cause instanceof Error ? cause.message : "Latest transaction could not load."); if (cause instanceof MoneyApiError && cause.status === 401) props.onSessionExpired(); }
    finally { setReviewing(false); }
  }
  function useLatest(value: MoneyEvent) {
    setBasis(value); setOccurrenceBase(value); setAmount(minorUnitsToInput(value.amount_minor, value.currency)); setAccount(value.movements[0]?.account_id ?? "");
    setFrom(value.movements.find((m) => BigInt(m.amount_minor) < 0n)?.account_id ?? ""); setTo(value.movements.find((m) => BigInt(m.amount_minor) > 0n)?.account_id ?? "");
    setWhen(localDateTimeValue(new Date(value.occurred_at), props.timeZone)); setChoice(originalChoice(value, props.timeZone)); setNote(value.note);
    setParts(value.allocations.map((a) => ({ category: a.category_id, amount: minorUnitsToInput(a.amount_minor, value.currency) }))); setStale(false); setLatest(null); setError("");
  }
  async function submit(e: FormEvent) {
    e.preventDefault(); if (blocked || stale) return; setError("");
    let input: EventEdit;
    try {
      const amount_minor = parseAmountToMinor(amount, initial.currency);
      for (const id of initial.kind === "transfer" ? [from, to] : [account]) {
        const selected = props.accounts.find((a) => a.id === id && a.currency === initial.currency);
        if (!selected || (selected.archived_at && !basis.movements.some((m) => m.account_id === id))) throw new Error("Choose an active account; this account can no longer be retained.");
      }
      const occurred_at = when === localDateTimeValue(new Date(occurrenceBase.occurred_at), props.timeZone) && choice === originalChoice(occurrenceBase, props.timeZone) ? occurrenceBase.occurred_at : toOccurredAt(when, props.timeZone, choice || undefined);
      if (initial.kind === "transfer") {
        if (!from || !to || from === to) throw new Error("Choose two different accounts in the same currency.");
        input = { kind: "transfer", currency: initial.currency, amount_minor, occurred_at, note, from_account_id: from, to_account_id: to };
      } else {
        if (!account) throw new Error("Choose an account.");
        const allocations = parts.map((p) => {
          const selected = props.categories.find((c) => c.id === p.category && c.kind === initial.kind);
          if (p.category && (!selected || (selected.archived_at && !basis.allocations.some((a) => a.category_id === p.category)))) throw new Error("Choose an active category; this category can no longer be retained.");
          if (!p.category) throw new Error("Choose a category for every allocation, or remove it for Uncategorized."); return { category_id: p.category, amount_minor: parseAmountToMinor(p.amount, initial.currency) }; });
        if (new Set(allocations.map((a) => a.category_id)).size !== allocations.length) throw new Error("Each category can appear only once.");
        if (allocations.length && allocations.reduce((sum, a) => sum + BigInt(a.amount_minor), 0n) !== BigInt(amount_minor)) throw new Error("Category amounts must add up exactly to the transaction amount.");
        input = { kind: initial.kind, currency: initial.currency, amount_minor, occurred_at, note, account_id: account, allocations };
      }
    } catch (cause) { setError(cause instanceof Error ? cause.message : "Check the transaction details."); return; }
    setPending(true); onBusy(true);
    try { await editMoneyEvent(props.csrfToken, initial.id, basis.event_revision, input); props.onSaved(); }
    catch (cause) {
      setError(pendingFinancialWrite.status().kind === "pending" ? "" : cause instanceof Error ? cause.message : "Save could not complete. Your draft is kept.");
      if (cause instanceof MoneyApiError && cause.status === 412) { setStale(true); setError(""); }
      if (cause instanceof MoneyApiError && cause.status === 401) props.onSessionExpired();
    } finally { setPending(false); onBusy(false); }
  }
  return <form className="money-form money-edit-form" onSubmit={(e) => void submit(e)}>
    <fieldset disabled={blocked}><div className="money-field-grid">
      <label>Amount ({initial.currency})<input autoFocus required inputMode="decimal" value={amount} onChange={(e) => { setAmount(e.target.value); changed(); }} /></label>
      {initial.kind === "transfer" ? <>{(["From account", "To account"] as const).map((label, i) => <label key={label}>{label}<select required value={i ? to : from} onChange={(e) => { (i ? setTo : setFrom)(e.target.value); changed(); }}>{allowedAccounts.map((a) => <option key={a.id} value={a.id}>{a.name}{a.archived_at ? " (archived)" : ""}</option>)}</select></label>)}</> : <label>Account<select required value={account} onChange={(e) => { setAccount(e.target.value); changed(); }}>{allowedAccounts.map((a) => <option key={a.id} value={a.id}>{a.name}{a.archived_at ? " (archived)" : ""}</option>)}</select></label>}
      <label>Date and time · {props.timeZone}<input required type="datetime-local" value={when} onChange={(e) => { setWhen(e.target.value); setChoice(""); changed(); }} /></label>
      {matches.length > 1 && <label>This time occurs twice<select required value={choice} onChange={(e) => { setChoice(e.target.value); changed(); }}><option value="">Choose an occurrence</option>{matches.map((v, i) => <option key={v} value={v}>{i ? "Second" : "First"} · {formatLocalTimeWithZone(v, props.timeZone)}</option>)}</select></label>}
    </div>
    {initial.kind !== "transfer" && <section className="money-edit-allocations" aria-label="Category allocations"><h3>Categories</h3>{parts.length === 0 && <p className="money-help">Uncategorized</p>}{parts.map((part, i) => <div className="money-edit-part" key={i}><label>Category {i + 1}<select required value={part.category} onChange={(e) => { setParts(parts.map((p, n) => n === i ? { ...p, category: e.target.value } : p)); changed(); }}><option value="">Choose category</option>{allowedCategories.map((c) => <option key={c.id} value={c.id}>{c.name}{c.archived_at ? " (archived)" : ""}</option>)}</select></label><label>Category amount {i + 1}<input required inputMode="decimal" value={part.amount} onChange={(e) => { setParts(parts.map((p, n) => n === i ? { ...p, amount: e.target.value } : p)); changed(); }} /></label><button type="button" className="money-text-action" aria-label={`Remove category ${i + 1}`} onClick={() => { setParts(parts.filter((_, n) => n !== i)); changed(); }}>Remove</button></div>)}<button type="button" className="button button-outline" onClick={() => { setParts([...parts, { category: "", amount: parts.length ? "" : amount }]); changed(); }}>+ Category allocation</button></section>}
    <label>Note <span className="money-optional">optional</span><textarea rows={3} maxLength={1000} value={note} onChange={(e) => { setNote(e.target.value); changed(); }} /></label>
    </fieldset>
    {error && <p className="money-field-error" role="alert">{error}</p>}
    {stale && <section className="money-confirm" aria-label="Review changed transaction"><p role="alert">This transaction changed. Your draft is kept. Review the latest transaction before saving again.</p><button ref={reviewButton} type="button" className="button button-outline" disabled={blocked || reviewing} onClick={() => void reviewLatest()}>{reviewing ? "Loading…" : "Review latest transaction"}</button>{latest && <><p><strong>Latest:</strong> {formatMinorUnits(latest.amount_minor, latest.currency)} · {latest.status} · {localDateTimeValue(new Date(latest.occurred_at), props.timeZone).replace("T", " ")}</p><p>{latest.movements.map((m) => props.accounts.find((a) => a.id === m.account_id)?.name ?? "Account").join(" → ")}</p><p>{latest.note || "No note"}</p><ul>{latest.allocations.map((a) => <li key={a.id}>{props.categories.find((c) => c.id === a.category_id)?.name ?? "Category"}: {formatMinorUnits(a.amount_minor, a.currency)}</li>)}</ul>{latest.status === "posted" && <div className="money-confirm-actions"><button type="button" className="button button-outline" disabled={blocked} onClick={() => useLatest(latest)}>Use latest values</button><button type="button" className="button button-outline" disabled={blocked} onClick={() => { setBasis(latest); setLatest(null); setStale(false); setError(""); }}>Keep my changes for this version</button></div>}</>}</section>}
    <div className="money-confirm-actions"><button type="button" className="button button-outline" disabled={pending} onClick={onCancel}>Cancel</button><button type="submit" className="button button-primary" disabled={blocked || stale}>{pending ? "Saving…" : "Save changes"}</button></div>
  </form>;
}
