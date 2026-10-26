import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it, vi } from "vitest";

import { MoneyApiError } from "./api/money";
import { FinancialWriteRecovery } from "./FinancialWriteRecovery";
import { pendingFinancialWrite, type PendingStatus } from "./pendingFinancialWrite";

const frozen: Extract<PendingStatus, { kind: "pending" }> = {
  kind: "pending",
  command: {
    version: 1, owner: "owner-A", key: "11111111-1111-4111-8111-111111111111",
    operation: "event.create", target: null, path: "/api/v1/events",
    body: '{"note":"private synthetic note"}', checksum: "0".repeat(64),
  },
  sending: false,
};

function render(status: PendingStatus) {
  vi.spyOn(pendingFinancialWrite, "status").mockReturnValue(status);
  return renderToStaticMarkup(<FinancialWriteRecovery csrfToken="test-csrf" onRecovered={() => undefined} onSessionExpired={() => undefined} />);
}

afterEach(() => vi.restoreAllMocks());

describe("Financial Save recovery presentation", () => {
  it("gives an unresolved Save one concise action and keeps discarding behind acknowledgement", () => {
    const html = render(frozen);
    expect(html).toContain("Check your last Save");
    expect(html).toContain("may have succeeded. Retry safely to check without creating a duplicate.");
    expect(html.match(/Retry Save/g)).toHaveLength(1);
    expect(html).toContain("More recovery options");
    expect(html).toContain("Refreshing activity does not confirm or discard this Save.");
    expect(html).toMatch(/Clear recovery record<\/button>/);
    expect(html).toMatch(/disabled=""[^>]*>Clear recovery record/);
    expect(html).not.toContain("private synthetic note");
  });

  it("hides another owner's command and explains storage failure without promising key recovery", () => {
    const other = render({ kind: "other_owner" });
    expect(other).toContain("Another account has an unresolved Save");
    expect(other).not.toContain("owner-A");
    expect(other).not.toContain("Retry Save</button>");
    expect(render({ kind: "corrupted" })).toContain("A lost request key cannot be recovered.");
    expect(render({ kind: "unavailable" })).toContain("New financial Saves are blocked.");
    expect(render({ kind: "pending", command: frozen.command, sending: true })).toBe("");
  });

  it("keeps key conflicts explicit instead of suggesting a fresh Save", () => {
    expect(new MoneyApiError(409, "idempotency_key_conflict").message).toContain("do not submit it with a new key");
  });
});
