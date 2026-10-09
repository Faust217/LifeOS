export interface CurrentUser {
  id: string;
  email: string;
  is_owner_admin: boolean;
  csrf_token: string;
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
