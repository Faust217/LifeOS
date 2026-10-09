package main

import "testing"

func TestOperatorRequiresExplicitConfirmationBeforeDatabaseAccess(t *testing.T) {
	for _, args := range [][]string{{"bootstrap", "--email=owner@example.com"}, {"invite", "--email=tester@example.com", "--admin-id=owner"}, {"revoke-bootstrap"}} {
		if err := run(args); err == nil {
			t.Fatalf("unconfirmed operator action was accepted: %v", args)
		}
	}
}

func TestLoopbackDatabaseHostOnly(t *testing.T) {
	for _, host := range []string{"localhost", "127.0.0.1", "::1"} {
		if !isLoopbackHost(host) {
			t.Errorf("rejected loopback host %q", host)
		}
	}
	for _, host := range []string{"postgres", "example.com", "0.0.0.0", "192.168.1.1"} {
		if isLoopbackHost(host) {
			t.Errorf("accepted non-loopback host %q", host)
		}
	}
}
