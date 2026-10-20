// Package exporting orchestrates module-owned, normal owner-data snapshots.
package exporting

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"time"

	"github.com/Faust217/LifeOS/internal/finance"
	"github.com/Faust217/LifeOS/internal/identity"
	"github.com/Faust217/LifeOS/internal/planning"
	"github.com/Faust217/LifeOS/internal/platform/exportdata"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrBusy   = errors.New("export busy")
	ErrLimit  = errors.New("export size limit exceeded")
	ErrFormat = errors.New("invalid export format")
)

const MaxBytes = 64 << 20

type limits struct {
	bytes             int
	generation, write time.Duration
	concurrent        int
}
type Service struct {
	pool   *pgxpool.Pool
	limits limits
	mu     sync.Mutex
	owners map[string]bool
	active int
	// Tests pause/fail at collection boundaries; never configured by HTTP input.
	afterCollection func(string) error
	inspectSnapshot func(pgx.Tx) error
}

func NewService(pool *pgxpool.Pool) *Service {
	return &Service{pool: pool, limits: limits{MaxBytes, 120 * time.Second, 120 * time.Second, 2}, owners: map[string]bool{}}
}

type Artifact struct {
	Data                  []byte
	Filename, ContentType string
	release               func()
}

func (a *Artifact) Close() {
	if a.release != nil {
		a.release()
		a.release = nil
	}
	a.Data = nil
}

func (s *Service) acquire(owner identity.OwnerID) (func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.owners[owner.String()] || s.active >= s.limits.concurrent {
		return nil, ErrBusy
	}
	s.owners[owner.String()] = true
	s.active++
	var once sync.Once
	return func() {
		once.Do(func() { s.mu.Lock(); defer s.mu.Unlock(); delete(s.owners, owner.String()); s.active-- })
	}, nil
}

type boundedBuffer struct {
	bytes.Buffer
	ctx   context.Context
	limit int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if err := b.ctx.Err(); err != nil {
		return 0, err
	}
	if len(p) > b.limit-b.Len() {
		return 0, ErrLimit
	}
	return b.Buffer.Write(p)
}
func (b *boundedBuffer) WriteString(s string) (int, error) { return b.Write([]byte(s)) }

// Generate completes the artifact before returning it; callers must defer Close.
func (s *Service) Generate(parent context.Context, owner identity.OwnerID, format string) (artifact *Artifact, err error) {
	if !owner.Valid() {
		return nil, identity.ErrNotAuthorized
	}
	if format != "json" && format != "finance_csv" {
		return nil, ErrFormat
	}
	release, err := s.acquire(owner)
	if err != nil {
		return nil, err
	}
	success := false
	defer func() {
		if !success {
			release()
		}
	}()
	ctx, cancel := context.WithTimeout(parent, s.limits.generation)
	defer cancel()
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, err
	}
	defer func() {
		cleanup, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		_ = tx.Rollback(cleanup)
	}()
	// The first module read acquires the common MVCC snapshot.
	profile, err := identity.ExportProfile(ctx, tx, owner)
	if err != nil {
		return nil, err
	}
	if s.inspectSnapshot != nil {
		if err = s.inspectSnapshot(tx); err != nil {
			return nil, err
		}
	}
	var created time.Time
	if err = tx.QueryRow(ctx, `SELECT transaction_timestamp()`).Scan(&created); err != nil {
		return nil, err
	}
	buffer := &boundedBuffer{ctx: ctx, limit: s.limits.bytes}
	var sink artifactSink
	if format == "json" {
		sink, err = newJSONSink(buffer, created, profile)
	} else {
		sink, err = newCSVSink(buffer, created, profile, s.limits.bytes)
	}
	if err != nil {
		return nil, err
	}
	wrapper := hookSink{Sink: sink, hook: s.afterCollection}
	if err = finance.WriteExport(ctx, tx, owner, wrapper); err != nil {
		return nil, err
	}
	if format == "json" {
		if err = planning.WriteExport(ctx, tx, owner, wrapper); err != nil {
			return nil, err
		}
	}
	if err = sink.Finish(); err != nil {
		return nil, err
	}
	if err = ctx.Err(); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	// Generation's context can now be released; the artifact remains bounded in memory.
	filename := "lifeos-export-v1-" + created.UTC().Format("20060102T150405Z") + ".json"
	contentType := "application/json; charset=utf-8"
	if format == "finance_csv" {
		filename = "lifeos-finance-csv-v1-" + created.UTC().Format("20060102T150405Z") + ".zip"
		contentType = "application/zip"
	}
	artifact = &Artifact{Data: buffer.Bytes(), Filename: filename, ContentType: contentType, release: release}
	success = true
	return artifact, nil
}

type artifactSink interface {
	exportdata.Sink
	Finish() error
}
type hookSink struct {
	exportdata.Sink
	hook func(string) error
}

func (s hookSink) End(name string) error {
	if err := s.Sink.End(name); err != nil {
		return err
	}
	if s.hook != nil {
		return s.hook(name)
	}
	return nil
}
