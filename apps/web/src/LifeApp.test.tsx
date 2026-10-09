import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { App, AuthenticatedShell } from "./LifeApp";

const user = {
  id: "11111111-1111-4111-8111-111111111111",
  email: "person@example.test",
  is_owner_admin: false,
  csrf_token: "test-only-csrf",
};

const renderArea = (area: "today" | "money" | "plans" | "me" | null) =>
  renderToStaticMarkup(<AuthenticatedShell area={area} user={user} onSignOut={() => undefined} />);

describe("LifeOS application shell", () => {
  it("hides all personal pages until session confirmation completes", () => {
    const html = renderToStaticMarkup(<App />);
    expect(html).toContain("Checking your LifeOS session");
    expect(html).not.toContain(user.email);
    expect(html).not.toContain("Primary navigation");
  });

  it("renders the same four real destinations for desktop and mobile", () => {
    const html = renderArea("today");
    expect(html).toContain('aria-label="Primary navigation"');
    expect(html).toContain('aria-label="Mobile navigation"');
    for (const href of ["/today", "/money", "/plans", "/me"]) {
      expect(html).toContain(`href="${href}"`);
    }
    expect(html.match(/aria-current="page"/g)).toHaveLength(2);
    expect(html).toContain('href="#main-content"');
  });

  it("gates Money data loading and keeps Plans honest", () => {
    const money = renderArea("money");
    const plans = renderArea("plans");
    expect(money).toContain("Loading your Money space");
    expect(money).not.toContain("MYR 0.00");
    expect(plans).toContain("not available in this shell yet");
    expect(plans).not.toContain("Add goal");
  });

  it("shows only the confirmed user's profile and a real sign-out control", () => {
    const html = renderArea("me");
    expect(html).toContain(user.email);
    expect(html).toContain(user.id);
    expect(html).toContain("Invited member");
    expect(html).toContain("Sign out</button>");
    expect(html).not.toContain(user.csrf_token);
  });

  it("keeps unknown direct URLs inside a recoverable authenticated shell", () => {
    const html = renderArea(null);
    expect(html).toContain("PAGE NOT FOUND");
    expect(html).toContain('href="/today"');
  });
});
