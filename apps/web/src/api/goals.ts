import type { components } from "./generated/schema";

export type Goal = components["schemas"]["Goal"];
export type GoalItem = components["schemas"]["GoalItem"];
type GoalErrorCode = components["schemas"]["GoalErrorResponse"]["error"]["code"];

export class GoalApiError extends Error {
  constructor(public readonly status: number, public readonly code: GoalErrorCode | "network_error") {
    super(goalErrorMessage(status, code));
    this.name = "GoalApiError";
  }
}

function goalErrorMessage(status: number, code: GoalErrorCode | "network_error"): string {
  if (status === 401) return "Your session has ended. Please sign in again.";
  switch (code) {
    case "network_error": case "session_unavailable": return "LifeOS could not connect. Check your Goals before trying again.";
    case "invalid_csrf": case "invalid_origin": return "Session protection failed. Reload the page and try again.";
    case "not_found": return "That Goal or step is no longer available. Refresh the list.";
    case "conflict": return "Finish every step before completing the Goal. Reopen a completed Goal before editing its checklist.";
    case "invalid_json": case "invalid_input": return "Check the title or action and try again.";
    default: return "The Goal change could not be completed. Check the list before trying again.";
  }
}

async function goalRequest<T>(path: string, method: "GET" | "POST" | "DELETE", csrfToken?: string, body?: object): Promise<T> {
  let response: Response;
  try {
    response = await fetch(path, {
      method,
      credentials: "same-origin",
      cache: "no-store",
      headers: csrfToken === undefined ? undefined : {
        "X-CSRF-Token": csrfToken,
        ...(body === undefined ? {} : { "Content-Type": "application/json" }),
      },
      body: body === undefined ? undefined : JSON.stringify(body),
    });
  } catch {
    throw new GoalApiError(0, "network_error");
  }
  if (!response.ok) {
    const payload = await response.json().catch(() => null) as { error?: { code?: GoalErrorCode } } | null;
    throw new GoalApiError(response.status, payload?.error?.code ?? "internal_error");
  }
  try {
    return await response.json() as T;
  } catch {
    throw new GoalApiError(response.status, "internal_error");
  }
}

const goalPath = (id: string) => `/api/v1/goals/${encodeURIComponent(id)}`;
const itemPath = (goalID: string, itemID: string) => `${goalPath(goalID)}/items/${encodeURIComponent(itemID)}`;

export const listGoals = () => goalRequest<components["schemas"]["GoalsResponse"]>("/api/v1/goals", "GET");
export const getGoal = (id: string) => goalRequest<Goal>(goalPath(id), "GET");
export const createGoal = (csrf: string, title: string) => goalRequest<Goal>("/api/v1/goals", "POST", csrf, { title });
export const renameGoal = (csrf: string, id: string, title: string) => goalRequest<Goal>(`${goalPath(id)}/rename`, "POST", csrf, { title });
export const setGoalCompletion = (csrf: string, id: string, completed: boolean) => goalRequest<Goal>(`${goalPath(id)}/completion`, "POST", csrf, { completed });
export const archiveGoal = (csrf: string, id: string) => goalRequest<Goal>(`${goalPath(id)}/archive`, "POST", csrf);
export const restoreGoal = (csrf: string, id: string) => goalRequest<Goal>(`${goalPath(id)}/restore`, "POST", csrf);
export const addGoalItem = (csrf: string, id: string, title: string) => goalRequest<Goal>(`${goalPath(id)}/items`, "POST", csrf, { title });
export const renameGoalItem = (csrf: string, goalID: string, itemID: string, title: string) => goalRequest<Goal>(`${itemPath(goalID, itemID)}/rename`, "POST", csrf, { title });
export const setGoalItemCompletion = (csrf: string, goalID: string, itemID: string, completed: boolean) => goalRequest<Goal>(`${itemPath(goalID, itemID)}/completion`, "POST", csrf, { completed });
export const removeGoalItem = (csrf: string, goalID: string, itemID: string) => goalRequest<Goal>(itemPath(goalID, itemID), "DELETE", csrf);
