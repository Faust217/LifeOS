export interface CurrentUser {
  id: string;
  email: string;
  is_owner_admin: boolean;
  csrf_token: string;
  time_zone: string | null;
}

export class SessionExpiredError extends Error {
  constructor() { super("Please sign in again."); }
}

export async function updateTimeZone(csrfToken: string, timeZone: string): Promise<void> {
  const response = await fetch("/api/v1/me/timezone", {
    method: "PUT", credentials: "same-origin",
    headers: { "Content-Type": "application/json", "X-CSRF-Token": csrfToken },
    body: JSON.stringify({ time_zone: timeZone }),
  });
  if (response.status === 401) throw new SessionExpiredError();
  if (!response.ok) throw new Error(response.status === 400 ? "Choose a valid IANA timezone." : "Unable to save timezone. Please retry.");
}

export async function fetchCurrentUser(): Promise<CurrentUser | null> {
  const response = await fetch("/api/v1/me", { credentials: "same-origin", cache: "no-store" });
  if (response.status === 401) return null;
  if (!response.ok) throw new Error("Unable to check sign-in status");
  return (await response.json()) as CurrentUser;
}

export async function signOut(csrfToken: string): Promise<void> {
  const response = await fetch("/api/v1/auth/logout", {
    method: "POST",
    credentials: "same-origin",
    headers: { "X-CSRF-Token": csrfToken },
  });
  if (response.status === 401) return;
  if (!response.ok) throw new Error("Unable to sign out");
}
