package app

import (
	"fmt"
	"net/http"
)

// Keep the audit and encrypted originals, but remove the record from active use.
func (a *App) companySetupDelete(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if u.Role != "Super Admin" {
		return nil, forbidden()
	}
	d, e := readBody(w, r)
	if e != nil {
		return nil, e
	}
	password, _ := d["password"].(string)
	if len(password) > 128 {
		return nil, bad("Enter your current password.")
	}
	attempts, err := a.Redis.Eval(r.Context(), `local n=redis.call('INCR',KEYS[1]); if n==1 then redis.call('EXPIRE',KEYS[1],900) end; return n`, []string{fmt.Sprintf("company-delete:%d", u.ID)}).Int64()
	if err != nil {
		return nil, err
	}
	if attempts > 8 {
		return nil, failure{429, "Too many deletion attempts. Try again in 15 minutes."}
	}
	var hash string
	if e = a.DB.QueryRow(r.Context(), `SELECT password_hash FROM users WHERE id=$1`, u.ID).Scan(&hash); e != nil {
		return nil, e
	}
	if !checkPassword(password, hash) {
		return nil, forbidden()
	}
	ctx := r.Context()
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	c, e := a.setupRow(ctx, tx, setupID(r), true)
	if e != nil {
		return nil, e
	}
	if id(d, "version") != id(c, "version") {
		return nil, failure{409, "This company changed. Reload before deleting."}
	}
	confirmation, ok := d["confirmation_name"].(string)
	if !ok || confirmation != str(c, "legal_name") {
		return nil, bad("Type the full company name exactly as shown to delete it.")
	}
	if _, e = tx.Exec(ctx, `UPDATE company_setups SET deleted_at=now(),deleted_by=$1,version=version+1,updated_at=now() WHERE id=$2`, u.ID, id(c, "id")); e != nil {
		return nil, e
	}
	if _, e = tx.Exec(ctx, `UPDATE company_reminders SET active=false WHERE company_id=$1`, id(c, "id")); e != nil {
		return nil, e
	}
	if e = setupHistory(ctx, tx, u, c, "Deleted", "Super Admin confirmed the full company name and current password"); e != nil {
		return nil, e
	}
	if _, e = tx.Exec(ctx, `UPDATE companies SET status='Rejected' WHERE setup_id=$1`, id(c, "id")); e != nil {
		return nil, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, e
	}
	a.Redis.Del(ctx, fmt.Sprintf("company-delete:%d", u.ID))
	return M{"ok": true}, nil
}
