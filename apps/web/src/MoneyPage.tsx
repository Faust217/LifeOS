import { Fragment, useCallback, useEffect, useMemo, useRef, useState, type FormEvent } from "react";

import {
  archiveMoneyAccount, archiveMoneyCategory, createMoneyAccount, createMoneyCategory, deleteMoneyEvent, getMoneyBalance,
  listMoneyAccounts, listMoneyCategories, listRunningHistory, MoneyApiError,
  postMoneyEvent, postMoneyTransfer, renameMoneyCategory, restoreMoneyAccount, restoreMoneyCategory,
  type AccountCreate, type CategoryCreate, type EventCreate, type MoneyAccount,
  type MoneyBalance, type MoneyCategory, type MoneyEvent, type TransferCreate, type RunningHistory, type HistoryPosition,
} from "./api/money";
import { FinancialWriteRecovery, usePendingFinancialWrite } from "./FinancialWriteRecovery";
import { FinancialRecoveryError, pendingFinancialWrite } from "./pendingFinancialWrite";
import { OpeningBalanceSheet, type OpeningBalanceDraft } from "./OpeningBalanceSheet";
import { EditTransactionSheet } from "./EditTransactionSheet";
import { HistoryPages } from "./historyPages";
import { captureHistoryLocation, restoreHistoryLocation, type HistoryLocation } from "./historyPosition";
import { Icon } from "./Icons";
import { currencyDigits, formatLocalDate, formatLocalTime, formatLocalTimeWithZone, formatMinorUnits, localDateKey, localDateTimeValue, matchingInstants, minorUnitsToInput, parseAmountToMinor, toOccurredAt } from "./moneyAmount";
import "./money.css";

const PAGE_SIZE = 25;
const RESTORE_BATCH_LIMIT = 8;
const AUTO_VOID_REASON = "No reason provided by owner";
type TransactionAction = "income" | "expense" | "transfer";
type RunMutation = (message: string, action: () => Promise<unknown>, location?: HistoryLocation) => Promise<boolean>;
type MoneyData = {
  accounts: MoneyAccount[];
  categories: MoneyCategory[];
  balances: Record<string, MoneyBalance>;
  events: MoneyEvent[];
  hasNext: boolean;
  history?: RunningHistory;
};
type MoneyView = { status: "loading" } | { status: "error"; message: string } | { status: "ready"; data: MoneyData };

function savingLabel() {
  const status = pendingFinancialWrite.status();
  return status.kind === "pending" && !status.sending ? "Save needs recovery" : "Saving…";
}

function messageOf(error: unknown): string {
  return error instanceof Error ? error.message : "Something went wrong. Please try again.";
}

function accountName(accounts: Pick<MoneyAccount, "id" | "name">[], id: string): string {
  return accounts.find((account) => account.id === id)?.name ?? "Unavailable account";
}

function categoryName(categories: Pick<MoneyCategory, "id" | "name">[], id: string): string {
  return categories.find((category) => category.id === id)?.name ?? "Unavailable category";
}

function historyTitle(event: MoneyEvent, categories: Pick<MoneyCategory, "id" | "name">[]): string {
  if (event.note?.trim()) return event.note.trim();
  if (event.kind === "transfer") return "Transfer between accounts";
  const names = event.allocations.map((allocation) => categoryName(categories, allocation.category_id));
  return names.length ? names.join(" + ") : "Uncategorized";
}

function historyAccounts(event: MoneyEvent, accounts: Pick<MoneyAccount, "id" | "name">[]): string {
  if (event.kind !== "transfer") return accountName(accounts, event.movements[0]?.account_id ?? "");
  const from = event.movements.find((movement) => BigInt(movement.amount_minor) < 0n);
  const to = event.movements.find((movement) => BigInt(movement.amount_minor) > 0n);
  return `${accountName(accounts, from?.account_id ?? "")} → ${accountName(accounts, to?.account_id ?? "")}`;
}

export function MoneyPage({ csrfToken, timeZone, onSessionExpired, onTimeZoneSaved }: { csrfToken: string; timeZone: string | null; onSessionExpired: () => void; onTimeZoneSaved?: (zone: string) => void }) {
  const financialRecovery = usePendingFinancialWrite();
  const financialBlocked = financialRecovery.kind !== "empty" && financialRecovery.kind !== "hidden";
  const [editing, setEditing] = useState<{ id: string; zone: string; location: HistoryLocation } | null>(null);
  const [entryZone, setEntryZone] = useState(timeZone);
  const [setupZone, setSetupZone] = useState(timeZone);
  const pages = useRef(new HistoryPages(PAGE_SIZE));
  const [historyChanged, setHistoryChanged] = useState(false);
  const [balanceAccount, setBalanceAccount] = useState<MoneyAccount | null>(null);
  const [balanceZone, setBalanceZone] = useState(timeZone);
  const openingDrafts = useRef<Record<string, OpeningBalanceDraft>>({});
  const [panel, setPanel] = useState<"overview" | "transactions">("transactions");
  const activePanel = useRef(panel);
  activePanel.current = panel;
  const staleLocation = useRef<HistoryLocation | null>(null);
  const panelScroll = useRef({ overview: 0, transactions: 0 });
  const [historyNotice, setHistoryNotice] = useState("");
  const [appendError, setAppendError] = useState("");
  const [appending, setAppending] = useState(false);
  const reading = useRef(false);
  const [view, setView] = useState<MoneyView>({ status: "loading" });
  const [setupAction, setSetupAction] = useState<"account" | "category">("account");
  const [setupOpen, setSetupOpen] = useState(false);
  const [transactionAction, setTransactionAction] = useState<TransactionAction>("expense");
  const [transactionOpen, setTransactionOpen] = useState(false);
  const [formSession, setFormSession] = useState(0);
  const [replacement, setReplacement] = useState<MoneyEvent | null>(null);
  const [pending, setPending] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [notice, setNotice] = useState("");
  const [operationError, setOperationError] = useState("");
  const pendingRef = useRef(false);
  const requestSequence = useRef(0);
  const sessionExpired = useRef(onSessionExpired);
  sessionExpired.current = onSessionExpired;
  const dialogRef = useRef<HTMLDialogElement>(null);
  const returnFocusRef = useRef<HTMLElement | null>(null);
  const setupDialogRef = useRef<HTMLDialogElement>(null);
  const setupReturnFocusRef = useRef<HTMLElement | null>(null);

  const loadMoney = useCallback(async (restoreBatches = 1): Promise<boolean> => {
    const request = ++requestSequence.current;
    reading.current = true;
    setAppending(false);
    setAppendError("");
    setRefreshing(true);
    try {
      const fresh = new HistoryPages(PAGE_SIZE);
      const [accountResponse, categoryResponse, historyResponse] = await Promise.all([
        listMoneyAccounts(), listMoneyCategories(), listRunningHistory(fresh.query()),
      ]);
      let history = fresh.accept(historyResponse);
      const balanceItems = await Promise.all(accountResponse.items.map((account) => getMoneyBalance(account.id)));
      // Rebuild only an already visited, bounded prefix. Never combine revisions,
      // and never download the entire owner history to find an edited record.
      for (let batch = 1; batch < Math.min(restoreBatches, RESTORE_BATCH_LIMIT) && history.has_more; batch++) {
        history = fresh.accept(await listRunningHistory(fresh.query()));
        if (request !== requestSequence.current) return false;
      }
      if (request !== requestSequence.current) return false;
      pages.current = fresh;
      setHistoryChanged(false);
      setView({
        status: "ready",
        data: {
          accounts: accountResponse.items,
          categories: categoryResponse.items,
          balances: Object.fromEntries(balanceItems.map((balance) => [balance.account_id, balance])),
          events: history.items,
          hasNext: history.has_more,
          history,
        },
      });
      setOperationError("");
      setRefreshing(false);
      return true;
    } catch (error) {
      if (request !== requestSequence.current) return false;
      if (error instanceof MoneyApiError && error.status === 401) sessionExpired.current();
      if (error instanceof MoneyApiError && error.code === "history_changed") {
        staleLocation.current ??= captureHistoryLocation();
        setHistoryChanged(true); setRefreshing(false); setOperationError(""); return false;
      }
      setView((current) => current.status === "ready" ? current : { status: "error", message: messageOf(error) });
      setRefreshing(false);
      setOperationError(messageOf(error));
      return false;
    } finally {
      if (request === requestSequence.current) reading.current = false;
    }
  }, []);

  useEffect(() => {
    void loadMoney();
    return () => { requestSequence.current += 1; };
  }, [loadMoney]);

  useEffect(() => {
    const dialog = dialogRef.current;
    if (!dialog) return;
    if (transactionOpen && !dialog.open) dialog.showModal();
    if (!transactionOpen && dialog.open) dialog.close();
  }, [transactionOpen, view.status]);

  useEffect(() => {
    const dialog = setupDialogRef.current;
    if (!dialog) return;
    if (setupOpen && !dialog.open) dialog.showModal();
    if (!setupOpen && dialog.open) dialog.close();
  }, [setupOpen, view.status]);

  function closeTransaction() {
    setTransactionOpen(false);
    setReplacement(null);
    const target = returnFocusRef.current;
    returnFocusRef.current = null;
    window.setTimeout(() => target?.focus(), 0);
  }

  function openTransaction(kind: TransactionAction = "expense", original: MoneyEvent | null = null) {
    if (!activeZone || financialBlocked) return;
    setEntryZone(activeZone);
    returnFocusRef.current = document.querySelector<HTMLElement>(".money-quick-action");
    setTransactionAction(kind);
    setReplacement(original);
    setFormSession((value) => value + 1);
    setOperationError("");
    setNotice("");
    setTransactionOpen(true);
  }

  function chooseSetup(item: "account" | "category") {
    if (financialBlocked) return;
    setupReturnFocusRef.current = dialogRef.current?.open
      ? document.querySelector<HTMLElement>(".money-quick-action")
      : document.activeElement as HTMLElement;
    returnFocusRef.current = null;
    setReplacement(null);
    setTransactionOpen(false);
    setSetupZone(activeZone);
    setSetupAction(item);
    setOperationError("");
    setSetupOpen(true);
  }

  function closeSetup() {
    if (pendingRef.current) return;
    setSetupOpen(false);
    const target = setupReturnFocusRef.current;
    setupReturnFocusRef.current = null;
    window.setTimeout(() => target?.focus(), 0);
  }

  const runMutation: RunMutation = async (successMessage, work, location) => {
    if (pendingRef.current) return false;
    pendingRef.current = true;
    setPending(true);
    setNotice("");
    setOperationError("");
    try {
      await work();
      const refreshed = await refreshHistory(location);
      setNotice(refreshed ? successMessage : "Saved, but the latest Money data could not be loaded. Check recent activity before doing this again.");
      return true;
    } catch (error) {
      const recoveryStatus = pendingFinancialWrite.status();
      if (error instanceof MoneyApiError && error.status === 401) onSessionExpired();
      if (recoveryStatus.kind === "empty" || recoveryStatus.kind === "hidden") {
        setOperationError(error instanceof FinancialRecoveryError ? error.message : error instanceof MoneyApiError && (error.status === 0 || error.status >= 500 || (error.status >= 200 && error.status < 300))
          ? "The result is uncertain. Check recent activity before trying again."
          : messageOf(error));
      }
      return false;
    } finally {
      pendingRef.current = false;
      setPending(false);
    }
  };

  async function refreshHistory(location = staleLocation.current ?? captureHistoryLocation()) {
    const loaded = await loadMoney(Math.max(1, pages.current.count));
    if (loaded) staleLocation.current = null;
    if (loaded) window.requestAnimationFrame(() => {
      if (activePanel.current !== "transactions") return;
      const primary = location.ids[0] && document.getElementById(location.ids[0]);
      if (!restoreHistoryLocation(location)) {
        document.getElementById("money-history-title")?.focus();
        setHistoryNotice("History refreshed from the latest transactions. The previous position is outside the refreshed section; load more to continue.");
      } else if (location.occurredAt && (!primary || primary.getAttribute("data-occurred-at") !== location.occurredAt)) {
        setHistoryNotice(primary ? "Transaction date changed. Showing it at its new position." : "Transaction updated. Its position changed; showing the nearest previously viewed transaction.");
      } else setHistoryNotice("");
    });
    return loaded;
  }
  async function loadMoreHistory(manual = false) {
    if (reading.current || pendingRef.current || historyChanged || !pages.current.current?.has_more) return;
    reading.current = true;
    const request = ++requestSequence.current;
    setAppending(true); setAppendError("");
    try {
      const response = await listRunningHistory(pages.current.query());
      if (request !== requestSequence.current) return;
      const history = pages.current.accept(response);
      setView((current) => current.status !== "ready" ? current : { status: "ready", data: { ...current.data, events: history.items, history, hasNext: history.has_more } });
      if (manual && response.items[0]) window.requestAnimationFrame(() => document.getElementById(`money-event-${response.items[0].id}`)?.focus());
    } catch (error) {
      if (request !== requestSequence.current) return;
      if (error instanceof MoneyApiError && error.status === 401) sessionExpired.current();
      if (error instanceof MoneyApiError && error.code === "history_changed") { staleLocation.current ??= captureHistoryLocation(); setHistoryChanged(true); }
      else setAppendError(messageOf(error));
    } finally {
      if (request === requestSequence.current) { reading.current = false; setAppending(false); }
    }
  }
  function closeEditing() {
    const id = editing?.id;
    setEditing(null);
    window.setTimeout(() => {
      const record = id ? document.getElementById(`money-event-${id}`) : null;
      if (record) record.focus({ preventScroll: true });
      else (document.getElementById("money-history-title") ?? document.getElementById("money-history-refresh"))?.focus();
    }, 0);
  }
  const data = view.status === "ready" ? view.data : null;
  const activeZone = data?.history ? data.history.time_zone : timeZone;
  useEffect(() => {
    if (activeZone && activeZone !== timeZone) onTimeZoneSaved?.(activeZone);
  }, [activeZone, timeZone, onTimeZoneSaved]);
  function switchPanel(next: "overview" | "transactions") {
    if (next === panel) return;
    panelScroll.current[panel] = window.scrollY;
    setPanel(next);
    window.requestAnimationFrame(() => window.scrollTo({ top: panelScroll.current[next], behavior: "instant" }));
  }
  function showOverview() { switchPanel("overview"); }
  const hasActiveAccount = data?.accounts.some((account) => !account.archived_at) ?? false;
  const hasExpenseCategory = data?.categories.some((category) => !category.archived_at && category.kind === "expense") ?? false;
  return (
    <div className="page-stack money-page">
      {editing && data && <EditTransactionSheet id={editing.id} csrfToken={csrfToken} timeZone={editing.zone} accounts={data.accounts} categories={data.categories} onClose={closeEditing} onSaved={() => { setEditing(null); setNotice("Transaction updated."); void refreshHistory(editing.location); }} onSessionExpired={onSessionExpired} />}
      {balanceAccount && (activeZone ? <OpeningBalanceSheet account={balanceAccount} csrfToken={csrfToken} timeZone={balanceZone!} draft={openingDrafts.current[balanceAccount.id]} onDraft={(draft) => { openingDrafts.current[balanceAccount.id] = draft; }} onClose={() => setBalanceAccount(null)} onSaved={() => { delete openingDrafts.current[balanceAccount.id]; setBalanceAccount(null); setNotice("Balance date confirmed."); void refreshHistory(); }} onRecovered={() => { setBalanceAccount(null); setNotice("Save confirmed. Showing current records."); void refreshHistory(); }} onSessionExpired={onSessionExpired} /> : <section className="money-state"><p>Confirm your timezone in Me before choosing a balance date.</p><a className="button button-primary" href="/me#timezone">Open timezone settings</a><button type="button" className="money-text-action" onClick={() => setBalanceAccount(null)}>Cancel</button></section>)}
      <header className="money-page-header">
        <h1>Money</h1>
        {data && activeZone && <div className="money-header-actions"><button type="button" className="button button-primary money-quick-action" onClick={() => openTransaction()} disabled={pending || financialBlocked}>+ Add transaction</button><a className="button button-outline money-settings-action" href="/me#timezone">Settings</a></div>}
      </header>
      <FinancialWriteRecovery csrfToken={csrfToken} onSessionExpired={onSessionExpired} onRecovered={() => { closeTransaction(); closeSetup(); setBalanceAccount(null); setNotice("Save confirmed. Showing current records."); void refreshHistory(); }} />
      {notice && <p className="money-quick-feedback" role="status">{notice}</p>}
      {historyNotice && <p className="money-help" role="status">{historyNotice}</p>}

      {operationError && <div className="money-feedback money-error money-retry-read"><p role="alert">{operationError}</p><button type="button" className="button button-outline" onClick={() => void refreshHistory()}>{financialBlocked ? "Refresh data (Save stays pending)" : "Refresh accounts and activity"}</button></div>}
      {refreshing && data && <p className="money-refreshing" role="status">Refreshing Money…</p>}

      {view.status === "loading" && <section className="money-state" role="status" aria-live="polite"><span className="loading-orb" aria-hidden="true" /><h2>Loading your Money space…</h2><p>Checking your latest accounts and activity.</p></section>}
      {view.status === "error" && <section className="money-state" role="alert"><Icon name="shield" /><h2>Money could not load.</h2><p>{view.message}</p><button className="button button-primary" type="button" onClick={() => void refreshHistory()}>Try again</button></section>}

      {data && <>
        {!activeZone && <section className="money-timezone-card"><h2>Set your transaction timezone</h2><p>Use the same dates and times on every device.</p><a className="button button-primary" href="/me#timezone">Confirm timezone in Me</a></section>}
        {activeZone && <nav className="money-view-switch" aria-label="Money views"><button type="button" aria-pressed={panel === "overview"} disabled={pending} onClick={() => switchPanel("overview")}>Overview</button><button type="button" aria-pressed={panel === "transactions"} disabled={pending} onClick={() => switchPanel("transactions")}>Transactions</button></nav>}
        {activeZone && !historyChanged && (!hasActiveAccount || (data.events.length === 0)) && <section className="money-onboarding" aria-label="Get started with Money"><p className="eyebrow">GET STARTED</p><h2>Three small steps to your first record.</h2><ol><li className={hasActiveAccount ? "is-done" : ""}><span>1. Account</span>{hasActiveAccount ? "Ready" : <button type="button" disabled={financialBlocked} onClick={() => chooseSetup("account")}>Create an account</button>}</li><li className={hasExpenseCategory ? "is-done" : ""}><span>2. Category (optional)</span>{hasExpenseCategory ? "Ready" : <button type="button" disabled={financialBlocked} onClick={() => chooseSetup("category")}>Add a category</button>}</li><li className={data.events.length > 0 ? "is-done" : ""}><span>3. First transaction</span>{data.events.length > 0 ? "Ready" : <button type="button" disabled={financialBlocked} onClick={() => openTransaction("expense")}>Record an expense</button>}</li></ol><p>Categories are optional: you can record income or expenses as Uncategorized. For existing accounts, confirm a balance date in Overview before relying on older entries. New accounts use the balance and date you confirm.</p></section>}
        <div hidden={!!activeZone && panel !== "overview"}><AccountOverview onBalanceDate={(account) => { setBalanceZone(activeZone); setBalanceAccount(account); }} data={data} csrfToken={csrfToken} pending={pending || financialBlocked} runMutation={runMutation} operationError={operationError} onAddAccount={() => chooseSetup("account")} onCategories={() => chooseSetup("category")} /></div>
        {activeZone && <div hidden={panel !== "transactions"}>{historyChanged ? <HistoryChanged onRefresh={() => void refreshHistory()} refreshing={refreshing} /> : <HistorySection data={data} timeZone={activeZone} onOverview={showOverview} onRefresh={() => void refreshHistory()} onLoadMore={(manual) => void loadMoreHistory(manual)} loadingMore={appending} loadError={appendError} autoLoad={panel === "transactions" && !editing && !transactionOpen && !setupOpen && !balanceAccount && !financialBlocked && !refreshing} csrfToken={csrfToken} pending={pending || financialBlocked || refreshing} runMutation={runMutation} onEdit={(id) => { const location = captureHistoryLocation(id); location.occurredAt = data.events.find((item) => item.id === id)?.occurred_at; setEditing({ id, zone: activeZone, location }); }} operationError={operationError} />}</div>}
        <dialog ref={setupDialogRef} className="money-transaction-dialog money-setup-dialog" aria-labelledby="money-setup-dialog-title" onCancel={(event) => { if (pending) event.preventDefault(); }} onClose={() => { if (!setupDialogRef.current?.open) closeSetup(); }} onClick={(event) => { if (event.target === setupDialogRef.current) closeSetup(); }}>
          <div className="money-dialog-content"><div className="money-dialog-head"><h2 id="money-setup-dialog-title">{setupAction === "account" ? "Add account" : "Categories"}</h2><button type="button" className="money-dialog-close" onClick={closeSetup} disabled={pending} aria-label="Close setup panel">×</button></div><FinancialWriteRecovery csrfToken={csrfToken} onSessionExpired={onSessionExpired} onRecovered={() => { closeTransaction(); closeSetup(); void refreshHistory(); }} />
            <div className="money-setup-pane" hidden={setupAction !== "account"}><AccountForm open={setupOpen && setupAction === "account"} timeZone={setupZone} csrfToken={csrfToken} pending={pending || financialBlocked} runMutation={runMutation} operationError={operationError} onSaved={closeSetup} /></div>
            <div className="money-setup-pane" hidden={setupAction !== "category"}><CategoryPanel categories={data.categories} csrfToken={csrfToken} pending={pending} runMutation={runMutation} operationError={operationError} /></div>
          </div>
        </dialog>
        {activeZone && <dialog ref={dialogRef} className="money-transaction-dialog" aria-labelledby="money-dialog-title" onClose={closeTransaction} onClick={(event) => { if (event.target === dialogRef.current) closeTransaction(); }}>
          <div className="money-dialog-content"><div className="money-dialog-head"><h2 id="money-dialog-title">{replacement ? "Review a replacement" : "Add transaction"}</h2><button type="button" className="money-dialog-close" onClick={closeTransaction} aria-label="Close transaction form">×</button></div><FinancialWriteRecovery csrfToken={csrfToken} onSessionExpired={onSessionExpired} onRecovered={() => { closeTransaction(); closeSetup(); void refreshHistory(); }} />
            {replacement && <p className="money-help">The original remains voided. Review every field and explicitly confirm the new transaction before saving.</p>}
            <div className="money-action-tabs" role="group" aria-label="Transaction type">{(["expense", "income", "transfer"] as const).map((item) => <button key={item} type="button" className={`money-action-tab${transactionAction === item ? " is-active" : ""}`} aria-pressed={transactionAction === item} disabled={pending || financialBlocked} onClick={() => { setTransactionAction(item); setReplacement(null); setOperationError(""); }}>{item[0].toUpperCase() + item.slice(1)}</button>)}</div>
            {(transactionAction === "expense" || transactionAction === "income") && <EntryForm key={`${formSession}-${transactionAction}-${replacement?.id ?? "new"}`} kind={transactionAction} timeZone={entryZone ?? activeZone!} accounts={data.accounts} categories={data.categories} csrfToken={csrfToken} pending={pending || financialBlocked} runMutation={runMutation} onChooseAction={chooseSetup} initial={replacement} operationError={operationError} onSaved={closeTransaction} />}
            {transactionAction === "transfer" && <TransferForm key={`${formSession}-${replacement?.id ?? "new"}`} timeZone={entryZone ?? activeZone!} accounts={data.accounts} csrfToken={csrfToken} pending={pending || financialBlocked} runMutation={runMutation} onChooseAction={chooseSetup} initial={replacement} operationError={operationError} onSaved={closeTransaction} />}
          </div>
        </dialog>}
      </>}
    </div>
  );
}

function AccountOverview({ onBalanceDate, data, csrfToken, pending, runMutation, operationError, onAddAccount, onCategories }: { onBalanceDate: (account: MoneyAccount) => void; data: MoneyData; csrfToken: string; pending: boolean; runMutation: RunMutation; operationError: string; onAddAccount: () => void; onCategories: () => void }) {
  const [archiveID, setArchiveID] = useState<string | null>(null);
  const [restoreID, setRestoreID] = useState<string | null>(null);
  const [localNotice, setLocalNotice] = useState("");
  const active = data.accounts.filter((account) => !account.archived_at);
  const archived = data.accounts.filter((account) => account.archived_at);
  return (
    <section className="money-section" aria-labelledby="money-accounts-title">
      <div className="section-heading money-accounts-heading"><div><p className="eyebrow">YOUR ACCOUNTS</p><h2 id="money-accounts-title" tabIndex={-1}>A clear view of each balance.</h2></div><div className="money-setup-actions"><button type="button" className="button button-outline" onClick={onCategories} disabled={pending}>Categories</button><button type="button" className="button button-outline" onClick={onAddAccount} disabled={pending}>+ Account</button></div></div>
      {localNotice && <p className="money-feedback money-success" role="status">{localNotice}</p>}
      {data.accounts.length === 0 ? <div className="money-empty"><Icon name="money" /><h3>No accounts yet</h3><p>Use + Account to begin recording your money. No balance is assumed for you.</p></div> : <>
        <div className="money-account-grid">
          {active.map((account) => <article className="money-account-card" key={account.id}>
            <div className="money-account-top"><span className="icon-tile icon-tile-money"><Icon name="money" /></span><span className="money-account-type">{account.type.replace("_", " ")}</span></div>
            <h3>{account.name}</h3>
            <p className="money-balance-label">{account.type === "credit_card" && BigInt(data.balances[account.id].amount_minor) < 0n ? "Amount owed · liability" : "Current balance"}</p>
            <p className="money-balance">{formatMinorUnits(data.balances[account.id].amount_minor, account.currency)}</p>
            {!account.opening_balance_at && <button type="button" className="money-text-action" disabled={pending} onClick={() => onBalanceDate(account)}>Confirm balance date</button>}
            {archiveID === account.id ? <div className="money-confirm" role="group" aria-label={`Archive ${account.name}`}><p>Archive this account? Its history and balance will stay available, but you cannot post new activity to it.</p>{BigInt(data.balances[account.id].amount_minor) !== 0n && <p><strong>This account has a nonzero balance of {formatMinorUnits(data.balances[account.id].amount_minor, account.currency)}.</strong> Archiving does not settle or transfer it.</p>}{operationError && <p className="money-field-error" role="alert">{operationError}</p>}<div className="money-confirm-actions"><button type="button" className="button button-outline" onClick={() => setArchiveID(null)} disabled={pending}>Keep account</button><button type="button" className="button button-danger" disabled={pending} onClick={() => void runMutation("Account archived.", () => archiveMoneyAccount(csrfToken, account.id)).then((saved) => { if (saved) { setArchiveID(null); setLocalNotice(`${account.name} archived. History and balance are preserved.`); } })}>Confirm archive</button></div></div>
              : <button className="money-text-action" type="button" onClick={() => setArchiveID(account.id)} disabled={pending}>Archive account</button>}
          </article>)}
        </div>
        {active.length === 0 && <p className="money-soft-note">All accounts are archived. Add a new account to record future activity.</p>}
        {archived.length > 0 && <details className="money-archived"><summary>Archived accounts ({archived.length})</summary><ul>{archived.map((account) => <li key={account.id}><strong>{account.name}</strong><span>{formatMinorUnits(data.balances[account.id].amount_minor, account.currency)}</span>{restoreID === account.id ? <div className="money-confirm"><p>Restore {account.name} for new transactions? Its history and balance will remain unchanged.</p>{operationError && <p className="money-field-error" role="alert">{operationError}</p>}<div className="money-confirm-actions"><button type="button" className="button button-outline" onClick={() => setRestoreID(null)} disabled={pending}>Cancel</button><button type="button" className="button button-primary" disabled={pending} onClick={() => void runMutation("Account restored.", () => restoreMoneyAccount(csrfToken, account.id)).then((saved) => { if (saved) { setRestoreID(null); setLocalNotice(`${account.name} restored. Its history and balance are unchanged.`); } })}>Confirm restore</button></div></div> : <button type="button" className="money-text-action" onClick={() => setRestoreID(account.id)} disabled={pending}>Restore account</button>}</li>)}</ul></details>}
      </>}
    </section>
  );
}

function AccountForm({ open, timeZone, csrfToken, pending, runMutation, operationError, onSaved }: { open: boolean; timeZone: string | null; csrfToken: string; pending: boolean; runMutation: RunMutation; operationError: string; onSaved: () => void }) {
  const [currentInstant, setCurrentInstant] = useState(() => new Date().toISOString());
  const [historical, setHistorical] = useState(false);
  const [when, setWhen] = useState(() => timeZone ? localDateTimeValue(new Date(), timeZone) : "");
  const [chosenInstant, setChosenInstant] = useState("");
  const [name, setName] = useState("");
  const [type, setType] = useState<AccountCreate["type"]>("bank");
  const [currency, setCurrency] = useState("MYR");
  const [opening, setOpening] = useState("0");
  const [error, setError] = useState("");
  useEffect(() => {
    if (open && !name && opening === "0" && !historical) {
      const now = new Date();
      setCurrentInstant(now.toISOString());
      if (timeZone) setWhen(localDateTimeValue(now, timeZone));
    }
  }, [open, timeZone, name, opening, historical]);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    try {
      if (!name.trim() || name.trim().length > 120) throw new Error("Enter an account name of up to 120 characters.");
      const opening_balance_minor = parseAmountToMinor(opening, currency, true);
      if (!timeZone) throw new Error("Confirm your timezone in Me first.");
      const opening_balance_at = historical ? toOccurredAt(when, timeZone, chosenInstant || undefined) : currentInstant;
      const input: AccountCreate = { name: name.trim(), type, currency, opening_balance_minor, opening_balance_at };
      if (await runMutation("Account created.", () => createMoneyAccount(csrfToken, input))) { setName(""); setOpening("0"); onSaved(); }
    } catch (cause) { setError(messageOf(cause)); }
  }
  return <form className="money-form" onSubmit={(event) => void submit(event)}>
    <div><p className="eyebrow">NEW ACCOUNT</p><h3>Add an account</h3><p>Enter its balance at the date shown. Earlier transactions stay in history without being counted twice.</p></div>
    {!timeZone && <p><a href="/me#timezone">Confirm your timezone in Me</a> before adding an account.</p>}
    <fieldset disabled={pending || !timeZone}><div className="money-field-grid">
      <label>Account name<input required maxLength={120} value={name} onChange={(event) => setName(event.target.value)} placeholder="Everyday bank" aria-invalid={!!error && !name.trim()} aria-describedby={error ? "money-account-error" : undefined} /></label>
      <label>Account type<select value={type} onChange={(event) => setType(event.target.value as AccountCreate["type"])}><option value="bank">Bank</option><option value="cash">Cash</option><option value="ewallet">E-wallet</option><option value="savings">Savings</option><option value="credit_card">Credit card</option><option value="other">Other</option></select></label>
      <label>Currency code<input required maxLength={3} pattern="[A-Z]{3}" autoCapitalize="characters" value={currency} onChange={(event) => setCurrency(event.target.value.toUpperCase())} aria-describedby="money-currency-help" /></label>
      <label>{historical ? "Balance at this date" : "Balance now"}<input required inputMode="decimal" value={opening} onChange={(event) => setOpening(event.target.value)} placeholder="0.00" aria-invalid={!!error && !!name.trim()} aria-describedby={error ? "money-account-error" : undefined} /></label>
    </div><p id="money-currency-help" className="money-help">Use an ISO code such as MYR, USD, or JPY. For a credit card, enter an amount owed as a negative balance.</p>
      {timeZone && <>
        <label className="money-choice"><input type="checkbox" checked={historical} onChange={(event) => setHistorical(event.target.checked)} />Use an earlier balance date</label>
        {historical && <DateTimeField when={when} onChange={(value) => { setWhen(value); setChosenInstant(""); }} timeZone={timeZone} chosen={chosenInstant} onChosen={setChosenInstant} invalid={false} errorID="money-account-error" />}
        <p className="money-help money-opening-review">{(() => { try { const at = historical ? toOccurredAt(when, timeZone, chosenInstant || undefined) : currentInstant; return `Confirm ${formatMinorUnits(parseAmountToMinor(opening, currency, true), currency)} on ${formatLocalDate(at, timeZone)} · ${formatLocalTimeWithZone(at, timeZone)} (${timeZone}).`; } catch { return "Choose a valid balance and date before saving."; } })()}</p>
      </>}
      {error && <p id="money-account-error" className="money-field-error" role="alert">{error}</p>}
      {operationError && <p className="money-field-error" role="alert">{operationError}</p>}
      <button className="button button-primary" type="submit" disabled={pending}>{pending ? savingLabel() : "Create account"}</button></fieldset>
  </form>;
}

function CategoryPanel({ categories, csrfToken, pending, runMutation, operationError }: { categories: MoneyCategory[]; csrfToken: string; pending: boolean; runMutation: RunMutation; operationError: string }) {
  const [editingID, setEditingID] = useState<string | null>(null);
  const [draftName, setDraftName] = useState("");
  const [confirm, setConfirm] = useState<{ id: string; action: "archive" | "restore" } | null>(null);
  const [localError, setLocalError] = useState("");
  const active = categories.filter((category) => !category.archived_at);
  const archived = categories.filter((category) => category.archived_at);
  function restoreActionFocus(id: string, action: string) {
    const target = action === "archived-summary" ? "archived-summary" : `${id}-${action}`;
    window.setTimeout(() => document.querySelector<HTMLElement>(`[data-category-action="${target}"]`)?.focus(), 0);
  }
  function cancelAction() {
    if (editingID) restoreActionFocus(editingID, "rename");
    if (confirm) restoreActionFocus(confirm.id, confirm.action);
    setEditingID(null); setConfirm(null); setLocalError("");
  }
  async function submitRename(event: FormEvent<HTMLFormElement>, category: MoneyCategory) {
    event.preventDefault();
    const name = draftName.trim();
    if (!name || name.length > 120 || name.toLowerCase() === "uncategorized") { setLocalError("Enter a distinct name of up to 120 characters; Uncategorized is reserved."); return; }
    if (name === category.name) { cancelAction(); return; }
    if (await runMutation("Category renamed. Historical transactions now show its new name.", () => renameMoneyCategory(csrfToken, category.id, { name }))) {
      setEditingID(null); setLocalError(""); restoreActionFocus(category.id, "rename");
    }
  }
  async function confirmChange(category: MoneyCategory, action: "archive" | "restore") {
    const saved = await runMutation(action === "archive" ? "Category archived. Historical transactions are unchanged." : "Category restored.",
      () => action === "archive" ? archiveMoneyCategory(csrfToken, category.id) : restoreMoneyCategory(csrfToken, category.id));
    if (saved) { setConfirm(null); restoreActionFocus(category.id, action === "archive" ? "archived-summary" : "rename"); }
  }
  function row(category: MoneyCategory) {
    return <li key={category.id} className="money-category-item">
      <div className="money-category-row"><span><strong>{category.name}</strong><span className="money-category-kind"> · {category.kind}</span></span><div className="money-category-actions"><button type="button" className="money-text-action" data-category-action={`${category.id}-rename`} disabled={pending} onClick={() => { setEditingID(category.id); setDraftName(category.name); setConfirm(null); setLocalError(""); }}>Rename</button><button type="button" className="money-text-action" data-category-action={`${category.id}-${category.archived_at ? "restore" : "archive"}`} disabled={pending} onClick={() => { setConfirm({ id: category.id, action: category.archived_at ? "restore" : "archive" }); setEditingID(null); setLocalError(""); }}>{category.archived_at ? "Restore" : "Archive"}</button></div></div>
      {editingID === category.id && <form className="money-category-edit money-confirm" onSubmit={(event) => void submitRename(event, category)}><label>New name<input autoFocus required maxLength={120} value={draftName} onChange={(event) => { setDraftName(event.target.value); setLocalError(""); }} aria-invalid={!!localError} aria-describedby={localError ? "money-category-edit-error" : undefined} /></label><p className="money-help">This keeps the same category and updates the name shown on past transactions. Its {category.kind} type cannot change.</p>{localError && <p id="money-category-edit-error" className="money-field-error" role="alert">{localError}</p>}{operationError && <p className="money-field-error" role="alert">{operationError}</p>}<div className="money-confirm-actions"><button type="button" className="button button-outline" disabled={pending} onClick={cancelAction}>Cancel</button><button type="submit" className="button button-primary" disabled={pending}>{pending ? savingLabel() : "Save name"}</button></div></form>}
      {confirm?.id === category.id && <div className="money-confirm" role="group" aria-label={`${confirm.action} ${category.name}`}><p>{confirm.action === "archive" ? `Archive ${category.name}? Past transactions and balances stay unchanged. It will no longer be available for new transactions.` : `Restore ${category.name} for new ${category.kind} transactions? Past transactions stay unchanged.`}</p>{operationError && <p className="money-field-error" role="alert">{operationError}</p>}<div className="money-confirm-actions"><button type="button" className="button button-outline" disabled={pending} onClick={cancelAction}>Cancel</button><button type="button" className={confirm.action === "archive" ? "button button-danger" : "button button-primary"} disabled={pending} onClick={() => void confirmChange(category, confirm.action)}>{pending ? savingLabel() : `Confirm ${confirm.action}`}</button></div></div>}
    </li>;
  }
  return <div className="money-category-panel">
    <div><p className="eyebrow">YOUR CATEGORIES</p><p className="money-help">Choose these when recording income or expenses.</p></div>
    {active.length === 0 ? <p className="money-soft-note">No active categories. New transactions can stay Uncategorized.</p> : <ul className="money-category-list">{active.map(row)}</ul>}
    {archived.length > 0 && <details className="money-archived money-category-archived-section"><summary data-category-action="archived-summary">Archived categories ({archived.length})</summary><ul className="money-category-list">{archived.map(row)}</ul></details>}
    <CategoryForm csrfToken={csrfToken} pending={pending} runMutation={runMutation} operationError={operationError} />
  </div>;
}

function CategoryForm({ csrfToken, pending, runMutation, operationError }: { csrfToken: string; pending: boolean; runMutation: RunMutation; operationError: string }) {
  const [name, setName] = useState("");
  const [kind, setKind] = useState<CategoryCreate["kind"]>("expense");
  const [error, setError] = useState("");
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setError("");
    if (!name.trim() || name.trim().length > 120 || name.trim().toLowerCase() === "uncategorized") { setError("Enter a category name of up to 120 characters; Uncategorized is reserved."); return; }
    if (await runMutation("Category created.", () => createMoneyCategory(csrfToken, { name: name.trim(), kind }))) setName("");
  }
  return <form className="money-form" onSubmit={(event) => void submit(event)}>
    <div><p className="eyebrow">NEW CATEGORY</p><h3>Organize activity your way.</h3><p>Income and expense categories stay separate.</p></div>
    <fieldset disabled={pending}><div className="money-field-grid"><label>Category name<input required maxLength={120} value={name} onChange={(event) => setName(event.target.value)} placeholder="Groceries" aria-invalid={!!error} aria-describedby={error ? "money-category-error" : undefined} /></label><label>Category type<select value={kind} onChange={(event) => setKind(event.target.value as CategoryCreate["kind"])}><option value="expense">Expense</option><option value="income">Income</option></select></label></div>
      {error && <p id="money-category-error" className="money-field-error" role="alert">{error}</p>}
      {operationError && <p className="money-field-error" role="alert">{operationError}</p>}
      <button className="button button-primary" type="submit" disabled={pending}>{pending ? savingLabel() : "Create category"}</button></fieldset>
  </form>;
}

function DateTimeField({ when, onChange, timeZone, chosen, onChosen, invalid, errorID }: { when: string; onChange: (value: string) => void; timeZone: string; chosen: string; onChosen: (value: string) => void; invalid: boolean; errorID: string }) {
  const matches = useMemo(() => { try { return matchingInstants(when, timeZone); } catch { return []; } }, [when, timeZone]);
  return <div className="money-datetime-field"><label>Date and time · {timeZone}<input required type="datetime-local" value={when} onChange={(event) => { onChange(event.target.value); onChosen(""); }} aria-invalid={invalid} aria-describedby={invalid ? errorID : undefined} /></label>
    {matches.length > 1 && <label>This local time occurs twice. Choose one:<select required value={chosen} onChange={(event) => onChosen(event.target.value)}><option value="">Choose the earlier or later occurrence</option>{matches.map((instant, index) => <option key={instant} value={instant}>{index === 0 ? "First" : "Second"} · {formatLocalTimeWithZone(instant, timeZone)} · {instant} UTC</option>)}</select></label>}
  </div>;
}

function replacementOccurrence(initial: MoneyEvent | null, timeZone: string): string {
  if (!initial) return "";
  const instant = new Date(initial.occurred_at);
  const wall = localDateTimeValue(instant, timeZone);
  return matchingInstants(wall, timeZone).find((candidate) => Math.abs(Date.parse(candidate) - instant.getTime()) < 60_000) ?? "";
}

function EntryForm({ kind, timeZone, accounts, categories, csrfToken, pending, runMutation, onChooseAction, initial, operationError, onSaved }: { kind: "income" | "expense"; timeZone: string; accounts: MoneyAccount[]; categories: MoneyCategory[]; csrfToken: string; pending: boolean; runMutation: RunMutation; onChooseAction: (action: "account" | "category") => void; initial: MoneyEvent | null; operationError: string; onSaved: () => void }) {
  const activeAccounts = accounts.filter((account) => !account.archived_at);
  const activeCategories = categories.filter((category) => !category.archived_at && category.kind === kind);
  const [accountID, setAccountID] = useState(initial?.movements[0]?.account_id ?? "");
  const [categoryID, setCategoryID] = useState(initial?.allocations[0]?.category_id ?? "");
  const [amount, setAmount] = useState(initial ? minorUnitsToInput(initial.amount_minor, initial.currency) : "");
  const [when, setWhen] = useState(() => localDateTimeValue(initial ? new Date(initial.occurred_at) : new Date(), timeZone));
  const [chosenInstant, setChosenInstant] = useState(() => replacementOccurrence(initial, timeZone));
  const [note, setNote] = useState(initial?.note ?? "");
  const [reviewed, setReviewed] = useState(false);
  const [error, setError] = useState<{ field: string; message: string } | null>(null);
  const selectedAccount = activeAccounts.find((account) => account.id === accountID);
  function changed() { setReviewed(false); setError(null); }
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setError(null);
    try {
      if (!selectedAccount) { setError({ field: "account", message: "Choose an active account." }); return; }
      if (categoryID && !activeCategories.some((category) => category.id === categoryID)) { setError({ field: "category", message: `Choose an active ${kind} category or Uncategorized.` }); return; }
      let amount_minor: string;
      try { amount_minor = parseAmountToMinor(amount, selectedAccount.currency); }
      catch (cause) { setError({ field: "amount", message: messageOf(cause) }); return; }
      let occurred_at: string;
      try { occurred_at = toOccurredAt(when, timeZone, chosenInstant || undefined); }
      catch (cause) { setError({ field: "when", message: messageOf(cause) }); return; }
      if (initial && !reviewed) { setReviewed(true); return; }
      const input: EventCreate = { kind, account_id: accountID, amount_minor, currency: selectedAccount.currency, occurred_at, note: note.trim(), allocations: categoryID ? [{ category_id: categoryID, amount_minor }] : [] };
      if (await runMutation(`${kind === "income" ? "Income" : "Expense"} recorded.`, () => postMoneyEvent(csrfToken, input))) onSaved();
    } catch (cause) { setError({ field: "form", message: messageOf(cause) }); }
  }
  return <form className="money-form" noValidate onSubmit={(event) => void submit(event)}>
    <div><p className="eyebrow">RECORD {kind.toUpperCase()}</p><h3>{kind === "income" ? "Money coming in." : "Money going out."}</h3><p>The selected account sets the currency. Choose a category or leave this transaction Uncategorized.</p></div>
    {activeAccounts.length === 0 && <p className="money-help">You need an active account. <button type="button" className="money-inline-action" disabled={pending} onClick={() => onChooseAction("account")}>Create an account</button>.</p>}
    {activeCategories.length === 0 && <p className="money-help">You can record this as Uncategorized, or <button type="button" className="money-inline-action" disabled={pending} onClick={() => onChooseAction("category")}>create a category</button>.</p>}
    <fieldset disabled={pending || activeAccounts.length === 0}><div className="money-field-grid">
      <label>Account<select required value={accountID} onChange={(event) => { setAccountID(event.target.value); changed(); }} aria-invalid={error?.field === "account"} aria-describedby={error?.field === "account" ? "money-entry-error" : undefined}><option value="">Choose an account</option>{activeAccounts.map((account) => <option key={account.id} value={account.id}>{account.name} · {account.currency}</option>)}</select></label>
      <label>{kind === "income" ? "Income" : "Expense"} category <span className="money-optional">optional</span><select value={categoryID} onChange={(event) => { setCategoryID(event.target.value); changed(); }} aria-invalid={error?.field === "category"} aria-describedby={error?.field === "category" ? "money-entry-error" : undefined}><option value="">Uncategorized</option>{activeCategories.map((category) => <option key={category.id} value={category.id}>{category.name}</option>)}</select></label>
      <label>Amount {selectedAccount && `(${selectedAccount.currency}, ${currencyDigits(selectedAccount.currency)} decimals)`}<input required inputMode="decimal" value={amount} onChange={(event) => { setAmount(event.target.value); changed(); }} placeholder="0.00" aria-invalid={error?.field === "amount"} aria-describedby={error?.field === "amount" ? "money-entry-error" : undefined} /></label>
      <DateTimeField when={when} onChange={(value) => { setWhen(value); changed(); }} timeZone={timeZone} chosen={chosenInstant} onChosen={(value) => { setChosenInstant(value); changed(); }} invalid={error?.field === "when"} errorID="money-entry-error" />
    </div><label>Note <span className="money-optional">optional</span><textarea maxLength={1000} rows={3} value={note} onChange={(event) => { setNote(event.target.value); changed(); }} placeholder="A detail to remember" /></label>
      {error && <p id="money-entry-error" className="money-field-error" role="alert">{error.message}</p>}
      {operationError && <p className="money-field-error" role="alert">{operationError}</p>}
      {initial && reviewed && <p className="money-confirm" role="status">Review complete. Confirm to create a new record; the voided original will remain in history.</p>}
      <button className="button button-primary" type="submit" disabled={pending || activeAccounts.length === 0}>{pending ? savingLabel() : initial ? reviewed ? "Confirm replacement" : "Review replacement" : `Record ${kind}`}</button></fieldset>
  </form>;
}

function TransferForm({ timeZone, accounts, csrfToken, pending, runMutation, onChooseAction, initial, operationError, onSaved }: { timeZone: string; accounts: MoneyAccount[]; csrfToken: string; pending: boolean; runMutation: RunMutation; onChooseAction: (action: "account" | "category") => void; initial: MoneyEvent | null; operationError: string; onSaved: () => void }) {
  const active = accounts.filter((account) => !account.archived_at);
  const [fromID, setFromID] = useState(initial?.movements.find((movement) => BigInt(movement.amount_minor) < 0n)?.account_id ?? "");
  const source = active.find((account) => account.id === fromID);
  const destinations = active.filter((account) => account.id !== fromID && account.currency === source?.currency);
  const [toID, setToID] = useState(initial?.movements.find((movement) => BigInt(movement.amount_minor) > 0n)?.account_id ?? "");
  const destinationID = destinations.some((account) => account.id === toID) ? toID : "";
  const [amount, setAmount] = useState(initial ? minorUnitsToInput(initial.amount_minor, initial.currency) : "");
  const [when, setWhen] = useState(() => localDateTimeValue(initial ? new Date(initial.occurred_at) : new Date(), timeZone));
  const [chosenInstant, setChosenInstant] = useState(() => replacementOccurrence(initial, timeZone));
  const [note, setNote] = useState(initial?.note ?? "");
  const [reviewed, setReviewed] = useState(false);
  const [error, setError] = useState<{ field: string; message: string } | null>(null);
  function changed() { setReviewed(false); setError(null); }
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setError(null);
    try {
      if (!source) { setError({ field: "from", message: "Choose a source account." }); return; }
      if (!destinationID || destinationID === fromID) { setError({ field: "to", message: "Choose a different active account in the same currency." }); return; }
      let amount_minor: string;
      try { amount_minor = parseAmountToMinor(amount, source.currency); }
      catch (cause) { setError({ field: "amount", message: messageOf(cause) }); return; }
      let occurred_at: string;
      try { occurred_at = toOccurredAt(when, timeZone, chosenInstant || undefined); }
      catch (cause) { setError({ field: "when", message: messageOf(cause) }); return; }
      if (initial && !reviewed) { setReviewed(true); return; }
      const input: TransferCreate = { from_account_id: fromID, to_account_id: destinationID, amount_minor, currency: source.currency, occurred_at, note: note.trim() };
      if (await runMutation("Transfer recorded. It is not counted as spending.", () => postMoneyTransfer(csrfToken, input))) onSaved();
    } catch (cause) { setError({ field: "form", message: messageOf(cause) }); }
  }
  return <form className="money-form" noValidate onSubmit={(event) => void submit(event)}>
    <div><p className="eyebrow">TRANSFER</p><h3>Move money between accounts.</h3><p>A transfer changes two balances but does not create income or spending.</p></div>
    {active.length < 2 && <p className="money-help">You need two active accounts. <button type="button" className="money-inline-action" disabled={pending} onClick={() => onChooseAction("account")}>Create an account</button>.</p>}
    {source && destinations.length === 0 && <p className="money-help">Add another account in {source.currency} to make a transfer.</p>}
    <fieldset disabled={pending || active.length < 2}><div className="money-field-grid">
      <label>From account<select required value={fromID} onChange={(event) => { setFromID(event.target.value); setToID(""); changed(); }} aria-invalid={error?.field === "from"} aria-describedby={error?.field === "from" ? "money-transfer-error" : undefined}><option value="">Choose source</option>{active.map((account) => <option key={account.id} value={account.id}>{account.name} · {account.currency}</option>)}</select></label>
      <label>To account<select required value={destinationID} onChange={(event) => { setToID(event.target.value); changed(); }} aria-invalid={error?.field === "to"} aria-describedby={error?.field === "to" ? "money-transfer-error" : undefined}><option value="">Choose destination</option>{destinations.map((account) => <option key={account.id} value={account.id}>{account.name} · {account.currency}</option>)}</select></label>
      <label>Amount {source && `(${source.currency})`}<input required inputMode="decimal" value={amount} onChange={(event) => { setAmount(event.target.value); changed(); }} placeholder="0.00" aria-invalid={error?.field === "amount"} aria-describedby={error?.field === "amount" ? "money-transfer-error" : undefined} /></label>
      <DateTimeField when={when} onChange={(value) => { setWhen(value); changed(); }} timeZone={timeZone} chosen={chosenInstant} onChosen={(value) => { setChosenInstant(value); changed(); }} invalid={error?.field === "when"} errorID="money-transfer-error" />
    </div><label>Note <span className="money-optional">optional</span><textarea maxLength={1000} rows={3} value={note} onChange={(event) => { setNote(event.target.value); changed(); }} placeholder="Why this money moved" /></label>
      {error && <p id="money-transfer-error" className="money-field-error" role="alert">{error.message}</p>}
      {operationError && <p className="money-field-error" role="alert">{operationError}</p>}
      {initial && reviewed && <p className="money-confirm" role="status">Review complete. Confirm to create a new transfer; the voided original will remain in history.</p>}
      <button className="button button-primary" type="submit" disabled={pending || active.length < 2}>{pending ? savingLabel() : initial ? reviewed ? "Confirm replacement" : "Review replacement" : "Record transfer"}</button></fieldset>
  </form>;
}

export function HistorySection({ data, timeZone, onLoadMore, onRefresh, loadingMore = false, loadError = "", autoLoad = false, csrfToken, pending, runMutation, onEdit, operationError, onOverview }: { onOverview?: () => void; data: MoneyData; timeZone: string; onLoadMore?: (manual?: boolean) => void; onRefresh?: () => void; loadingMore?: boolean; loadError?: string; autoLoad?: boolean; csrfToken: string; pending: boolean; runMutation: RunMutation; onEdit: (id: string) => void; operationError: string }) {
  const accounts = data.history?.accounts ?? data.accounts;
  const categories = data.history?.categories ?? data.categories;
  const positions = new Map(data.history?.items.map((event) => [event.id, event.account_balances]));
  const legacy = [...positions.values()].some((values) => values.some((value) => value.unavailable_reason === "anchor_required"));
  const reconstructed = [...positions.values()].some((values) => values.some((value) => value.basis === "reconstructed_pre_anchor"));
  const [deleteID, setDeleteID] = useState<string | null>(null);
  const sentinel = useRef<HTMLDivElement>(null);
  useEffect(() => {
    if (!autoLoad || pending || loadingMore || loadError || !data.hasNext || !onLoadMore || !sentinel.current || typeof IntersectionObserver === "undefined") return;
    const observer = new IntersectionObserver((entries) => {
      if (entries.some((entry) => entry.isIntersecting)) onLoadMore();
    }, { rootMargin: "400px 0px" });
    observer.observe(sentinel.current);
    return () => observer.disconnect();
  }, [autoLoad, pending, loadingMore, loadError, data.hasNext, onLoadMore]);
  useEffect(() => {
    if (deleteID) document.getElementById(`money-delete-cancel-${deleteID}`)?.focus();
  }, [deleteID]);
  async function confirmDelete(event: MoneyEvent) {
    if (await runMutation("Transaction deleted from LifeOS views.", () => deleteMoneyEvent(csrfToken, event.id, event.event_revision), captureHistoryLocation(event.id))) {
      setDeleteID(null);
    }
  }
  function cancelDelete(eventID: string) {
    setDeleteID(null);
    window.setTimeout(() => document.getElementById(`money-delete-${eventID}`)?.focus(), 0);
  }
  return <section className="money-section" aria-labelledby="money-history-title">
    <h2 id="money-history-title" className="money-screen-reader-only" tabIndex={-1}>Transactions</h2>
    {legacy && onOverview && <button type="button" className="money-text-action money-history-balance-link" onClick={onOverview}>Confirm balance dates in Overview</button>}
    {reconstructed && <details className="money-history-explanation"><summary>About balance dates</summary><p>Before a confirmed balance date, open a transaction to see its reconstructed balance. These are calculated from recorded transactions, not verified bank balances.</p></details>}
    {data.events.length === 0 ? <div className="money-empty"><Icon name="today" /><h3>No transactions yet</h3><p>Recorded income, expenses, and transfers will appear here.</p></div> : <ol className="money-history-list">{data.events.map((event, index) => <Fragment key={event.id}>
      {(index === 0 || localDateKey(data.events[index - 1].occurred_at, timeZone) !== localDateKey(event.occurred_at, timeZone)) && <li className="money-date-heading"><h3>{formatLocalDate(event.occurred_at, timeZone)}</h3></li>}
      <li className="money-history-item"><details className="money-event-details"><summary data-occurred-at={event.occurred_at} id={`money-event-${event.id}`} aria-describedby={positions.has(event.id) ? `money-balances-${event.id}` : undefined} aria-label={`View details for ${historyTitle(event, categories)}`}>
      <div className="money-history-main"><div className="money-history-copy"><strong>{historyTitle(event, categories)} <span className="money-row-chevron" aria-hidden="true">▾</span></strong><span className="money-history-meta">{event.kind === "transfer" ? "Transfer" : event.allocations.length === 0 ? "Uncategorized" : event.allocations.map((allocation) => categoryName(categories, allocation.category_id)).join(" + ")} · {positions.has(event.id) ? null : `${historyAccounts(event, accounts)} · `}{formatLocalTime(event.occurred_at, timeZone)}{event.status === "voided" ? " · Voided" : ""}</span></div><strong className={`money-history-amount${event.status === "voided" ? " is-voided" : ""}`}>{event.kind === "expense" ? "−" : event.kind === "income" ? "+" : "↔ "}{formatMinorUnits(event.amount_minor, event.currency)}</strong></div>{positions.get(event.id)?.length ? <ul id={`money-balances-${event.id}`} className="money-history-positions" aria-label="Account balances after transaction">{positions.get(event.id)!.map((position) => <HistoryBalanceLine key={position.movement_id} position={position} name={accountName(accounts, position.account_id)} />)}</ul> : null}</summary>
        {event.note && <p><strong>Note:</strong> {event.note}</p>}
        <ReconstructedDetails positions={positions.get(event.id) ?? []} accounts={data.history?.accounts ?? []} timeZone={timeZone} />
        <p><strong>Account movements</strong></p><ul>{event.movements.map((movement) => <li key={movement.id}>{accountName(accounts, movement.account_id)}: {formatMinorUnits(movement.amount_minor, movement.currency)}</li>)}</ul>
        {event.kind !== "transfer" && (event.allocations.length > 0 ? <><p><strong>Categories</strong></p><ul>{event.allocations.map((allocation) => <li key={allocation.id}>{categoryName(categories, allocation.category_id)}: {formatMinorUnits(allocation.amount_minor, allocation.currency)}</li>)}</ul></> : <p><strong>Category:</strong> Uncategorized</p>)}
        {event.status === "voided" && <p><strong>Void reason:</strong> {event.void_reason === AUTO_VOID_REASON ? "N/A" : event.void_reason || "Unavailable"}</p>}
        <div className="money-event-actions">
          {event.status === "posted" && <button type="button" className="button button-primary" aria-label="Edit transaction" disabled={pending} onClick={() => onEdit(event.id)}>Edit</button>}
          {deleteID !== event.id && <button id={`money-delete-${event.id}`} type="button" className="money-text-action money-delete-action" aria-label="Delete transaction" disabled={pending} onClick={() => setDeleteID(event.id)}>Delete</button>}
        </div>
        {deleteID === event.id ? <div className="money-confirm" role="group" aria-label="Confirm transaction deletion"><p>Delete this transaction? It will disappear from LifeOS. If posted, it will stop affecting balances and spending. Its data may remain in internal records and backups. You cannot undo this in the app.</p>{operationError && <p className="money-field-error" role="alert">{operationError}</p>}<div className="money-confirm-actions"><button id={`money-delete-cancel-${event.id}`} type="button" className="button button-outline" disabled={pending} onClick={() => cancelDelete(event.id)}>Keep transaction</button><button type="button" className="button button-danger" disabled={pending} onClick={() => void confirmDelete(event)}>Confirm delete</button></div></div>
          : null}
      </details>
    </li></Fragment>)}</ol>}
    <div ref={sentinel} className="money-history-load" aria-busy={loadingMore}>
      {loadError && <><p className="money-field-error" role="alert">Could not load older transactions. {loadError}</p>{onRefresh && <button type="button" className="money-text-action" disabled={pending || loadingMore} onClick={onRefresh}>Refresh history</button>}</>}
      {data.hasNext ? <button type="button" className="button button-outline" disabled={pending || loadingMore} onClick={() => onLoadMore?.(true)}>{loadingMore ? "Loading older transactions…" : loadError ? "Retry loading" : "Load more"}</button> : data.events.length > 0 ? <p className="money-help" role="status">All transactions loaded.</p> : null}
      <span className="money-screen-reader-only" role="status">{loadingMore ? "Loading older transactions." : `${data.events.length} transactions loaded.`}</span>
    </div>
  </section>;
}

export function HistoryChanged({ onRefresh, refreshing }: { onRefresh: () => void; refreshing: boolean }) {
  return <section className="money-state money-history-changed" role="status"><h2>History changed</h2><p>Refresh to see the latest transactions. Your open drafts and any pending Save are kept.</p><button id="money-history-refresh" type="button" className="button button-primary" disabled={refreshing} onClick={onRefresh}>Refresh history</button></section>;
}
export function HistoryBalanceLine({ position, name }: { position: HistoryPosition; name: string }) {
  const reason = position.unavailable_reason;
  const label = reason === "voided" ? "Voided: no balance effect" : reason === "anchor_required" ? "Confirm balance date in Overview" : position.basis === "reconstructed_pre_anchor" ? "Before balance date" : reason === "amount_out_of_range" ? "Balance outside supported range" : position.balance_after_minor === null ? "Balance unavailable" : `After ${formatMinorUnits(position.balance_after_minor, position.currency)}`;
  return <li><span className="money-history-position-name">{name}</span><span className="money-history-position-value">{label}</span></li>;
}

export function ReconstructedDetails({ positions, accounts, timeZone }: { positions: HistoryPosition[]; accounts: RunningHistory["accounts"]; timeZone: string }) {
  const reconstructed = positions.filter((position) => position.basis === "reconstructed_pre_anchor" && position.unavailable_reason !== "voided");
  if (!reconstructed.length) return null;
  return <section className="money-reconstructed-details" aria-label="Reconstructed balances"><p><strong>Reconstructed balances · Not bank verified</strong></p><p className="money-help">Calculated backwards from a later confirmed balance using recorded transactions. Older history may be incomplete.</p><ul>{reconstructed.map((position) => {
    const account = accounts.find((item) => item.id === position.account_id);
    return <li key={position.movement_id}><strong>{account?.name ?? "Unavailable account"}</strong>: {position.balance_after_minor !== null ? `after ${formatMinorUnits(position.balance_after_minor, position.currency)}` : "Balance outside supported range"}{account?.opening_balance_at && <span className="money-help"> · Balance date: {formatLocalDate(account.opening_balance_at, timeZone)}, {formatLocalTimeWithZone(account.opening_balance_at, timeZone)}</span>}</li>;
  })}</ul></section>;
}
