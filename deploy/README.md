# Local PostgreSQL Development

`compose.yaml` provides the project-local PostgreSQL 18 service for Foundation development. It does not define API, web, or production services.

## Local Environment

Create a root `.env` from `.env.example` before starting Compose and replace `LIFEOS_DB_PASSWORD=CHANGE_ME` with a local development password. The `.env` file is Git-ignored and must never be committed. Run the commands below from the repository root; `--env-file .env` makes that root file explicit.

PostgreSQL is published only on `127.0.0.1` using `LIFEOS_DB_PORT` (default `5432`). It is not exposed on all network interfaces.

## Commands

Start only PostgreSQL:

```text
docker compose --env-file .env -f deploy/compose.yaml up -d postgres
```

Inspect container and health status:

```text
docker compose --env-file .env -f deploy/compose.yaml ps postgres
```

Stop and remove the Compose container and network:

```text
docker compose --env-file .env -f deploy/compose.yaml down
```

The health check uses `pg_isready` with the configured PostgreSQL database and user. Dependent services added in later issues must wait for PostgreSQL to report healthy.

## Data Persistence

PostgreSQL data is stored in the Docker named volume `postgres_data`, mounted at `/var/lib/postgresql` for PostgreSQL 18. A normal `docker compose down` preserves this volume and its development data.

Deleting the named volume destroys the disposable development database. Do not remove it unless that data loss is intentional and explicitly approved.
