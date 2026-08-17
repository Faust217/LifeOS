# ADR-006: AI Provider Abstraction with Ollama First

**Status:** Accepted

**Date:** 2026-08-17

## Context

Runtime AI providers differ in privacy, cost, availability, model quality, tool calling, structured output, vision, quotas, and hardware requirements. Provider names, model names, prices, and subscription capabilities will change over the lifetime of LifeOS.

LifeOS must not couple finance, planning, assets, or other core data to one vendor. It also must not assume that a consumer chat subscription provides runtime API access.

## Decision

LifeOS will place runtime AI behind a provider-neutral AI Gateway.

- `None` is a permanent provider option and preserves complete no-AI operation.
- Ollama/local models are the first experimental implementation path for structured-output and tool-calling work.
- OpenAI API, Gemini API, and other compatible cloud providers may be added later when a real use case requires them.
- Provider selection is based on declared capabilities such as tool calling, structured output, and vision rather than a hard-coded model name.
- Provider-specific configuration and metadata remain outside core domain tables.
- API credentials and provider secrets are configuration secrets and are never exposed to clients or models.
- The architecture will implement only providers required by an actual milestone; it will not build every possible adapter in advance.

## Alternatives Considered

### Lock LifeOS to one cloud provider

This could reduce initial adapter work but would create vendor, price, availability, and data-model coupling.

### Support many providers immediately

This appears flexible but would create unneeded integrations and testing work before the product has proven AI use cases.

### Support local models only forever

This maximizes local control but may prevent future tasks that need capabilities unavailable or impractical on the owner's hardware.

## Consequences

### Positive

- Providers and models can change without changing core domain semantics.
- Local experimentation is possible without a cloud API bill.
- Cloud BYOK options can be added later without becoming mandatory.
- Capability-based selection avoids treating temporary model names as architecture.

### Costs and Trade-Offs

- The gateway needs a small common contract and provider-specific adapters.
- Provider behavior will not be perfectly identical, so capability checks and adapter tests are required.
- Local model quality and speed depend on the owner's hardware and cannot be assumed to match cloud models.

## Data Migration Impact

Provider configuration and invocation metadata must be isolated from core domain records. Adding or removing a provider should require configuration or support-table changes, not Finance/Planning/Asset schema changes. Provider-specific metadata may be retained only where needed for audit or operations.

## Security and AI Impact

Local models can reduce cloud data exposure but do not bypass Tool Gateway permissions. Cloud providers require explicit credentials and task-appropriate context minimization. Provider secrets must not appear in logs, web responses, prompts, or tool results.

## Rollback or Exit Path

Any provider adapter can be disabled or removed while `None` preserves core behavior. A provider change should not migrate core records. Replacing the gateway abstraction or making one provider mandatory requires a superseding ADR that covers portability, secret handling, compatibility, and no-AI operation.
