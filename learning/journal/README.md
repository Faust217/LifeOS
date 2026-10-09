# Learning Journal

This directory holds dated, non-normative notes from concrete LifeOS work.

Use filenames such as `YYYY-MM-DD-short-topic.md`. A useful entry explains what changed, why, the concepts involved, a recommended file-reading order, checks performed, and unresolved questions.

For every implementation issue, include a concise Function-level Data Flow using actual file paths and function names: User Input → Frontend function → API request → Backend handler → Service → Repository → Database → Response → UI update. Explain each step's input/output, logic, next call, error handling, and relevant tests. Mark proposed names and reasoned N/A steps, then update to actual code before the issue closes. Put the same flow in the issue completion report.

Do not store secrets, API keys, passwords, raw private financial data, or full sensitive prompts here. Move durable product rules to `docs/`, architectural decisions to `docs/adr/`, and current handoff status to `docs/PROJECT_STATE.md`.

Current entries:

- [2026-10-09: Google OIDC and sessions](2026-10-09-google-oidc-sessions.md) - server verification, invitations, protected cookies, local operator, tests, and function-level data flow.
- [2026-10-09: Identity persistence foundation](2026-10-09-identity-persistence.md) - user and invitation schema, Google `sub` binding, operator bootstrap boundary, and function-level data flow.
- [2026-08-17: Monorepo skeleton](2026-08-17-monorepo-skeleton.md) - dependency-free Go/web workspace structure, ownership boundaries, and structural checks.
