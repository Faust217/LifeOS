import { useCallback, useEffect, useRef, useState } from "react";

import { fetchCurrentUser, signOut, type CurrentUser } from "./api/auth";
import { Icon } from "./Icons";
import { GoalsPage } from "./GoalsPage";
import { MoneyPage } from "./MoneyPage";
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
    const user = session.user;
    requestSequence.current += 1;
    signingOut.current = true;
    setSession({ status: "signing-out", user });
    setSignOutError("");
    try {
      await signOut(user.csrf_token);
      setSession({ status: "signed-out" });
    } catch {
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

export function AuthenticatedShell({ area, goalID = null, user, onSignOut, onSessionExpired = () => undefined, signingOut = false, signOutError = "" }: {
  area: Area | null;
  goalID?: string | null;
  user: CurrentUser;
  onSignOut: () => void;
  onSessionExpired?: () => void;
  signingOut?: boolean;
  signOutError?: string;
}) {
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
          {area === "today" && <TodayPage />}
          {area === "money" && <MoneyPage csrfToken={user.csrf_token} onSessionExpired={onSessionExpired} />}
          {area === "plans" && <GoalsPage goalID={goalID} csrfToken={user.csrf_token} onSessionExpired={onSessionExpired} />}
          {area === "me" && <MePage user={user} onSignOut={onSignOut} signingOut={signingOut} signOutError={signOutError} />}
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

function TodayPage() {
  return (
    <div className="page-stack">
      <section className="today-hero" aria-labelledby="today-title">
        <div className="today-hero-copy">
          <p className="eyebrow">TODAY · YOUR SPACE</p>
          <h1 id="today-title">A clearer view of your day.</h1>
          <p>Your everyday life, gathered in one place. Begin with Money today; your Goals overview will grow here later.</p>
        </div>
        <div className="hero-shape" aria-hidden="true"><span /><span /><span /></div>
      </section>
      <div className="section-heading">
        <div><p className="eyebrow">START HERE</p><h2>One place, at your pace.</h2></div>
        <p>Explore the spaces that will bring your day together.</p>
      </div>
      <div className="overview-grid">
        <a className="overview-card" href="/money">
          <span className="icon-tile icon-tile-money"><Icon name="money" /></span>
          <span className="card-kicker">YOUR FINANCES</span>
          <strong>Money</strong>
          <span>See your accounts, record activity, and keep each currency clear.</span>
          <span className="card-link">Explore Money <Icon name="arrow" /></span>
        </a>
        <a className="overview-card" href="/plans">
          <span className="icon-tile icon-tile-plans"><Icon name="plans" /></span>
          <span className="card-kicker">WHAT COMES NEXT</span>
          <strong>Plans</strong>
          <span>Create a Goal, keep its steps together, and see your progress.</span>
          <span className="card-link">Explore Plans <Icon name="arrow" /></span>
        </a>
      </div>
    </div>
  );
}

function MePage({ user, onSignOut, signingOut, signOutError }: { user: CurrentUser; onSignOut: () => void; signingOut: boolean; signOutError: string }) {
  return (
    <div className="page-stack">
      <PageHeading eyebrow="ME" title="Your space, your way." description="The account behind your private LifeOS space." />
      <section className="profile-card" aria-labelledby="profile-title">
        <div className="profile-top"><UserAvatar email={user.email} /><div><p className="eyebrow">YOUR PROFILE</p><h2 id="profile-title">Account details</h2></div></div>
        <dl className="profile-details">
          <div><dt>Email address</dt><dd>{user.email}</dd></div>
          <div><dt>Access</dt><dd>{user.is_owner_admin ? "Owner administrator" : "Invited member"}</dd></div>
          <div><dt>LifeOS ID</dt><dd className="id-value">{user.id}</dd></div>
        </dl>
      </section>
      <section className="privacy-card" aria-labelledby="privacy-title"><span className="icon-tile icon-tile-privacy"><Icon name="shield" /></span><div><h2 id="privacy-title">Private by design</h2><p>Your LifeOS account keeps your information separate from other members. Data-management tools will be added in a later milestone.</p></div></section>
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
