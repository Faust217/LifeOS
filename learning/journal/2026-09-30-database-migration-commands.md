# Foundation Database Migration Commands

## What did we add?

A pinned, repository-local `golang-migrate` command, an immutable no-domain production baseline, and an isolated migration smoke fixture.

## Why does LifeOS need it?

LifeOS must upgrade years of personal data predictably without resetting the database or rewriting migration history.

## What should JIALE learn later?

Study ordered up/down migrations, explicit PostgreSQL transactions, schema-version and dirty-state metadata, backup boundaries, and why `force` records recovery rather than performing it.
