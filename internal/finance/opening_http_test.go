package finance

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestOpeningDatesAlwaysUseUTC(t *testing.T) {
	at := time.Date(2026, 10, 1, 8, 0, 0, 123456000, time.FixedZone("preview-device", 8*3600))
	data, err := json.Marshal(accountDTO(Account{OpeningBalanceAt: &at, OpeningConfirmedAt: &at}))
	if err != nil || !strings.Contains(string(data), `"opening_balance_at":"2026-10-01T00:00:00.123456Z"`) || !strings.Contains(string(data), `"opening_balance_confirmed_at":"2026-10-01T00:00:00.123456Z"`) {
		t.Fatal(string(data), err)
	}
	if utcOpeningInstant(nil) != nil {
		t.Fatal("legacy date fabricated")
	}
}
