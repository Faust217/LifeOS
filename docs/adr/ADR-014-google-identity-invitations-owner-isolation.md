# ADR-014: Google Identity, Invitations, Sessions, and User Isolation

**Status:** Accepted

**Date:** 2026-10-09

## Context

Issue #16 approves a small invite-only multi-user Alpha. ADR-008's owner-scoping principle remains useful, but its single-user assumption and deferred remote authentication no longer fit V0.1. Google sign-in proves an identity; it does not establish LifeOS admission or permission. The current Foundation API has only operational endpoints and no identity or user-data schema.

## Decision

- Use Google OpenID Connect for V0.1 sign-in. The server must validate the identity assertion and bind the stable Google `sub` claim to a separate LifeOS user/owner ID. Email is display/contact and invitation-matching information, not the permanent user key.
- Admission requires a current LifeOS invitation in addition to successful Google authentication. If an invitation is matched by email, require Google's verified-email claim before matching; bind the admitted account to stable `sub` afterward. No public self-registration or automatic admission follows a Google login. The exact invitation provisioning and expiry fields are implementation details to specify in the first identity issue.
- Establish a revocable LifeOS session after both identity and invitation checks pass. Protected requests must validate the session and current access status. Sign-out terminates the session; expired or revoked sessions fail closed.
- Use secure browser session handling for the Web/PWA: HTTPS in exposed environments, HttpOnly and Secure cookies, an appropriate SameSite policy, and CSRF protection for state-changing requests. Do not expose Google or LifeOS credentials in browser-visible configuration or logs.
- Each admitted LifeOS user is an independent owner boundary. Derive `owner_id` from verified server-side session context. Application services and repositories scope every user-owned read, write, list, summary, import/export, and future sync action; never treat a client-supplied owner ID as authorization.
- Keep owner-only administration separate from ordinary user capabilities. Only an explicitly designated Product Owner administrator may manage invitations or later view aggregate operational metrics. No ordinary user can elevate themselves. A future Admin Dashboard must avoid unnecessary private financial detail and have its own authorization and audit tests.
- Test both successful access and denied cases: uninvited Google user, invalid identity assertion, expired/revoked session, cross-user record ID, export isolation, and admin-path access.

This ADR sets boundaries, not endpoint names, table schemas, a hosted identity vendor, or a broad role system. The first identity implementation issue must document the concrete invitation bootstrap/revocation path and session lifecycle before coding. Acceptance of this ADR does not authorize identity, user-owned schema, or network-exposed product implementation.

## Alternatives Considered

- Treat any Google login as admission: conflicts with the approved invite-only Alpha.
- Use email as the permanent account key: email can change, risking incorrect account binding.
- Trust a client-provided owner ID or UI-only filtering: permits cross-user data exposure when requests are changed or new clients appear.
- Build organizations and complex RBAC now: exceeds a few independent invited users and the owner-only admin boundary.

## Consequences

Identity and invitation checks precede product access. Session revocation and owner scope must be reusable across modules, while service and repository tests prove isolation. The system can later add another login provider without changing LifeOS owner IDs or domain semantics. Invitation administration needs a small, auditable provisioning path even though a full Admin Dashboard is deferred.

## Data Migration Impact

No schema changes occur in this ADR. Later issues must add new numbered migrations for LifeOS users/owners, Google identity bindings, invitations, and any persisted session or audit state they choose. User-owned domain tables carry owner scope from their first migration. Upgrade tests must cover clean creation and supported upgrades without resetting personal data. Login-provider changes must preserve the LifeOS owner ID and its data relationships.

## Security and AI Impact

Server validation must check Google token signature, issuer, audience, expiry, and applicable replay/anti-forgery state before trusting identity. Invitation checks and session authorization are separate from authentication. Secrets and tokens remain server-side. Logs omit credentials and sensitive personal data. Owner-only admin actions are separately authorized and auditable; aggregate metrics do not justify raw cross-user finance access. Future AI/MCP adapters must establish the same authorized owner context and retain their stricter tool policies.

## Rollback or Exit Path

Disable new invitations or exposed access without deleting user data. Revoke sessions and credentials if the identity provider or deployment is compromised. A future provider migration maps new verified identities to existing LifeOS owner IDs through a reviewed, auditable flow. Removing owner isolation or admitting public users requires a superseding ADR, migration plan, and security review.

## References

- [Google OpenID Connect validation](https://developers.google.com/identity/openid-connect/openid-connect)
- [Google Sign in best practices and stable `sub`](https://developers.google.com/identity/siwg/best-practices)
- [OWASP multi-tenant security guidance](https://cheatsheetseries.owasp.org/cheatsheets/Multi_Tenant_Security_Cheat_Sheet.html)
