// Command lifeos-admin is a localhost-only, explicitly confirmed Alpha operator tool.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/netip"
	"os"
	"time"

	"github.com/Faust217/LifeOS/internal/identity"
	"github.com/Faust217/LifeOS/internal/platform/config"
	"github.com/Faust217/LifeOS/internal/platform/database"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "lifeos-admin:", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		return errors.New("usage: lifeos-admin <bootstrap|revoke-bootstrap|invite|revoke> [flags]")
	}
	action := args[0]
	flags := flag.NewFlagSet(action, flag.ContinueOnError)
	confirm := flags.Bool("confirm-local-operator", false, "confirm this privileged local action")
	email := flags.String("email", "", "invited email address")
	adminID := flags.String("admin-id", "", "current owner administrator LifeOS ID")
	invitationID := flags.String("invitation-id", "", "invitation UUID to revoke")
	hours := flags.Int("hours", 24, "invitation lifetime in hours (1-168)")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	if flags.NArg() != 0 || !*confirm {
		return errors.New("explicit --confirm-local-operator and valid flags are required")
	}
	if *hours < 1 || *hours > 168 {
		return errors.New("--hours must be from 1 to 168")
	}
	if action != "bootstrap" && action != "revoke-bootstrap" && action != "invite" && action != "revoke" {
		return errors.New("unsupported operator action")
	}
	if (action == "bootstrap" || action == "invite") && *email == "" {
		return errors.New("--email is required")
	}
	if (action == "invite" || action == "revoke") && *adminID == "" {
		return errors.New("--admin-id is required")
	}
	if action == "revoke" && *invitationID == "" {
		return errors.New("--invitation-id is required")
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}
	if !isLoopbackHost(cfg.Database.Host) {
		return errors.New("operator command requires a loopback PostgreSQL host")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	postgres, err := database.OpenPostgreSQL(ctx, cfg.Database)
	if err != nil {
		return err
	}
	defer postgres.Close()
	if err := postgres.Ping(ctx); err != nil {
		return errors.New("local PostgreSQL unavailable")
	}
	operator, err := identity.NewLocalOperator(postgres.Pool(), *confirm)
	if err != nil {
		return err
	}
	expiresAt := time.Now().Add(time.Duration(*hours) * time.Hour)
	switch action {
	case "bootstrap":
		id, err := operator.Bootstrap(ctx, *email, expiresAt)
		if err != nil {
			return err
		}
		fmt.Println("bootstrap invitation id:", id)
	case "revoke-bootstrap":
		if err := operator.RevokeBootstrap(ctx); err != nil {
			return err
		}
		fmt.Println("bootstrap invitation revoked")
	case "invite":
		id, err := operator.Invite(ctx, *adminID, *email, expiresAt)
		if err != nil {
			return err
		}
		fmt.Println("invitation id:", id)
	case "revoke":
		if err := operator.Revoke(ctx, *adminID, *invitationID); err != nil {
			return err
		}
		fmt.Println("invitation revoked")
	}
	return nil
}

func isLoopbackHost(host string) bool {
	if host == "localhost" {
		return true
	}
	if address, err := netip.ParseAddr(host); err == nil {
		return address.IsLoopback()
	}
	return false
}
