# Issue #19 Local Authentication Setup

**Status:** Issue #19 local implementation and Product Owner live Google OIDC verification passed on 2026-10-09. Public Alpha deployment remains out of scope.

## Google setup

1. In a Google Cloud project, configure the OAuth consent screen for the intended small Alpha audience. Add each tester as a test user while the app is in testing mode.
2. Create an OAuth client of type **Web application**. Add the exact authorized redirect URI `http://127.0.0.1:4173/api/v1/auth/callback` for the default Compose demo. Add `http://127.0.0.1:4173` as an authorized JavaScript origin if the console requests one. The server requests `openid` and `email` scopes.
3. Copy `.env.example` to the ignored project-root `.env`. Replace `LIFEOS_DB_PASSWORD` with a local password. Set `LIFEOS_OIDC_CLIENT_ID` and `LIFEOS_OIDC_CLIENT_SECRET` from the Web client. Keep `LIFEOS_AUTH_PUBLIC_ORIGIN=http://127.0.0.1:4173`. Never paste these values into Git, logs, screenshots, or a browser `VITE_*` setting.
4. For Vite development instead of Compose web, register `http://127.0.0.1:5173/api/v1/auth/callback` as a second redirect URI and set `LIFEOS_AUTH_PUBLIC_ORIGIN=http://127.0.0.1:5173`. Vite proxies `/api` to the local Go API.

Google's current setup instructions: [OpenID Connect](https://developers.google.com/identity/openid-connect/openid-connect). A production deployment requires HTTPS and a separate security/hosting review; this issue does not deploy one.

The included local nginx proxy suppresses `/api/` access and error logs because callback URLs contain authorization codes. Any later public reverse proxy must also avoid logging callback query strings and session cookies.

## Local demo

From the repository root in PowerShell:

```powershell
docker compose --env-file .env -f deploy/compose.yaml up --build -d
go run ./cmd/lifeos-admin bootstrap --confirm-local-operator --email=owner@example.com
```

Replace the example email with the Product Owner's exact Google account email. Run the operator command only from a trusted local terminal with local PostgreSQL access. The command never grants administrator status to the first arbitrary registrant: it designates one email as the owner bootstrap invitation. Open `http://127.0.0.1:4173`, select **Sign in with Google**, and use that account. A signed-in owner can verify `/api/v1/admin/status` returns 204; an ordinary invited user receives 403. The web page shows current-user status and signs out through a CSRF-protected POST.

To invite a tester after owner admission, obtain the owner's LifeOS ID from `/api/v1/me` in the signed-in browser and run:

```powershell
go run ./cmd/lifeos-admin invite --confirm-local-operator --admin-id=OWNER_LIFEOS_ID --email=tester@example.com
```

The operator command verifies the supplied ID is an active owner administrator in PostgreSQL. An unaccepted invitation can be revoked with `revoke --confirm-local-operator --admin-id=OWNER_LIFEOS_ID --invitation-id=INVITATION_ID`. Before any owner is admitted, `revoke-bootstrap --confirm-local-operator` withdraws the bootstrap invitation. This is a trusted local tool, not an HTTP admin endpoint or Admin Dashboard. The command refuses non-loopback database hosts; local database access and the explicit confirmation flag are operator trust boundaries.

If credentials are absent in development, the Foundation API starts without auth routes, and the page reports no signed-in user. The automated OIDC test uses a local signed provider double; it verifies server-side token checks and LifeOS admission. Separately, the Product Owner tested real Google sign-in, invitation denial, owner access, session persistence, and logout on local Compose on 2026-10-09. Repeat this manual check for a new OAuth client or deployment environment.

The API runtime image includes the standard CA certificate bundle for HTTPS verification during Google OIDC discovery. If startup reports `x509: certificate signed by unknown authority`, inspect the runtime trust store and local HTTPS interception before changing trust settings. Never disable OIDC TLS verification to make a test pass.
