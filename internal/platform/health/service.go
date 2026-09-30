// Package health evaluates operational liveness dependencies without domain behavior.
package health

import "context"

const (
	StatusReady       = "ready"
	StatusUnavailable = "unavailable"
	StatusUnknown     = "unknown"
	StatusMissing     = "missing"
	StatusDirty       = "dirty"
	StatusBehind      = "behind"
	StatusAhead       = "ahead"
)

const (
	ErrorDatabaseUnavailable        = "database_unavailable"
	ErrorMigrationStatusUnavailable = "migration_status_unavailable"
	ErrorMigrationNotReady          = "migration_not_ready"
	ErrorMigrationDirty             = "migration_dirty"
)

// Database exposes only the operational PostgreSQL checks required by readiness.
type Database interface {
	Ping(context.Context) error
	MigrationVersion(context.Context) (version uint, dirty bool, present bool, err error)
}

// Result is the transport-neutral operational readiness result.
type Result struct {
	Ready            bool
	DatabaseStatus   string
	MigrationStatus  string
	MigrationVersion uint
	HasVersion       bool
	ExpectedVersion  uint
	Dirty            bool
	ErrorCode        string
}

// Service evaluates database connectivity and migration state.
type Service struct {
	database        Database
	expectedVersion uint
}

// NewService creates a readiness service for the required production migration version.
func NewService(database Database, expectedVersion uint) *Service {
	return &Service{database: database, expectedVersion: expectedVersion}
}

// Check evaluates PostgreSQL connectivity before inspecting migration metadata.
func (service *Service) Check(ctx context.Context) Result {
	result := Result{
		DatabaseStatus:  StatusUnavailable,
		MigrationStatus: StatusUnknown,
		ExpectedVersion: service.expectedVersion,
	}

	if err := service.database.Ping(ctx); err != nil {
		result.ErrorCode = ErrorDatabaseUnavailable
		return result
	}
	result.DatabaseStatus = StatusReady

	version, dirty, present, err := service.database.MigrationVersion(ctx)
	if err != nil {
		result.ErrorCode = ErrorMigrationStatusUnavailable
		return result
	}
	if !present {
		result.MigrationStatus = StatusMissing
		result.ErrorCode = ErrorMigrationNotReady
		return result
	}

	result.HasVersion = true
	result.MigrationVersion = version
	result.Dirty = dirty
	if dirty {
		result.MigrationStatus = StatusDirty
		result.ErrorCode = ErrorMigrationDirty
		return result
	}
	if version < service.expectedVersion {
		result.MigrationStatus = StatusBehind
		result.ErrorCode = ErrorMigrationNotReady
		return result
	}
	if version > service.expectedVersion {
		result.MigrationStatus = StatusAhead
		result.ErrorCode = ErrorMigrationNotReady
		return result
	}

	result.Ready = true
	result.MigrationStatus = StatusReady
	return result
}
