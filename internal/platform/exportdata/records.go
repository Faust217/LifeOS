// Package exportdata defines the narrow, module-owned export projection boundary.
package exportdata

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

type Record struct {
	Fields map[string]any
	Extra  map[string]any // CSV-only derived relationships, never JSON fields.
}

type Sink interface {
	Begin(string) error
	Row(string, Record) error
	End(string) error
}

type Profile struct {
	ID        string  `json:"id"`
	Email     string  `json:"email"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
	TimeZone  *string `json:"-"`
}

func UTC(t time.Time) string { return t.UTC().Format("2006-01-02T15:04:05.999999Z") }

// WriteRows streams an explicitly selected module projection on the shared tx.
func WriteRows(ctx context.Context, tx pgx.Tx, owner, collection, query string, sink Sink) error {
	if owner == "" {
		return errors.New("verified export owner required")
	}
	if err := sink.Begin(collection); err != nil {
		return err
	}
	rows, err := tx.Query(ctx, query, owner)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := ctx.Err(); err != nil {
			return err
		}
		values, err := rows.Values()
		if err != nil {
			return err
		}
		record := Record{Fields: map[string]any{}, Extra: map[string]any{}}
		for i, field := range rows.FieldDescriptions() {
			value := values[i]
			if t, ok := value.(time.Time); ok {
				value = UTC(t)
			}
			name := field.Name
			if len(name) > 0 && name[0] == '_' {
				record.Extra[name] = value
			} else {
				record.Fields[name] = value
			}
		}
		if err := sink.Row(collection, record); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	rows.Close()
	return sink.End(collection)
}
