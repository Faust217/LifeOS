# ADR-007: MCP as an External Adapter

**Status:** Accepted

**Date:** 2026-08-17

## Context

LifeOS may later be used from ChatGPT, Codex, or another compatible external AI client through MCP. Product capabilities and subscription rules for those clients can change, and a chat subscription is not the same thing as runtime API credit.

If MCP owns separate business logic or becomes required for normal use, LifeOS would be coupled to an external client and could behave differently from the Web/PWA.

## Decision

MCP will be an optional external adapter over the same Tool Registry and application services used by runtime AI and other clients.

- MCP is not a core product dependency.
- Core manual workflows remain complete without MCP or any external AI client.
- MCP tools use registered schemas, permission tiers, confirmation policy, owner scope, application validation, and audit.
- A network-exposed MCP endpoint requires its own authentication and authorization.
- Write tools default to conservative permissions, and destructive/bulk tools are disabled for external AI by default.
- MCP-specific session and adapter metadata remain outside core domain tables.
- MCP implementation is deferred to its roadmap milestone; earlier work only preserves the boundary.

## Alternatives Considered

### Make an external AI client the primary interface

This could offer a conversational experience quickly but would make LifeOS dependent on another product's availability, subscription, and UI behavior.

### Implement separate MCP business logic

This could optimize directly for an AI client but would duplicate rules and allow MCP behavior to diverge from the core application.

### Never support external tools

This avoids integration risk but gives up a useful future access path that can remain safely optional.

## Consequences

### Positive

- External clients can be added or disconnected without changing core behavior.
- The same validation and permission rules apply across Web/PWA, runtime AI, and MCP.
- LifeOS is not tied to ChatGPT or another particular client.

### Costs and Trade-Offs

- The adapter needs separate authentication, transport handling, and compatibility testing.
- Tool contracts must remain stable enough for external clients.
- Some external-client actions will require explicit previews or confirmations.

## Data Migration Impact

MCP does not own Finance, Planning, Wishlist, Asset, People, or Split data. Adding or removing the adapter should not migrate core records. Only optional integration/session metadata and audit records may require support migrations.

## Security and AI Impact

An MCP client's request is not authorization. The Tool Gateway decides whether a tool is registered and permitted. Remote exposure requires authentication, least-privilege tools, safe logging, confirmation for writes, and audit. Database credentials, provider secrets, and arbitrary filesystem access are never exposed.

## Rollback or Exit Path

Disable or remove the MCP adapter and revoke its credentials; the Web/PWA, API, domain data, and manual workflows continue. If a future client uses another protocol, it can implement a new adapter over the same application services. Making MCP part of the core requires a superseding ADR.
