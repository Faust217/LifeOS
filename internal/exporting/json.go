package exporting

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"github.com/Faust217/LifeOS/internal/platform/exportdata"
)

var collections = []string{"accounts", "categories", "events", "movements", "allocations", "goals", "checklist_items"}

type jsonSink struct {
	w      io.Writer
	counts map[string]int
	index  int
	first  bool
}

func writeJSON(w io.Writer, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = w.Write(b)
	return err
}
func newJSONSink(w io.Writer, created time.Time, p exportdata.Profile, versions ...int) (*jsonSink, error) {
	s := &jsonSink{w: w, counts: map[string]int{}}
	version := 1
	if len(versions) > 0 {
		version = versions[0]
	}
	if _, err := io.WriteString(w, fmt.Sprintf(`{"format":"lifeos-export-v%d","format_version":%d,"created_at":`, version, version)); err != nil {
		return nil, err
	}
	if err := writeJSON(w, exportdata.UTC(created)); err != nil {
		return nil, err
	}
	if _, err := io.WriteString(w, `,"owner":`); err != nil {
		return nil, err
	}
	if err := writeJSON(w, p); err != nil {
		return nil, err
	}
	if _, err := io.WriteString(w, `,"preferences":`); err != nil {
		return nil, err
	}
	if err := writeJSON(w, map[string]any{"time_zone": p.TimeZone}); err != nil {
		return nil, err
	}
	if _, err := io.WriteString(w, `,"finance":{`); err != nil {
		return nil, err
	}
	return s, nil
}
func (s *jsonSink) Begin(name string) error {
	if s.index >= len(collections) || collections[s.index] != name {
		return fmt.Errorf("invalid export collection sequence")
	}
	prefix := ""
	if s.index == 5 {
		prefix = `},"planning":{`
	} else if s.index > 0 {
		prefix = ","
	}
	_, err := io.WriteString(s.w, prefix+`"`+name+`":[`)
	s.first = true
	s.counts[name] = 0
	return err
}
func (s *jsonSink) Row(name string, r exportdata.Record) error {
	if !s.first {
		if _, err := io.WriteString(s.w, ","); err != nil {
			return err
		}
	}
	s.first = false
	if err := writeJSON(s.w, r.Fields); err != nil {
		return err
	}
	s.counts[name]++
	return nil
}
func (s *jsonSink) End(string) error { _, err := io.WriteString(s.w, "]"); s.index++; return err }
func (s *jsonSink) Finish() error {
	if s.index != len(collections) {
		return fmt.Errorf("incomplete export")
	}
	if _, err := io.WriteString(s.w, `},"record_counts":`); err != nil {
		return err
	}
	if err := writeJSON(s.w, s.counts); err != nil {
		return err
	}
	_, err := io.WriteString(s.w, "}\n")
	return err
}
