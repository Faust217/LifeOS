# Foundation API Health and Readiness

## What did we add?

A minimal `chi` HTTP server with liveness, PostgreSQL readiness, migration-version checks, request IDs, structured logs, and focused tests.

## Why does LifeOS need it?

Operators and Compose need to distinguish a running process from one that is actually ready to use its required database schema.

## What should JIALE learn later?

Study liveness versus readiness, thin HTTP handlers, dependency injection for tests, graceful shutdown, and why operational errors expose stable codes instead of secret connection details.
