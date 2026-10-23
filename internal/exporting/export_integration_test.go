package exporting

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Faust217/LifeOS/internal/finance"
	"github.com/Faust217/LifeOS/internal/identity"
	"github.com/Faust217/LifeOS/internal/planning"
	"github.com/Faust217/LifeOS/internal/platform/config"
	"github.com/Faust217/LifeOS/internal/platform/health"
	"github.com/Faust217/LifeOS/internal/platform/httpapi"
	"github.com/go-chi/chi/v5"
	"github.com/golang-migrate/migrate/v4"
	_ "github.com/golang-migrate/migrate/v4/database/postgres"
	_ "github.com/golang-migrate/migrate/v4/source/file"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const origin = "http://127.0.0.1:5195"

type readiness struct{}
type safeLog struct {
	mu     sync.Mutex
	buffer bytes.Buffer
}

func (b *safeLog) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buffer.Write(p)
}
func (b *safeLog) String() string { b.mu.Lock(); defer b.mu.Unlock(); return b.buffer.String() }

func (readiness) Check(context.Context) health.Result { return health.Result{Ready: true} }

func TestOwnerExportIntegration(t *testing.T) {
	dsn := os.Getenv("LIFEOS_EXPORT_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set LIFEOS_EXPORT_TEST_DATABASE_URL to a fresh disposable *_test database")
	}
	parsed, err := url.Parse(dsn)
	if err != nil || !strings.HasSuffix(strings.TrimPrefix(parsed.Path, "/"), "_test") {
		t.Fatal("export integration requires disposable *_test database")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	var exists bool
	if err = pool.QueryRow(ctx, `SELECT to_regclass('public.schema_migrations') IS NOT NULL`).Scan(&exists); err != nil || exists {
		t.Fatal("export integration database must be fresh and empty")
	}
	migrationPath, _ := filepath.Abs("../../db/migrations")
	runner, err := migrate.New((&url.URL{Scheme: "file", Path: filepath.ToSlash(migrationPath)}).String(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer runner.Close()
	if err = runner.Up(); err != nil {
		t.Fatal(err)
	}
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"issuer": "http://" + r.Host, "authorization_endpoint": "http://" + r.Host + "/authorize", "token_endpoint": "http://" + r.Host + "/token", "jwks_uri": "http://" + r.Host + "/keys", "response_types_supported": []string{"code"}, "subject_types_supported": []string{"public"}, "id_token_signing_alg_values_supported": []string{"RS256"}})
	}))
	defer provider.Close()
	auth, err := identity.NewAuth(ctx, pool, config.AuthConfig{ClientID: "export-test", ClientSecret: "synthetic", Issuer: provider.URL, PublicOrigin: origin})
	if err != nil {
		t.Fatal(err)
	}
	owners := []string{"11111111-1111-4111-8111-111111111111", "22222222-2222-4222-8222-222222222222", "33333333-3333-4333-8333-333333333333"}
	secrets := []string{"export-owner-synthetic", "export-other-synthetic", "export-empty-synthetic"}
	verified := make([]identity.OwnerID, 3)
	csrf := make([]string, 3)
	for i, id := range owners {
		if _, err = pool.Exec(ctx, `INSERT INTO lifeos_users(id,email,time_zone) VALUES($1,$2,$3)`, id, fmt.Sprintf("export%d@example.invalid", i), func() any {
			if i == 0 {
				return "Asia/Kuala_Lumpur"
			}
			return nil
		}()); err != nil {
			t.Fatal(err)
		}
		hash := sha256.Sum256([]byte(secrets[i]))
		if _, err = pool.Exec(ctx, `INSERT INTO lifeos_sessions(token_hash,user_id,expires_at) VALUES($1,$2,now()+interval '1 hour')`, hash[:], id); err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest("GET", "/", nil)
		req.AddCookie(&http.Cookie{Name: "lifeos_session", Value: secrets[i]})
		rr := httptest.NewRecorder()
		auth.RequireRead(func(w http.ResponseWriter, _ *http.Request, o identity.OwnerID) { verified[i] = o }).ServeHTTP(rr, req)
		if !verified[i].Valid() {
			t.Fatal("owner not verified")
		}
	}
	service := NewService(pool)
	router := chi.NewRouter()
	auth.RegisterRoutes(router)
	finance.RegisterRoutes(router, auth, finance.NewService(pool))
	planning.RegisterRoutes(router, auth, planning.NewService(pool))
	RegisterRoutes(router, auth, service)
	var logs safeLog
	server := httptest.NewServer(httpapi.NewRouter(slog.New(slog.NewJSONHandler(&logs, nil)), readiness{}, router))
	defer server.Close()
	request := func(who int, method, path, body, requestOrigin, token string) (int, http.Header, []byte) {
		t.Helper()
		req, _ := http.NewRequest(method, server.URL+path, strings.NewReader(body))
		if who >= 0 {
			req.AddCookie(&http.Cookie{Name: "lifeos_session", Value: secrets[who]})
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", requestOrigin)
		req.Header.Set("X-CSRF-Token", token)
		response, err := server.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		data, err := io.ReadAll(response.Body)
		if err != nil {
			t.Fatal(err)
		}
		return response.StatusCode, response.Header, data
	}
	for i := range owners {
		status, _, body := request(i, "GET", "/api/v1/me", "", origin, "")
		var me struct {
			CSRF string `json:"csrf_token"`
		}
		if status != 200 || json.Unmarshal(body, &me) != nil {
			t.Fatal("failed obtaining CSRF")
		}
		csrf[i] = me.CSRF
	}
	export := func(who int, format string) (http.Header, []byte) {
		t.Helper()
		status, headers, body := request(who, "POST", "/api/v1/me/export", `{"format":"`+format+`"}`, origin, csrf[who])
		if status != 200 {
			t.Fatalf("export status %d: %s", status, body)
		}
		if !strings.Contains(headers.Get("Cache-Control"), "private, no-store") || headers.Get("X-Content-Type-Options") != "nosniff" || headers.Get("Content-Disposition") == "" {
			t.Fatalf("unsafe download headers: %v", headers)
		}
		return headers, body
	}
	readJSON := func(data []byte) map[string]any {
		t.Helper()
		var v map[string]any
		if err := json.Unmarshal(data, &v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	owner, other := verified[0], verified[1]
	f := finance.NewService(pool)
	p := planning.NewService(pool)
	account := func(o identity.OwnerID, name, code string, amount int64) finance.Account {
		t.Helper()
		id := fmt.Sprintf("%08x-aaaa-4aaa-8aaa-%012x", len(name)+len(code), time.Now().UnixNano()&0xffffffffffff)
		_, err := pool.Exec(ctx, `INSERT INTO finance_accounts(id,owner_id,name,account_type,currency,opening_balance_minor) VALUES($1,$2,$3,'bank',$4,$5)`, id, o.String(), name, code, amount)
		a := finance.Account{ID: id, Name: name, Type: "bank", Currency: code, OpeningBalanceMinor: amount}
		if err != nil {
			t.Fatal(err)
		}
		return a
	}
	bank := account(owner, "=PRIVATE-CANARY, \"bank\"\n第二行", "MYR", 10000)
	saving := account(owner, "Savings", "MYR", 2000)
	for _, v := range []struct {
		code string
		n    int64
	}{{"JPY", math.MinInt64}, {"MYR", math.MaxInt64}, {"KWD", 1234}, {"CLF", 12345}, {"CHF", 1}} {
		account(owner, "Unused "+v.code, v.code, v.n)
	}
	otherAccount := account(other, "OTHER-OWNER-PRIVATE", "MYR", 100)
	cat, err := f.CreateCategory(ctx, owner, finance.CategoryInput{Name: "Medical", Kind: "expense"})
	if err != nil {
		t.Fatal(err)
	}
	cat2, err := f.CreateCategory(ctx, owner, finance.CategoryInput{Name: "Food", Kind: "expense"})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 10, 20, 3, 4, 5, 123456000, time.UTC)
	post := func(o identity.OwnerID, a finance.Account, n int64, kind, note string, alloc []finance.AllocationInput) finance.Event {
		t.Helper()
		in := finance.EventInput{AccountID: a.ID, AmountMinor: n, Currency: a.Currency, OccurredAt: now, Note: note, Allocations: alloc}
		var e finance.Event
		var err error
		if kind == "income" {
			e, err = f.PostIncome(ctx, o, in)
		} else {
			e, err = f.PostExpense(ctx, o, in)
		}
		if err != nil {
			t.Fatal(err)
		}
		return e
	}
	expense := post(owner, bank, 100, "expense", "Two categories", []finance.AllocationInput{{CategoryID: cat.ID, AmountMinor: 70}, {CategoryID: cat2.ID, AmountMinor: 30}})
	income := post(owner, bank, 1000, "income", "Uncategorized income", nil)
	post(owner, bank, 50, "expense", "Uncategorized", nil)
	voided := post(owner, bank, 75, "expense", "Visible voided", []finance.AllocationInput{{CategoryID: cat.ID, AmountMinor: 75}})
	if err = f.VoidEvent(ctx, owner, voided.ID, ""); err != nil {
		t.Fatal(err)
	}
	deleted := post(owner, bank, 33, "expense", "DELETED-CANARY", []finance.AllocationInput{{CategoryID: cat.ID, AmountMinor: 33}})
	if err = f.DeleteEvent(ctx, owner, deleted.ID); err != nil {
		t.Fatal(err)
	}
	deletedVoid := post(owner, bank, 10, "income", "DELETED-VOID-CANARY", nil)
	if err = f.VoidEvent(ctx, owner, deletedVoid.ID, "explicit audit"); err != nil {
		t.Fatal(err)
	}
	if err = f.DeleteEvent(ctx, owner, deletedVoid.ID); err != nil {
		t.Fatal(err)
	}
	transfer, err := f.Transfer(ctx, owner, finance.TransferInput{FromAccountID: bank.ID, ToAccountID: saving.ID, AmountMinor: 100, Currency: "MYR", OccurredAt: now, Note: "One transfer"})
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 25; i++ {
		post(owner, bank, 1, "income", fmt.Sprintf("Beyond page %d", i), nil)
	}
	post(other, otherAccount, 10, "expense", "OTHER-PRIVATE-EVENT", nil)
	goal, err := p.CreateGoal(ctx, owner, "Owner Goal")
	if err != nil {
		t.Fatal(err)
	}
	goal, err = p.AddItem(ctx, owner, goal.ID, "Done step")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = p.SetItemCompleted(ctx, owner, goal.ID, goal.Items[0].ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err = p.SetGoalCompleted(ctx, owner, goal.ID, true); err != nil {
		t.Fatal(err)
	}
	if _, err = p.ArchiveGoal(ctx, owner, goal.ID); err != nil {
		t.Fatal(err)
	}
	if err = f.ArchiveCategory(ctx, owner, cat.ID); err != nil {
		t.Fatal(err)
	}
	if err = f.ArchiveAccount(ctx, owner, saving.ID); err != nil {
		t.Fatal(err)
	}
	before, err := f.Balance(ctx, owner, bank.ID)
	if err != nil {
		t.Fatal(err)
	}

	t.Run("complete exact normal snapshot", func(t *testing.T) {
		_, data := export(0, "json")
		root := readJSON(data)
		verifyJSONShape(t, root)
		if root["format"] != "lifeos-export-v1" || root["format_version"] != float64(1) {
			t.Fatal("wrong format")
		}
		if strings.Contains(string(data), deleted.ID) || strings.Contains(string(data), deletedVoid.ID) || strings.Contains(string(data), "OTHER-") || strings.Contains(string(data), "csrf") || strings.Contains(string(data), secrets[0]) {
			t.Fatal("private/deleted/security data leaked")
		}
		fin := root["finance"].(map[string]any)
		if len(fin["events"].([]any)) != 30 {
			t.Fatalf("history truncated: %d", len(fin["events"].([]any)))
		}
		e := findRecord(t, fin["events"], voided.ID)
		if e["status"] != "voided" || e["void_reason"] != "No reason provided by owner" {
			t.Fatal("void audit changed")
		}
		if findRecord(t, fin["events"], expense.ID)["amount_minor"] != "100" {
			t.Fatal("money is not exact text")
		}
		if findRecord(t, fin["accounts"], saving.ID)["archived_at"] == nil {
			t.Fatal("archive state lost")
		}
		goals := root["planning"].(map[string]any)
		g := findRecord(t, goals["goals"], goal.ID)
		if g["completed_at"] == nil || g["archived_at"] == nil || len(goals["checklist_items"].([]any)) != 1 {
			t.Fatal("Goal state lost")
		}
	})
	t.Run("CSV exact amounts links and guards", func(t *testing.T) {
		_, data := export(0, "finance_csv")
		files := unzip(t, data)
		if len(files) != 7 {
			t.Fatalf("ZIP files %d", len(files))
		}
		rows := csvRows(t, files["transactions.csv"])
		if len(rows) != 31 || len(rows[0]) != 18 {
			t.Fatal("CSV shape")
		}
		one := 0
		for _, row := range rows[1:] {
			if row[0] == transfer.ID {
				one++
				if row[10] != "" || row[11] != bank.ID || row[12] != saving.ID || row[14] != "false" {
					t.Fatal("transfer changed")
				}
			}
			if row[0] == voided.ID && row[13] != "false" {
				t.Fatal("voided balance flag")
			}
		}
		if one != 1 {
			t.Fatal("duplicated transfer")
		}
		amounts := csvRows(t, files["accounts.csv"])
		for _, row := range amounts[1:] {
			if row[0] == bank.ID && row[1] != "'=PRIVATE-CANARY, \"bank\"\n第二行" {
				t.Fatal("unsafe/changed user text")
			}
			if row[4] == "'-9223372036854775808" && row[5] != "'-9223372036854775808" {
				t.Fatal("zero-scale minimum changed")
			}
		}
		allocations := csvRows(t, files["allocations.csv"])
		matches := 0
		for _, row := range allocations[1:] {
			if row[1] == expense.ID {
				matches++
			}
			if row[1] == deleted.ID {
				t.Fatal("deleted allocation leaked")
			}
		}
		if matches != 2 {
			t.Fatal("allocation split lost")
		}
		if !bytes.Contains(files["README.txt"], []byte("1234 -> 1.234")) {
			t.Fatal("README decimal examples missing")
		}
	})
	t.Run("empty and other owner", func(t *testing.T) {
		for _, who := range []int{1, 2} {
			_, data := export(who, "json")
			root := readJSON(data)
			verifyJSONShape(t, root)
			if strings.Contains(string(data), bank.ID) || strings.Contains(string(data), goal.ID) {
				t.Fatal("cross-owner export leak")
			}
			if root["preferences"].(map[string]any)["time_zone"] != nil {
				t.Fatal("timezone fabricated")
			}
			if who == 2 && len(root["finance"].(map[string]any)["events"].([]any)) != 0 {
				t.Fatal("empty export not empty")
			}
		}
	})
	t.Run("guards and strict download request", func(t *testing.T) {
		for _, tc := range []struct {
			who                          int
			method, path, body, o, token string
			want                         int
		}{
			{-1, "POST", "/api/v1/me/export", `{"format":"json"}`, origin, "", 401},
			{0, "POST", "/api/v1/me/export", `{"format":"json"}`, "https://evil.invalid", csrf[0], 403},
			{0, "POST", "/api/v1/me/export", `{"format":"json"}`, origin, "bad", 403},
			{0, "GET", "/api/v1/me/export", "", origin, "", 405},
			{0, "POST", "/api/v1/me/export?owner_id=" + owners[1], `{"format":"json"}`, origin, csrf[0], 400},
			{0, "POST", "/api/v1/me/export?limit=1", `{"format":"json"}`, origin, csrf[0], 400},
			{0, "POST", "/api/v1/me/export", `{"format":"json","owner_id":"` + owners[1] + `"}`, origin, csrf[0], 400},
			{0, "POST", "/api/v1/me/export", `{"format":"json"}{}`, origin, csrf[0], 400},
			{0, "POST", "/api/v1/me/export", `{"format":"bad"}`, origin, csrf[0], 400},
		} {
			status, h, body := request(tc.who, tc.method, tc.path, tc.body, tc.o, tc.token)
			if status != tc.want || h.Get("Content-Disposition") != "" || !strings.Contains(h.Get("Cache-Control"), "no-store") {
				t.Fatalf("guard %s %d want %d: %s", tc.path, status, tc.want, body)
			}
		}
	})
	t.Run("slot limits cancellation and generation failures", func(t *testing.T) {
		a, err := service.Generate(ctx, owner, "json")
		if err != nil {
			t.Fatal(err)
		}
		b, err := service.Generate(ctx, other, "json")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = service.Generate(ctx, owner, "json"); !errors.Is(err, ErrBusy) {
			t.Fatal("per-owner slot bypass")
		}
		if _, err = service.Generate(ctx, verified[2], "json"); !errors.Is(err, ErrBusy) {
			t.Fatal("global slots bypass")
		}
		for _, who := range []int{0, 2} {
			status, h, _ := request(who, "POST", "/api/v1/me/export", `{"format":"json"}`, origin, csrf[who])
			if status != 429 || h.Get("Retry-After") != "5" || h.Get("Content-Disposition") != "" || !strings.Contains(h.Get("Cache-Control"), "no-store") {
				t.Fatal("busy response lost retry/privacy guarantees")
			}
		}
		a.Close()
		b.Close()
		service.limits.bytes = 64
		status, h, _ := request(0, "POST", "/api/v1/me/export", `{"format":"json"}`, origin, csrf[0])
		if status != 422 || h.Get("Content-Disposition") != "" {
			t.Fatal("partial limit file")
		}
		status, h, _ = request(0, "POST", "/api/v1/me/export", `{"format":"finance_csv"}`, origin, csrf[0])
		if status != 422 || h.Get("Content-Disposition") != "" {
			t.Fatal("partial ZIP limit file")
		}
		service.limits.bytes = MaxBytes
		service.limits.generation = time.Nanosecond
		status, _, _ = request(0, "POST", "/api/v1/me/export", `{"format":"json"}`, origin, csrf[0])
		if status != 503 {
			t.Fatal("timeout not handled")
		}
		service.limits.generation = 120 * time.Second
		service.afterCollection = func(string) error { return errors.New("PRIVATE-FAILURE-CANARY") }
		status, _, body := request(0, "POST", "/api/v1/me/export", `{"format":"finance_csv"}`, origin, csrf[0])
		if status != 500 || bytes.Contains(body, []byte("PRIVATE-FAILURE")) {
			t.Fatal("failure exposed or succeeded")
		}
		service.afterCollection = nil
		cancelled, cancel := context.WithCancel(ctx)
		service.afterCollection = func(string) error { cancel(); return nil }
		if _, err = service.Generate(cancelled, owner, "json"); !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation=%v", err)
		}
		service.afterCollection = nil
		_, _ = export(0, "json")
	})
	t.Run("database snapshot is repeatable read and read only", func(t *testing.T) {
		service.inspectSnapshot = func(tx pgx.Tx) error {
			var isolation, readOnly string
			if err := tx.QueryRow(ctx, `SHOW transaction_isolation`).Scan(&isolation); err != nil {
				return err
			}
			if err := tx.QueryRow(ctx, `SHOW transaction_read_only`).Scan(&readOnly); err != nil {
				return err
			}
			if isolation != "repeatable read" || readOnly != "on" {
				return fmt.Errorf("unsafe snapshot %s/%s", isolation, readOnly)
			}
			return nil
		}
		_, _ = export(0, "json")
		service.inspectSnapshot = func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, `UPDATE finance_accounts SET name='Forbidden read-only change' WHERE owner_id=$1`, owner.String())
			return err
		}
		if _, err := service.Generate(ctx, owner, "json"); err == nil {
			t.Fatal("snapshot allowed business write")
		}
		service.inspectSnapshot = nil
	})
	t.Run("expired disabled and mismatched sessions", func(t *testing.T) {
		hash := sha256.Sum256([]byte(secrets[0]))
		for _, sql := range []string{`UPDATE lifeos_sessions SET created_at=now()-interval '2 hours',expires_at=now()-interval '1 second' WHERE token_hash=$1`, `UPDATE lifeos_sessions SET revoked_at=now() WHERE token_hash=$1`} {
			if _, err := pool.Exec(ctx, sql, hash[:]); err != nil {
				t.Fatal(err)
			}
			status, h, _ := request(0, "POST", "/api/v1/me/export", `{"format":"json"}`, origin, csrf[0])
			if status != 401 || h.Get("Content-Disposition") != "" {
				t.Fatal("inactive session exported")
			}
			if _, err := pool.Exec(ctx, `UPDATE lifeos_sessions SET expires_at=now()+interval '1 hour',revoked_at=NULL WHERE token_hash=$1`, hash[:]); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := pool.Exec(ctx, `UPDATE lifeos_users SET status='disabled' WHERE id=$1`, owner.String()); err != nil {
			t.Fatal(err)
		}
		status, _, _ := request(0, "POST", "/api/v1/me/export", `{"format":"json"}`, origin, csrf[0])
		if status != 401 {
			t.Fatal("disabled owner exported")
		}
		if _, err := pool.Exec(ctx, `UPDATE lifeos_users SET status='active' WHERE id=$1`, owner.String()); err != nil {
			t.Fatal(err)
		}
		req := httptest.NewRequest("POST", "/", nil)
		req.AddCookie(&http.Cookie{Name: "lifeos_session", Value: secrets[1]})
		if !errors.Is(auth.RevalidateOwner(req, owner), identity.ErrNotAuthorized) {
			t.Fatal("owner switched at artifact release")
		}
	})
	t.Run("single snapshot across concurrent changes", func(t *testing.T) {
		reached, release := make(chan struct{}), make(chan struct{})
		service.afterCollection = func(name string) error {
			if name == "accounts" {
				close(reached)
				<-release
			}
			return nil
		}
		result := make(chan *Artifact, 1)
		failure := make(chan error, 1)
		go func() {
			a, e := service.Generate(ctx, owner, "json")
			if e != nil {
				failure <- e
			} else {
				result <- a
			}
		}()
		<-reached
		if _, err = pool.Exec(ctx, `UPDATE finance_accounts SET name='Renamed after snapshot' WHERE owner_id=$1 AND id=$2`, owner.String(), bank.ID); err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, `UPDATE lifeos_users SET time_zone='America/New_York' WHERE id=$1`, owner.String()); err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, `UPDATE goals SET title='Changed after snapshot' WHERE owner_id=$1 AND id=$2`, owner.String(), goal.ID); err != nil {
			t.Fatal(err)
		}
		if err = f.DeleteEvent(ctx, owner, income.ID); err != nil {
			t.Fatal(err)
		}
		close(release)
		var a *Artifact
		select {
		case a = <-result:
		case e := <-failure:
			t.Fatal(e)
		}
		service.afterCollection = nil
		root := readJSON(a.Data)
		a.Close()
		fin := root["finance"].(map[string]any)
		if findRecord(t, fin["accounts"], bank.ID)["name"] != "=PRIVATE-CANARY, \"bank\"\n第二行" || root["preferences"].(map[string]any)["time_zone"] != "Asia/Kuala_Lumpur" || findRecord(t, root["planning"].(map[string]any)["goals"], goal.ID)["title"] != "Owner Goal" {
			t.Fatal("mixed snapshot")
		}
		_ = findRecord(t, fin["events"], income.ID)
		_, next := export(0, "json")
		if bytes.Contains(next, []byte(income.ID)) || !bytes.Contains(next, []byte("Renamed after snapshot")) {
			t.Fatal("fresh retry did not use new snapshot")
		}
	})
	t.Run("substantial complete dataset", func(t *testing.T) {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		if _, err = tx.Exec(ctx, `INSERT INTO financial_events(id,owner_id,kind,amount_minor,currency,occurred_at,note) SELECT md5('i32-large-'||g)::uuid,$1,'income',1,'MYR',now(),'Bulk snapshot fixture' FROM generate_series(1,10000) g`, owner.String()); err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(ctx, `INSERT INTO account_movements(id,owner_id,event_id,account_id,amount_minor,currency) SELECT md5('i32-movement-'||e.id::text)::uuid,e.owner_id,e.id,$2,1,'MYR' FROM financial_events e WHERE e.owner_id=$1 AND e.note='Bulk snapshot fixture'`, owner.String(), bank.ID); err != nil {
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		_, data := export(0, "json")
		root := readJSON(data)
		verifyJSONShape(t, root)
		if len(root["finance"].(map[string]any)["events"].([]any)) != 10029 {
			t.Fatal("large data truncated")
		}
		t.Logf("complete 10,029-event JSON artifact: %d bytes", len(data))
		_, data = export(0, "finance_csv")
		files := unzip(t, data)
		if len(csvRows(t, files["transactions.csv"])) != 10030 {
			t.Fatal("large CSV truncated")
		}
		t.Logf("complete CSV ZIP artifact: %d bytes", len(data))
	})
	t.Run("v2 dates and v1 compatibility", func(t *testing.T) {
		at := now.Add(time.Microsecond)
		preview, err := f.PreviewOpeningBalance(ctx, owner, bank.ID, at, nil)
		if err != nil {
			t.Fatal(err)
		}
		if preview.BeforeCount+preview.OnOrAfterCount < 10000 || *preview.BalanceAfterMinor != preview.BalanceBeforeMinor {
			t.Fatal("preview used incomplete history", preview)
		}
		amount, err := strconv.ParseInt(*preview.ChosenOpeningMinor, 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		revision, _ := strconv.ParseInt(preview.BalanceRevision, 10, 64)
		reached, release := make(chan struct{}), make(chan struct{})
		service.afterCollection = func(name string) error {
			if name == "accounts" {
				close(reached)
				<-release
			}
			return nil
		}
		type result struct {
			a   *Artifact
			err error
		}
		snapshot := make(chan result, 1)
		go func() { a, e := service.Generate(ctx, owner, "json", 2); snapshot <- result{a, e} }()
		<-reached
		if _, err = f.ConfirmOpeningBalance(ctx, owner, bank.ID, at, amount, revision, false); err != nil {
			close(release)
			t.Fatal(err)
		}
		close(release)
		old := <-snapshot
		service.afterCollection = nil
		if old.err != nil {
			t.Fatal(old.err)
		}
		oldRoot := readJSON(old.a.Data)
		old.a.Close()
		oldAccount := findRecord(t, oldRoot["finance"].(map[string]any)["accounts"], bank.ID)
		if oldAccount["opening_balance_at"] != nil || oldAccount["opening_balance_minor"] != "10000" {
			t.Fatal("conversion mixed export snapshot", oldAccount)
		}
		for _, format := range []string{"json", "finance_csv"} {
			status, headers, body := request(0, "POST", "/api/v1/me/export", `{"format":"`+format+`"}`, origin, csrf[0])
			if status != 409 || headers.Get("Content-Disposition") != "" || !bytes.Contains(body, []byte("export_version_incompatible")) {
				t.Fatalf("v1 silently redefined: %d %s", status, body)
			}
			status, headers, body = request(0, "POST", "/api/v1/me/export", `{"format":"`+format+`","export_version":2}`, origin, csrf[0])
			if status != 200 || !strings.Contains(headers.Get("Content-Disposition"), "-v2-") {
				t.Fatalf("v2 failed: %d %s", status, body)
			}
			if format == "json" {
				root := readJSON(body)
				if root["format"] != "lifeos-export-v2" || root["format_version"] != float64(2) {
					t.Fatal("wrong v2 identity")
				}
				fin := root["finance"].(map[string]any)
				a := findRecord(t, fin["accounts"], bank.ID)
				if len(a) != 11 || a["opening_balance_basis"] != "known_balance_before_instant" || a["opening_balance_at"] != at.Format(time.RFC3339Nano) || a["opening_balance_minor"] != *preview.ChosenOpeningMinor || a["opening_balance_confirmed_at"] == nil {
					t.Fatalf("wrong account: %v", a)
				}
				legacy := findRecord(t, fin["accounts"], saving.ID)
				if legacy["opening_balance_at"] != nil || legacy["opening_balance_confirmed_at"] != nil || legacy["opening_balance_basis"] != "legacy_all_movements" {
					t.Fatal("legacy semantics changed")
				}
				// All pre-date events remain exported; reconstruct using complete movements and each account's own date.
				sums := map[string]*big.Int{}
				events := map[string]map[string]any{}
				accounts := map[string]map[string]any{}
				for _, v := range fin["accounts"].([]any) {
					r := v.(map[string]any)
					accounts[r["id"].(string)] = r
					sums[r["id"].(string)], _ = new(big.Int).SetString(r["opening_balance_minor"].(string), 10)
				}
				for _, v := range fin["events"].([]any) {
					r := v.(map[string]any)
					events[r["id"].(string)] = r
				}
				for _, v := range fin["movements"].([]any) {
					r := v.(map[string]any)
					e := events[r["event_id"].(string)]
					a := accounts[r["account_id"].(string)]
					occurrence, _ := time.Parse(time.RFC3339Nano, e["occurred_at"].(string))
					if e["status"] != "posted" {
						continue
					}
					if a["opening_balance_at"] != nil {
						anchor, _ := time.Parse(time.RFC3339Nano, a["opening_balance_at"].(string))
						if occurrence.Before(anchor) {
							continue
						}
					}
					n, _ := new(big.Int).SetString(r["amount_minor"].(string), 10)
					sums[r["account_id"].(string)].Add(sums[r["account_id"].(string)], n)
				}
				balance, err := f.Balance(ctx, owner, bank.ID)
				if err != nil || sums[bank.ID].String() != strconv.FormatInt(balance.AmountMinor, 10) {
					t.Fatal("v2 could not reconstruct balance", err)
				}
				_ = findRecord(t, fin["events"], expense.ID)
				if bytes.Contains(body, []byte("balance_revision")) || bytes.Contains(body, []byte("assertion_choice")) || bytes.Contains(body, []byte(deleted.ID)) {
					t.Fatal("internal/deleted record leaked")
				}
			} else {
				files := unzip(t, body)
				rows := csvRows(t, files["accounts.csv"])
				if strings.Join(rows[0], ",") != "account_id,name,account_type,currency,opening_balance_minor,opening_balance_decimal,opening_balance_basis,opening_balance_at_utc,opening_balance_confirmed_at_utc,archived_at_utc,created_at_utc,updated_at_utc" {
					t.Fatal("v2 CSV account order")
				}
				for _, name := range []string{"categories", "events", "movements", "allocations"} {
					file := name
					if name == "events" {
						file = "transactions"
					}
					if strings.Join(csvRows(t, files[file+".csv"])[0], ",") != strings.Join(csvHeaders[name], ",") {
						t.Fatal("other CSV header changed")
					}
				}
				if !bytes.Contains(files["manifest.json"], []byte("lifeos-finance-csv-v2")) || !bytes.Contains(files["README.txt"], []byte("opening_balance_at")) {
					t.Fatal("v2 explanation absent")
				}
			}
		}
		// Unconverted owners retain v1; unsupported versions are rejected before file release.
		_, _ = export(1, "json")
		for _, version := range []string{"0", "3", "null"} {
			status, h, _ := request(0, "POST", "/api/v1/me/export", `{"format":"json","export_version":`+version+`}`, origin, csrf[0])
			if status != 400 {
				t.Fatalf("invalid version accepted %s: %d", version, status)
			}
			if h.Get("Content-Disposition") != "" {
				t.Fatal("invalid request released artifact")
			}
		}
	})
	t.Run("session revoked before finished artifact release", func(t *testing.T) {
		service.afterCollection = func(name string) error {
			if name == "allocations" {
				_, err := pool.Exec(ctx, `UPDATE lifeos_sessions SET revoked_at=now() WHERE user_id=$1`, owner.String())
				return err
			}
			return nil
		}
		status, h, _ := request(0, "POST", "/api/v1/me/export", `{"format":"json","export_version":2}`, origin, csrf[0])
		service.afterCollection = nil
		if status != 401 || h.Get("Content-Disposition") != "" {
			t.Fatal("revoked artifact released")
		}
	})
	// Export itself leaves financial calculations unchanged (the intentional concurrent Delete above removed income).
	after, err := f.Balance(ctx, owner, bank.ID)
	if err != nil || after.AmountMinor != before.AmountMinor-1000+10000 {
		t.Fatalf("export changed money: before=%d after=%d err=%v", before.AmountMinor, after.AmountMinor, err)
	}
	if strings.Contains(logs.String(), "PRIVATE-CANARY") || strings.Contains(logs.String(), "PRIVATE-FAILURE") || strings.Contains(logs.String(), secrets[0]) || strings.Contains(logs.String(), "export0@example.invalid") {
		t.Fatal("sensitive log content")
	}
}

func findRecord(t *testing.T, records any, id string) map[string]any {
	t.Helper()
	for _, r := range records.([]any) {
		row := r.(map[string]any)
		if row["id"] == id {
			return row
		}
	}
	t.Fatalf("record %s missing", id)
	return nil
}
func unzip(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	files := map[string][]byte{}
	for _, f := range r.File {
		stream, err := f.Open()
		if err != nil {
			t.Fatal(err)
		}
		b, err := io.ReadAll(stream)
		stream.Close()
		if err != nil {
			t.Fatal(err)
		}
		if _, exists := files[f.Name]; exists {
			t.Fatal("duplicate ZIP name")
		}
		files[f.Name] = b
	}
	return files
}
func csvRows(t *testing.T, b []byte) [][]string {
	t.Helper()
	if !bytes.HasPrefix(b, []byte{0xef, 0xbb, 0xbf}) || !bytes.HasSuffix(b, []byte("\r\n")) {
		t.Fatal("CSV BOM/CRLF missing")
	}
	r, err := csv.NewReader(bytes.NewReader(b[3:])).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func verifyJSONShape(t *testing.T, root map[string]any) {
	t.Helper()
	if len(root) != 8 {
		t.Fatalf("root fields %d", len(root))
	}
	expected := map[string][]string{
		"accounts":        strings.Split("id,name,account_type,currency,opening_balance_minor,archived_at,created_at,updated_at", ","),
		"categories":      strings.Split("id,name,kind,archived_at,created_at,updated_at", ","),
		"events":          strings.Split("id,kind,amount_minor,currency,occurred_at,note,status,voided_at,void_reason,created_at", ","),
		"movements":       strings.Split("id,event_id,account_id,amount_minor,currency,created_at", ","),
		"allocations":     strings.Split("id,event_id,category_id,kind,amount_minor,currency,created_at", ","),
		"goals":           strings.Split("id,title,completed_at,archived_at,created_at,updated_at", ","),
		"checklist_items": strings.Split("id,goal_id,title,completed_at,created_at,updated_at", ","),
	}
	ids := map[string]map[string]bool{}
	counts := root["record_counts"].(map[string]any)
	for _, collection := range collections {
		module := "finance"
		if collection == "goals" || collection == "checklist_items" {
			module = "planning"
		}
		rows := root[module].(map[string]any)[collection].([]any)
		if counts[collection] != float64(len(rows)) {
			t.Fatal("wrong counts")
		}
		ids[collection] = map[string]bool{}
		for _, v := range rows {
			r := v.(map[string]any)
			if len(r) != len(expected[collection]) {
				t.Fatalf("extra/missing %s fields: %v", collection, r)
			}
			for _, k := range expected[collection] {
				value, ok := r[k]
				if !ok {
					t.Fatalf("missing %s.%s", collection, k)
				}
				if strings.HasSuffix(k, "_minor") {
					if _, ok := value.(string); !ok {
						t.Fatal("money is not a string")
					}
				}
				ids[collection][r["id"].(string)] = true
			}
		}
	}
	for _, v := range root["finance"].(map[string]any)["movements"].([]any) {
		r := v.(map[string]any)
		if !ids["events"][r["event_id"].(string)] || !ids["accounts"][r["account_id"].(string)] {
			t.Fatal("dangling movement")
		}
	}
	for _, v := range root["finance"].(map[string]any)["allocations"].([]any) {
		r := v.(map[string]any)
		if !ids["events"][r["event_id"].(string)] || !ids["categories"][r["category_id"].(string)] {
			t.Fatal("dangling allocation")
		}
	}
	for _, v := range root["planning"].(map[string]any)["checklist_items"].([]any) {
		r := v.(map[string]any)
		if !ids["goals"][r["goal_id"].(string)] {
			t.Fatal("dangling checklist")
		}
	}
}
