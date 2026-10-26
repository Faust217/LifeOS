import type { components } from "./generated/schema";
import { pendingFinancialWrite, type PendingFinancialCommand } from "../pendingFinancialWrite";

export type MoneyAccount = components["schemas"]["MoneyAccount"];
export type MoneyBalance = components["schemas"]["MoneyBalance"];
export type MoneyCategory = components["schemas"]["MoneyCategory"];
export type MoneyEvent = components["schemas"]["MoneyEvent"];
export type AccountCreate = components["schemas"]["MoneyAccountCreate"];
export type CategoryCreate = components["schemas"]["MoneyCategoryCreate"];
export type CategoryRename = components["schemas"]["MoneyCategoryRename"];
export type EventCreate = components["schemas"]["MoneyEventCreate"];
export type TransferCreate = components["schemas"]["MoneyTransferCreate"];
export type MoneyHistory = components["schemas"]["MoneyHistoryResponse"];
type MoneyErrorCode = components["schemas"]["MoneyErrorResponse"]["error"]["code"];

export class MoneyApiError extends Error {
  constructor(public readonly status: number, public readonly code: MoneyErrorCode | "network_error") {
    super(moneyErrorMessage(status, code));
    this.name = "MoneyApiError";
  }
}

function moneyErrorMessage(status: number, code: MoneyErrorCode | "network_error"): string {
  if (status === 401) return "Your session has ended. Please sign in again.";
  switch (code) {
    case "idempotency_key_conflict": return "This recovery key conflicts with a different Save. Check activity before clearing recovery; do not submit it with a new key.";
    case "idempotency_capacity_exceeded": return "Financial write capacity is full. Contact the LifeOS operator; existing Saves can still be retried.";
    case "financial_write_busy": return "LifeOS is busy. Keep the original Save and use Retry Save.";
    case "financial_write_outcome_unknown": case "idempotency_version_unsupported": return "Save is unresolved. Keep its recovery record and retry the same Save.";
    case "network_error": case "session_unavailable": return "LifeOS could not connect. Please try again.";
    case "invalid_csrf": case "invalid_origin": return "Session protection failed. Reload the page and try again.";
    case "currency_mismatch": return "These accounts use different currencies. Choose compatible accounts.";
    case "not_found": return "That record is no longer available. Refresh and try again.";
    case "archived_record": return "That account or category is archived. Choose an active one.";
    case "conflict": return "This change conflicts with the current record state. Refresh and try again.";
    case "amount_overflow": return "The amount is outside the supported range.";
    case "invalid_input": case "invalid_json": return "Please check the form details and try again.";
    default: return "Money could not complete that request. Please try again.";
  }
}

async function moneyRequest<T>(path: string, csrfToken?: string, body?: object): Promise<T> {
  let response: Response;
  try {
    response = await fetch(path, {
      method: csrfToken === undefined ? "GET" : "POST",
      credentials: "same-origin",
      cache: "no-store",
      headers: csrfToken === undefined ? undefined : { "Content-Type": "application/json", "X-CSRF-Token": csrfToken },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  } catch {
    throw new MoneyApiError(0, "network_error");
  }
  if (!response.ok) {
    const payload = await response.json().catch(() => null) as { error?: { code?: MoneyErrorCode } } | null;
    throw new MoneyApiError(response.status, payload?.error?.code ?? "internal_error");
  }
  if (response.status === 204) return undefined as T;
  try {
    return await response.json() as T;
  } catch {
    throw new MoneyApiError(response.status, "internal_error");
  }
}

export type FinancialReplay = { outcome: "already_applied" };

// This path sends the exact persisted bytes. Never reconstruct a retry from a form.
async function financialRequest<T>(csrfToken: string, command: PendingFinancialCommand, retry: boolean): Promise<T | FinancialReplay> {
  await pendingFinancialWrite.start(command);
  let resolved = false;
  try {
    let response: Response;
    try {
      response = await fetch(command.path, {
        method: "POST", credentials: "same-origin", cache: "no-store",
        headers: { "Content-Type": "application/json", "X-CSRF-Token": csrfToken, "Idempotency-Key": command.key },
        body: command.body ?? undefined,
      });
    } catch { throw new MoneyApiError(0, "network_error"); }
    if (!response.ok) {
      const payload = await response.json().catch(() => null) as { error?: { code?: MoneyErrorCode } } | null;
      const code = payload?.error?.code ?? "internal_error";
      // A failed retry cannot establish that an earlier attempt did not commit.
      resolved = !retry && [400, 404, 409, 413, 415, 422].includes(response.status) && code !== "idempotency_key_conflict";
      throw new MoneyApiError(response.status, code);
    }
    const replayed = response.headers.get("Idempotency-Replayed");
    if (response.status === 200 && replayed === "true") {
      const receipt = await response.json().catch(() => null) as FinancialReplay | null;
      if (!receipt || Object.keys(receipt).length !== 1 || receipt.outcome !== "already_applied") throw new MoneyApiError(response.status, "internal_error");
      resolved = true;
      return receipt;
    }
    const noContent = command.operation === "event.void" || command.operation === "event.delete";
    const expectedStatus = noContent ? 204 : command.operation === "opening_balance.confirm" ? 200 : 201;
    if (replayed !== "false" || response.status !== expectedStatus) throw new MoneyApiError(response.status, "internal_error");
    if (noContent) { resolved = true; return undefined as T; }
    const result = await response.json().catch(() => null) as Record<string, unknown> | null;
    if (!result || typeof result.id !== "string" || !result.id || typeof result.currency !== "string" ||
      (command.operation === "account.create" || command.operation === "opening_balance.confirm" ? typeof result.opening_balance_minor !== "string" : typeof result.amount_minor !== "string" || !Array.isArray(result.movements))) throw new MoneyApiError(response.status, "internal_error");
    resolved = true;
    return result as T;
  } finally { pendingFinancialWrite.finish(command, resolved); }
}

async function prepareFinancialRequest<T>(path: string, csrfToken: string, body?: object): Promise<T | FinancialReplay> {
  return financialRequest<T>(csrfToken, await pendingFinancialWrite.prepare(path, body === undefined ? null : JSON.stringify(body)), false);
}

export const retryFinancialWrite = (csrfToken: string) => financialRequest<unknown>(csrfToken, pendingFinancialWrite.retry(), true);

export const listMoneyAccounts = () => moneyRequest<components["schemas"]["MoneyAccountsResponse"]>("/api/v1/accounts");
export const getMoneyBalance = (id: string) => moneyRequest<MoneyBalance>(`/api/v1/accounts/${encodeURIComponent(id)}/balance`);
export const listMoneyCategories = () => moneyRequest<components["schemas"]["MoneyCategoriesResponse"]>("/api/v1/categories");
export const listMoneyHistory = (limit: number, offset: number) => moneyRequest<MoneyHistory>(`/api/v1/events?limit=${limit}&offset=${offset}`);

export const createMoneyAccount = (csrfToken: string, input: AccountCreate) => prepareFinancialRequest<MoneyAccount>("/api/v1/accounts", csrfToken, input);
export const createMoneyCategory = (csrfToken: string, input: CategoryCreate) => moneyRequest<MoneyCategory>("/api/v1/categories", csrfToken, input);
export const renameMoneyCategory = (csrfToken: string, id: string, input: CategoryRename) => moneyRequest<MoneyCategory>(`/api/v1/categories/${encodeURIComponent(id)}/rename`, csrfToken, input);
export const archiveMoneyCategory = (csrfToken: string, id: string) => moneyRequest<void>(`/api/v1/categories/${encodeURIComponent(id)}/archive`, csrfToken);
export const restoreMoneyCategory = (csrfToken: string, id: string) => moneyRequest<void>(`/api/v1/categories/${encodeURIComponent(id)}/restore`, csrfToken);
export const postMoneyEvent = (csrfToken: string, input: EventCreate) => prepareFinancialRequest<MoneyEvent>("/api/v1/events", csrfToken, input);
export const postMoneyTransfer = (csrfToken: string, input: TransferCreate) => prepareFinancialRequest<MoneyEvent>("/api/v1/transfers", csrfToken, input);
export const archiveMoneyAccount = (csrfToken: string, id: string) => moneyRequest<void>(`/api/v1/accounts/${encodeURIComponent(id)}/archive`, csrfToken);
export const restoreMoneyAccount = (csrfToken: string, id: string) => moneyRequest<void>(`/api/v1/accounts/${encodeURIComponent(id)}/restore`, csrfToken);
export const voidMoneyEvent = (csrfToken: string, id: string, reason = "") => prepareFinancialRequest<void>(`/api/v1/events/${encodeURIComponent(id)}/void`, csrfToken, { reason } satisfies components["schemas"]["MoneyVoidRequest"]);
export const deleteMoneyEvent = (csrfToken: string, id: string) => prepareFinancialRequest<void>(`/api/v1/events/${encodeURIComponent(id)}/delete`, csrfToken);

export type OpeningPreview = components["schemas"]["MoneyOpeningPreview"];
export const previewOpeningBalance = (csrfToken: string, id: string, input: components["schemas"]["MoneyOpeningPreviewRequest"]) => moneyRequest<OpeningPreview>(`/api/v1/accounts/${encodeURIComponent(id)}/opening-balance/preview`, csrfToken, input);
export const confirmOpeningBalance = (csrfToken: string, id: string, input: components["schemas"]["MoneyOpeningConfirmRequest"]) => prepareFinancialRequest<MoneyAccount>(`/api/v1/accounts/${encodeURIComponent(id)}/opening-balance/confirm`, csrfToken, input);
