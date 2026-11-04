package finance

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Opt-in destructive workload isolation: only a NEW *_test database on the
// dedicated named container is accepted. Restart clears PG shared buffers, not
// the host OS disk cache; this is deliberately not called a cold-disk benchmark.
func TestHistoryPerformance(t *testing.T) {
	dsn := os.Getenv("LIFEOS_HISTORY_BENCH_DATABASE_URL")
	if dsn == "" {
		t.Skip("opt-in synthetic 10k/100k benchmark")
	}
	if !strings.Contains(dsn, "@127.0.0.1:55436/lifeos_i36_") || os.Getenv("LIFEOS_HISTORY_BENCH_RESTART_CONTAINER") != "lifeos-i36-20261102" {
		t.Fatal("benchmark requires the dedicated Issue #36 container and explicit restart opt-in")
	}
	fixture, runner := financeTestDatabase(t, "LIFEOS_HISTORY_BENCH_DATABASE_URL")
	ctx := context.Background()
	if err := runner.Migrate(10); err != nil {
		t.Fatal(err)
	}
	owners := []string{"10101010-1010-4010-8010-101010101010", "20202020-2020-4020-8020-202020202020"}
	counts := []int{10000, 100000}
	for i, owner := range owners {
		verifiedTestOwner(t, fixture, owner, fmt.Sprintf("bench-%d", i))
		if _, err := fixture.Exec(ctx, `UPDATE lifeos_users SET time_zone='Asia/Kuala_Lumpur' WHERE id=$1`, owner); err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.Exec(ctx, `INSERT INTO finance_accounts(id,owner_id,name,account_type,currency,opening_balance_minor,opening_balance_at,opening_balance_confirmed_at)
   SELECT md5($1||j::text)::uuid,$1::uuid,'Synthetic '||j,'bank','MYR',1000000000,'2020-01-01T00:00:00Z'::timestamptz,now() FROM generate_series(1,2) j`, owner); err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.Exec(ctx, `INSERT INTO financial_events(id,owner_id,kind,amount_minor,currency,occurred_at,note,status,voided_at,void_reason,deleted_at)
   SELECT md5($1||'event'||j::text)::uuid,$1::uuid,CASE WHEN j%20=0 THEN 'transfer' WHEN j%10=1 THEN 'income' ELSE 'expense' END,100,'MYR',
   '2026-01-01T00:00:00Z'::timestamptz+j*interval '1 minute','Synthetic benchmark',CASE WHEN j%20=2 THEN 'voided' ELSE 'posted' END,
   CASE WHEN j%20=2 THEN now() END,CASE WHEN j%20=2 THEN 'Synthetic void' END,CASE WHEN j%20=3 THEN now() END
   FROM generate_series(1,$2::integer) j`, owner, counts[i]); err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.Exec(ctx, `INSERT INTO account_movements(id,owner_id,event_id,account_id,amount_minor,currency)
   SELECT md5(e.id::text||'source')::uuid,e.owner_id,e.id,md5($1||'1')::uuid,CASE WHEN e.kind='income' THEN 100 ELSE -100 END,'MYR' FROM financial_events e WHERE e.owner_id=$1::uuid
   UNION ALL SELECT md5(e.id::text||'destination')::uuid,e.owner_id,e.id,md5($1||'2')::uuid,100,'MYR' FROM financial_events e WHERE e.owner_id=$1::uuid AND e.kind='transfer'`, owner); err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.Exec(ctx, `INSERT INTO finance_categories(id,owner_id,name,kind,archived_at)
 SELECT md5($1||part)::uuid,$1::uuid,'Synthetic '||part,'expense',CASE WHEN part='medical' THEN now() END FROM unnest(ARRAY['food','medical']) part`, owner); err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.Exec(ctx, `INSERT INTO category_allocations(id,owner_id,event_id,category_id,kind,amount_minor,currency)
 SELECT md5(e.id::text||part)::uuid,e.owner_id,e.id,md5($1||part)::uuid,'expense',CASE WHEN part='food' THEN 60 ELSE 40 END,'MYR'
 FROM financial_events e CROSS JOIN unnest(ARRAY['food','medical']) part WHERE e.owner_id=$1::uuid AND e.kind='expense' AND get_byte(uuid_send(e.id),0)%4=0`, owner); err != nil {
			t.Fatal(err)
		}
		if _, err := fixture.Exec(ctx, `SELECT capture_financial_event_revision(owner_id,id,'legacy_baseline',NULL) FROM financial_events WHERE owner_id=$1`, owner); err != nil {
			t.Fatal(err)
		}
	}
	if err := runner.Up(); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.Exec(ctx, `ANALYZE`); err != nil {
		t.Fatal(err)
	}
	fixture.Close() // Never restart while other suite databases are in use.
	provider := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"issuer": "http://" + r.Host, "authorization_endpoint": "http://" + r.Host + "/authorize", "token_endpoint": "http://" + r.Host + "/token", "jwks_uri": "http://" + r.Host + "/keys", "response_types_supported": []string{"code"}, "subject_types_supported": []string{"public"}, "id_token_signing_alg_values_supported": []string{"RS256"}})
	}))
	defer provider.Close()
	for i, owner := range owners {
		cold, warm := []float64{}, []float64{}
		for round := 0; round < 3; round++ {
			if data, err := exec.Command("docker", "restart", "lifeos-i36-20261102").CombinedOutput(); err != nil {
				t.Fatalf("dedicated restart: %v %s", err, data)
			}
			ready := false
			for n := 0; n < 100; n++ {
				if exec.Command("docker", "exec", "lifeos-i36-20261102", "pg_isready", "-U", "postgres").Run() == nil {
					ready = true
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
			if !ready {
				t.Fatal("dedicated database did not restart")
			}
			pool, err := pgxpool.New(ctx, dsn)
			if err != nil {
				t.Fatal(err)
			}
			router, _, _ := idempotencyTestRouter(t, pool, provider.URL)
			server := httptest.NewServer(router)
			client := server.Client()
			read := func() float64 {
				req, _ := http.NewRequest("GET", server.URL+"/api/v1/events?view=running_balances&limit=10", nil)
				req.AddCookie(&http.Cookie{Name: "lifeos_session", Value: fmt.Sprintf("bench-%d", i)})
				begin := time.Now()
				res, e := client.Do(req)
				if e != nil {
					t.Fatal(e)
				}
				payload, e := io.ReadAll(res.Body)
				res.Body.Close()
				if e != nil || res.StatusCode != 200 {
					t.Fatalf("benchmark read %d %s %v", res.StatusCode, payload, e)
				}
				var page runningHistoryResponse
				if json.Unmarshal(payload, &page) != nil || len(page.Items) != 10 || len(page.Items[0].Positions) == 0 {
					t.Fatal("invalid benchmark result")
				}
				return float64(time.Since(begin).Microseconds()) / 1000
			}
			cold = append(cold, read())
			for n := 0; n < 10; n++ {
				warm = append(warm, read())
			}
			if round == 2 {
				var size int64
				_ = pool.QueryRow(ctx, `SELECT pg_database_size(current_database())`).Scan(&size)
				var settings string
				_ = pool.QueryRow(ctx, `SELECT json_build_object('version',version(),'shared_buffers',current_setting('shared_buffers'),'work_mem',current_setting('work_mem'),'max_connections',current_setting('max_connections'))::text`).Scan(&settings)
				t.Logf("RESOURCE owner_events=%d database_bytes=%d settings=%s", counts[i], size, settings)
				rows, e := pool.Query(ctx, `EXPLAIN(ANALYZE,BUFFERS,SETTINGS) `+runningBalancesSQL, owner, []string{md5BenchmarkID(t, pool, owner, "1"), md5BenchmarkID(t, pool, owner, "2")}, []string{md5BenchmarkID(t, pool, owner, "event"+fmt.Sprint(counts[i]))})
				if e != nil {
					t.Fatal(e)
				}
				for rows.Next() {
					var line string
					if e = rows.Scan(&line); e != nil {
						t.Fatal(e)
					}
					t.Log("EXPLAIN", line)
				}
				if e = rows.Err(); e != nil {
					t.Fatal(e)
				}
				rows.Close()
			}
			server.Close()
			pool.Close()
		}
		sort.Float64s(warm)
		t.Logf("BENCH events=%d cold_shared_buffers_ms=%v warm_n=%d warm_median_ms=%.3f warm_p95_ms=%.3f warm_max_ms=%.3f", counts[i], cold, len(warm), warm[len(warm)/2], warm[(len(warm)*95+99)/100-1], warm[len(warm)-1])
	}
}

func md5BenchmarkID(t *testing.T, pool *pgxpool.Pool, owner, suffix string) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(context.Background(), `SELECT md5($1||$2)::uuid`, owner, suffix).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
