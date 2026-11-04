package finance

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"
)

func TestHistoryCursorAndToken(t *testing.T) {
	owner := "36363636-3636-4636-8636-363636363636"
	epoch := "11111111-1111-4111-8111-111111111111"
	zone := "Asia/Kuala_Lumpur"
	h := historyProjectionToken(owner, &epoch, 7, &zone)
	if h != "cb3869cf69637f28a34ee6cafe66261c0214d37a73e24368c5320870a471bb9d" {
		t.Fatal("token byte contract changed")
	}
	if len(h) != 64 || h == historyProjectionToken("other", &epoch, 7, &zone) || h == historyProjectionToken(owner, &epoch, 8, &zone) {
		t.Fatal("token binding")
	}
	c := historyCursor{h, 10, time.Date(2026, 10, 1, 0, 0, 0, 123456000, time.UTC), owner}
	encoded := encodeHistoryCursor(c)
	if encoded != "WzEsImNiMzg2OWNmNjk2MzdmMjhhMzRlZTZjYWZlNjYyNjFjMDIxNGQzN2E3M2UyNDM2OGM1MzIwODcwYTQ3MWJiOWQiLDEwLCIyMDI2LTEwLTAxVDAwOjAwOjAwLjEyMzQ1NloiLCIzNjM2MzYzNi0zNjM2LTQ2MzYtODYzNi0zNjM2MzYzNjM2MzYiXQ" {
		t.Fatal("cursor byte contract changed")
	}
	decoded, err := decodeHistoryCursor(encoded)
	if err != nil || decoded != c {
		t.Fatal(decoded, err)
	}
	for _, bad := range []string{"!", encoded + "=", strings.Repeat("a", 385), base64.RawURLEncoding.EncodeToString([]byte(`[2,"` + h + `",10,"2026-10-01T00:00:00.123456Z","` + owner + `"]`)), base64.RawURLEncoding.EncodeToString([]byte(`[1,"` + h + `",10,"2026-10-01T00:00:00Z","` + owner + `"]`))} {
		if _, err := decodeHistoryCursor(bad); err == nil {
			t.Fatal("accepted bad cursor")
		}
	}
	upper := c
	upper.ID = "AAAAAAAA-AAAA-4AAA-8AAA-AAAAAAAAAAAA"
	if _, err := decodeHistoryCursor(encodeHistoryCursor(upper)); err == nil {
		t.Fatal("noncanonical UUID accepted")
	}
}
