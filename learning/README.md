# LifeOS Learning Area

The `learning/` directory records the owner's engineering learning while LifeOS becomes a real product. It is intentionally separate from normative project documentation.

## What Belongs Here

- what a scoped implementation changed;
- why the chosen design was used;
- relevant Go, SQL, PostgreSQL, API, testing, security, architecture, or operations concepts;
- a recommended order for reading the changed files;
- commands or experiments that are safe and useful to repeat;
- unresolved questions and topics to revisit.
- for every implementation issue, a Function-level Data Flow using the actual code path required by `AGENTS.md`.

## What Does Not Belong Here

- architecture or domain decisions that should be an ADR;
- the only copy of a product requirement or project status;
- secrets, API keys, passwords, raw private financial data, or sensitive prompts;
- copied chat transcripts without durable technical value;
- claims that have not been verified against the implementation.

Normative material belongs in `docs/`. Current handoff state belongs in `docs/PROJECT_STATE.md`. Architecture decisions belong in `docs/adr/`.

## Structure

```text
learning/
├─ README.md
├─ LEARNING_MAP.md
└─ journal/
```

`LEARNING_MAP.md` is the maintained skill and topic map. `journal/` contains dated entries tied to actual work.

## Journal Naming

Use:

```text
YYYY-MM-DD-short-topic.md
```

If several entries share a day, add a clear topic or sequence suffix.

## Suggested Journal Entry

```markdown
# Topic

Date: YYYY-MM-DD
Related task/issue:
Related ADRs:

## What Changed

## Why This Design

## Concepts Learned

## Recommended Reading Order

## Function-level Data Flow

User Input → Frontend function → API request → Backend handler → Service → Repository → Database → Response → UI update.

For each step: exact file path and function name; input and output; main processing; next function; error handling; relevant tests. Label not-yet-implemented names **proposed** and explain **N/A** steps. Replace proposed names with actual names before issue closure.

## Checks Performed

## Questions and Follow-Up
```

Every implementation issue needs a learning entry and the same concise flow in its completion report. Documentation-only or trivial non-implementation edits need an entry only when there is something concrete to learn from.
