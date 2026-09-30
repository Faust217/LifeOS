// Command lifeos-migrate runs the pinned LifeOS database migration workflow.
package main

import (
	"errors"
	"flag"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"

	"github.com/Faust217/LifeOS/internal/platform/config"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

const defaultMigrationPath = "db/migrations"

var migrationTablePattern = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "migration command failed: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	flags := flag.NewFlagSet("lifeos-migrate", flag.ContinueOnError)
	migrationPath := flags.String("path", defaultMigrationPath, "directory containing migration files")
	migrationTable := flags.String("table", "schema_migrations", "migration metadata table")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 {
		return errors.New("usage: lifeos-migrate [-path directory] [-table name] <up|down|version>")
	}
	if !migrationTablePattern.MatchString(*migrationTable) {
		return errors.New("migration table must contain only lowercase letters, digits, and underscores and start with a letter")
	}

	action := flags.Arg(0)
	if action != "up" && action != "down" && action != "version" {
		return fmt.Errorf("unsupported action %q; expected up, down, or version", action)
	}

	applicationConfig, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	sourceURL, err := migrationSourceURL(*migrationPath)
	if err != nil {
		return err
	}
	databaseURL := migrationDatabaseURL(applicationConfig, *migrationTable)

	runner, err := migrate.New(sourceURL, databaseURL)
	if err != nil {
		return fmt.Errorf("initialize migration runner: %w", err)
	}
	defer func() {
		_, _ = runner.Close()
	}()

	switch action {
	case "up":
		if err := runner.Up(); errors.Is(err, migrate.ErrNoChange) {
			fmt.Println("migration up: no change")
			return nil
		} else if err != nil {
			return fmt.Errorf("apply migrations: %w", err)
		}
		fmt.Println("migration up: applied")
	case "down":
		if err := runner.Steps(-1); errors.Is(err, migrate.ErrNoChange) {
			fmt.Println("migration down: no change")
			return nil
		} else if err != nil {
			return fmt.Errorf("roll back one migration: %w", err)
		}
		fmt.Println("migration down: rolled back one version")
	case "version":
		version, dirty, err := runner.Version()
		if errors.Is(err, migrate.ErrNilVersion) {
			fmt.Println("migration version: nil dirty=false")
			return nil
		}
		if err != nil {
			return fmt.Errorf("read migration version: %w", err)
		}
		fmt.Printf("migration version: %d dirty=%t\n", version, dirty)
		if dirty {
			return errors.New("database migration state is dirty; stop and follow the documented manual recovery process")
		}
	}

	return nil
}

func migrationSourceURL(path string) (string, error) {
	absolutePath, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve migration path: %w", err)
	}
	info, err := os.Stat(absolutePath)
	if err != nil {
		return "", fmt.Errorf("inspect migration path: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("migration path %q is not a directory", path)
	}

	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(absolutePath)}).String(), nil
}

func migrationDatabaseURL(applicationConfig config.Config, migrationTable string) string {
	databaseURL := &url.URL{
		Scheme: "postgres",
		User:   url.UserPassword(applicationConfig.Database.User, applicationConfig.Database.Password),
		Host:   net.JoinHostPort(applicationConfig.Database.Host, fmt.Sprintf("%d", applicationConfig.Database.Port)),
		Path:   applicationConfig.Database.Name,
	}
	query := databaseURL.Query()
	query.Set("sslmode", "disable")
	query.Set("x-migrations-table", migrationTable)
	databaseURL.RawQuery = query.Encode()
	return databaseURL.String()
}
