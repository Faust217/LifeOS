// Command lifeos-api is the composition root for the LifeOS server.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Faust217/LifeOS/internal/identity"
	"github.com/Faust217/LifeOS/internal/platform/config"
	"github.com/Faust217/LifeOS/internal/platform/database"
	"github.com/Faust217/LifeOS/internal/platform/health"
	"github.com/Faust217/LifeOS/internal/platform/httpapi"
	"github.com/Faust217/LifeOS/internal/platform/migrations"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "lifeos-api: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	applicationConfig, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	logger := newLogger(applicationConfig.Logging.Level)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	postgres, err := database.OpenPostgreSQL(ctx, applicationConfig.Database)
	if err != nil {
		return err
	}
	defer postgres.Close()

	readiness := health.NewService(postgres, migrations.CurrentVersion)
	auth, err := identity.NewAuth(ctx, postgres.Pool(), applicationConfig.Auth)
	if err != nil {
		return fmt.Errorf("initialize authentication: %w", err)
	}
	var authHandler http.Handler
	if auth != nil {
		authHandler = auth.Router()
	}
	address := net.JoinHostPort(applicationConfig.API.Bind.String(), fmt.Sprintf("%d", applicationConfig.API.Port))
	server := &http.Server{
		Addr:              address,
		Handler:           httpapi.NewRouter(logger, readiness, authHandler),
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	errorsChannel := make(chan error, 1)
	go func() {
		logger.Info("server starting", "address", address)
		errorsChannel <- server.ListenAndServe()
	}()

	select {
	case <-ctx.Done():
		shutdownContext, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownContext); err != nil {
			return fmt.Errorf("shutdown HTTP server: %w", err)
		}
		logger.Info("server stopped")
		return nil
	case err := <-errorsChannel:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("serve HTTP: %w", err)
	}
}

func newLogger(level config.LogLevel) *slog.Logger {
	logLevel := slog.LevelInfo
	switch level {
	case config.LogLevelDebug:
		logLevel = slog.LevelDebug
	case config.LogLevelWarn:
		logLevel = slog.LevelWarn
	case config.LogLevelError:
		logLevel = slog.LevelError
	}
	return slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: logLevel}))
}
