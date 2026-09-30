# Foundation Compose Stack

## What did we add?

Reproducible API and web container builds plus one Compose stack that orders PostgreSQL, migrations, API readiness, and the web shell.

## Why does LifeOS need it?

The local Foundation must start predictably with private service communication, localhost-only ports, persistent database storage, and no browser-visible database secrets.

## What should JIALE learn later?

Study multi-stage images, Compose health dependencies, one-shot migration jobs, container DNS, same-origin proxying, and the difference between build-time browser values and server secrets.
