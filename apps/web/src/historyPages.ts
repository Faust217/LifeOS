import type { RunningHistory } from "./api/money";

// Only read state lives here. A replacement stream is built separately, then
// published whole; a failed/stale batch can never contaminate the visible stream.
export class HistoryPages {
  private value: RunningHistory | undefined;
  private batches = 0;
  constructor(private readonly limit: number) {}
  get count() { return this.batches; }
  get current() { return this.value; }
  query() {
    if (this.value && !this.value.has_more) throw new Error("All transactions are loaded.");
    return { limit: this.limit, history_revision: this.value?.history_revision, cursor: this.value?.next_cursor ?? undefined };
  }
  accept(response: RunningHistory): RunningHistory {
    const old = this.value;
    if (response.limit !== this.limit || (response.has_more && (!response.next_cursor || !response.items.length)) ||
      (old && (old.history_revision !== response.history_revision || old.time_zone !== response.time_zone || !old.has_more || old.next_cursor === response.next_cursor))) {
      throw new Error("History changed. Refresh history before loading more.");
    }
    const ids = new Set(old?.items.map((item) => item.id));
    for (const item of response.items) {
      // A duplicate indicates an invalid continuation; do not hide a missing-row
      // defect by silently deduplicating or partly appending the response.
      if (ids.has(item.id)) throw new Error("History continuation repeated a transaction. Refresh history.");
      ids.add(item.id);
    }
    const merge = <T extends { id: string }>(previous: T[], next: T[]) => [...new Map([...previous, ...next].map((item) => [item.id, item])).values()];
    this.value = { ...response, items: [...(old?.items ?? []), ...response.items],
      accounts: merge(old?.accounts ?? [], response.accounts), categories: merge(old?.categories ?? [], response.categories) };
    this.batches += 1;
    return this.value;
  }
}
