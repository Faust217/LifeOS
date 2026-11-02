import { describe, expect, it } from "vitest";

import type { Goal } from "./api/goals";
import type { MoneyAccount, MoneyCategory, MoneyEvent } from "./api/money";
import { activeGoalsOnly, activityDateTime, eventAccounts, eventLabel } from "./TodayPage";

const fromID = "11111111-1111-4111-8111-111111111111";
const toID = "22222222-2222-4222-8222-222222222222";
const accounts = [
  { id: fromID, name: "Cash", currency: "MYR" },
  { id: toID, name: "Savings", currency: "MYR" },
] as MoneyAccount[];
const categories = [{ id: "33333333-3333-4333-8333-333333333333", name: "Food" }] as MoneyCategory[];

describe("Today read-only projections", () => {
  it("labels a transfer using its two movements, without an expense category", () => {
    const transfer = {
      id: "44444444-4444-4444-8444-444444444444", kind: "transfer", note: "", amount_minor: "2500", currency: "MYR",
      occurred_at: "2026-10-13T00:00:00Z", event_revision: "1", status: "posted",
      movements: [
        { id: "55555555-5555-4555-8555-555555555555", account_id: fromID, amount_minor: "-2500", currency: "MYR" },
        { id: "66666666-6666-4666-8666-666666666666", account_id: toID, amount_minor: "2500", currency: "MYR" },
      ],
      allocations: [],
    } satisfies MoneyEvent;
    expect(eventLabel(transfer, categories)).toBe("Transfer between accounts");
    expect(eventAccounts(transfer, accounts)).toBe("Cash → Savings");
  });

  it("uses the recorded category when an expense has no note", () => {
    const expense = {
      kind: "expense", note: "", movements: [{ account_id: fromID, amount_minor: "-500" }],
      allocations: [{ category_id: categories[0].id, amount_minor: "500" }],
    } as MoneyEvent;
    expect(eventLabel(expense, categories)).toBe("Food");
    expect(eventAccounts(expense, accounts)).toBe("Cash");
  });

  it("labels income and expenses with no allocations as Uncategorized", () => {
    for (const kind of ["income", "expense"] as const) {
      const event = {
        id: "77777777-7777-4777-8777-777777777777", kind, note: "", amount_minor: "100", currency: "MYR",
        occurred_at: "2026-10-13T00:00:00Z", event_revision: "1", status: "posted",
        allocations: [], movements: [{ id: "88888888-8888-4888-8888-888888888888", account_id: fromID, amount_minor: kind === "expense" ? "-100" : "100", currency: "MYR" }],
      } satisfies MoneyEvent;
      expect(eventLabel(event, categories)).toBe("Uncategorized");
      expect(eventAccounts(event, accounts)).toBe("Cash");
    }
  });

  it("shows only unfinished, unarchived Goals with their server progress intact", () => {
    const goal = { id: "active", completed_count: 1, total_count: 3 } as Goal;
    const completed = { id: "completed", completed_at: "2026-10-13T00:00:00Z" } as Goal;
    const archived = { id: "archived", archived_at: "2026-10-13T00:00:00Z" } as Goal;
    expect(activeGoalsOnly([goal, completed, archived])).toEqual([goal]);
    expect(goal.completed_count).toBe(1);
    expect(goal.total_count).toBe(3);
  });

  it("uses the saved LifeOS timezone for concise recent-activity dates", () => {
    const now = new Date("2026-10-14T04:00:00Z");
    expect(activityDateTime("2026-10-13T17:30:00Z", "Asia/Kuala_Lumpur", now)).toBe("01:30");
    expect(activityDateTime("2026-10-13T17:30:00Z", "America/New_York", now)).toBe("Oct 13 · 13:30");
  });
});
