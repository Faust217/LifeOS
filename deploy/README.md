# Local Foundation Stack

`compose.yaml` provides the local PostgreSQL 18, one-shot migration, Go API, and web shell services. Images build from repository source and committed lockfiles.

## Local Environment

Create a root `.env` from `.env.example` before starting Compose and replace `LIFEOS_DB_PASSWORD=CHANGE_ME` with a local development password. The `.env` file is Git-ignored and must never be committed. Run the commands below from the repository root; `--env-file .env` makes that root file explicit.

PostgreSQL, API, and web ports publish only on `127.0.0.1`. Defaults are PostgreSQL `5432`, API `8080`, and web `4173`. Containers communicate over the project-owned `foundation` bridge network. Database credentials are passed only to PostgreSQL, migration, and API services; they are never web build arguments or browser values.

## Commands

Build the API and web images:

```text
docker compose --env-file .env -f deploy/compose.yaml build api migrate web
```

Start the complete stack and wait for health:

```text
docker compose --env-file .env -f deploy/compose.yaml up -d --wait
```

Inspect status and logs:

```text
docker compose --env-file .env -f deploy/compose.yaml ps -a
docker compose --env-file .env -f deploy/compose.yaml logs api web postgres migrate
```

Open the web shell at `http://127.0.0.1:4173`. The API remains directly available at `http://127.0.0.1:8080`; the web server also proxies operational endpoints on its own origin.

Stop and remove containers and the project network:

```text
docker compose --env-file .env -f deploy/compose.yaml down
```

PostgreSQL must become healthy before the one-shot migration service runs. The API starts only after migrations succeed and becomes healthy only when PostgreSQL and the required migration version are ready. The web service starts after API health succeeds.

## Data Persistence

PostgreSQL data is stored in the Docker named volume `postgres_data`, mounted at `/var/lib/postgresql` for PostgreSQL 18. A normal `docker compose down` preserves this volume and its development data.

Deleting the named volume destroys the development database. Do not remove it unless that data loss is intentional and explicitly approved. Normal `down` preserves it.
