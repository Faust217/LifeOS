package exporting

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"io"
	"strings"
	"testing"
)

func TestExactDecimalAmounts(t *testing.T) {
	for _, tc := range []struct{ currency, minor, want string }{
		{"MYR", "1234", "12.34"}, {"MYR", "1", "0.01"}, {"MYR", "0", "0.00"}, {"MYR", "-1", "-0.01"},
		{"JPY", "1234", "1234"}, {"JPY", "0", "0"}, {"KWD", "1234", "1.234"}, {"KWD", "1", "0.001"}, {"CLF", "12345", "1.2345"}, {"CHF", "1", "0.01"},
		{"MYR", "9223372036854775807", "92233720368547758.07"}, {"MYR", "-9223372036854775808", "-92233720368547758.08"},
	} {
		t.Run(tc.currency+tc.minor, func(t *testing.T) {
			got, err := decimalAmount(tc.minor, tc.currency)
			if err != nil || got != tc.want {
				t.Fatalf("got %q err=%v want %q", got, err, tc.want)
			}
		})
	}
	for _, tc := range [][2]string{{"1", "ZZZ"}, {"1", "myr"}, {"9223372036854775808", "MYR"}, {"1.1", "MYR"}, {"+1", "MYR"}, {"01", "MYR"}} {
		if _, err := decimalAmount(tc[0], tc[1]); err == nil {
			t.Fatalf("accepted invalid %v", tc)
		}
	}
}

func TestQuoteAllCSVAndBounds(t *testing.T) {
	var buf bytes.Buffer
	s := &csvSink{current: &buf, limit: 1024}
	cells := []string{"'=1+1", "'\t@SUM(1)", "'－1", "'a,\"quoted\"\nline", "'-92233720368547758.08"}
	if err := s.line(cells); err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(buf.String(), "\r\n") {
		t.Fatal("missing CRLF")
	}
	got, err := csv.NewReader(strings.NewReader(buf.String())).Read()
	if err != nil {
		t.Fatal(err)
	}
	for i, want := range cells {
		if got[i] != want {
			t.Fatalf("cell %d changed: %q", i, got[i])
		}
	}
	s.limit = s.raw
	if !errors.Is(s.line([]string{"x"}), ErrLimit) {
		t.Fatal("raw bytes limit bypassed")
	}
	b := &boundedBuffer{ctx: context.Background(), limit: 2}
	if _, err := io.WriteString(b, "123"); !errors.Is(err, ErrLimit) || b.Len() != 0 {
		t.Fatal("StringWriter bypassed bounded artifact")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	b.ctx = ctx
	if _, err := b.Write([]byte("a")); !errors.Is(err, context.Canceled) {
		t.Fatal("buffer ignored cancellation")
	}
}
