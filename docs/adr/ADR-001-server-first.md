# ADR-001: Server-First, Client-Agnostic Core

**Status:** Accepted

**Date:** 2026-08-17

## Context

LifeOS is intended to be self-hostable, browser-accessible, and usable from more than one kind of client over time. V0.x needs a practical first client, while later versions may add a native app, CLI, runtime AI, or an external MCP client.

If business rules are implemented only inside the first user interface, every later client would need to reproduce them. That would create inconsistent behavior and make the first client difficult to replace. Dashboard layout is also presentation preference data, not a source of domain truth.

## Decision

LifeOS will be server-first and client-agnostic.

- The LifeOS server, domain model, and application services own business rules and authoritative behavior.
- Clients use stable API or tool contracts rather than accessing storage directly.
- V0.x will start with a responsive Web/PWA because it is practical for local and self-hosted use.
- A future native client, CLI, runtime AI, or MCP client must use the same application rules.
- Native development will begin only when real needs such as stronger offline behavior, notifications, widgets, or operating-system integration justify it.
- Dashboard composition and layout remain separate from domain data.

## Alternatives Considered

### Client-first application

Put the data model and business behavior primarily in the first web or desktop client. This may appear faster initially, but it would duplicate rules across later clients and weaken the server as the durable core.

### Native-first application

Start with Flutter or another native client. This adds early platform complexity before a native-only requirement has been proven.

### Build several clients immediately

Develop web, native, CLI, and AI integrations together. This increases scope and slows the first complete, usable workflow.

## Consequences

### Positive

- All clients share one set of finance, planning, asset, split, authorization, and validation rules.
- The Web/PWA can be replaced or supplemented without redefining core semantics.
- Self-hosting and future integrations use an explicit server boundary.
- Dashboard changes cannot silently change domain meaning.

### Costs and Trade-Offs

- Even local use requires a server process and API boundary.
- API contracts and server-side validation must be maintained from the beginning.
- Offline-first behavior is not the initial architectural priority.

## Data Migration Impact

There is no existing application data to migrate. Future client changes should not require domain-data migrations merely because presentation technology changes. Any breaking server contract must follow the API versioning and migration rules.

## Security and AI Impact

The server boundary centralizes owner scoping, authentication, authorization, validation, and audit. Any installation exposed beyond localhost requires appropriate authentication, HTTPS, and backup.

Runtime AI and external AI clients must call the same controlled application services as other clients. They do not receive direct database access.

## Rollback or Exit Path

The Web/PWA is replaceable because it is not the system of record. A different client can be added against the same contracts.

Moving business authority away from the server would change a non-negotiable principle and requires a superseding ADR that explains contract compatibility, data impact, and how multiple clients will remain consistent.
