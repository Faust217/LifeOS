package finance

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/Faust217/LifeOS/internal/identity"
)

func TestMoneyValidation(t *testing.T) {
	for _, code := range []string{"MYR", "USD", "JPY"} {
		if !validCurrency(code) {
			t.Fatalf("rejected ISO currency %q", code)
		}
	}
	for _, code := range []string{"", "myr", "ZZZ", "US", "USDD"} {
		if validCurrency(code) {
			t.Fatalf("accepted invalid currency %q", code)
		}
	}
	if _, err := addMoney(math.MaxInt64, 1); !errors.Is(err, ErrOverflow) {
		t.Fatalf("positive overflow=%v", err)
	}
	if _, err := addMoney(math.MinInt64, -1); !errors.Is(err, ErrOverflow) {
		t.Fatalf("negative overflow=%v", err)
	}
	if got, err := addMoney(-100, 50); err != nil || got != -50 {
		t.Fatalf("signed addition=%d err=%v", got, err)
	}
	if _, err := parseMoney("9223372036854775808"); !errors.Is(err, ErrOverflow) {
		t.Fatalf("aggregate overflow=%v", err)
	}
	if validID("client-owner-id") || !validID("11111111-1111-4111-8111-111111111111") {
		t.Fatal("record ID validation failed")
	}
}

func TestUnverifiedOwnerFailsClosed(t *testing.T) {
	service := NewService(nil)
	if _, err := service.CreateAccount(context.Background(), identity.OwnerID{}, AccountInput{}); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("account creation with zero owner=%v", err)
	}
	if _, err := service.History(context.Background(), identity.OwnerID{}, 10, 0); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("history with zero owner=%v", err)
	}
	if err := service.VoidEvent(context.Background(), identity.OwnerID{}, "", ""); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("void with zero owner=%v", err)
	}
}
