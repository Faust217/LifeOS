# LifeOS Migration Command

This repository-local Go command runs the pinned `golang-migrate` library against the PostgreSQL settings resolved by `internal/platform/config`.

Run it from the repository root:

```text
go run ./cmd/lifeos-migrate up
go run ./cmd/lifeos-migrate down
go run ./cmd/lifeos-migrate version
```

`down` rolls back exactly one version. Review the migration and backup requirements before using it. Dirty-state repair and `force` are deliberately not ordinary command actions.
