import { useCallback, useEffect, useRef, useState, type FormEvent } from "react";

import {
  archiveMoneyAccount, archiveMoneyCategory, createMoneyAccount, createMoneyCategory, getMoneyBalance,
  listMoneyAccounts, listMoneyCategories, listMoneyHistory, MoneyApiError,
  postMoneyEvent, postMoneyTransfer, renameMoneyCategory, restoreMoneyAccount, restoreMoneyCategory, voidMoneyEvent,
  type AccountCreate, type CategoryCreate, type EventCreate, type MoneyAccount,
  type MoneyBalance, type MoneyCategory, type MoneyEvent, type TransferCreate,
} from "./api/money";
import { Icon } from "./Icons";
import { currencyDigits, formatMinorUnits, localDateTimeValue, minorUnitsToInput, parseAmountToMinor, toOccurredAt } from "./moneyAmount";
import "./money.css";

const PAGE_SIZE = 10;
const AUTO_VOID_REASON = "No reason provided by owner";
type TransactionAction = "income" | "expense" | "transfer";
type RunMutation = (message: string, action: () => Promise<unknown>) => Promise<boolean>;
type MoneyData = {
  accounts: MoneyAccount[];
  categories: MoneyCategory[];
  balances: Record<string, MoneyBalance>;
  events: MoneyEvent[];
  hasNext: boolean;
};
type MoneyView = { status: "loading" } | { status: "error"; message: string } | { status: "ready"; data: MoneyData };

function messageOf(error: unknown): string {
  return error instanceof Error ? error.message : "Something went wrong. Please try again.";
}

function accountName(accounts: MoneyAccount[], id: string): string {
  return accounts.find((account) => account.id === id)?.name ?? "Unavailable account";
}

function categoryName(categories: MoneyCategory[], id: string): string {
  return categories.find((category) => category.id === id)?.name ?? "Unavailable category";
}

function historyTitle(event: MoneyEvent, categories: MoneyCategory[]): string {
  if (event.note?.trim()) return event.note.trim();
  if (event.kind === "transfer") return "Transfer between accounts";
  const names = event.allocations.map((allocation) => categoryName(categories, allocation.category_id));
  return names.length ? names.join(" + ") : "Uncategorized";
}

function historyAccounts(event: MoneyEvent, accounts: MoneyAccount[]): string {
  if (event.kind !== "transfer") return accountName(accounts, event.movements[0]?.account_id ?? "");
  const from = event.movements.find((movement) => BigInt(movement.amount_minor) < 0n);
  const to = event.movements.find((movement) => BigInt(movement.amount_minor) > 0n);
  return `${accountName(accounts, from?.account_id ?? "")} → ${accountName(accounts, to?.account_id ?? "")}`;
}

export function MoneyPage({ csrfToken, onSessionExpired }: { csrfToken: string; onSessionExpired: () => void }) {
  const [page, setPage] = useState(0);
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
  const dialogRef = useRef<HTMLDialogElement>(null);
  const returnFocusRef = useRef<HTMLElement | null>(null);
  const setupDialogRef = useRef<HTMLDialogElement>(null);
  const setupReturnFocusRef = useRef<HTMLElement | null>(null);

  const loadMoney = useCallback(async (wantedPage: number): Promise<boolean> => {
    const request = ++requestSequence.current;
    setRefreshing(true);
    try {
      const [accountResponse, categoryResponse, historyResponse] = await Promise.all([
        listMoneyAccounts(), listMoneyCategories(), listMoneyHistory(PAGE_SIZE + 1, wantedPage * PAGE_SIZE),
      ]);
      const balanceItems = await Promise.all(accountResponse.items.map((account) => getMoneyBalance(account.id)));
      if (request !== requestSequence.current) return false;
      setView({
        status: "ready",
        data: {
          accounts: accountResponse.items,
          categories: categoryResponse.items,
          balances: Object.fromEntries(balanceItems.map((balance) => [balance.account_id, balance])),
          events: historyResponse.items.slice(0, PAGE_SIZE),
          hasNext: historyResponse.items.length > PAGE_SIZE,
        },
      });
      setOperationError("");
      setRefreshing(false);
      return true;
    } catch (error) {
      if (request !== requestSequence.current) return false;
      if (error instanceof MoneyApiError && error.status === 401) onSessionExpired();
      setView((current) => current.status === "ready" ? current : { status: "error", message: messageOf(error) });
      setRefreshing(false);
      setOperationError(messageOf(error));
      return false;
    }
  }, [onSessionExpired]);

  useEffect(() => {
    void loadMoney(page);
    return () => { requestSequence.current += 1; };
  }, [loadMoney, page]);

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
    returnFocusRef.current = document.querySelector<HTMLElement>(".money-quick-action");
    setTransactionAction(kind);
    setReplacement(original);
    setFormSession((value) => value + 1);
    setOperationError("");
    setNotice("");
    setTransactionOpen(true);
  }

  function chooseSetup(item: "account" | "category") {
    setupReturnFocusRef.current = dialogRef.current?.open
      ? document.querySelector<HTMLElement>(".money-quick-action")
      : document.activeElement as HTMLElement;
    returnFocusRef.current = null;
    setReplacement(null);
    setTransactionOpen(false);
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

  const runMutation: RunMutation = async (successMessage, work) => {
    if (pendingRef.current) return false;
    pendingRef.current = true;
    setPending(true);
    setNotice("");
    setOperationError("");
    try {
      await work();
      const refreshed = await loadMoney(page);
      setNotice(refreshed ? successMessage : "Saved, but the latest Money data could not be loaded. Check recent activity before doing this again.");
      return true;
    } catch (error) {
      if (error instanceof MoneyApiError && error.status === 401) onSessionExpired();
      setOperationError(error instanceof MoneyApiError && (error.status === 0 || error.status >= 500 || (error.status >= 200 && error.status < 300))
        ? "The result is uncertain. This change may have saved. Check recent activity before trying again."
        : messageOf(error));
      return false;
    } finally {
      pendingRef.current = false;
      setPending(false);
    }
  };

  const data = view.status === "ready" ? view.data : null;
  const hasActiveAccount = data?.accounts.some((account) => !account.archived_at) ?? false;
  const hasExpenseCategory = data?.categories.some((category) => !category.archived_at && category.kind === "expense") ?? false;
  return (
    <div className="page-stack money-page">
      <header className="page-heading">
        <p className="eyebrow">MONEY · YOUR OWN RECORDS</p>
        <h1>Clarity for the everyday.</h1>
        <p>Accounts, activity, and the money that moves between them. Each amount keeps its own currency.</p>
      </header>

      {data && <div className="money-quick-row"><button type="button" className="button button-primary money-quick-action" onClick={() => openTransaction()} disabled={pending}>+ Add transaction</button>{notice ? <p className="money-quick-feedback" role="status">{notice}</p> : <span>Expense, income, or transfer</span>}</div>}

      {operationError && <div className="money-feedback money-error money-retry-read"><p role="alert">{operationError}</p><button type="button" className="button button-outline" onClick={() => void loadMoney(page)}>Refresh accounts and activity</button></div>}
      {refreshing && data && <p className="money-refreshing" role="status">Refreshing Money…</p>}

      {view.status === "loading" && <section className="money-state" role="status" aria-live="polite"><span className="loading-orb" aria-hidden="true" /><h2>Loading your Money space…</h2><p>Checking your latest accounts and activity.</p></section>}
      {view.status === "error" && <section className="money-state" role="alert"><Icon name="shield" /><h2>Money could not load.</h2><p>{view.message}</p><button className="button button-primary" type="button" onClick={() => void loadMoney(page)}>Try again</button></section>}

      {data && <>
        {(!hasActiveAccount || (page === 0 && data.events.length === 0)) && <section className="money-onboarding" aria-label="Get started with Money"><p className="eyebrow">GET STARTED</p><h2>Three small steps to your first record.</h2><ol><li className={hasActiveAccount ? "is-done" : ""}><span>1. Account</span>{hasActiveAccount ? "Ready" : <button type="button" onClick={() => chooseSetup("account")}>Create an account</button>}</li><li className={hasExpenseCategory ? "is-done" : ""}><span>2. Category (optional)</span>{hasExpenseCategory ? "Ready" : <button type="button" onClick={() => chooseSetup("category")}>Add a category</button>}</li><li className={data.events.length > 0 ? "is-done" : ""}><span>3. First transaction</span>{data.events.length > 0 ? "Ready" : <button type="button" onClick={() => openTransaction("expense")}>Record an expense</button>}</li></ol><p>Categories are optional: you can record income or expenses as Uncategorized. Opening balance is the amount at the point you start tracking; older entries can distort the current balance until dated opening balances are supported.</p></section>}
        <AccountOverview data={data} csrfToken={csrfToken} pending={pending} runMutation={runMutation} operationError={operationError} onAddAccount={() => chooseSetup("account")} onCategories={() => chooseSetup("category")} />
        <HistorySection data={data} page={page} setPage={setPage} csrfToken={csrfToken} pending={pending} runMutation={runMutation} onReplace={openTransaction} operationError={operationError} />
        <dialog ref={setupDialogRef} className="money-transaction-dialog money-setup-dialog" aria-labelledby="money-setup-dialog-title" onCancel={(event) => { if (pending) event.preventDefault(); }} onClose={() => { if (!setupDialogRef.current?.open) closeSetup(); }} onClick={(event) => { if (event.target === setupDialogRef.current) closeSetup(); }}>
          <div className="money-dialog-content"><div className="money-dialog-head"><h2 id="money-setup-dialog-title">{setupAction === "account" ? "Add account" : "Categories"}</h2><button type="button" className="money-dialog-close" onClick={closeSetup} disabled={pending} aria-label="Close setup panel">×</button></div>
            <div className="money-setup-pane" hidden={setupAction !== "account"}><AccountForm csrfToken={csrfToken} pending={pending} runMutation={runMutation} operationError={operationError} onSaved={closeSetup} /></div>
            <div className="money-setup-pane" hidden={setupAction !== "category"}><CategoryPanel categories={data.categories} csrfToken={csrfToken} pending={pending} runMutation={runMutation} operationError={operationError} /></div>
          </div>
        </dialog>
        <dialog ref={dialogRef} className="money-transaction-dialog" aria-labelledby="money-dialog-title" onClose={closeTransaction} onClick={(event) => { if (event.target === dialogRef.current) closeTransaction(); }}>
          <div className="money-dialog-content"><div className="money-dialog-head"><h2 id="money-dialog-title">{replacement ? "Review a replacement" : "Add transaction"}</h2><button type="button" className="money-dialog-close" onClick={closeTransaction} aria-label="Close transaction form">×</button></div>
            {replacement && <p className="money-help">The original remains voided. Review every field and explicitly confirm the new transaction before saving.</p>}
            <div className="money-action-tabs" role="group" aria-label="Transaction type">{(["expense", "income", "transfer"] as const).map((item) => <button key={item} type="button" className={`money-action-tab${transactionAction === item ? " is-active" : ""}`} aria-pressed={transactionAction === item} disabled={pending} onClick={() => { setTransactionAction(item); setReplacement(null); setOperationError(""); }}>{item[0].toUpperCase() + item.slice(1)}</button>)}</div>
            {(transactionAction === "expense" || transactionAction === "income") && <EntryForm key={`${formSession}-${transactionAction}-${replacement?.id ?? "new"}`} kind={transactionAction} accounts={data.accounts} categories={data.categories} csrfToken={csrfToken} pending={pending} runMutation={runMutation} onChooseAction={chooseSetup} initial={replacement} operationError={operationError} onSaved={closeTransaction} />}
            {transactionAction === "transfer" && <TransferForm key={`${formSession}-${replacement?.id ?? "new"}`} accounts={data.accounts} csrfToken={csrfToken} pending={pending} runMutation={runMutation} onChooseAction={chooseSetup} initial={replacement} operationError={operationError} onSaved={closeTransaction} />}
          </div>
        </dialog>
      </>}
    </div>
  );
}

function AccountOverview({ data, csrfToken, pending, runMutation, operationError, onAddAccount, onCategories }: { data: MoneyData; csrfToken: string; pending: boolean; runMutation: RunMutation; operationError: string; onAddAccount: () => void; onCategories: () => void }) {
  const [archiveID, setArchiveID] = useState<string | null>(null);
  const [restoreID, setRestoreID] = useState<string | null>(null);
  const [localNotice, setLocalNotice] = useState("");
  const active = data.accounts.filter((account) => !account.archived_at);
  const archived = data.accounts.filter((account) => account.archived_at);
  return (
    <section className="money-section" aria-labelledby="money-accounts-title">
      <div className="section-heading money-accounts-heading"><div><p className="eyebrow">YOUR ACCOUNTS</p><h2 id="money-accounts-title">A clear view of each balance.</h2></div><div className="money-setup-actions"><button type="button" className="button button-outline" onClick={onCategories} disabled={pending}>Categories</button><button type="button" className="button button-outline" onClick={onAddAccount} disabled={pending}>+ Account</button></div></div>
      {localNotice && <p className="money-feedback money-success" role="status">{localNotice}</p>}
      {data.accounts.length === 0 ? <div className="money-empty"><Icon name="money" /><h3>No accounts yet</h3><p>Use + Account to begin recording your money. No balance is assumed for you.</p></div> : <>
        <div className="money-account-grid">
          {active.map((account) => <article className="money-account-card" key={account.id}>
            <div className="money-account-top"><span className="icon-tile icon-tile-money"><Icon name="money" /></span><span className="money-account-type">{account.type.replace("_", " ")}</span></div>
            <h3>{account.name}</h3>
            <p className="money-balance-label">{account.type === "credit_card" && BigInt(data.balances[account.id].amount_minor) < 0n ? "Amount owed · liability" : "Current balance"}</p>
            <p className="money-balance">{formatMinorUnits(data.balances[account.id].amount_minor, account.currency)}</p>
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

function AccountForm({ csrfToken, pending, runMutation, operationError, onSaved }: { csrfToken: string; pending: boolean; runMutation: RunMutation; operationError: string; onSaved: () => void }) {
  const [name, setName] = useState("");
  const [type, setType] = useState<AccountCreate["type"]>("bank");
  const [currency, setCurrency] = useState("MYR");
  const [opening, setOpening] = useState("0");
  const [error, setError] = useState("");
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError("");
    try {
      if (!name.trim() || name.trim().length > 120) throw new Error("Enter an account name of up to 120 characters.");
      const opening_balance_minor = parseAmountToMinor(opening, currency, true);
      const input: AccountCreate = { name: name.trim(), type, currency, opening_balance_minor };
      if (await runMutation("Account created.", () => createMoneyAccount(csrfToken, input))) { setName(""); setOpening("0"); onSaved(); }
    } catch (cause) { setError(messageOf(cause)); }
  }
  return <form className="money-form" onSubmit={(event) => void submit(event)}>
    <div><p className="eyebrow">NEW ACCOUNT</p><h3>Give your money a home.</h3><p>Opening balance is the amount at the point you start tracking. Entering older transactions later can distort your current balance until dated opening balances are supported.</p></div>
    <fieldset disabled={pending}><div className="money-field-grid">
      <label>Account name<input required maxLength={120} value={name} onChange={(event) => setName(event.target.value)} placeholder="Everyday bank" aria-invalid={!!error && !name.trim()} aria-describedby={error ? "money-account-error" : undefined} /></label>
      <label>Account type<select value={type} onChange={(event) => setType(event.target.value as AccountCreate["type"])}><option value="bank">Bank</option><option value="cash">Cash</option><option value="ewallet">E-wallet</option><option value="savings">Savings</option><option value="credit_card">Credit card</option><option value="other">Other</option></select></label>
      <label>Currency code<input required maxLength={3} pattern="[A-Z]{3}" autoCapitalize="characters" value={currency} onChange={(event) => setCurrency(event.target.value.toUpperCase())} aria-describedby="money-currency-help" /></label>
      <label>Opening balance<input required inputMode="decimal" value={opening} onChange={(event) => setOpening(event.target.value)} placeholder="0.00" aria-invalid={!!error && !!name.trim()} aria-describedby={error ? "money-account-error" : undefined} /></label>
    </div><p id="money-currency-help" className="money-help">Use an ISO code such as MYR, USD, or JPY. For a credit card, enter an amount owed as a negative opening balance.</p>
      {error && <p id="money-account-error" className="money-field-error" role="alert">{error}</p>}
      {operationError && <p className="money-field-error" role="alert">{operationError}</p>}
      <button className="button button-primary" type="submit" disabled={pending}>{pending ? "Saving…" : "Create account"}</button></fieldset>
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
      {editingID === category.id && <form className="money-category-edit money-confirm" onSubmit={(event) => void submitRename(event, category)}><label>New name<input autoFocus required maxLength={120} value={draftName} onChange={(event) => { setDraftName(event.target.value); setLocalError(""); }} aria-invalid={!!localError} aria-describedby={localError ? "money-category-edit-error" : undefined} /></label><p className="money-help">This keeps the same category and updates the name shown on past transactions. Its {category.kind} type cannot change.</p>{localError && <p id="money-category-edit-error" className="money-field-error" role="alert">{localError}</p>}{operationError && <p className="money-field-error" role="alert">{operationError}</p>}<div className="money-confirm-actions"><button type="button" className="button button-outline" disabled={pending} onClick={cancelAction}>Cancel</button><button type="submit" className="button button-primary" disabled={pending}>{pending ? "Saving…" : "Save name"}</button></div></form>}
      {confirm?.id === category.id && <div className="money-confirm" role="group" aria-label={`${confirm.action} ${category.name}`}><p>{confirm.action === "archive" ? `Archive ${category.name}? Past transactions and balances stay unchanged. It will no longer be available for new transactions.` : `Restore ${category.name} for new ${category.kind} transactions? Past transactions stay unchanged.`}</p>{operationError && <p className="money-field-error" role="alert">{operationError}</p>}<div className="money-confirm-actions"><button type="button" className="button button-outline" disabled={pending} onClick={cancelAction}>Cancel</button><button type="button" className={confirm.action === "archive" ? "button button-danger" : "button button-primary"} disabled={pending} onClick={() => void confirmChange(category, confirm.action)}>{pending ? "Saving…" : `Confirm ${confirm.action}`}</button></div></div>}
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
      <button className="button button-primary" type="submit" disabled={pending}>{pending ? "Saving…" : "Create category"}</button></fieldset>
  </form>;
}

function EntryForm({ kind, accounts, categories, csrfToken, pending, runMutation, onChooseAction, initial, operationError, onSaved }: { kind: "income" | "expense"; accounts: MoneyAccount[]; categories: MoneyCategory[]; csrfToken: string; pending: boolean; runMutation: RunMutation; onChooseAction: (action: "account" | "category") => void; initial: MoneyEvent | null; operationError: string; onSaved: () => void }) {
  const activeAccounts = accounts.filter((account) => !account.archived_at);
  const activeCategories = categories.filter((category) => !category.archived_at && category.kind === kind);
  const [accountID, setAccountID] = useState(initial?.movements[0]?.account_id ?? "");
  const [categoryID, setCategoryID] = useState(initial?.allocations[0]?.category_id ?? "");
  const [amount, setAmount] = useState(initial ? minorUnitsToInput(initial.amount_minor, initial.currency) : "");
  const [when, setWhen] = useState(initial ? localDateTimeValue(new Date(initial.occurred_at)) : localDateTimeValue);
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
      try { occurred_at = toOccurredAt(when); }
      catch (cause) { setError({ field: "when", message: messageOf(cause) }); return; }
      if (initial && !reviewed) { setReviewed(true); return; }
      const input: EventCreate = { kind, account_id: accountID, amount_minor, currency: selectedAccount.currency, occurred_at, note: note.trim(), allocations: categoryID ? [{ category_id: categoryID, amount_minor }] : [] };
      if (await runMutation(`${kind === "income" ? "Income" : "Expense"} recorded.`, () => postMoneyEvent(csrfToken, input))) onSaved();
    } catch (cause) { setError({ field: "form", message: messageOf(cause) }); }
  }
  return <form className="money-form" noValidate onSubmit={(event) => void submit(event)}>
    <div><p className="eyebrow">RECORD {kind.toUpperCase()}</p><h3>{kind === "income" ? "Money coming in." : "Money going out."}</h3><p>The selected account sets the currency. Choose a category or leave this transaction Uncategorized.</p></div>
    {activeAccounts.length === 0 && <p className="money-help">You need an active account. <button type="button" className="money-inline-action" onClick={() => onChooseAction("account")}>Create an account</button>.</p>}
    {activeCategories.length === 0 && <p className="money-help">You can record this as Uncategorized, or <button type="button" className="money-inline-action" onClick={() => onChooseAction("category")}>create a category</button>.</p>}
    <fieldset disabled={pending || activeAccounts.length === 0}><div className="money-field-grid">
      <label>Account<select required value={accountID} onChange={(event) => { setAccountID(event.target.value); changed(); }} aria-invalid={error?.field === "account"} aria-describedby={error?.field === "account" ? "money-entry-error" : undefined}><option value="">Choose an account</option>{activeAccounts.map((account) => <option key={account.id} value={account.id}>{account.name} · {account.currency}</option>)}</select></label>
      <label>{kind === "income" ? "Income" : "Expense"} category <span className="money-optional">optional</span><select value={categoryID} onChange={(event) => { setCategoryID(event.target.value); changed(); }} aria-invalid={error?.field === "category"} aria-describedby={error?.field === "category" ? "money-entry-error" : undefined}><option value="">Uncategorized</option>{activeCategories.map((category) => <option key={category.id} value={category.id}>{category.name}</option>)}</select></label>
      <label>Amount {selectedAccount && `(${selectedAccount.currency}, ${currencyDigits(selectedAccount.currency)} decimals)`}<input required inputMode="decimal" value={amount} onChange={(event) => { setAmount(event.target.value); changed(); }} placeholder="0.00" aria-invalid={error?.field === "amount"} aria-describedby={error?.field === "amount" ? "money-entry-error" : undefined} /></label>
      <label>Date and time<input required type="datetime-local" value={when} onChange={(event) => { setWhen(event.target.value); changed(); }} aria-invalid={error?.field === "when"} aria-describedby={error?.field === "when" ? "money-entry-error" : undefined} /></label>
    </div><label>Note <span className="money-optional">optional</span><textarea maxLength={1000} rows={3} value={note} onChange={(event) => { setNote(event.target.value); changed(); }} placeholder="A detail to remember" /></label>
      {error && <p id="money-entry-error" className="money-field-error" role="alert">{error.message}</p>}
      {operationError && <p className="money-field-error" role="alert">{operationError}</p>}
      {initial && reviewed && <p className="money-confirm" role="status">Review complete. Confirm to create a new record; the voided original will remain in history.</p>}
      <button className="button button-primary" type="submit" disabled={pending || activeAccounts.length === 0}>{pending ? "Saving…" : initial ? reviewed ? "Confirm replacement" : "Review replacement" : `Record ${kind}`}</button></fieldset>
  </form>;
}

function TransferForm({ accounts, csrfToken, pending, runMutation, onChooseAction, initial, operationError, onSaved }: { accounts: MoneyAccount[]; csrfToken: string; pending: boolean; runMutation: RunMutation; onChooseAction: (action: "account" | "category") => void; initial: MoneyEvent | null; operationError: string; onSaved: () => void }) {
  const active = accounts.filter((account) => !account.archived_at);
  const [fromID, setFromID] = useState(initial?.movements.find((movement) => BigInt(movement.amount_minor) < 0n)?.account_id ?? "");
  const source = active.find((account) => account.id === fromID);
  const destinations = active.filter((account) => account.id !== fromID && account.currency === source?.currency);
  const [toID, setToID] = useState(initial?.movements.find((movement) => BigInt(movement.amount_minor) > 0n)?.account_id ?? "");
  const destinationID = destinations.some((account) => account.id === toID) ? toID : "";
  const [amount, setAmount] = useState(initial ? minorUnitsToInput(initial.amount_minor, initial.currency) : "");
  const [when, setWhen] = useState(initial ? localDateTimeValue(new Date(initial.occurred_at)) : localDateTimeValue);
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
      try { occurred_at = toOccurredAt(when); }
      catch (cause) { setError({ field: "when", message: messageOf(cause) }); return; }
      if (initial && !reviewed) { setReviewed(true); return; }
      const input: TransferCreate = { from_account_id: fromID, to_account_id: destinationID, amount_minor, currency: source.currency, occurred_at, note: note.trim() };
      if (await runMutation("Transfer recorded. It is not counted as spending.", () => postMoneyTransfer(csrfToken, input))) onSaved();
    } catch (cause) { setError({ field: "form", message: messageOf(cause) }); }
  }
  return <form className="money-form" noValidate onSubmit={(event) => void submit(event)}>
    <div><p className="eyebrow">TRANSFER</p><h3>Move money between accounts.</h3><p>A transfer changes two balances but does not create income or spending.</p></div>
    {active.length < 2 && <p className="money-help">You need two active accounts. <button type="button" className="money-inline-action" onClick={() => onChooseAction("account")}>Create an account</button>.</p>}
    {source && destinations.length === 0 && <p className="money-help">Add another account in {source.currency} to make a transfer.</p>}
    <fieldset disabled={pending || active.length < 2}><div className="money-field-grid">
      <label>From account<select required value={fromID} onChange={(event) => { setFromID(event.target.value); setToID(""); changed(); }} aria-invalid={error?.field === "from"} aria-describedby={error?.field === "from" ? "money-transfer-error" : undefined}><option value="">Choose source</option>{active.map((account) => <option key={account.id} value={account.id}>{account.name} · {account.currency}</option>)}</select></label>
      <label>To account<select required value={destinationID} onChange={(event) => { setToID(event.target.value); changed(); }} aria-invalid={error?.field === "to"} aria-describedby={error?.field === "to" ? "money-transfer-error" : undefined}><option value="">Choose destination</option>{destinations.map((account) => <option key={account.id} value={account.id}>{account.name} · {account.currency}</option>)}</select></label>
      <label>Amount {source && `(${source.currency})`}<input required inputMode="decimal" value={amount} onChange={(event) => { setAmount(event.target.value); changed(); }} placeholder="0.00" aria-invalid={error?.field === "amount"} aria-describedby={error?.field === "amount" ? "money-transfer-error" : undefined} /></label>
      <label>Date and time<input required type="datetime-local" value={when} onChange={(event) => { setWhen(event.target.value); changed(); }} aria-invalid={error?.field === "when"} aria-describedby={error?.field === "when" ? "money-transfer-error" : undefined} /></label>
    </div><label>Note <span className="money-optional">optional</span><textarea maxLength={1000} rows={3} value={note} onChange={(event) => { setNote(event.target.value); changed(); }} placeholder="Why this money moved" /></label>
      {error && <p id="money-transfer-error" className="money-field-error" role="alert">{error.message}</p>}
      {operationError && <p className="money-field-error" role="alert">{operationError}</p>}
      {initial && reviewed && <p className="money-confirm" role="status">Review complete. Confirm to create a new transfer; the voided original will remain in history.</p>}
      <button className="button button-primary" type="submit" disabled={pending || active.length < 2}>{pending ? "Saving…" : initial ? reviewed ? "Confirm replacement" : "Review replacement" : "Record transfer"}</button></fieldset>
  </form>;
}

function HistorySection({ data, page, setPage, csrfToken, pending, runMutation, onReplace, operationError }: { data: MoneyData; page: number; setPage: (page: number) => void; csrfToken: string; pending: boolean; runMutation: RunMutation; onReplace: (kind: TransactionAction, initial: MoneyEvent | null) => void; operationError: string }) {
  const [voidID, setVoidID] = useState<string | null>(null);
  const [replacementCandidate, setReplacementCandidate] = useState<MoneyEvent | null>(null);
  const [reason, setReason] = useState("");
  const [reasonError, setReasonError] = useState("");
  async function confirmVoid(event: MoneyEvent) {
    const trimmed = reason.trim();
    if (Array.from(trimmed).length > 500) { setReasonError("The optional note must be 500 characters or fewer."); return; }
    if (await runMutation("Transaction voided. Its history remains visible.", () => voidMoneyEvent(csrfToken, event.id, trimmed))) { setVoidID(null); setReason(""); setReasonError(""); setReplacementCandidate(event); }
  }
  return <section className="money-section" aria-labelledby="money-history-title">
    <div className="section-heading"><div><p className="eyebrow">RECENT ACTIVITY</p><h2 id="money-history-title">Your transaction history.</h2></div></div>
    <p className="money-help">Browse recent activity by page. Search and filters are planned for a later Money enhancement.</p>
    {replacementCandidate && <div className="money-replacement" role="status"><p>The transaction was voided. Your place in history is preserved.</p>{(replacementCandidate.kind === "transfer" || replacementCandidate.allocations.length <= 1) ? <button type="button" className="button button-outline" onClick={() => { onReplace(replacementCandidate.kind, replacementCandidate); setReplacementCandidate(null); }}>Review prefilled replacement</button> : <p>This record has multiple category allocations. Add a new transaction manually to avoid dropping any allocation.</p>}<button type="button" className="money-text-action" onClick={() => setReplacementCandidate(null)}>Dismiss</button></div>}
    {data.events.length === 0 ? <div className="money-empty"><Icon name="today" /><h3>{page === 0 ? "No transactions yet" : "No more transactions"}</h3><p>{page === 0 ? "Recorded income, expenses, and transfers will appear here." : "Go back to the previous page to see your activity."}</p></div> : <ol className="money-history-list">{data.events.map((event) => <li className="money-history-item" key={event.id}>
      <div className="money-history-main"><span className={`money-event-icon money-event-${event.kind}`}><Icon name={event.kind === "transfer" ? "arrow" : "money"} /></span><div className="money-history-copy"><strong>{historyTitle(event, data.categories)}{event.status === "voided" ? " · Voided" : ""}</strong><span>{historyAccounts(event, data.accounts)} · {new Date(event.occurred_at).toLocaleString()}{event.kind !== "transfer" && event.allocations.length === 0 ? " · Uncategorized" : ""}</span></div><strong className={`money-history-amount${event.status === "voided" ? " is-voided" : ""}`}>{event.kind === "expense" ? "−" : event.kind === "income" ? "+" : "↔ "}{formatMinorUnits(event.amount_minor, event.currency)}</strong></div>
      <details className="money-event-details"><summary>View details</summary>
        {event.note && <p><strong>Note:</strong> {event.note}</p>}
        <p><strong>Account movements</strong></p><ul>{event.movements.map((movement) => <li key={movement.id}>{accountName(data.accounts, movement.account_id)}: {formatMinorUnits(movement.amount_minor, movement.currency)}</li>)}</ul>
        {event.kind !== "transfer" && (event.allocations.length > 0 ? <><p><strong>Categories</strong></p><ul>{event.allocations.map((allocation) => <li key={allocation.id}>{categoryName(data.categories, allocation.category_id)}: {formatMinorUnits(allocation.amount_minor, allocation.currency)}</li>)}</ul></> : <p><strong>Category:</strong> Uncategorized</p>)}
        {event.status === "voided" && <p><strong>Void reason:</strong> {event.void_reason === AUTO_VOID_REASON ? "N/A" : event.void_reason || "Unavailable"}</p>}
        {event.status === "posted" && (voidID === event.id ? <div className="money-confirm"><p>Void this transaction? Its balance effects will be removed while its history remains visible.</p><label>Note for voiding (optional)<textarea rows={2} value={reason} onChange={(change) => { setReason(change.target.value); setReasonError(""); }} placeholder="Add a note if useful" aria-invalid={!!reasonError} aria-describedby={reasonError ? "money-void-error" : undefined} /></label><p className="money-help">You can leave this blank. The history will show that no reason was provided.</p>{reasonError && <p id="money-void-error" className="money-field-error" role="alert">{reasonError}</p>}{operationError && <p className="money-field-error" role="alert">{operationError}</p>}<div className="money-confirm-actions"><button type="button" className="button button-outline" disabled={pending} onClick={() => { setVoidID(null); setReason(""); setReasonError(""); }}>Keep transaction</button><button type="button" className="button button-danger" disabled={pending} onClick={() => void confirmVoid(event)}>Confirm void</button></div></div>
          : <button type="button" className="money-text-action" disabled={pending} onClick={() => { setVoidID(event.id); setReason(""); setReasonError(""); }}>Void transaction</button>)}
      </details>
    </li>)}</ol>}
    <div className="money-pagination"><button type="button" className="button button-outline" disabled={page === 0 || pending} onClick={() => setPage(page - 1)}>Previous</button><span>Page {page + 1}</span><button type="button" className="button button-outline" disabled={!data.hasNext || pending} onClick={() => setPage(page + 1)}>Next</button></div>
  </section>;
}
