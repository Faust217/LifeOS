# Migration Chain CI

## What did we add?

A shared migration verification script and a focused GitHub Actions workflow using a clean PostgreSQL 18 service.

## Why does LifeOS need it?

Every migration change must prove that an empty database reaches the latest clean version and that the isolated rollback fixture still behaves predictably.

## What should JIALE learn later?

Study CI service containers, deterministic upgrade-chain checks, migration ordering failures, and why CI credentials must be disposable and never logged.
