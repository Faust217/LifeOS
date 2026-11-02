import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import type { MoneyEvent } from "./api/money";
import { HistorySection } from "./MoneyPage";

const event: MoneyEvent = {
  id: "11111111-1111-4111-8111-111111111111",
  event_revision: "2", kind: "expense", currency: "MYR", amount_minor: "1200",
  occurred_at: "2026-10-20T05:23:00Z", status: "posted", note: "Lunch and medicine",
  movements: [{ id: "22222222-2222-4222-8222-222222222222", account_id: "33333333-3333-4333-8333-333333333333", amount_minor: "-1200", currency: "MYR" }],
  allocations: [],
};

function renderHistory(record: MoneyEvent, pending = false) {
  return renderToStaticMarkup(<HistorySection
    data={{ accounts: [], categories: [], balances: {}, events: [record], hasNext: false }}
    timeZone="Asia/Kuala_Lumpur" page={0} setPage={() => undefined} csrfToken="test-only"
    pending={pending} runMutation={async () => true} onEdit={() => undefined} operationError=""
  />);
}

describe("Money transaction actions", () => {
  it.each(["expense", "income", "transfer"] as const)("offers Edit and Delete without an ordinary Void action for posted %s", (kind) => {
    const movements = kind === "transfer" ? [
      event.movements[0],
      { ...event.movements[0], id: "44444444-4444-4444-8444-444444444444", account_id: "55555555-5555-4555-8555-555555555555", amount_minor: "1200" },
    ] : [{ ...event.movements[0], amount_minor: kind === "income" ? "1200" : "-1200" }];
    const html = renderHistory({ ...event, kind, movements });
    expect(html).toMatch(/class="button button-primary" aria-label="Edit transaction"[^>]*>Edit<\/button>/);
    expect(html).toContain('aria-label="Delete transaction"');
    expect(html).not.toContain("Void transaction");
    expect(html).not.toContain("Confirm void");
    expect(html).not.toContain("Confirm delete");
    expect(html).not.toContain("More actions");
  });

  it.each([
    ["No reason provided by owner", "N/A"],
    ["Entered twice", "Entered twice"],
  ])("preserves visible Voided history and its stored reason: %s", (reason, display) => {
    const html = renderHistory({ ...event, status: "voided", void_reason: reason });
    expect(html).toContain(" · Voided");
    expect(html).toContain("is-voided");
    expect(html).toContain(`<strong>Void reason:</strong> ${display}`);
    expect(html).not.toContain('aria-label="Edit transaction"');
    expect(html).not.toContain("Void transaction");
    expect(html).toContain('aria-label="Delete transaction"');
  });

  it("keeps both actions disabled during a mutation or protected uncertain Save", () => {
    const html = renderHistory(event, true);
    expect(html).toMatch(/aria-label="Edit transaction" disabled=""/);
    expect(html).toMatch(/aria-label="Delete transaction" disabled=""/);
  });
});
