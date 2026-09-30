package health

import (
	"context"
	"errors"
	"testing"
)

type fakeDatabase struct {
	pingErr    error
	version    uint
	dirty      bool
	present    bool
	versionErr error
}

func (database fakeDatabase) Ping(context.Context) error {
	return database.pingErr
}

func (database fakeDatabase) MigrationVersion(context.Context) (uint, bool, bool, error) {
	return database.version, database.dirty, database.present, database.versionErr
}

func TestServiceReady(t *testing.T) {
	result := NewService(fakeDatabase{version: 1, present: true}, 1).Check(context.Background())
	if !result.Ready || result.DatabaseStatus != StatusReady || result.MigrationStatus != StatusReady {
		t.Fatalf("unexpected readiness result: %+v", result)
	}
}

func TestServiceDatabaseUnavailable(t *testing.T) {
	result := NewService(fakeDatabase{pingErr: errors.New("unavailable")}, 1).Check(context.Background())
	if result.Ready || result.ErrorCode != ErrorDatabaseUnavailable {
		t.Fatalf("unexpected readiness result: %+v", result)
	}
}

func TestServiceMigrationNotReady(t *testing.T) {
	testCases := map[string]fakeDatabase{
		"missing":     {present: false},
		"behind":      {version: 0, present: true},
		"ahead":       {version: 2, present: true},
		"dirty":       {version: 1, dirty: true, present: true},
		"query error": {versionErr: errors.New("query failed")},
	}

	for name, database := range testCases {
		t.Run(name, func(t *testing.T) {
			result := NewService(database, 1).Check(context.Background())
			if result.Ready || result.ErrorCode == "" {
				t.Fatalf("unexpected readiness result: %+v", result)
			}
		})
	}
}
