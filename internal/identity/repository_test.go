package identity

import (
	"errors"
	"testing"
	"time"
)

func TestInvitationState(t *testing.T) {
	now := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	timestamp := now.Add(-time.Hour)
	cases := []struct {
		name     string
		accepted *time.Time
		revoked  *time.Time
		expires  time.Time
		want     InvitationState
	}{
		{"pending", nil, nil, now.Add(time.Hour), InvitationPending},
		{"expired at boundary", nil, nil, now, InvitationExpired},
		{"accepted", &timestamp, nil, now.Add(time.Hour), InvitationAccepted},
		{"revoked", nil, &timestamp, now.Add(time.Hour), InvitationRevoked},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := State(tc.accepted, tc.revoked, tc.expires, now); got != tc.want {
				t.Fatalf("State() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestNormalizeEmail(t *testing.T) {
	email, err := normalizeEmail("  Owner@Example.COM  ")
	if err != nil || email != "owner@example.com" {
		t.Fatalf("normalizeEmail() = %q, %v", email, err)
	}
	for _, input := range []string{"", "no-at", "a b@example.com"} {
		if _, err := normalizeEmail(input); !errors.Is(err, ErrInvalidInput) {
			t.Fatalf("normalizeEmail(%q) error = %v", input, err)
		}
	}
}
