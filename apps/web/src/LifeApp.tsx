import { useCallback, useEffect, useRef, useState } from "react";

import { fetchCurrentUser, signOut, type CurrentUser } from "./api/auth";
import { Icon } from "./Icons";
import { GoalsPage } from "./GoalsPage";
import { TimeZoneSetting } from "./TimeZoneSetting";
import { OwnerExport } from "./OwnerExport";
import { timeZoneLabel } from "./moneyAmount";
import "./me.css";
import { pendingFinancialWrite } from "./pendingFinancialWrite";
import { MoneyPage } from "./MoneyPage";
import { TodayPage } from "./TodayPage";
import { goalIDFromPath, primaryAreas, resolveArea, type Area } from "./navigation";

type SessionState =
  | { status: "loading" }
  | { status: "signed-out" }
  | { status: "unavailable" }
  | { status: "ready" | "signing-out"; user: CurrentUser };

export function App() {
  const [session, setSession] = useState<SessionState>({ status: "loading" });
  const [signOutError, setSignOutError] = useState("");
  const requestSequence = useRef(0);
  const signingOut = useRef(false);

  const refreshSession = useCallback(async () => {
    const request = ++requestSequence.current;
    setSession({ status: "loading" });
    setSignOutError("");
    try {
      const user = await fetchCurrentUser();
      if (request === requestSequence.current) {
        if (!user) pendingFinancialWrite.bindOwner(null);
        setSession(user ? { status: "ready", user } : { status: "signed-out" });
      }
    } catch {
      if (request === requestSequence.current) setSession({ status: "unavailable" });
    }
  }, []);

  useEffect(() => {
    void refreshSession();
    const onVisible = () => {
      if (document.visibilityState === "visible" && !signingOut.current) void refreshSession();
    };
    const onPageShow = (event: PageTransitionEvent) => {
      if (event.persisted && !signingOut.current) void refreshSession();
    };
    document.addEventListener("visibilitychange", onVisible);
    window.addEventListener("pageshow", onPageShow);
    return () => {
      requestSequence.current += 1;
      document.removeEventListener("visibilitychange", onVisible);
      window.removeEventListener("pageshow", onPageShow);
    };
  }, [refreshSession]);

  async function handleSignOut() {
    if (session.status !== "ready") return;
    const recovery = pendingFinancialWrite.status();
    if (recovery.kind === "pending" && recovery.sending) { setSignOutError("Wait for the current Save to finish before signing out."); return; }
    if (recovery.kind !== "empty" && recovery.kind !== "hidden" && !window.confirm("A Save may be unresolved. Signing out clears this tab's private recovery record and loses its retry key. Check your activity first. Sign out?")) return;
    const user = session.user;
    // An asynchronous draft preparation must not begin a Save during explicit sign-out.
    pendingFinancialWrite.bindOwner(null);
    requestSequence.current += 1;
    signingOut.current = true;
    setSession({ status: "signing-out", user });
    setSignOutError("");
    try {
      await signOut(user.csrf_token);
      try { pendingFinancialWrite.clearAcknowledged(); } catch { window.alert("Signed out. Local recovery storage could not be cleared. Close this tab to remove its private session storage; lost keys cannot be recovered."); }
      pendingFinancialWrite.bindOwner(null);
      setSession({ status: "signed-out" });
    } catch {
      pendingFinancialWrite.bindOwner(user.id);
      setSession({ status: "ready", user });
      setSignOutError("Sign-out could not be completed. Please try again.");
    } finally {
      signingOut.current = false;
    }
  }

  if (session.status === "loading") return <LoadingScreen />;
  if (session.status === "unavailable") return <UnavailableScreen onRetry={() => void refreshSession()} />;
  if (session.status === "signed-out") return <SignedOutScreen />;

  return (
    <AuthenticatedShell
      area={resolveArea(window.location.pathname)}
      goalID={goalIDFromPath(window.location.pathname)}
      user={session.user}
      onSignOut={() => void handleSignOut()}
      onSessionExpired={refreshSession}
      onTimeZoneSaved={(timeZone) => setSession((current) => current.status === "ready" ? { ...current, user: { ...current.user, time_zone: timeZone } } : current)}
      signingOut={session.status === "signing-out"}
      signOutError={signOutError}
    />
  );
}

function Brand() {
  return (
    <span className="brand">
      <span className="brand-mark" aria-hidden="true"><span /><span /><span /><span /></span>
      <span className="brand-name">lifeos<span className="brand-dot">.</span></span>
    </span>
  );
}

function LoadingScreen() {
  return (
    <main className="gate-screen" aria-busy="true">
      <div className="gate-card" role="status" aria-live="polite">
        <Brand />
        <span className="loading-orb" aria-hidden="true" />
        <h1>Finding your space.</h1>
        <p>Checking your LifeOS session before showing anything personal.</p>
      </div>
    </main>
  );
}

function UnavailableScreen({ onRetry }: { onRetry: () => void }) {
  return (
    <main className="gate-screen">
      <div className="gate-card" role="alert">
        <Brand />
        <span className="gate-symbol"><Icon name="shield" /></span>
        <h1>We couldn’t confirm your session.</h1>
        <p>LifeOS is having trouble connecting. Your private pages will stay hidden until the connection is restored.</p>
        <button className="button button-primary" type="button" onClick={onRetry}>Try again <Icon name="arrow" /></button>
      </div>
    </main>
  );
}

function SignedOutScreen() {
  return (
    <main className="gate-screen">
      <div className="gate-card gate-card-welcome">
        <Brand />
        <div className="welcome-art" aria-hidden="true"><span /><span /><span /></div>
        <p className="eyebrow">A place for your everyday life</p>
        <h1>Make room for what matters.</h1>
        <p>LifeOS brings your money and plans together in one calm, private space.</p>
        <a className="button button-primary" href="/api/v1/auth/start">Continue with Google <Icon name="arrow" /></a>
        <p className="gate-footnote">Access is currently limited to invited Google accounts.</p>
      </div>
    </main>
  );
}

function Navigation({ area, placement }: { area: Area | null; placement: "side" | "bottom" }) {
  return (
    <nav className={`primary-nav primary-nav-${placement}`} aria-label={placement === "side" ? "Primary navigation" : "Mobile navigation"}>
      {primaryAreas.map((item) => (
        <a key={item.id} className={`nav-item${area === item.id ? " nav-item-active" : ""}`} href={item.href} aria-current={area === item.id ? "page" : undefined}>
          <Icon name={item.id} />
          <span>{item.label}</span>
        </a>
      ))}
    </nav>
  );
}

function UserAvatar({ email }: { email: string }) {
  return <span className="user-avatar" aria-hidden="true">{email.slice(0, 1).toUpperCase()}</span>;
}

export function AuthenticatedShell({ area, goalID = null, user, onSignOut, onSessionExpired = () => undefined, onTimeZoneSaved = () => undefined, signingOut = false, signOutError = "" }: {
  area: Area | null;
  goalID?: string | null;
  user: CurrentUser;
  onSignOut: () => void;
  onSessionExpired?: () => void;
  onTimeZoneSaved?: (timeZone: string) => void;
  signingOut?: boolean;
  signOutError?: string;
}) {
  useEffect(() => {
    pendingFinancialWrite.bindOwner(user.id);
    const warn = (event: BeforeUnloadEvent) => { const s = pendingFinancialWrite.status(); if (s.kind === "pending" || s.kind === "corrupted") { event.preventDefault(); event.returnValue = ""; } };
    window.addEventListener("beforeunload", warn);
    return () => { window.removeEventListener("beforeunload", warn); pendingFinancialWrite.bindOwner(null); };
  }, [user.id]);
  return (
    <div className="app-layout">
      <a className="skip-link" href="#main-content">Skip to content</a>
      <aside className="sidebar">
        <a className="brand-link" href="/today" aria-label="LifeOS Today"><Brand /></a>
        <p className="sidebar-intro">A little more clarity<br />for everyday life.</p>
        <Navigation area={area} placement="side" />
        <div className="sidebar-account">
          <UserAvatar email={user.email} />
          <div><span className="sidebar-account-label">Signed in as</span><span className="sidebar-account-email">{user.email}</span></div>
        </div>
      </aside>

      <div className="app-column">
        <header className="topbar">
          <a className="mobile-brand-link" href="/today" aria-label="LifeOS Today"><Brand /></a>
          <span className="topbar-label">YOUR PRIVATE SPACE</span>
          <a className="topbar-account" href="/me" aria-label={`Your profile: ${user.email}`}><UserAvatar email={user.email} /><span>{user.email}</span></a>
        </header>
        <main id="main-content" className="page-content" tabIndex={-1}>
          {area === "today" && <TodayPage timeZone={user.time_zone} onSessionExpired={onSessionExpired} />}
          {area === "money" && <MoneyPage csrfToken={user.csrf_token} timeZone={user.time_zone} onSessionExpired={onSessionExpired} />}
          {area === "plans" && <GoalsPage goalID={goalID} csrfToken={user.csrf_token} onSessionExpired={onSessionExpired} />}
          {area === "me" && <MePage user={user} onSignOut={onSignOut} signingOut={signingOut} signOutError={signOutError} onTimeZoneSaved={onTimeZoneSaved} onSessionExpired={onSessionExpired} />}
          {area === null && <NotFoundPage />}
        </main>
      </div>

      <Navigation area={area} placement="bottom" />
    </div>
  );
}

function PageHeading({ eyebrow, title, description }: { eyebrow: string; title: string; description: string }) {
  return (
    <header className="page-heading">
      <p className="eyebrow">{eyebrow}</p>
      <h1>{title}</h1>
      <p>{description}</p>
    </header>
  );
}

function MePage({ user, onSignOut, signingOut, signOutError, onTimeZoneSaved, onSessionExpired }: { user: CurrentUser; onSignOut: () => void; signingOut: boolean; signOutError: string; onTimeZoneSaved: (zone: string) => void; onSessionExpired: () => void }) {
  const [settingsRequest, setSettingsRequest] = useState(0);
  useEffect(() => {
    if (window.location.hash === "#timezone") {
      document.getElementById("timezone")?.scrollIntoView({ block: "start" });
      document.querySelector<HTMLButtonElement>("#timezone button")?.focus({ preventScroll: true });
    }
  }, []);
  return (
    <div className="page-stack me-page">
      <header className="me-heading"><h1>Me</h1></header>
      <section className="profile-card" aria-labelledby="profile-title">
        <div className="profile-top"><UserAvatar email={user.email} /><h2 id="profile-title">Profile</h2></div>
        <dl className="profile-details">
          <div><dt>Email address</dt><dd>{user.email}</dd></div>
          <div><dt>Access</dt><dd>{user.is_owner_admin ? "Owner administrator" : "Invited member"}</dd></div>
          <div><dt>LifeOS ID</dt><dd className="id-value">{user.id}</dd></div>
        </dl>
      </section>
      <section id="timezone" className="me-card" aria-labelledby="timezone-title"><h2 id="timezone-title">Timezone</h2>{user.time_zone && <><p>{timeZoneLabel(user.time_zone)}</p><p className="me-help">{user.time_zone}</p><button type="button" className="button button-outline timezone-settings-action" onClick={() => setSettingsRequest((value) => value + 1)}>Change timezone</button></>}<TimeZoneSetting timeZone={user.time_zone} csrfToken={user.csrf_token} openRequest={settingsRequest} onRequestOpen={() => setSettingsRequest((value) => value + 1)} onSaved={onTimeZoneSaved} onSessionExpired={onSessionExpired} /></section>
      <OwnerExport csrfToken={user.csrf_token} onSessionExpired={onSessionExpired} />
      <section className="signout-card" aria-labelledby="session-title"><div><h2 id="session-title">Your session</h2><p>Sign out when you’re finished using this device.</p></div><button className="button button-outline" type="button" onClick={onSignOut} disabled={signingOut}><Icon name="logout" />{signingOut ? "Signing out…" : "Sign out"}</button></section>
      {signOutError && <p className="inline-error" role="alert">{signOutError}</p>}
    </div>
  );
}

function NotFoundPage() {
  return (
    <div className="page-stack"><PageHeading eyebrow="PAGE NOT FOUND" title="Let’s find your way back." description="This address doesn’t match a LifeOS area." /><a className="button button-primary" href="/today">Go to Today <Icon name="arrow" /></a></div>
  );
}
