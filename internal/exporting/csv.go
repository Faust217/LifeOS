package exporting

import (
	"archive/zip"
	"errors"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/Faust217/LifeOS/internal/platform/exportdata"
	"golang.org/x/text/currency"
)

var csvHeaders = map[string][]string{
	"accounts":    strings.Split("account_id,name,account_type,currency,opening_balance_minor,opening_balance_decimal,archived_at_utc,created_at_utc,updated_at_utc", ","),
	"categories":  strings.Split("category_id,name,kind,archived_at_utc,created_at_utc,updated_at_utc", ","),
	"events":      strings.Split("event_id,kind,status,amount_minor,amount_decimal,currency,occurred_at_utc,occurred_at_local,time_zone,note,account_id,from_account_id,to_account_id,affects_balances,counts_as_spending,voided_at_utc,void_reason,created_at_utc", ","),
	"movements":   strings.Split("movement_id,event_id,account_id,amount_minor,amount_decimal,currency,status,created_at_utc", ","),
	"allocations": strings.Split("allocation_id,event_id,category_id,kind,amount_minor,amount_decimal,currency,status,created_at_utc", ","),
}

// decimalAmount uses only currency scale metadata and decimal digit placement.
func decimalAmount(minor, code string) (string, error) {
	unit, err := currency.ParseISO(code)
	if err != nil || len(code) != 3 || code != strings.ToUpper(code) {
		return "", errors.New("invalid export currency")
	}
	scale, _ := currency.Standard.Rounding(unit)
	integer, err := strconv.ParseInt(minor, 10, 64)
	if err != nil {
		return "", err
	}
	if strconv.FormatInt(integer, 10) != minor {
		return "", errors.New("noncanonical minor amount")
	}
	sign := ""
	digits := minor
	if strings.HasPrefix(digits, "-") {
		sign = "-"
		digits = digits[1:]
	}
	if digits == "0" {
		sign = ""
	}
	if scale == 0 {
		return sign + digits, nil
	}
	if len(digits) <= scale {
		digits = strings.Repeat("0", scale+1-len(digits)) + digits
	}
	return sign + digits[:len(digits)-scale] + "." + digits[len(digits)-scale:], nil
}

type csvSink struct {
	zip               *zip.Writer
	current           io.Writer
	counts            map[string]int
	raw, limit, index int
	version           int
	created           time.Time
	profile           exportdata.Profile
}

func newCSVSink(w io.Writer, created time.Time, p exportdata.Profile, limit int, versions ...int) (*csvSink, error) {
	version := 1
	if len(versions) > 0 {
		version = versions[0]
	}
	return &csvSink{version: version, zip: zip.NewWriter(w), counts: map[string]int{}, limit: limit, created: created, profile: p}, nil
}
func (s *csvSink) write(w io.Writer, b []byte) error {
	if len(b) > s.limit-s.raw {
		return ErrLimit
	}
	n, err := w.Write(b)
	s.raw += n
	if err == nil && n != len(b) {
		return io.ErrShortWrite
	}
	return err
}
func (s *csvSink) line(fields []string) error {
	var b strings.Builder
	for i, v := range fields {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteByte('"')
		b.WriteString(strings.ReplaceAll(v, `"`, `""`))
		b.WriteByte('"')
	}
	b.WriteString("\r\n")
	return s.write(s.current, []byte(b.String()))
}
func (s *csvSink) Begin(name string) error {
	if s.index >= 5 || collections[s.index] != name {
		return errors.New("invalid CSV collection sequence")
	}
	file := name
	if name == "events" {
		file = "transactions"
	}
	w, err := s.zip.Create(file + ".csv")
	if err != nil {
		return err
	}
	s.current = w
	s.counts[file] = 0
	if err := s.write(w, []byte{0xef, 0xbb, 0xbf}); err != nil {
		return err
	}
	return s.line(s.headers(name))
}
func (s *csvSink) Row(name string, r exportdata.Record) error {
	values := make([]string, 0, len(s.headers(name)))
	for _, column := range s.headers(name) {
		key := strings.TrimSuffix(column, "_utc")
		var value any
		switch {
		case (name == "accounts" && key == "account_id") || (name == "categories" && key == "category_id") || (name == "events" && key == "event_id") || (name == "movements" && key == "movement_id") || (name == "allocations" && key == "allocation_id"):
			value = r.Fields["id"]
		case strings.HasSuffix(key, "_decimal"):
			minorKey := strings.TrimSuffix(key, "_decimal") + "_minor"
			decimal, err := decimalAmount(r.Fields[minorKey].(string), r.Fields["currency"].(string))
			if err != nil {
				return err
			}
			value = decimal
		case name == "events" && (key == "account_id" || key == "from_account_id" || key == "to_account_id"):
			value = r.Extra["_"+key]
		case key == "affects_balances":
			value = r.Fields["status"] == "posted"
		case key == "counts_as_spending":
			value = r.Fields["status"] == "posted" && r.Fields["kind"] == "expense"
		case key == "status" && name != "events":
			value = r.Extra["_status"]
		case key == "time_zone":
			if s.profile.TimeZone != nil {
				value = *s.profile.TimeZone
			}
		case key == "occurred_at_local":
			if s.profile.TimeZone != nil {
				zone, err := time.LoadLocation(*s.profile.TimeZone)
				if err != nil {
					return err
				}
				at, err := time.Parse(time.RFC3339Nano, r.Fields["occurred_at"].(string))
				if err != nil {
					return err
				}
				value = at.In(zone).Format(time.RFC3339Nano)
			}
		default:
			value = r.Fields[key]
		}
		text := ""
		if value != nil {
			switch v := value.(type) {
			case string:
				text = v
			case bool:
				text = strconv.FormatBool(v)
			default:
				return errors.New("invalid CSV value")
			}
			if key == "name" || key == "note" || key == "void_reason" || strings.HasSuffix(key, "_minor") || strings.HasSuffix(key, "_decimal") {
				text = "'" + text
			}
		}
		values = append(values, text)
	}
	if err := s.line(values); err != nil {
		return err
	}
	file := name
	if name == "events" {
		file = "transactions"
	}
	s.counts[file]++
	return nil
}
func (s *csvSink) End(string) error { s.index++; return nil }
func (s *csvSink) Finish() error {
	if s.index != 5 {
		return errors.New("incomplete CSV export")
	}
	files := []string{"accounts.csv", "categories.csv", "transactions.csv", "movements.csv", "allocations.csv"}
	manifest := map[string]any{"format": "lifeos-finance-csv-v" + strconv.Itoa(s.version), "format_version": s.version, "created_at": exportdata.UTC(s.created), "owner_time_zone": s.profile.TimeZone, "record_counts": s.counts, "files": files, "csv_encoding": "UTF-8-BOM", "csv_text_guard": "apostrophe", "minor_units_encoding": "apostrophe-prefixed-decimal-string", "soft_deleted_events": "excluded"}
	w, err := s.zip.Create("manifest.json")
	if err != nil {
		return err
	}
	if err = writeJSON(rawWriter{s, w}, manifest); err != nil {
		return err
	}
	w, err = s.zip.Create("README.txt")
	if err != nil {
		return err
	}
	readme := csvReadme
	if s.version == 2 {
		readme = strings.ReplaceAll(readme, "LifeOS Finance CSV v1", "LifeOS Finance CSV v2")
		readme += "\nIn v2, affects_balances is POSTED event eligibility, not a guarantee that every account leg changes its current balance. Apply each account's opening_balance_basis and opening_balance_at_utc to the joined movement/event instant. A transfer may contribute to one account but not the other. counts_as_spending still includes POSTED pre-date expenses.\n"
		readme = strings.ReplaceAll(readme, "Opening balance plus POSTED movements reconstructs a current balance; no historical running balance is claimed.", "For legacy_all_movements: opening plus all POSTED eligible movements. For known_balance_before_instant: opening plus POSTED eligible movements with occurred_at >= opening_balance_at_utc. Earlier events remain history/spending but do not change current balance. The instant precedes all events at exactly that instant. No verified historical bank balance is claimed.")
		readme += "\nAccount date/confirmation fields are UTC; null dates mean the unchanged legacy formula. Timezone changes do not move UTC dates. Older v1 files keep their original interpretation.\n"
	}
	if err = s.write(w, []byte(readme)); err != nil {
		return err
	}
	return s.zip.Close()
}

type rawWriter struct {
	s *csvSink
	w io.Writer
}

func (w rawWriter) Write(b []byte) (int, error) {
	err := w.s.write(w.w, b)
	if err != nil {
		return 0, err
	}
	return len(b), nil
}

const csvReadme = `LifeOS Finance CSV v1
This ZIP contains private Finance data only. JSON is the authoritative structured export, including Goals and preferences. These are normal exports, not backups/import files.
Join transactions.event_id to movements.event_id and allocations.event_id. Join account/category IDs to accounts/categories. One transaction row is one event, including a transfer; never sum transfer rows as spending. status=voided rows and their movements/allocations are retained audit history and have no balance/spending effect. affects_balances and counts_as_spending apply to the event, not independent extra amounts.
All unused/archived accounts/categories are included; archived_at_utc identifies archival. Soft Deleted events and all dependent rows are excluded at capture time. Names are current names. Empty allocation collections mean Uncategorized; no global category is invented. A later deletion cannot revoke this saved copy.
Amounts: *_minor are exact integer text. *_decimal are exact fixed-scale text from the same integer and standard currency scale, not cash rounding. No floating point, rounding, grouping or currency conversion. Currency is in each row. Opening balance plus POSTED movements reconstructs a current balance; no historical running balance is claimed.
Every monetary cell and user-controlled name/note/void_reason starts with one apostrophe display guard. Remove exactly ONE leading apostrophe to recover the logical text. Null cells are empty. Import monetary columns as Text; spreadsheet numeric conversion can lose large digits. Never use formula wrappers. CSV escaping/guards are intended for initial spreadsheet opening; editing and resaving may remove protection, and no universal spreadsheet guarantee is made. Use JSON for exact original text line endings.
Logical examples (minor -> decimal): MYR(2): 1234 -> 12.34; 1 -> 0.01; 0 -> 0.00; -1 -> -0.01. JPY(0): 1234 -> 1234. KWD(3): 1234 -> 1.234. CLF(4): 12345 -> 1.2345. MYR: 9223372036854775807 -> 92233720368547758.07; -9223372036854775808 -> -92233720368547758.08. Zero is used only where valid, e.g. opening balances.
Actual encoded monetary pair: "'1234","'12.34"; negative pair: "'-1","'-0.01".
MYR transfer: ONE transactions row (100,1.00), source movement (-100,-1.00), destination movement (100,1.00). ONE expense can have allocation pairs (70,0.70) and (30,0.30) without extra transaction rows.
UTC timestamps end in Z. occurred_at_local uses the saved owner timezone with its actual offset/DST. If no timezone has been confirmed, local/time_zone cells are empty; UTC is still available. Downloaded files are private data outside LifeOS access/revocation controls. Keep them safe.
`

func (s *csvSink) headers(name string) []string {
	if name == "accounts" && s.version == 2 {
		return strings.Split("account_id,name,account_type,currency,opening_balance_minor,opening_balance_decimal,opening_balance_basis,opening_balance_at_utc,opening_balance_confirmed_at_utc,archived_at_utc,created_at_utc,updated_at_utc", ",")
	}
	return csvHeaders[name]
}
