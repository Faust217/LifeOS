// DOM position is presentation state only, never a financial snapshot or cursor.
export type HistoryLocation = { ids: string[]; top: number; occurredAt?: string };

export function captureHistoryLocation(id?: string): HistoryLocation {
  const records = [...document.querySelectorAll<HTMLElement>("summary[id^='money-event-']")];
  const index = id ? records.findIndex((record) => record.id === `money-event-${id}`) : records.findIndex((record) => record.getBoundingClientRect().bottom > 0);
  const record = records[Math.max(0, index)];
  const neighbors = index >= 0 ? [records[index], records[index + 1], records[index - 1]] : [record];
  return { ids: neighbors.filter(Boolean).map((item) => item.id), top: record?.getBoundingClientRect().top ?? 0 };
}

export function restoreHistoryLocation(location: HistoryLocation): boolean {
  const record = location.ids.map((id) => document.getElementById(id)).find((item) => item && !item.closest("[hidden]"));
  if (!record) return false;
  record.focus({ preventScroll: true });
  window.scrollBy({ top: record.getBoundingClientRect().top - location.top, behavior: "instant" });
  return true;
}
