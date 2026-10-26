package finance

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/Faust217/LifeOS/internal/identity"
)

func TestFinancialWriteKeyAndJSONContract(t *testing.T) {
	for _, key := range []string{"", " ", "11111111-1111-1111-8111-111111111111", "11111111-1111-4111-7111-111111111111", "AAAAAAAA-1111-4111-8111-111111111111", newFinancialWriteKey() + ", " + newFinancialWriteKey()} {
		if validateRequestKey(key) == nil {
			t.Fatalf("accepted invalid key %q", key)
		}
	}
	if err := validateRequestKey(newFinancialWriteKey()); err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{`null`, `[]`, `{"reason":"a","reason":"b"}`, `{"reason":"a","Reason":"b"}`, `{"x":[{"a":1,"a":2}]}`, `{} {}`, `{"x":`} {
		if uniqueJSONObject([]byte(data)) {
			t.Fatalf("ambiguous JSON accepted: %s", data)
		}
	}
	if !uniqueJSONObject([]byte(`{"reason":"","x":[{},[1,true,null,"x"]]}`)) {
		t.Fatal("valid JSON token walk failed")
	}
	for _, tt := range []struct {
		n    int64
		band string
	}{{0, "available"}, {89999, "available"}, {90000, "approaching"}, {99999, "approaching"}, {100000, "full"}, {100001, "full"}} {
		if got := capacityBand(tt.n, FinancialWriteKeyLimit); got != tt.band {
			t.Fatalf("capacity %d=%s", tt.n, got)
		}
	}
}

func TestFinancialFingerprintGoldenFixtures(t *testing.T) {
	data, err := os.ReadFile("testdata/financial-command-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []struct {
		Owner, Operation  string
		Target            any
		Parameters        []any
		Canonical, SHA256 string
	}
	if err = json.Unmarshal(data, &fixtures); err != nil {
		t.Fatal(err)
	}
	for _, fixture := range fixtures {
		t.Run(fixture.Operation, func(t *testing.T) {
			bytes, digest, err := canonicalFinancialBytes(fixture.Owner, financialCommand{operation: fixture.Operation, target: fixture.Target, parameters: fixture.Parameters})
			if err != nil || string(bytes) != fixture.Canonical || hex.EncodeToString(digest[:]) != fixture.SHA256 {
				t.Fatal("immutable v1 fingerprint changed", err)
			}
		})
	}
}

func TestFinancialCanonicalV1Golden(t *testing.T) {
	// Encoder fixture intentionally uses a zero owner; service entry points reject it.
	// The guarded integration suite also verifies the stored real-owner fingerprint.
	cmd := financialCommand{operation: "event.create", parameters: []any{"expense", "aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa", "9223372036854775807", "MYR", "2026-10-01T00:00:00.123456Z", "<coffee>\n", []any{[]any{"bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb", "9223372036854775807"}}}}
	got, _, err := canonicalFinancialCommand(identity.OwnerID{}, cmd)
	want := `["lifeos-finance-command-v1","","event.create",null,["expense","aaaaaaaa-aaaa-4aaa-8aaa-aaaaaaaaaaaa","9223372036854775807","MYR","2026-10-01T00:00:00.123456Z","\u003ccoffee\u003e\n",[["bbbbbbbb-bbbb-4bbb-8bbb-bbbbbbbbbbbb","9223372036854775807"]]]]`
	if err != nil || string(got) != want {
		t.Fatalf("v1 bytes changed: %s %v", got, err)
	}
	at, _ := time.Parse(time.RFC3339Nano, "2026-10-01T08:00:00.123456999+08:00")
	if canonicalInstant(at) != "2026-10-01T00:00:00.123456Z" {
		t.Fatal("timestamp normalization changed")
	}
	if !errors.Is(financialWriteError(ErrOverflow), ErrOverflow) {
		t.Fatal("domain error lost")
	}
}
