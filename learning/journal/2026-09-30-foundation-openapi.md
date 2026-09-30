# Foundation OpenAPI Contract

## What did we add?

A canonical OpenAPI 3.1 document for the existing liveness and readiness endpoints plus pinned structural/lint validation.

## Why does LifeOS need it?

The server and every future client need one reviewable external contract that is separate from database tables and router implementation details.

## What should JIALE learn later?

Study OpenAPI paths, response schemas, stable error codes, additive versioning, and the distinction between transport DTOs and persistence models.
