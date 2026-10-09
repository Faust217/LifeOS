# Issue #18: Identity Persistence Foundation

## What changed and why

Issue #18 adds three tables through `db/migrations/000002_identity_foundation.up.sql`: `lifeos_users` has an internal UUID and an active/disabled status; `google_identities` binds one unique Google `sub` to one internal user; `invitations` records who invited whom and the acceptance or revocation timestamps. Email helps match a verified invitation but is not the permanent identity key. Expiry is derived from `expires_at` at read time, so a background expiry job is unnecessary.

`internal/identity/repository.go` uses existing `pgx` access. The package-private `acceptInvitation` locks one invitation, checks its verified email and expiry, creates the user and Google binding, and consumes the invitation in one transaction. A failed insertion rolls the whole transaction back. Database checks, unique indexes, and foreign keys protect the same relationships if later code makes a mistake.

UUIDv4 is generated locally with Go's cryptographic random source, without a new dependency. The architecture prefers UUIDv7 when useful, but both are stable opaque UUIDs; a future ID generator can change without replacing existing user IDs.

## Small Alpha operator path

The package-private `bootstrapOwnerInvitation` and `revokeBootstrapInvitation` require an internal `operatorAuthorization{approved: true}`. There is no production issuer for that authorization and no HTTP route in #18, so bootstrap fails closed until a later trusted local operator command authenticates the operator. With no users present, the operator can create one short-lived owner invitation for a chosen email. The same row can be revoked or corrected before first admission, avoiding a locked-out Alpha if the address is mistyped or expires. Only a later verified Google identity with the matching verified email can consume that designated bootstrap invitation and become the sole owner administrator; an ordinary invitation never grants that flag, even when accepted first or concurrently. Subsequent package-private `createInvitation` and `revokeInvitation` calls check that the supplied internal user ID belongs to an active owner administrator. A future authenticated service must derive that ID from the server-side session, never from client input. An accepted invitation cannot be revoked retroactively; disabling a user's access and revoking sessions belong to later issues.

This is a persistence foundation, not an operational invitation command or sign-in flow. Before Alpha use, a separately reviewed local operator command and OIDC/session service must call these repository methods securely. Do not expose bootstrap to ordinary network requests.

## Function-level Data Flow

The exercised input is a package-internal test caller supplying an invitation ID and `verifiedGoogleIdentity`. That type and the admission method are unexported. No production code can construct or pass it today. A later server-side OIDC verifier must validate signature, issuer, audience, expiry, email verification, and anti-forgery state before constructing it; repository admission must never treat browser claims as proof.

| Step | Actual path/function | Input → output, logic, next call, errors/tests |
| --- | --- | --- |
| User input | **N/A**: no user-facing flow in #18 | The integration test supplies an invitation ID and package-private identity value. A later sign-in flow will receive the user's Google response. |
| Frontend function | **N/A**: no frontend authentication in #18 | A later issue will add the function that starts Google sign-in. |
| API request | **N/A**: no identity endpoint in #18 | A later issue will define the request and its CSRF/anti-forgery protection. |
| Backend handler | **N/A**: no identity handler in #18 | A later handler must pass the Google response to server-side verification, not copy browser claims into repository inputs. |
| Service | **N/A**: no OIDC/session service in #18 | A later service must verify signature, issuer, audience, expiry, verified email, and anti-forgery state before it can construct `verifiedGoogleIdentity` and call `(*repository).acceptInvitation`. |
| Repository admission | `internal/identity/repository.go` `(*repository).acceptInvitation` | Input: invitation UUID and package-private `verifiedGoogleIdentity`. Output: internal `User` or an error. Normalizes email, requires `emailVerified`, starts a transaction, and calls SQL `SELECT ... FOR UPDATE`. Invalid input returns `ErrInvalidInput`; missing, expired, revoked, mismatched, or consumed invitations return `ErrInvitationUnavailable`. Tested in `repository_integration_test.go`. |
| Database read | `db/migrations/000002_identity_foundation.up.sql` `invitations` table; `acceptInvitation`'s `SELECT ... FOR UPDATE` | Input: invitation ID. Output: stored email, expiry, outcome, bootstrap flag, database time. A row lock serializes competing acceptances. Next: `acceptInvitation` inserts user and binding. Missing row and invalid state fail closed; concurrent acceptance test covers this. |
| Database write | `acceptInvitation`'s `INSERT INTO lifeos_users`, `INSERT INTO google_identities`, `UPDATE invitations` | Input: new UUID, normalized email, unique Google `sub`, invitation ID. Output: one durable user/binding/accepted invitation on commit. Constraints and indexes in migration 000002 reject duplicate subjects, multiple admins, invalid invitation shapes, or broken foreign keys. On any failure, deferred rollback leaves the invitation usable. Tested by duplicate-sub and constraint cases. |
| Response | `acceptInvitation` returns `User` or error | The returned internal ID is the later owner boundary. `(*repository).findByGoogleSub` resolves that same ID without using email as a key. Tests verify the binding and duplicate-sub rollback. |
| UI update | **N/A**: no UI in #18 | A later sign-in issue will map a successful admission to a protected session and UI state. |

The future local operator path will call `(*repository).bootstrapOwnerInvitation` or `(*repository).revokeBootstrapInvitation` after issuing the internal operator authorization; these use a transaction and PostgreSQL advisory transaction lock, then write or revoke the single bootstrap row. The operator command is not implemented in #18. Normal invitation administration calls `(*repository).createInvitation` or `(*repository).revokeInvitation`; their SQL checks the active owner-admin record before changing an invitation.

## Reading order and checks

Read ADR-014, migration 000002, `internal/identity/repository.go`, then its unit and integration tests. The PostgreSQL integration test was explicitly run with `LIFEOS_TEST_DATABASE_URL` and reported `--- PASS`, so it did not skip. It upgraded an empty disposable database from version 1 to 2 and exercised operator authorization denial, ordinary-user administrator isolation during concurrent acceptance, uniqueness, invalid states, expiry, revocation, and duplicate-sub rollback. It also attempted a down migration after user data existed: the migration refused to drop the tables and the user rows survived. That refusal may leave `golang-migrate` metadata dirty in the disposable database; on real data, stop and follow manual recovery rather than force or reset. A separate clean disposable database passed `scripts/verify-migrations.ps1` at version 2 with `dirty=false`; `go test -count=1 ./...`, `go vet ./...`, and `git diff --check` passed. No OAuth, session, frontend, or finance behavior is tested because those do not exist in #18.

## Open decisions

- A later issue must choose the trusted local operator command and its audit/logging behavior before Alpha invitations can be provisioned operationally.
- A later OIDC/session issue must verify Google signatures, issuer, audience, expiry, and anti-forgery state; it must derive admin identity from a protected session.
- User disablement and session revocation need a later access-lifecycle design. The schema reserves `disabled`, but #18 does not expose it as a workflow.
