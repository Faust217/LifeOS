// One recovery record per tab. This is not an offline queue or background replay.
export const FINANCIAL_PENDING_STORAGE = "lifeos.financial-write.v1";
const KEY = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;
const UUID = "[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}";
type StorageAccess = Pick<Storage, "getItem" | "setItem" | "removeItem">;
export type PendingFinancialCommand = Readonly<{ version: 1 | 2; ifMatch?: string | null; owner: string; key: string; operation: string; target: string | null; path: string; body: string | null; checksum: string }>;
export type PendingStatus = { kind: "empty" | "hidden" | "unavailable" | "corrupted" | "other_owner" } | { kind: "pending"; command: PendingFinancialCommand; sending: boolean };

export class FinancialRecoveryError extends Error {
  constructor(public readonly reason: "pending" | "storage" | "corrupted" | "owner" | "other_owner") {
    super(reason === "pending" ? "An earlier Save is unresolved. Close this form and use Retry Save; its original command and key have been kept."
      : reason === "owner" ? "Sign in again before saving."
      : reason === "other_owner" ? "This tab retains an unresolved Save for another account. Sign in to that account to retry, or explicitly resolve its recovery record before a new Save."
      : reason === "corrupted" ? "The saved recovery record is unreadable. Its lost request key cannot be recovered. Check activity before clearing recovery."
      : "Recovery storage is unavailable or full. No new request was sent. If an earlier Save was uncertain, check activity; a lost key cannot be recovered.");
    this.name = "FinancialRecoveryError";
  }
}

function operation(path: string): { operation: string; target: string | null } | null {
  if (path === "/api/v1/accounts") return { operation: "account.create", target: null };
  if (path === "/api/v1/events") return { operation: "event.create", target: null };
  if (path === "/api/v1/transfers") return { operation: "transfer.create", target: null };
  const confirm = path.match(new RegExp(`^/api/v1/accounts/(${UUID})/opening-balance/confirm$`));
  if (confirm) return { operation: "opening_balance.confirm", target: confirm[1] };
  const event = path.match(new RegExp(`^/api/v1/events/(${UUID})/(void|delete|edit)$`));
  return event ? { operation: `event.${event[2]}`, target: event[1] } : null;
}

function validRecord(value: unknown): value is PendingFinancialCommand {
  if (!value || typeof value !== "object" || Array.isArray(value)) return false;
  const v = value as Record<string, unknown>;
  if (Object.keys(v).sort().join() !== (v.version === 2 ? "body,checksum,ifMatch,key,operation,owner,path,target,version" : "body,checksum,key,operation,owner,path,target,version") || typeof v.checksum !== "string" || !/^[0-9a-f]{64}$/.test(v.checksum) || (v.version !== 1 && v.version !== 2) || typeof v.owner !== "string" || !v.owner || v.owner.length > 128 || typeof v.key !== "string" || !KEY.test(v.key) || typeof v.path !== "string") return false;
  const expected = operation(v.path);
  if (!expected || v.operation !== expected.operation || v.target !== expected.target) return false;
  if (v.version === 1 && expected.operation === "event.edit") return false;
  if (v.version === 2 && (v.ifMatch !== null && (typeof v.ifMatch !== "string" || !/^"[1-9][0-9]*"$/.test(v.ifMatch)))) return false;
  if (v.version === 2 && ["event.edit","event.void","event.delete"].includes(expected.operation) && !v.ifMatch) return false;
  if (expected.operation === "event.delete") return v.body === null;
  if (typeof v.body !== "string" || new TextEncoder().encode(v.body).length > 16 * 1024) return false;
  try { const body = JSON.parse(v.body); return body !== null && typeof body === "object" && !Array.isArray(body); } catch { return false; }
}

function newKey(): string {
  if (typeof crypto.randomUUID === "function") return crypto.randomUUID();
  const b = crypto.getRandomValues(new Uint8Array(16));
  b[6] = (b[6] & 15) | 64; b[8] = (b[8] & 63) | 128;
  const hex = Array.from(b, (v) => v.toString(16).padStart(2, "0")).join("");
  return `${hex.slice(0, 8)}-${hex.slice(8, 12)}-${hex.slice(12, 16)}-${hex.slice(16, 20)}-${hex.slice(20)}`;
}

async function commandChecksum(command: Omit<PendingFinancialCommand, "checksum">): Promise<string> {
  const bytes = new TextEncoder().encode(JSON.stringify(command.version === 1 ? [command.version, command.owner, command.key, command.operation, command.target, command.path, command.body] : [command.version, command.owner, command.key, command.operation, command.target, command.path, command.body, command.ifMatch]));
  const digest = await crypto.subtle.digest("SHA-256", bytes);
  return Array.from(new Uint8Array(digest), (v) => v.toString(16).padStart(2, "0")).join("");
}

export class PendingFinancialWrites {
  private owner: string | null = null;
  private sending: PendingFinancialCommand | null = null;
  private preparing = false;
  private corruptKey: string | null = null;
  private listeners = new Set<() => void>();
  constructor(private storage: () => StorageAccess, private makeKey: () => string = newKey) {}
  subscribe = (listener: () => void) => { this.listeners.add(listener); return () => { this.listeners.delete(listener); }; };
  private notify() { this.listeners.forEach((listener) => listener()); }

  bindOwner(owner: string | null) {
    this.owner = owner;
    // Preserve an unresolved command across owner changes. The new owner sees
    // only a blocker, never the former owner's command or retry key.
    this.notify();
  }

  private read(): PendingStatus {
    let raw: string | null;
    try { raw = this.storage().getItem(FINANCIAL_PENDING_STORAGE); } catch { return { kind: "unavailable" }; }
    if (raw === null) return { kind: "empty" };
    if (new TextEncoder().encode(raw).length > 32 * 1024) return { kind: "corrupted" };
    try { const record: unknown = JSON.parse(raw); return validRecord(record) && record.key !== this.corruptKey ? { kind: "pending", command: Object.freeze(record), sending: this.sending !== null } : { kind: "corrupted" }; }
    catch { return { kind: "corrupted" }; }
  }

  status(): PendingStatus {
    if (!this.owner) return { kind: "hidden" };
    const current = this.read();
    if (current.kind === "pending" && current.command.owner !== this.owner) return { kind: "other_owner" };
    return current;
  }

  async prepare(path: string, body: string | null, ifMatch: string | null = null): Promise<PendingFinancialCommand> {
    if (!this.owner) throw new FinancialRecoveryError("owner");
    const current = this.status();
    if (this.preparing || this.sending || current.kind === "pending") throw new FinancialRecoveryError("pending");
    if (current.kind !== "empty") throw new FinancialRecoveryError(current.kind === "corrupted" ? "corrupted" : current.kind === "other_owner" ? "other_owner" : "storage");
    const intent = operation(path);
    if (!intent) throw new Error("Unsupported financial command");
    this.preparing = true;
    try {
    const base = { version: 2 as const, ifMatch, owner: this.owner, key: this.makeKey(), ...intent, path, body };
    const record = Object.freeze({ ...base, checksum: await commandChecksum(base) });
    if (this.owner !== base.owner || this.read().kind !== "empty") throw new FinancialRecoveryError("pending");
    if (!validRecord(record)) throw new FinancialRecoveryError("corrupted");
    const serialized = JSON.stringify(record);
    if (new TextEncoder().encode(serialized).length > 32 * 1024) throw new FinancialRecoveryError("storage");
    try {
      this.storage().setItem(FINANCIAL_PENDING_STORAGE, serialized);
      if (this.storage().getItem(FINANCIAL_PENDING_STORAGE) !== serialized) throw new Error("Recovery write verification failed");
    } catch { this.notify(); throw new FinancialRecoveryError("storage"); }
    this.notify();
    return record;
    } finally { this.preparing = false; this.notify(); }
  }

  retry(): PendingFinancialCommand {
    const status = this.status();
    if (status.kind !== "pending") throw new FinancialRecoveryError(status.kind === "corrupted" ? "corrupted" : status.kind === "other_owner" ? "other_owner" : "storage");
    if (this.sending) throw new FinancialRecoveryError("pending");
    return status.command;
  }

  async start(record: PendingFinancialCommand) {
    const status = this.status();
    if (this.sending || status.kind !== "pending" || JSON.stringify(status.command) !== JSON.stringify(record)) throw new FinancialRecoveryError("pending");
    this.sending = record;
    this.notify();
    try {
      if (await commandChecksum(record) !== record.checksum) { this.corruptKey = record.key; throw new FinancialRecoveryError("corrupted"); }
      const current = this.status();
      if (current.kind !== "pending" || JSON.stringify(current.command) !== JSON.stringify(record)) throw new FinancialRecoveryError("pending");
    } catch (error) { this.sending = null; this.notify(); throw error; }
  }

  finish(record: PendingFinancialCommand, resolved: boolean) {
    if (resolved) {
      const status = this.read();
      // An older asynchronous completion cannot erase a newer owner's command.
      if (status.kind === "pending" && JSON.stringify(status.command) === JSON.stringify(record)) {
        try { this.storage().removeItem(FINANCIAL_PENDING_STORAGE); } catch { /* Keep recovery rather than overwrite a possibly retained key. */ }
      }
    }
    if (this.sending?.key === record.key && this.sending.owner === record.owner) this.sending = null;
    this.notify();
  }

  // Only after explicit recovery acknowledgement or successful explicit sign-out.
  clearAcknowledged() {
    if (this.sending || this.preparing) throw new FinancialRecoveryError("pending");
    try { this.storage().removeItem(FINANCIAL_PENDING_STORAGE); if (this.storage().getItem(FINANCIAL_PENDING_STORAGE) !== null) throw new Error("Not cleared"); }
    catch { throw new FinancialRecoveryError("storage"); }
    this.corruptKey = null;
    this.notify();
  }
}

export const pendingFinancialWrite = new PendingFinancialWrites(() => window.sessionStorage);
