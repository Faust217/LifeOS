export const primaryAreas = [
  { id: "today", label: "Today", href: "/today" },
  { id: "money", label: "Money", href: "/money" },
  { id: "plans", label: "Plans", href: "/plans" },
  { id: "me", label: "Me", href: "/me" },
] as const;

export type Area = (typeof primaryAreas)[number]["id"];

export function resolveArea(pathname: string): Area | null {
  const path = pathname.replace(/\/+$/, "") || "/";
  if (path === "/") return "today";
  if (goalIDFromPath(path)) return "plans";
  return primaryAreas.find((area) => area.href === path)?.id ?? null;
}

export function goalIDFromPath(pathname: string): string | null {
  const match = /^\/plans\/goals\/([0-9a-fA-F]{8}(?:-[0-9a-fA-F]{4}){3}-[0-9a-fA-F]{12})\/?$/.exec(pathname);
  return match?.[1] ?? null;
}
