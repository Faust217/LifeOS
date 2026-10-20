import { useEffect, useMemo, useRef, useState } from "react";
import { SessionExpiredError, updateTimeZone } from "./api/auth";
import { timeZoneLabel } from "./moneyAmount";
import "./money.css";

function browserTimeZone(): string {
  try { return Intl.DateTimeFormat().resolvedOptions().timeZone ?? ""; }
  catch { return ""; }
}

export function TimeZoneSetting({ timeZone, csrfToken, openRequest, onRequestOpen, onSaved, onSessionExpired }: { timeZone: string | null; csrfToken: string; openRequest: number; onRequestOpen: () => void; onSaved: (zone: string) => void; onSessionExpired: () => void }) {
  const [suggestedZone] = useState(browserTimeZone);
  const [draft, setDraft] = useState(timeZone ?? suggestedZone);
  const [search, setSearch] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const dialogRef = useRef<HTMLDialogElement>(null);
  const searchRef = useRef<HTMLInputElement>(null);
  const returnFocusRef = useRef<HTMLElement | null>(null);
  const zones = useMemo(() => {
    const preferred = timeZone ?? suggestedZone;
    const all = [...new Set(["UTC", ...Intl.supportedValuesOf("timeZone"), ...(preferred ? [preferred] : [])])].sort();
    return preferred ? [preferred, ...all.filter((zone) => zone !== preferred)] : all;
  }, [suggestedZone, timeZone]);
  const filteredZones = useMemo(() => zones.filter((zone) => `${zone} ${zone.replaceAll("_", " ").replaceAll("/", " ")}`.toLowerCase().includes(search.trim().toLowerCase())), [zones, search]);
  useEffect(() => {
    const dialog = dialogRef.current;
    if (openRequest === 0 || !dialog) return;
    if (!dialog.open) {
      const active = document.activeElement as HTMLElement | null;
      returnFocusRef.current = active?.matches("button, a, input, select, textarea, [tabindex]") ? active : document.querySelector<HTMLElement>(".timezone-settings-action");
      setDraft(timeZone ?? suggestedZone);
      setSearch("");
      setError("");
      dialog.showModal();
      searchRef.current?.focus();
    }
  }, [openRequest]);
  function closed() {
    window.setTimeout(() => {
      if (dialogRef.current?.open) return;
      const target = returnFocusRef.current;
      if (target?.isConnected) target.focus();
      else document.querySelector<HTMLElement>(".timezone-settings-action, #timezone button")?.focus();
    }, 0);
  }
  async function save(zone: string) {
    if (!zone) { setError("Choose a timezone from the list."); return; }
    setError(""); setSaving(true);
    try {
      await updateTimeZone(csrfToken, zone);
      const dialogWasOpen = !!dialogRef.current?.open;
      onSaved(zone);
      if (dialogWasOpen) dialogRef.current?.close();
      else window.setTimeout(() => document.querySelector<HTMLElement>(".timezone-settings-action")?.focus(), 0);
    } catch (cause) { if (cause instanceof SessionExpiredError) onSessionExpired(); else setError(cause instanceof Error ? cause.message : "Unable to save timezone. Please retry."); }
    finally { setSaving(false); }
  }
  return <>
    {!timeZone && <section className="money-timezone-card" aria-labelledby="money-timezone-title"><h2 id="money-timezone-title">Set your transaction timezone</h2><p>Use the same dates and times on every device.</p>{suggestedZone ? <><p className="money-timezone-suggestion">Suggested: <strong>{timeZoneLabel(suggestedZone)}</strong><span>{suggestedZone}</span></p><button type="button" className="button button-primary" disabled={saving} onClick={() => void save(suggestedZone)}>{saving ? "Saving…" : "Confirm timezone"}</button></> : <p>Choose your timezone to start recording transactions.</p>}<button type="button" className="button button-outline" disabled={saving} onClick={onRequestOpen}>Choose another timezone</button>{error && <p role="alert" className="money-field-error">{error}</p>}</section>}
    <dialog ref={dialogRef} className="money-transaction-dialog money-timezone-dialog" aria-labelledby="money-settings-title" onCancel={(event) => { if (saving) event.preventDefault(); }} onClose={closed} onClick={(event) => { if (event.target === dialogRef.current && !saving) dialogRef.current.close(); }}>
      <div className="money-dialog-content"><div className="money-dialog-head"><h2 id="money-settings-title">Timezone settings</h2><button type="button" className="money-dialog-close" aria-label="Close Timezone settings" disabled={saving} onClick={() => dialogRef.current?.close()}>×</button></div>
        <form className="money-timezone-form" onSubmit={(event) => { event.preventDefault(); void save(draft); }}>
          <p className="money-help">Transaction dates use your LifeOS timezone on every device. Changing it may regroup older dates; saved UTC times and amounts do not change.</p>
          <p className="money-timezone-selected"><strong>Selected:</strong> {draft ? timeZoneLabel(draft) : "Choose a timezone"}{draft && <span>{draft}</span>}</p>
          <label htmlFor="money-timezone-search">Search city or IANA timezone</label><input ref={searchRef} id="money-timezone-search" type="search" value={search} onChange={(event) => { const query = event.target.value; setSearch(query); if (draft && !`${draft} ${draft.replaceAll("_", " ").replaceAll("/", " ")}`.toLowerCase().includes(query.trim().toLowerCase())) setDraft(""); }} placeholder="Try Kuala Lumpur or New York" autoComplete="off" />
          <label htmlFor="money-timezone-list">Timezone</label><select id="money-timezone-list" size={7} value={filteredZones.includes(draft) ? draft : ""} onChange={(event) => setDraft(event.target.value)} aria-describedby="money-timezone-count"><option value="" disabled>Choose from the list</option>{filteredZones.map((zone) => <option key={zone} value={zone}>{timeZoneLabel(zone)} · {zone}</option>)}</select>
          <p id="money-timezone-count" className="money-help">{filteredZones.length ? `${filteredZones.length} matching timezones` : "No matching timezone. Try another city or IANA name."}</p>
          {error && <p role="alert" className="money-field-error">{error}</p>}
          <div className="money-confirm-actions"><button type="button" className="button button-outline" disabled={saving} onClick={() => dialogRef.current?.close()}>Cancel</button><button type="submit" className="button button-primary" disabled={saving || !draft}>{saving ? "Saving…" : timeZone ? "Save timezone" : "Confirm timezone"}</button></div>
        </form>
      </div>
    </dialog>
  </>;
}
