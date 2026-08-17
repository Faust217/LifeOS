# ADR-008: Single-User Product with Owner-Scoped Data

**Status:** Accepted

**Date:** 2026-08-17

## Context

The real V0.x user is the project owner. Building organizations, billing, subscriptions, collaboration, and complex role-based access for hypothetical future users would add scope without current value.

However, storing all data without an owner boundary would make a future move to multiple users painful and would weaken authorization discipline even for a personal self-hosted system.

## Decision

V0.x will be a single-user personal product, while all core data is owner-scoped from day one.

- Core records include `owner_id`.
- Repositories and application services scope queries to the authenticated/current owner.
- Services do not trust a client-supplied arbitrary owner ID.
- Local personal use is the initial experience.
- Organizations, billing, subscriptions, collaboration, and complex RBAC are not pre-built.
- Secure authentication is required before an installation is exposed to a network or the Internet; the detailed authentication implementation remains deferred to that decision point.

## Alternatives Considered

### Truly unscoped single-user schema

Omit `owner_id` because only one person uses V0.x. This is simpler initially but creates a broad, risky migration if another owner or stronger authorization boundary is ever needed.

### Full multi-tenant SaaS model now

Build users, organizations, memberships, roles, billing, and quotas immediately. This would over-engineer V0.x around demand that does not yet exist.

## Consequences

### Positive

- V0.x stays focused on one owner's real needs.
- Owner isolation is a normal invariant rather than a late retrofit.
- A future multi-user mode has a clearer data boundary.
- Hosted-SaaS concerns do not distort current architecture.

### Costs and Trade-Offs

- Queries and tests must carry owner scope even when only one owner exists.
- `owner_id` adds schema and service discipline before the UI exposes multiple users.
- Owner scoping alone does not provide full multi-tenant security; future auth work remains necessary.

## Data Migration Impact

Every core table must include an owner relationship when first introduced. Imports are assigned to the current owner through application services. A future multi-user release may add accounts, authentication, invitations, or organizations, but existing records already have an ownership anchor.

## Security and AI Impact

Every API, AI tool, import, export, analytics query, and MCP call must operate within the authorized owner scope. AI must never receive another owner's data. A caller cannot select an arbitrary `owner_id` to bypass authorization.

## Rollback or Exit Path

LifeOS can remain single-user indefinitely without removing owner scope. A future multi-user or hosted mode requires a new ADR covering authentication, authorization, tenancy, operations, and migration. Removing owner scoping would weaken the accepted security boundary and requires a superseding ADR and data migration plan.
