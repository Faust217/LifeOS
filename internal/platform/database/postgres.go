// Package database contains Foundation PostgreSQL adapters.
package database

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"

	"github.com/Faust217/LifeOS/internal/platform/config"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PostgreSQL owns the Foundation connection pool and operational queries.
type PostgreSQL struct {
	pool *pgxpool.Pool
}

// OpenPostgreSQL creates a lazy connection pool without logging its secret URL.
func OpenPostgreSQL(ctx context.Context, databaseConfig config.DatabaseConfig) (*PostgreSQL, error) {
	poolConfig, err := pgxpool.ParseConfig(postgresURL(databaseConfig))
	if err != nil {
		return nil, errors.New("parse PostgreSQL configuration")
	}
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		return nil, errors.New("create PostgreSQL connection pool")
	}
	return &PostgreSQL{pool: pool}, nil
}

// Close releases the PostgreSQL connection pool.
func (database *PostgreSQL) Close() {
	database.pool.Close()
}

// Pool is exposed only to the composition root for module-owned repositories.
func (database *PostgreSQL) Pool() *pgxpool.Pool { return database.pool }

// Ping reports whether PostgreSQL accepts a connection.
func (database *PostgreSQL) Ping(ctx context.Context) error {
	return database.pool.Ping(ctx)
}

// MigrationVersion reads golang-migrate metadata without exposing SQL outside the adapter.
func (database *PostgreSQL) MigrationVersion(ctx context.Context) (uint, bool, bool, error) {
	var version uint
	var dirty bool
	err := database.pool.QueryRow(ctx, "SELECT version, dirty FROM schema_migrations LIMIT 1").Scan(&version, &dirty)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, false, nil
	}
	var postgresError *pgconn.PgError
	if errors.As(err, &postgresError) && postgresError.Code == "42P01" {
		return 0, false, false, nil
	}
	if err != nil {
		return 0, false, false, fmt.Errorf("read migration metadata: %w", err)
	}
	return version, dirty, true, nil
}

func postgresURL(databaseConfig config.DatabaseConfig) string {
	databaseURL := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(databaseConfig.User, databaseConfig.Password),
		Host:   net.JoinHostPort(databaseConfig.Host, fmt.Sprintf("%d", databaseConfig.Port)),
		Path:   databaseConfig.Name,
	}
	query := databaseURL.Query()
	query.Set("sslmode", "disable")
	databaseURL.RawQuery = query.Encode()
	return databaseURL.String()
}
