# Internal Application Modules

LifeOS is one Go modular monolith. Each directory below is a code-ownership boundary, not a separately deployed service.

| Module | Ownership boundary |
| --- | --- |
| `identity` | Owner identity and access-boundary rules |
| `finance` | Accounts, financial events, movements, category allocations, and authoritative money rules |
| `planning` | Plans, travel intent, planned items, and checklists |
| `wishlist` | Desired purchases and their plan, asset, and purchase relationships |
| `assets` | Owned-item identity, acquisition, warranty, condition, and lifecycle |
| `people` | Local participant and contact records |
| `split` | Shared-cost ownership, shares, balances, and settlements |
| `analytics` | Deterministic projections over stable domain facts |
| `importing` | Source adapters, canonical import inputs, dry-runs, mapping, and batch orchestration |
| `ai` | Optional provider-neutral AI and controlled Tool Gateway orchestration |
| `integrations/mcp` | Optional MCP protocol adapter over registered tools and application services |
| `platform` | Database, configuration, logging, and authentication adapters |

## Boundary Rules

- The owning domain/application module contains its business rules and controls its persistence behavior.
- Modules communicate through explicit services or interfaces; they do not casually access another module's tables.
- `platform` supplies cross-cutting adapters and does not become a home for domain business rules.
- `ai` and `integrations/mcp` may invoke domain behavior only through the controlled application-service and Tool Gateway boundaries.
- Sharing one process and one future PostgreSQL database does not erase these ownership boundaries.

The domain packages remain documentation-only placeholders. `platform/config` implements the accepted Foundation runtime configuration boundary; no domain behavior or persistence implementation exists yet.
