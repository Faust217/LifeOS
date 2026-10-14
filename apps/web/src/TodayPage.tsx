import { useCallback, useEffect, useRef, useState } from "react";

import { GoalApiError, listGoals, type Goal } from "./api/goals";
import {
  getMoneyBalance, listMoneyAccounts, listMoneyCategories, listMoneyHistory,
  MoneyApiError, type MoneyAccount, type MoneyBalance, type MoneyCategory, type MoneyEvent,
} from "./api/money";
import { Icon } from "./Icons";
import { formatLocalTime, formatMinorUnits, localDateKey } from "./moneyAmount";
import "./today.css";

type MoneySnapshot = {
  accounts: MoneyAccount[];
  balances: MoneyBalance[];
  categories: MoneyCategory[];
  events: MoneyEvent[];
};
type ReadState<T> = { status: "loading" } | { status: "error"; message: string } | { status: "ready"; data: T };

function readMessage(error: unknown): string {
  return error instanceof Error ? error.message : "This section could not load. Please try again.";
}

function accountName(accounts: MoneyAccount[], id: string): string {
  return accounts.find((account) => account.id === id)?.name ?? "Unavailable account";
}

export function eventLabel(event: MoneyEvent, categories: MoneyCategory[]): string {
  if (event.note.trim()) return event.note.trim();
  if (event.kind === "transfer") return "Transfer between accounts";
  const names = event.allocations.map((allocation) => categories.find((category) => category.id === allocation.category_id)?.name ?? "Unavailable category");
  return names.length ? names.join(" + ") : "Uncategorized";
}

export function eventAccounts(event: MoneyEvent, accounts: MoneyAccount[]): string {
  if (event.kind !== "transfer") return accountName(accounts, event.movements[0]?.account_id ?? "");
  const from = event.movements.find((movement) => BigInt(movement.amount_minor) < 0n);
  const to = event.movements.find((movement) => BigInt(movement.amount_minor) > 0n);
  return `${accountName(accounts, from?.account_id ?? "")} → ${accountName(accounts, to?.account_id ?? "")}`;
}

async function loadMoneySnapshot(): Promise<MoneySnapshot> {
  const [accounts, categories, history] = await Promise.all([
    listMoneyAccounts(), listMoneyCategories(), listMoneyHistory(4, 0),
  ]);
  const balances = await Promise.all(accounts.items.map((account) => getMoneyBalance(account.id)));
  return { accounts: accounts.items, balances, categories: categories.items, events: history.items };
}

export function activeGoalsOnly(goals: Goal[]): Goal[] {
  return goals.filter((goal) => !goal.archived_at && !goal.completed_at);
}

export function activityDateTime(occurredAt: string, timeZone: string, now = new Date()): string {
  const time = formatLocalTime(occurredAt, timeZone);
  if (localDateKey(occurredAt, timeZone) === localDateKey(now.toISOString(), timeZone)) return time;
  const date = new Intl.DateTimeFormat("en", { timeZone, month: "short", day: "numeric" }).format(new Date(occurredAt));
  return `${date} · ${time}`;
}

export function TodayPage({ timeZone, onSessionExpired }: { timeZone: string | null; onSessionExpired: () => void }) {
  const [money, setMoney] = useState<ReadState<MoneySnapshot>>({ status: "loading" });
  const [goals, setGoals] = useState<ReadState<Goal[]>>({ status: "loading" });
  const sequence = useRef(0);

  const refresh = useCallback(() => {
    const request = ++sequence.current;
    setMoney({ status: "loading" });
    setGoals({ status: "loading" });
    void loadMoneySnapshot().then((data) => {
      if (request === sequence.current) setMoney({ status: "ready", data });
    }, (error: unknown) => {
      if (request !== sequence.current) return;
      if (error instanceof MoneyApiError && error.status === 401) onSessionExpired();
      setMoney({ status: "error", message: readMessage(error) });
    });
    void listGoals().then((response) => {
      if (request === sequence.current) setGoals({ status: "ready", data: response.items });
    }, (error: unknown) => {
      if (request !== sequence.current) return;
      if (error instanceof GoalApiError && error.status === 401) onSessionExpired();
      setGoals({ status: "error", message: readMessage(error) });
    });
  }, [onSessionExpired]);

  useEffect(() => {
    refresh();
    return () => { sequence.current += 1; };
  }, [refresh]);

  const activeGoals = goals.status === "ready" ? activeGoalsOnly(goals.data) : [];
  return <div className="page-stack today-page">
    <header className="today-heading"><h1>Today</h1></header>
    <div className="today-grid">
      <section className="today-panel" aria-labelledby="today-accounts-title">
        <div className="today-panel-head"><h2 id="today-accounts-title">Accounts</h2><a href="/money">Money <Icon name="arrow" /></a></div>
        {money.status === "loading" && <p role="status">Loading your accounts…</p>}
        {money.status === "error" && <div role="alert"><p>{money.message}</p><button className="button button-outline" type="button" onClick={refresh}>Try again</button></div>}
        {money.status === "ready" && (money.data.accounts.length === 0
          ? <p className="today-empty">No accounts yet. <a href="/money">Add one in Money.</a></p>
          : <ul className="today-account-list">{money.data.accounts.map((account) => {
            const balance = money.data.balances.find((item) => item.account_id === account.id);
            return <li key={account.id}><div><strong>{account.name}</strong><span>{account.archived_at ? "Archived" : account.type === "credit_card" && balance && BigInt(balance.amount_minor) < 0n ? "Amount owed · liability" : account.type.replaceAll("_", " ")}</span></div><strong>{balance ? formatMinorUnits(balance.amount_minor, account.currency) : "Balance unavailable"}</strong></li>;
          })}</ul>)}
      </section>
      <section className="today-panel" aria-labelledby="today-goals-title">
        <div className="today-panel-head"><h2 id="today-goals-title">Active Goals</h2><a href="/plans">Plans <Icon name="arrow" /></a></div>
        {goals.status === "loading" && <p role="status">Loading your Goals…</p>}
        {goals.status === "error" && <div role="alert"><p>{goals.message}</p><button className="button button-outline" type="button" onClick={refresh}>Try again</button></div>}
        {goals.status === "ready" && (activeGoals.length === 0
          ? <p className="today-empty">No active Goals. <a href="/plans">Start one in Plans.</a></p>
          : <ul className="today-goal-list">{activeGoals.slice(0, 4).map((goal) => <li key={goal.id}><a href={`/plans/goals/${goal.id}`}><strong>{goal.title}</strong><span>{goal.completed_count} of {goal.total_count} steps complete <Icon name="arrow" /></span><progress max={Math.max(goal.total_count, 1)} value={goal.completed_count} aria-label={`${goal.title} progress`} /></a></li>)}</ul>)}
      </section>
    </div>
    <section className="today-panel" aria-labelledby="today-activity-title">
      <div className="today-panel-head"><h2 id="today-activity-title">Recent transactions</h2><a href="/money">History <Icon name="arrow" /></a></div>
      {money.status === "loading" && <p role="status">Loading recent activity…</p>}
      {money.status === "error" && <p role="alert">Recent activity is unavailable while Money is loading.</p>}
      {money.status === "ready" && (money.data.events.length === 0
        ? <p className="today-empty">No transactions yet. <a href="/money">Add one in Money.</a></p>
        : <>{!timeZone && <p className="today-empty">Confirm your timezone in <a href="/money">Money</a> to see local transaction dates.</p>}<ol className="today-activity-list">{money.data.events.map((event) => {
          const label = eventLabel(event, money.data.categories);
          return <li key={event.id}><div><strong>{label}{event.status === "voided" ? " · Voided" : ""}</strong><span>{eventAccounts(event, money.data.accounts)}{timeZone ? ` · ${activityDateTime(event.occurred_at, timeZone)}` : ""}{event.kind !== "transfer" && event.allocations.length === 0 && label !== "Uncategorized" ? " · Uncategorized" : ""}</span></div><strong className={event.status === "voided" ? "is-voided" : ""}>{event.kind === "expense" ? "−" : event.kind === "income" ? "+" : "↔ "}{formatMinorUnits(event.amount_minor, event.currency)}</strong></li>;
        })}</ol></>)}
    </section>
  </div>;
}
