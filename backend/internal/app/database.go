package app

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Each query/transaction gets its own transaction-local identity. Never set
// session-wide user state on a pooled connection: it can leak to the next user.
type Database struct {
	*pgxpool.Pool
	scoped bool
}
type serviceKey struct{}

func (d *Database) Begin(ctx context.Context) (pgx.Tx, error) {
	tx, err := d.Pool.Begin(ctx)
	if err != nil || !d.scoped {
		return tx, err
	}
	identity := M{}
	if u, ok := ctx.Value(authKey{}).(*User); ok {
		identity = M{"id": u.ID, "role": u.Role, "developer": u.IsDeveloper, "scope": u.Scope, "companies": u.Companies, "teams": u.Teams, "lines": u.Lines, "orders": u.Orders, "units": u.Units, "view_orders": u.Can("view_orders"), "enquiries": enquiryRead(u), "exports": exportRead(u), "exports_edit": exportEditor(u), "view_setup": u.SetupCan("view"), "restrict_buyers": u.RestrictBuyers, "buyer_ids": u.BuyerIDs, "production_areas": u.ProductionAreas}
		identity["exports_files"] = exportCan(u, "files")
		identity["exports_download"] = exportCan(u, "download")
	}
	raw, err := json.Marshal(identity)
	if err == nil {
		_, err = tx.Exec(ctx, `SELECT set_config('erp.identity',$1,true),set_config('erp.service',$2,true)`, string(raw), serviceName(ctx))
	}
	if err != nil {
		tx.Rollback(ctx)
		return nil, err
	}
	return tx, nil
}
func serviceName(ctx context.Context) string { s, _ := ctx.Value(serviceKey{}).(string); return s }
func (d *Database) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if !d.scoped {
		return d.Pool.Exec(ctx, sql, args...)
	}
	tx, err := d.Begin(ctx)
	if err != nil {
		return pgconn.CommandTag{}, err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, sql, args...)
	if err == nil {
		err = tx.Commit(ctx)
	}
	return tag, err
}

type scopedRow struct {
	row pgx.Row
	tx  pgx.Tx
	ctx context.Context
	err error
}

func (r *scopedRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	defer r.tx.Rollback(r.ctx)
	if err := r.row.Scan(dest...); err != nil {
		return err
	}
	return r.tx.Commit(r.ctx)
}
func (d *Database) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if !d.scoped {
		return d.Pool.QueryRow(ctx, sql, args...)
	}
	tx, err := d.Begin(ctx)
	r := &scopedRow{tx: tx, ctx: ctx, err: err}
	if err == nil {
		r.row = tx.QueryRow(ctx, sql, args...)
	}
	return r
}

type scopedRows struct {
	pgx.Rows
	tx        pgx.Tx
	ctx       context.Context
	finished  bool
	commitErr error
}

func (r *scopedRows) Close() {
	if r.finished {
		return
	}
	r.finished = true
	r.Rows.Close()
	if r.Rows.Err() != nil {
		r.tx.Rollback(r.ctx)
	} else {
		r.commitErr = r.tx.Commit(r.ctx)
	}
}
func (r *scopedRows) Next() bool {
	ok := r.Rows.Next()
	if !ok {
		r.Close()
	}
	return ok
}
func (r *scopedRows) Err() error {
	if e := r.Rows.Err(); e != nil {
		return e
	}
	return r.commitErr
}
func (d *Database) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if !d.scoped {
		return d.Pool.Query(ctx, sql, args...)
	}
	tx, err := d.Begin(ctx)
	if err != nil {
		return nil, err
	}
	rs, err := tx.Query(ctx, sql, args...)
	if err != nil {
		tx.Rollback(ctx)
		return nil, err
	}
	return &scopedRows{Rows: rs, tx: tx, ctx: ctx}, nil
}
func (a *App) EnableRuntimeSecurity(ctx context.Context) error {
	var unsafe bool
	err := a.DB.Pool.QueryRow(ctx, `SELECT r.rolsuper OR r.rolbypassrls OR r.rolcreaterole OR r.rolcreatedb OR EXISTS(SELECT 1 FROM pg_tables WHERE schemaname='public' AND tableowner=current_user) FROM pg_roles r WHERE rolname=current_user`).Scan(&unsafe)
	if err != nil {
		return err
	}
	if unsafe {
		return errors.New("API must use a non-owner database login without administrative privileges; run scripts/provision_database.py")
	}
	a.DB.scoped = true
	return nil
}
