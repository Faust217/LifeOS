# Foundation Clean Start

Date: 2026-09-30  
Related issue: Foundation #15

## What Did We Add?

A single documented path now takes a clean LifeOS checkout through local configuration, pinned dependency bootstrap, source and contract checks, Compose startup, migration status, API readiness, and the minimal web shell. The path was exercised from an isolated committed snapshot.

## Why Does LifeOS Need It?

A foundation is useful only when another clean environment can reproduce it without hidden global tools, private data, or tribal knowledge. The workflow also proves that configuration, database, migration, API, contract generation, and web boundaries fit together before Finance work begins.

## What Should JIALE Learn Later?

- How lockfiles and pinned tool versions make builds repeatable.
- Why liveness, readiness, and migration status answer different operational questions.
- How Compose dependency health differs from simple container startup order.
- Why generated API types are checked for drift instead of edited by hand.
- Why normal shutdown preserves data and destructive volume removal needs a separate decision.

## Recommended Reading Order

1. `docs/FOUNDATION_STARTUP.md`
2. `.env.example` and `docs/CONFIGURATION.md`
3. `deploy/compose.yaml` and `deploy/README.md`
4. `db/migrations/README.md` and `cmd/lifeos-migrate/`
5. `cmd/lifeos-api/` and `internal/platform/health/`
6. `api/openapi/openapi.yaml` and `api/openapi/README.md`
7. `apps/web/src/api/` and `apps/web/README.md`
8. `.github/workflows/migrations.yml` and `scripts/verify-migrations.ps1`

The next learning checkpoint belongs to a separately authorized V0.1 Finance Core task, not this Foundation sprint.
