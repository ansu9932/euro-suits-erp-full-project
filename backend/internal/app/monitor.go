package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

func (a *App) health(w http.ResponseWriter, r *http.Request) (any, error) {
	ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
	defer cancel()
	if a.DB.Ping(ctx) != nil || a.Redis.Ping(ctx).Err() != nil {
		return nil, failure{503, "Service is temporarily unavailable."}
	}
	return M{"status": "ok"}, nil
}
func (a *App) metrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	fmt.Fprintf(w, "# TYPE erp_http_requests_total counter\nerp_http_requests_total %d\n# TYPE erp_http_errors_total counter\nerp_http_errors_total %d\n# TYPE erp_db_connections gauge\nerp_db_connections %d\n", a.Requests.Load(), a.Failures.Load(), a.DB.Stat().TotalConns())
	v, e := a.Redis.Get(r.Context(), "worker:last_success").Int64()
	if e == nil {
		fmt.Fprintf(w, "# TYPE erp_worker_last_success_seconds gauge\nerp_worker_last_success_seconds %d\n", v)
	}
}
func (a *App) Worker(ctx context.Context) {
	ctx = context.WithValue(ctx, serviceKey{}, "reminders")
	tick := time.NewTicker(time.Minute)
	defer tick.Stop()
	for {
		a.refreshReminders(ctx)
		a.companyEmailReminders(ctx)
		a.whatsappTick(ctx)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
func (a *App) refreshReminders(parent context.Context) {
	ctx, cancel := context.WithTimeout(parent, 25*time.Second)
	defer cancel()
	lockToken := token()
	ok, e := a.Redis.SetNX(ctx, "worker:reminders:lock", lockToken, 30*time.Second).Result()
	if e != nil || !ok {
		return
	}
	defer a.Redis.Eval(ctx, `if redis.call('GET',KEYS[1])==ARGV[1] then return redis.call('DEL',KEYS[1]) else return 0 end`, []string{"worker:reminders:lock"}, lockToken)
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return
	}
	defer tx.Rollback(ctx)
	_, e = tx.Exec(ctx, `INSERT INTO reminders(order_id,milestone_id,due_date) SELECT m.order_id,m.id,m.due_date FROM milestones m JOIN orders o ON o.id=m.order_id WHERE NOT m.done AND m.due_date<=CURRENT_DATE+3 AND o.stage<>'Dispatched' AND o.approval<>'Rejected' ON CONFLICT(order_id,milestone_id) DO UPDATE SET due_date=excluded.due_date,updated_at=now(); DELETE FROM reminders r WHERE NOT EXISTS(SELECT 1 FROM milestones m JOIN orders o ON o.id=m.order_id WHERE m.id=r.milestone_id AND NOT m.done AND m.due_date<=CURRENT_DATE+3 AND o.stage<>'Dispatched' AND o.approval<>'Rejected')`)
	if e != nil {
		slog.Error("reminder job failed", "error", e)
		return
	}
	if e = tx.Commit(ctx); e != nil {
		return
	}
	a.Redis.Set(ctx, "worker:last_success", time.Now().Unix(), 24*time.Hour)
}
