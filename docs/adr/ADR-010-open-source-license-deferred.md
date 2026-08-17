# ADR-010: Defer Open-Source License Selection

**Status:** Accepted

**Date:** 2026-08-17

## Context

LifeOS is intended to be self-hostable and may become open source, but the appropriate license depends on community and commercial intent that has not yet been decided.

Permissive licenses such as MIT or Apache-2.0 and a network-copyleft license such as AGPL express meaningfully different expectations. Selecting one without understanding the intended contribution, reuse, and hosted-modification model would create a difficult decision to reverse after public distribution.

## Decision

LifeOS will not select an open-source license in this ADR.

- License selection is intentionally deferred.
- A separate ADR must select and justify the license before the first public release.
- MIT, Apache-2.0, and AGPL are candidates for evaluation; this list is not a decision in favor of any one license.
- Public-release planning must treat the missing license decision as a blocking item.
- Product and architecture work may continue without pretending that the repository already has an accepted open-source license.

## Alternatives Considered

### Choose MIT now

This would be simple and permissive but would decide reuse rights before community/commercial intent is known.

### Choose Apache-2.0 now

This would be permissive with an explicit patent grant, but it still makes the core policy choice prematurely.

### Choose AGPL now

This could require network-hosted modifications to remain open, but that obligation should be chosen only after the desired hosting and contribution model is understood.

### Never release as open source

This remains a possible future strategic direction, but it is not the accepted product direction in the current blueprint and is not decided here.

## Consequences

### Positive

- The project avoids an accidental long-term license commitment.
- The eventual decision can reflect real community, contribution, and commercial goals.
- Current architecture work is not delayed by a policy question that is not yet at its decision point.

### Costs and Trade-Offs

- The project cannot complete a properly licensed public release until a later ADR is accepted.
- External contribution and redistribution expectations remain undefined for now.
- The license decision becomes harder if postponed beyond any public distribution, so the release gate must be enforced.

## Data Migration Impact

There is no application-data or schema impact. The later license decision affects repository distribution and governance rather than user data.

## Security and AI Impact

There is no direct runtime security or AI-permission change. When the license is selected, the project should review distribution implications for dependencies and bundled components, including any AI-provider or model assets that might be shipped.

## Rollback or Exit Path

A later license-selection ADR will supersede the deferral by choosing a license and documenting the reasoning. Before public release, changing the choice remains primarily a repository-governance action. After public releases or outside contributions, relicensing can become difficult, which is why the decision must be completed before that point.
