# ADR-005: AI-Optional Core and Controlled Tool Gateway

**Status:** Accepted

**Date:** 2026-08-17

## Context

LifeOS is intended to remain useful when no AI provider is configured. Models and providers can change, fail, become expensive, or be unavailable. AI also creates safety risks if it can bypass application rules, view excessive personal data, or perform destructive actions without confirmation.

Structured domain data must remain the source of truth, and deterministic code must own money arithmetic, split netting, budget deltas, and authorization.

## Decision

AI is an optional capability layer, never a core dependency or data authority.

- `None` is always a valid runtime AI configuration.
- Every implemented core workflow has a complete manual path.
- Models never receive database credentials, unrestricted SQL, another owner's data, provider secrets, arbitrary filesystem access, or unregistered destructive tools.
- AI actions pass through a Tool Gateway: Tool Registry, schema validation, owner/policy checks, confirmation when required, application service validation, repository, and audit.
- Risk tiers are R0 read, R1 draft/create, R2 modify, and R3 destructive/bulk.
- R1 and R2 actions require appropriate preview or explicit confirmation; R3 requires stronger confirmation and is disabled for external AI by default.
- Context is task-specific and minimal rather than a dump of raw history.
- AI may interpret, extract, recommend, or explain; deterministic Go/SQL code calculates authoritative results.

## Alternatives Considered

### AI-first product

Make chat or a model the main or only way to use LifeOS. This would make core workflows dependent on provider availability and model behavior.

### Direct model access to SQL or repositories

Allow a model to query or modify storage freely. This bypasses business validation, owner boundaries, confirmations, and stable contracts.

### Read-only AI forever

Permit summaries but never controlled writes. This is safer but unnecessarily prevents useful future actions that can be validated, confirmed, and audited.

## Consequences

### Positive

- LifeOS remains fully useful when AI is disabled or a provider changes.
- AI behavior uses the same domain rules as manual clients.
- Permission tiers, confirmation, and audit make write capabilities explicit.
- Context minimization reduces privacy exposure and token growth.

### Costs and Trade-Offs

- Every AI capability requires a registered schema, policy, service path, tests, and audit behavior.
- Some apparently convenient autonomous actions will require confirmation.
- The product must maintain complete manual interfaces alongside AI enhancements.

## Data Migration Impact

Core Finance, Planning, Wishlist, Asset, People, and Split schemas must not depend on an AI provider. Optional provider configuration and tool-invocation audit data belong in separate support records. Disabling AI must not require domain-data migration.

## Security and AI Impact

This ADR is the main AI security boundary. A model request is not authorization. The Tool Registry and Policy Engine decide what is allowed, and application services enforce owner scope and domain invariants. Sensitive full prompts are not persisted by default.

## Rollback or Exit Path

AI can be disabled by selecting the `None` provider and removing or disabling adapters/tools; core data and manual workflows remain intact. Any proposal for direct database access, implicit destructive permission, or AI-only core behavior requires a superseding ADR with a security and data-recovery plan.
