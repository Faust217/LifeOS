# LifeOS Foundation Configuration

**Status:** Foundation contract accepted; Issue #19 adds an identity configuration extension

**Last updated:** 2026-10-09

## Purpose and Boundary

The Foundation configuration contract covers runtime and platform concerns needed to connect the initial LifeOS processes:

- application environment;
- HTTP binding;
- PostgreSQL connectivity;
- logging level;
- web-client API location.

This contract defines names, defaults, validation, precedence, and secret-handling expectations. Issue #7 implemented the loader; Issue #19 extends it with server-only identity settings.

## Sources and Precedence

The application configuration loader resolves values in this order:

1. process environment variables;
2. the project-root `.env` file;
3. documented safe built-in defaults.

A higher-precedence value overrides a lower-precedence value. No CLI configuration flags or other configuration sources are part of the Foundation contract.

`.env.example` is a committed template, not a runtime configuration source. A developer may create a project-local `.env`, which Git ignores, and replace placeholders with local values. Real secrets must never be committed.

## Foundation Variables

| Key | Purpose | Default | Validation and sensitivity |
| --- | --- | --- | --- |
| `LIFEOS_ENV` | Application environment | `development` | One of `development`, `test`, or `production` |
| `LIFEOS_API_BIND` | API bind address | `127.0.0.1` | Must be a valid bind address; the default stays local-only |
| `LIFEOS_API_PORT` | API listening port | `8080` | Integer from `1` through `65535` |
| `LIFEOS_DB_HOST` | PostgreSQL host | `127.0.0.1` | Must identify a PostgreSQL host |
| `LIFEOS_DB_PORT` | PostgreSQL port | `5432` | Must be a valid TCP port |
| `LIFEOS_DB_NAME` | PostgreSQL database name | `lifeos` | Must be non-empty |
| `LIFEOS_DB_USER` | PostgreSQL user | `lifeos` | Must be non-empty |
| `LIFEOS_DB_PASSWORD` | PostgreSQL password | No default; required | Sensitive and non-empty; `CHANGE_ME` is always rejected as an unsafe placeholder |
| `LIFEOS_LOG_LEVEL` | Application log level | `info` | One of `debug`, `info`, `warn`, or `error` |
| `VITE_API_BASE_URL` | API location used by the web client | `http://127.0.0.1:8080` | Valid absolute `http` or `https` URL |
| `LIFEOS_WEB_PORT` | Compose-published web shell port | `4173` | Integer from `1` through `65535`; consumed by local Compose, not the Go server |

The PostgreSQL fields remain separate. LifeOS does not define a duplicate `DATABASE_URL` setting at this stage.

## Issue #19 Identity Extension

| Key | Purpose | Default | Validation and sensitivity |
| --- | --- | --- | --- |
| `LIFEOS_OIDC_CLIENT_ID` | Google Web application client ID | Empty in local Foundation mode | Required with the client secret to enable sign-in; required in production |
| `LIFEOS_OIDC_CLIENT_SECRET` | Server-side Google client secret | Empty in local Foundation mode | Required with client ID; never use a `VITE_*` key or commit it |
| `LIFEOS_OIDC_ISSUER` | OIDC discovery issuer | `https://accounts.google.com` | Fixed to Google outside tests; test mode permits a local provider double |
| `LIFEOS_AUTH_PUBLIC_ORIGIN` | Exact browser origin used for redirects and logout Origin checks | `http://127.0.0.1:4173` | Origin only, no path; HTTP is limited to localhost, and production requires HTTPS |

See [local authentication setup](AUTH_LOCAL_SETUP.md) for the Google redirect URI, invitation bootstrap, and demo procedure. The client ID may be public, but the secret and session cookies are server-only.

## Validation and Failure Behavior

The loader resolves the precedence chain and validates the resulting configuration before application startup. Startup fails when a value is:

- missing when required;
- malformed;
- outside its allowed range;
- an unsafe placeholder where a real secret is required.

Configuration errors must identify the affected key and the problem without echoing a secret value. The application must not silently continue with missing, invalid, or unsafe configuration.

## Sensitive Values and Logging

`LIFEOS_DB_PASSWORD` is sensitive. Errors and logs must never expose database passwords, secrets, tokens, or full sensitive connection strings.

Every `VITE_*` value is compiled or delivered to browser code and is therefore browser-visible. `VITE_API_BASE_URL` must never contain credentials, tokens, or any other secret.

`LIFEOS_WEB_PORT` is a Compose-only deployment value introduced when the web container became a real Foundation boundary. It does not belong to the Go runtime `Config` object.

This contract selects only the allowed log-level values. It does not select or implement a logging library.

## Future Configuration

Foundation configuration must not become an all-purpose application settings object. New values belong at the feature or configuration boundary that owns them.

The following are explicitly deferred:

- base currency and owner timezone/preferences;
- Finance or other domain settings;
- AI provider and model settings;
- MCP configuration;
- TLS;
- hosted secret management.

Each deferred area requires its real feature scope, validation rules, security review, and any necessary architecture decision before it is introduced.
