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
  return primaryAreas.find((area) => area.href === path)?.id ?? null;
}
