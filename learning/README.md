# LifeOS Learning Area

The `learning/` directory records the owner's engineering learning while LifeOS becomes a real product. It is intentionally separate from normative project documentation.

## What Belongs Here

- what a scoped implementation changed;
- why the chosen design was used;
- relevant Go, SQL, PostgreSQL, API, testing, security, architecture, or operations concepts;
- a recommended order for reading the changed files;
- commands or experiments that are safe and useful to repeat;
- unresolved questions and topics to revisit.

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

## Checks Performed

## Questions and Follow-Up
```

Only create an entry when there is something concrete to learn from. The journal is not a mandatory paperwork step for trivial edits.
