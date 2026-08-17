# LifeOS Project State

**Last updated:** 2026-08-17

**Current milestone:** Foundation

**Product milestone:** V0.1 Finance Core is scoped but not started

**Blueprint:** Master Project Blueprint v0.2

## Current Status

The repository documentation and governance foundation has been established from the English and Chinese v0.2 Master Project Blueprints. No product features, application skeleton, dependencies, migrations, or deployment configuration have been implemented.

## Completed

- Preserved both Master Project Blueprint v0.2 reference documents under `docs/reference/`.
- Created the root agent governance and local-machine safety policy in `AGENTS.md`.
- Replaced the placeholder README with project orientation and the documentation reading path.
- Extracted the product vision into `docs/VISION.md`.
- Extracted the architecture baseline into `docs/ARCHITECTURE.md`.
- Defined the conceptual domain vocabulary and invariants in `docs/DOMAIN_MODEL.md`.
- Defined the Finance Core milestone in `docs/V0.1_SCOPE.md`.
- Recorded the Foundation-to-V1.0 capability sequence in `docs/ROADMAP.md`.
- Established ADR policy, naming, template, and the initial decision-record backlog in `docs/adr/README.md`.
- Established the non-normative learning area, learning map, and journal guidance.

## In Progress

None. The current foundation task ends with documentation and governance only.

## Next Authorized Work

No implementation should begin without a new explicit task. The next useful sequence from the blueprint is:

1. Review and accept this extracted documentation baseline.
2. Record the accepted baseline decisions as dedicated ADR-001 through ADR-010 files.
3. Create the Foundation milestone/issues with objective acceptance criteria.
4. Inspect the existing development environment before proposing project setup changes.
5. Obtain and anonymize a representative Bluecoins CSV sample, then analyze its fields before implementing an importer.
6. In a separately authorized implementation task, initialize the Docker/PostgreSQL/Go/Web skeleton and migration path.

## Known Risks

| Risk | Current mitigation |
| --- | --- |
| Scope creep | Keep V0.1 limited to the published scope; put new ideas in a backlog |
| Over-engineering | Preserve boundaries but implement one database, provider path, and deployment path only when needed |
| Financial correctness | Integer money, deterministic calculations, service/integration tests, and import dry-runs |
| Database redesign and data loss | Stable meanings, forward migrations, upgrade tests, export, and restore-tested backups |
| AI vendor churn or unsafe writes | Provider abstraction, Tool Gateway, explicit capability tiers, confirmation, and audit |
| Self-host security mistakes | Local-only default; require authentication, TLS, and backup before Internet exposure |
| Project abandonment | Make V0.1 replace the real Bluecoins workflow and keep every milestone independently useful |
| Agent drift | Required reading, module rules, ADR gate, tests, and completion reporting in `AGENTS.md` |

## Pending Decisions

These decisions are intentionally deferred to their real decision points:

| Decision | Latest decision point | Baseline direction |
| --- | --- | --- |
| Migration tool | Before the first real migration | Choose `golang-migrate` or `goose` and record an ADR |
| Go HTTP router | Foundation implementation | Standard library or `chi`; keep handlers thin |
| Web UI component library | Before the first production UI screen | Choose an actively maintained, accessible library |
| Authentication implementation | Before network/Internet exposure | Start local-owner-first; require secure login remotely |
| First Ollama model | V0.3 | Evaluate against local hardware and tool-calling needs |
| Open-source license | Before the first public release | Decide through a separate ADR |
| Native Flutter client | After real Web/PWA use | Begin only if a native requirement is proven |

## Resume Checklist

When work resumes:

1. Read `AGENTS.md`.
2. Read `docs/VISION.md`, `docs/ARCHITECTURE.md`, and `docs/DOMAIN_MODEL.md`.
3. Read `docs/V0.1_SCOPE.md` and this file.
4. Read the relevant ADRs.
5. Confirm the task's acceptance criteria and non-goals.
6. Inspect the existing local environment before proposing setup or dependency changes.

## State History

### 2026-08-17

- Established the documentation, governance, ADR, project-state, and learning foundation from Master Project Blueprint v0.2.
- Kept all application implementation explicitly out of this task.
