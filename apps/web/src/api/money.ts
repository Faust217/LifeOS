import type { components } from "./generated/schema";

export type MoneyAccount = components["schemas"]["MoneyAccount"];
export type MoneyBalance = components["schemas"]["MoneyBalance"];
export type MoneyCategory = components["schemas"]["MoneyCategory"];
export type MoneyEvent = components["schemas"]["MoneyEvent"];
export type AccountCreate = components["schemas"]["MoneyAccountCreate"];
export type CategoryCreate = components["schemas"]["MoneyCategoryCreate"];
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

export const listMoneyAccounts = () => moneyRequest<components["schemas"]["MoneyAccountsResponse"]>("/api/v1/accounts");
export const getMoneyBalance = (id: string) => moneyRequest<MoneyBalance>(`/api/v1/accounts/${encodeURIComponent(id)}/balance`);
export const listMoneyCategories = () => moneyRequest<components["schemas"]["MoneyCategoriesResponse"]>("/api/v1/categories");
export const listMoneyHistory = (limit: number, offset: number) => moneyRequest<MoneyHistory>(`/api/v1/events?limit=${limit}&offset=${offset}`);

export const createMoneyAccount = (csrfToken: string, input: AccountCreate) => moneyRequest<MoneyAccount>("/api/v1/accounts", csrfToken, input);
export const createMoneyCategory = (csrfToken: string, input: CategoryCreate) => moneyRequest<MoneyCategory>("/api/v1/categories", csrfToken, input);
export const postMoneyEvent = (csrfToken: string, input: EventCreate) => moneyRequest<MoneyEvent>("/api/v1/events", csrfToken, input);
export const postMoneyTransfer = (csrfToken: string, input: TransferCreate) => moneyRequest<MoneyEvent>("/api/v1/transfers", csrfToken, input);
export const archiveMoneyAccount = (csrfToken: string, id: string) => moneyRequest<void>(`/api/v1/accounts/${encodeURIComponent(id)}/archive`, csrfToken);
export const restoreMoneyAccount = (csrfToken: string, id: string) => moneyRequest<void>(`/api/v1/accounts/${encodeURIComponent(id)}/restore`, csrfToken);
export const voidMoneyEvent = (csrfToken: string, id: string, reason: string) => moneyRequest<void>(`/api/v1/events/${encodeURIComponent(id)}/void`, csrfToken, { reason } satisfies components["schemas"]["MoneyVoidRequest"]);
