import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import type { MoneyEvent, RunningHistory } from "./api/money";
import { HistorySection, HistoryBalanceLine, HistoryChanged, ReconstructedDetails } from "./MoneyPage";

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
    timeZone="Asia/Kuala_Lumpur"  csrfToken="test-only"
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

describe("Historical balance presentation", () => {
  it("keeps pre-anchor numbers out of summaries and puts labelled reconstruction and date in details", () => {
    const position = { movement_id: "move", account_id: "bank", currency: "MYR", balance_after_minor: "100728", basis: "reconstructed_pre_anchor" as const, unavailable_reason: null };
    const collapsed = renderToStaticMarkup(<HistoryBalanceLine name="Everyday bank" position={position} />);
    expect(collapsed).toContain("Before balance date");
    expect(collapsed).not.toContain("1,007.28");
    const expanded = renderToStaticMarkup(<ReconstructedDetails positions={[position]} accounts={[{id:"bank",name:"Everyday bank",currency:"MYR",archived_at:null,balance_basis:"known_balance_before_instant",opening_balance_at:"2026-10-30T00:00:00Z"}]} timeZone="Asia/Kuala_Lumpur" />);
    expect(expanded).toContain("MYR 1,007.28");
    expect(expanded).toContain("Not bank verified");
    expect(expanded).toContain("Calculated backwards");
    expect(expanded).toContain("October 30, 2026");
    expect(renderToStaticMarkup(<ReconstructedDetails positions={[{...position,unavailable_reason:"voided",balance_after_minor:null}]} accounts={[]} timeZone="Asia/Kuala_Lumpur" />)).toBe("");
  });
  it("groups appended same-local-date records once and provides manual load, retry and end states", () => {
    const records = [event, {...event,id:"older",occurred_at:"2026-10-20T00:00:00Z"}];
    const render = (hasNext: boolean, loadError = "", loadingMore = false) => renderToStaticMarkup(<HistorySection data={{accounts:[],categories:[],balances:{},events:records,hasNext}} timeZone="Asia/Kuala_Lumpur" onLoadMore={()=>undefined} onRefresh={()=>undefined} loadError={loadError} loadingMore={loadingMore} csrfToken="test" pending={false} runMutation={async()=>true} onEdit={()=>undefined} operationError="" />);
    const html = render(true);
    expect(html.match(/class="money-date-heading"/g)).toHaveLength(1);
    expect(html).toContain("Load more");
    expect(html).not.toContain(">Previous<"); expect(html).not.toContain(">Next<");
    expect(render(true,"Offline")).toContain("Retry loading");
    expect(render(true,"Offline")).toContain("Refresh history");
    expect(render(true,"",true)).toContain("Loading older transactions");
    expect(render(false)).toContain("All transactions loaded");
  });
  it("keeps a Transfer as one record with two currency-labelled independent balances", () => {
    const to={...event.movements[0],id:"44444444-4444-4444-8444-444444444444",account_id:"55555555-5555-4555-8555-555555555555",amount_minor:"10000"};
    const transfer={...event,kind:"transfer" as const,amount_minor:"10000",note:"Move to cash",movements:[{...event.movements[0],amount_minor:"-10000"},to]};
    const history: RunningHistory={projection:"lifeos-running-history-v1",history_revision:"a".repeat(64),time_zone:"Asia/Kuala_Lumpur",limit:10,has_more:false,next_cursor:null,categories:[],accounts:transfer.movements.map((m,i)=>({id:m.account_id,name:i?"Cash":"Bank",currency:"MYR",archived_at:null,balance_basis:"known_balance_before_instant",opening_balance_at:"2026-10-01T00:00:00Z"})),items:[{...transfer,account_balances:transfer.movements.map((m,i)=>({movement_id:m.id,account_id:m.account_id,currency:"MYR",balance_after_minor:i?"30000":"85000",basis:"anchored",unavailable_reason:null}))}]};
    const html=renderToStaticMarkup(<HistorySection data={{accounts:[],categories:[],balances:{},events:[transfer],hasNext:false,history}} timeZone="Asia/Kuala_Lumpur"  csrfToken="test" pending={false} runMutation={async()=>true} onEdit={()=>undefined} operationError="" />);
    expect(html.match(/class="money-history-item"/g)).toHaveLength(1);
    expect(html.match(/class="money-history-position-name"/g)).toHaveLength(2);
    expect(html).toContain("MYR 850.00");expect(html).toContain("MYR 300.00");
    expect(html).toContain(`aria-describedby="money-balances-${event.id}"`);
    const mixed = {...history, items: history.items.map(item => ({...item, account_balances: item.account_balances.map((position,index) => index ? {...position,basis:"reconstructed_pre_anchor" as const} : position)}))};
    const mixedHTML = renderToStaticMarkup(<HistorySection data={{accounts:[],categories:[],balances:{},events:[transfer],hasNext:false,history:mixed}} timeZone="Asia/Kuala_Lumpur" csrfToken="test" pending={false} runMutation={async()=>true} onEdit={()=>undefined} operationError="" />);
    const summary = mixedHTML.match(new RegExp(`<summary[^>]*id="money-event-${event.id}"[\\s\\S]*?</summary>`))?.[0] ?? "";
    expect(summary).toContain("MYR 850.00"); expect(summary).toContain("Before balance date"); expect(summary).not.toContain("MYR 300.00");
    expect(mixedHTML).toContain("after MYR 300.00");
  });
  it("uses snapshot labels and exact balances rather than current overview balances", () => {
    const history: RunningHistory = { projection:"lifeos-running-history-v1",history_revision:"a".repeat(64),time_zone:"Asia/Kuala_Lumpur",limit:10,has_more:false,next_cursor:null,
      accounts:[{id:event.movements[0].account_id,name:"Snapshot bank",currency:"MYR",archived_at:null,balance_basis:"known_balance_before_instant",opening_balance_at:"2026-10-01T00:00:00Z"}],categories:[],
      items:[{...event,account_balances:[{movement_id:event.movements[0].id,account_id:event.movements[0].account_id,currency:"MYR",balance_after_minor:"98800",basis:"anchored",unavailable_reason:null}]}] };
    const html=renderToStaticMarkup(<HistorySection data={{accounts:[],categories:[],balances:{},events:[event],hasNext:false,history}} timeZone="Asia/Kuala_Lumpur"  csrfToken="test" pending={false} runMutation={async()=>true} onEdit={()=>undefined} operationError="" />);
    expect(html).toContain("Snapshot bank");expect(html).toContain("MYR 988.00");
    expect(html).toContain('aria-label="Account balances after transaction"');
    expect(html).not.toContain("Unavailable account");expect(html).not.toContain("Void transaction");
  });
  it.each([
    ["voided","anchored",null,"Voided: no balance effect"],
    ["anchor_required","legacy_unconfirmed",null,"Confirm balance date in Overview"],
    ["amount_out_of_range","anchored",null,"Balance outside supported range"],
    [null,"reconstructed_pre_anchor","-9223372036854775808","Before balance date"],
  ] as const)("labels unavailable and reconstructed positions: %s", (reason,basis,value,label) => {
    const html=renderToStaticMarkup(<HistoryBalanceLine name={"A very long account name ".repeat(5)} position={{movement_id:"movement",account_id:"account",currency:"MYR",balance_after_minor:value,basis,unavailable_reason:reason}} />);
    expect(html).toContain(label);if(reason) expect(html).not.toContain("MYR 0.00");
    else expect(html).not.toContain("92,233,720,368,547,758.08");
  });
  it("offers explicit stale recovery without a misleading old list or lost-command claim", () => {
    const html=renderToStaticMarkup(<HistoryChanged onRefresh={()=>undefined} refreshing={false}/>);
    expect(html).toContain("Refresh history");expect(html).toContain("pending Save are kept");
    expect(html).not.toContain("money-history-item");
  });
});
