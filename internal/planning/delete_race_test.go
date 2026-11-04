package planning

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/Faust217/LifeOS/internal/identity"
	"github.com/jackc/pgx/v5/pgxpool"
)

func verifyGoalDeleteRaces(t *testing.T, pool *pgxpool.Pool, service *Service, owner identity.OwnerID) {
	ctx := context.Background()
	_, err := pool.Exec(ctx, `CREATE FUNCTION pause_goal_writer() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF current_setting('application_name')='goal38-first' THEN PERFORM pg_advisory_xact_lock(380038); END IF; IF TG_OP='DELETE' THEN RETURN OLD; END IF; RETURN NEW; END $$;
 CREATE TRIGGER pause_goal_writer BEFORE UPDATE ON goals FOR EACH ROW EXECUTE FUNCTION pause_goal_writer();
 CREATE TRIGGER pause_goal_item BEFORE INSERT OR UPDATE OR DELETE ON goal_checklist_items FOR EACH ROW EXECUTE FUNCTION pause_goal_writer()`)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = pool.Exec(ctx, `DROP TRIGGER pause_goal_writer ON goals; DROP TRIGGER pause_goal_item ON goal_checklist_items; DROP FUNCTION pause_goal_writer()`)
	}()
	for _, operation := range []string{"rename", "archive", "restore", "complete", "reopen", "add-item", "rename-item", "complete-item", "reopen-item", "remove-item", "delete"} {
		for _, deleteFirst := range []bool{false, true} {
			order := "mutation-first"
			if deleteFirst {
				order = "delete-first"
			}
			t.Run(operation+"/"+order, func(t *testing.T) {
				g, e := service.CreateGoal(ctx, owner, "Race "+operation)
				if e != nil {
					t.Fatal(e)
				}
				g, e = service.AddItem(ctx, owner, g.ID, "Original step")
				if e != nil {
					t.Fatal(e)
				}
				if operation == "complete" || operation == "reopen" || operation == "reopen-item" {
					g, e = service.SetItemCompleted(ctx, owner, g.ID, g.Items[0].ID, true)
					if e != nil {
						t.Fatal(e)
					}
				}
				if operation == "reopen" {
					g, e = service.SetGoalCompleted(ctx, owner, g.ID, true)
					if e != nil {
						t.Fatal(e)
					}
				}
				if operation == "restore" {
					g, e = service.ArchiveGoal(ctx, owner, g.ID)
					if e != nil {
						t.Fatal(e)
					}
				}
				newPool := func(name string) *pgxpool.Pool {
					cfg := pool.Config().Copy()
					cfg.ConnConfig.RuntimeParams["application_name"] = name
					cfg.MaxConns = 2
					p, x := pgxpool.NewWithConfig(ctx, cfg)
					if x != nil {
						t.Fatal(x)
					}
					return p
				}
				firstPool, secondPool := newPool("goal38-first"), newPool("goal38-second")
				defer firstPool.Close()
				defer secondPool.Close()
				firstService, secondService := NewService(firstPool), NewService(secondPool)
				change := func(s *Service) error {
					var e error
					switch operation {
					case "rename":
						_, e = s.RenameGoal(ctx, owner, g.ID, "Changed before deletion")
					case "archive":
						_, e = s.ArchiveGoal(ctx, owner, g.ID)
					case "restore":
						_, e = s.RestoreGoal(ctx, owner, g.ID)
					case "complete":
						_, e = s.SetGoalCompleted(ctx, owner, g.ID, true)
					case "reopen":
						_, e = s.SetGoalCompleted(ctx, owner, g.ID, false)
					case "add-item":
						_, e = s.AddItem(ctx, owner, g.ID, "Added before deletion")
					case "rename-item":
						_, e = s.RenameItem(ctx, owner, g.ID, g.Items[0].ID, "Changed step")
					case "complete-item":
						_, e = s.SetItemCompleted(ctx, owner, g.ID, g.Items[0].ID, true)
					case "reopen-item":
						_, e = s.SetItemCompleted(ctx, owner, g.ID, g.Items[0].ID, false)
					case "remove-item":
						_, e = s.RemoveItem(ctx, owner, g.ID, g.Items[0].ID)
					case "delete":
						e = s.DeleteGoal(ctx, owner, g.ID)
					}
					return e
				}
				barrier, e := pool.Acquire(ctx)
				if e != nil {
					t.Fatal(e)
				}
				defer barrier.Release()
				if _, e = barrier.Exec(ctx, `SELECT pg_advisory_lock(380038)`); e != nil {
					t.Fatal(e)
				}
				released := false
				defer func() {
					if !released {
						_, _ = barrier.Exec(ctx, `SELECT pg_advisory_unlock(380038)`)
					}
				}()
				first, second := make(chan error, 1), make(chan error, 1)
				go func() {
					if deleteFirst {
						first <- firstService.DeleteGoal(ctx, owner, g.ID)
					} else {
						first <- change(firstService)
					}
				}()
				wait := func(app string) {
					t.Helper()
					deadline := time.Now().Add(time.Second)
					for time.Now().Before(deadline) {
						var waiting bool
						e := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE datname=current_database() AND application_name=$1 AND wait_event_type='Lock')`, app).Scan(&waiting)
						if e != nil {
							t.Fatal(e)
						}
						if waiting {
							return
						}
						time.Sleep(5 * time.Millisecond)
					}
					t.Fatal("writer did not reach lock barrier", app)
				}
				wait("goal38-first")
				go func() {
					if deleteFirst {
						second <- change(secondService)
					} else {
						second <- secondService.DeleteGoal(ctx, owner, g.ID)
					}
				}()
				wait("goal38-second")
				if _, e = barrier.Exec(ctx, `SELECT pg_advisory_unlock(380038)`); e != nil {
					t.Fatal(e)
				}
				released = true
				e1, e2 := <-first, <-second
				if deleteFirst {
					if e1 != nil {
						t.Fatal(e1)
					}
					if operation == "delete" {
						if e2 != nil {
							t.Fatal(e2)
						}
					} else if !errors.Is(e2, ErrNotFound) {
						t.Fatal("mutation bypassed deleted parent", e2)
					}
				} else {
					if e2 != nil {
						t.Fatal(e2)
					}
					if e1 != nil && !errors.Is(e1, ErrNotFound) {
						t.Fatal(e1)
					}
				}
				if _, e = service.GetGoal(ctx, owner, g.ID); !errors.Is(e, ErrNotFound) {
					t.Fatal("deleted Goal visible", e)
				}
				var deleted bool
				var title string
				var completed, archived *time.Time
				if e = pool.QueryRow(ctx, `SELECT deleted_at IS NOT NULL,title,completed_at,archived_at FROM goals WHERE id=$1`, g.ID).Scan(&deleted, &title, &completed, &archived); e != nil || !deleted {
					t.Fatal(e)
				}
				if !deleteFirst {
					var count, done int
					var itemTitle string
					if e = pool.QueryRow(ctx, `SELECT count(*),count(*) FILTER(WHERE completed_at IS NOT NULL),COALESCE(min(title),'') FROM goal_checklist_items WHERE goal_id=$1`, g.ID).Scan(&count, &done, &itemTitle); e != nil {
						t.Fatal(e)
					}
					switch operation {
					case "rename":
						if title != "Changed before deletion" {
							t.Fatal(title)
						}
					case "archive":
						if archived == nil {
							t.Fatal("archive lost")
						}
					case "restore":
						if archived != nil {
							t.Fatal("restore lost")
						}
					case "complete":
						if completed == nil {
							t.Fatal("completion lost")
						}
					case "reopen":
						if completed != nil {
							t.Fatal("reopen lost")
						}
					case "add-item":
						if count != 2 {
							t.Fatal(count)
						}
					case "remove-item":
						if count != 0 {
							t.Fatal(count)
						}
					case "rename-item":
						if itemTitle != "Changed step" {
							t.Fatal(itemTitle)
						}
					case "complete-item":
						if done != 1 {
							t.Fatal(done)
						}
					case "reopen-item":
						if done != 0 {
							t.Fatal(done)
						}
					}
				}
			})
		}
	}
}
