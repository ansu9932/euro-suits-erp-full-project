package app

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"
)

// Bootstrap is deliberately CLI-only. No web endpoint can grant owner access.
func (a *App) CreateDeveloper(ctx context.Context) error {
	email := strings.TrimSpace(os.Getenv("DEVELOPER_EMAIL"))
	password := os.Getenv("DEVELOPER_PASSWORD")
	if !validEmail(email) || !validPassword(password) {
		return fmt.Errorf("set a valid DEVELOPER_EMAIL and DEVELOPER_PASSWORD")
	}
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(824733)`); e != nil {
		return e
	}
	var exists bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE is_developer OR username='developer')`).Scan(&exists); e != nil {
		return e
	}
	if exists {
		return fmt.Errorf("developer account already exists; existing credentials were not changed")
	}
	var uid int64
	e = tx.QueryRow(ctx, `INSERT INTO users(username,name,password_hash,role,scope,companies,email,is_developer,mfa_enabled) VALUES('developer','Developer',$1,'Super Admin','company','{}',$2,true,true) RETURNING id`, passwordHash(password), email).Scan(&uid)
	if e != nil {
		return e
	}
	if e = accessAudit(ctx, tx, uid, "Developer account created", "User", uid, M{"mandatory_email_verification": true}); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func developerRequired(r *http.Request) error {
	if !current(r).IsDeveloper {
		return forbidden()
	}
	return nil
}
func (a *App) developerReauthenticate(r *http.Request, d M) error {
	if e := developerRequired(r); e != nil {
		return e
	}
	s := r.Context().Value(sessionKey{}).(Session)
	if !s.MFAVerified || time.Now().Unix()-s.IssuedAt > 900 {
		return failure{403, "Sign out and complete email verification again before making sensitive changes (15-minute limit)."}
	}
	if e := a.limit(r.Context(), fmt.Sprintf("developer:confirm:%d", current(r).ID), 15, 15*time.Minute); e != nil {
		return e
	}
	password, _ := d["password"].(string)
	if len(password) > 128 {
		return bad("Invalid developer password.")
	}
	var hash string
	if e := a.DB.QueryRow(r.Context(), `SELECT password_hash FROM users WHERE id=$1`, current(r).ID).Scan(&hash); e != nil {
		return e
	}
	if !checkPassword(password, hash) {
		return failure{403, "Your developer password is incorrect."}
	}
	return nil
}
func (a *App) developerData(w http.ResponseWriter, r *http.Request) (any, error) {
	if e := developerRequired(r); e != nil {
		return nil, e
	}
	ctx := r.Context()
	out := M{"user": current(r), "local": !a.Production, "current_ip": clientIP(r)}
	queries := map[string]string{
		"users":       `SELECT u.id,u.username,u.name,u.email,u.phone,u.is_developer,u.mfa_enabled,u.ip_restricted,u.allowed_ips,u.role,u.active,u.deleted_at,u.session_version AS version,d.name AS designation FROM users u LEFT JOIN access_designations d ON d.id=u.designation_id ORDER BY u.deleted_at NULLS FIRST,u.name`,
		"issues":      `SELECT i.*,u.username FROM system_issues i LEFT JOIN users u ON u.id=i.user_id ORDER BY CASE WHEN i.status='Resolved' THEN 1 ELSE 0 END,i.last_seen DESC LIMIT 200`,
		"history":     `SELECT h.*,u.username AS actor FROM access_history h JOIN users u ON u.id=h.actor_id ORDER BY h.id DESC LIMIT 100`,
		"companies":   `SELECT c.id,c.legal_name,COALESCE(n.recipients,'{}') AS recipients,COALESCE(n.version,0) AS notification_version FROM company_setups c LEFT JOIN company_notification_settings n ON n.company_id=c.id WHERE c.deleted_at IS NULL ORDER BY c.legal_name`,
		"reminders":   `SELECT cr.id,c.legal_name,cr.label,cr.email_status,cr.last_error,cr.sent_at,cr.next_attempt FROM company_reminders cr JOIN company_setups c ON c.id=cr.company_id WHERE cr.active AND c.deleted_at IS NULL ORDER BY cr.next_attempt LIMIT 100`,
		"flow_alerts": `SELECT id,po,stage,'Required work item is missing' AS issue FROM orders o WHERE approval='Approved' AND stage IN ('Development','Sampling') AND ((stage='Development' AND (NOT EXISTS(SELECT 1 FROM work_items w WHERE w.order_id=o.id AND w.kind='Style development') OR NOT EXISTS(SELECT 1 FROM work_items w WHERE w.order_id=o.id AND w.kind='CAD / pattern'))) OR (stage='Sampling' AND NOT EXISTS(SELECT 1 FROM work_items w WHERE w.order_id=o.id AND w.kind='Pre-production sample'))) ORDER BY id DESC LIMIT 100`,
	}
	for k, q := range queries {
		v, e := rows(ctx, a.DB, q)
		if e != nil {
			return nil, e
		}
		out[k] = v
	}
	notification, e := row(ctx, a.DB, `SELECT * FROM notification_settings WHERE id=1`)
	if e != nil {
		return nil, e
	}
	out["notification_settings"] = notification
	healthCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	out["health"] = M{"database": a.DB.Ping(healthCtx) == nil, "redis": a.Redis.Ping(healthCtx).Err() == nil, "email_configured": emailConfigured(), "email_sender": os.Getenv("SMTP_FROM"), "smtp_host": os.Getenv("SMTP_HOST"), "smtp_port": os.Getenv("SMTP_PORT"), "smtp_auth": os.Getenv("SMTP_AUTH_MODE"), "requests": a.Requests.Load(), "failures": a.Failures.Load()}
	last, _ := a.Redis.Get(ctx, "worker:last_success").Int64()
	out["worker_last_success"] = last
	return out, nil
}
func (a *App) developerUser(w http.ResponseWriter, r *http.Request) (any, error) {
	if e := developerRequired(r); e != nil {
		return nil, e
	}
	d, e := readBody(w, r)
	if e != nil {
		return nil, e
	}
	if e = allowedFields(d, "name", "email", "phone", "mfa_enabled", "new_password", "password", "version", "ip_restricted", "allowed_ips"); e != nil {
		return nil, e
	}
	if e = a.developerReauthenticate(r, d); e != nil {
		return nil, e
	}
	name, e := text(d, "name", 120)
	if e != nil {
		return nil, e
	}
	email := strings.ToLower(str(d, "email"))
	_, phone, contactErr := userContactInput(M{"phone": d["phone"]}, M{})
	if contactErr != nil {
		return nil, contactErr
	}
	enabled, ok := d["mfa_enabled"].(bool)
	if !ok || len(phone) > 50 || (email != "" && !validEmail(email)) || (enabled && email == "") {
		return nil, bad("Enter a valid email address for two-step verification and phone of at most 50 characters.")
	}
	next, _ := d["new_password"].(string)
	if next != "" && !validPassword(next) {
		return nil, bad("Use 12–128 characters (1234 is allowed only for local testing).")
	}
	ctx := r.Context()
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	before, e := row(ctx, tx, `SELECT id,name,email,phone,mfa_enabled,ip_restricted,allowed_ips,is_developer,session_version FROM users WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, setupID(r))
	if e != nil {
		return nil, e
	}
	if id(before, "session_version") != id(d, "version") {
		return nil, failure{409, "Account changed. Refresh before saving."}
	}
	if before["is_developer"] == true && (!enabled || email != str(before, "email")) {
		return nil, bad("The owner account must keep its configured verification email and mandatory two-step verification.")
	}
	restricted, ips, e := ipPolicyInput(d, before)
	if e != nil {
		return nil, e
	}
	hash := ""
	if next != "" {
		hash = passwordHash(next)
	}
	_, e = tx.Exec(ctx, `UPDATE users SET name=$1,email=$2,phone=$3,mfa_enabled=$4,password_hash=CASE WHEN $5='' THEN password_hash ELSE $5 END,ip_restricted=$7,allowed_ips=$8,session_version=session_version+1 WHERE id=$6`, name, email, phone, enabled, hash, setupID(r), restricted, ips)
	if e != nil {
		return nil, e
	}
	delete(before, "session_version")
	if e = accessAudit(ctx, tx, current(r).ID, "Account security updated", "User", setupID(r), M{"before": before, "after": M{"name": name, "email": email, "phone": phone, "mfa_enabled": enabled, "ip_restricted": restricted, "allowed_ips": ips}, "password_reset": next != ""}); e != nil {
		return nil, e
	}
	return M{"ok": true, "sign_in_again": setupID(r) == current(r).ID}, tx.Commit(ctx)
}
func (a *App) developerDeleteUser(w http.ResponseWriter, r *http.Request) (any, error) {
	if e := developerRequired(r); e != nil {
		return nil, e
	}
	d, e := readBody(w, r)
	if e != nil {
		return nil, e
	}
	if e = allowedFields(d, "password", "confirmation_username", "version"); e != nil {
		return nil, e
	}
	if e = a.developerReauthenticate(r, d); e != nil {
		return nil, e
	}
	ctx := r.Context()
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(824733)`); e != nil {
		return nil, e
	}
	u, e := row(ctx, tx, `SELECT id,username,is_developer,active,session_version FROM users WHERE id=$1 AND deleted_at IS NULL FOR UPDATE`, setupID(r))
	if e != nil {
		return nil, e
	}
	if u["is_developer"] == true || setupID(r) == current(r).ID {
		return nil, bad("The developer owner account cannot be deleted.")
	}
	if id(u, "session_version") != id(d, "version") {
		return nil, failure{409, "Account changed. Refresh before deleting."}
	}
	confirmation, _ := d["confirmation_username"].(string)
	if confirmation != str(u, "username") {
		return nil, bad("Type the exact username to confirm deletion.")
	}
	var reports int
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM users WHERE reports_to_id=$1 AND active AND deleted_at IS NULL`, setupID(r)).Scan(&reports); e != nil {
		return nil, e
	}
	if reports > 0 {
		return nil, bad("Replace this manager or reassign active team members before deleting the account.")
	}
	var assigned int
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM company_setups WHERE deleted_at IS NULL AND (assigned_to=$1 OR responsible_user_id=$1)`, setupID(r)).Scan(&assigned); e != nil {
		return nil, e
	}
	if assigned > 0 {
		return nil, bad("Replace this employee in Access & new users first to transfer company responsibilities automatically.")
	}
	if u["active"] == true {
		account, err := a.loadUser(ctx, setupID(r))
		if err != nil {
			return nil, err
		}
		var unfinished int
		if err = tx.QueryRow(ctx, `SELECT count(*) FROM orders o WHERE `+scopeSQL+` AND o.stage<>'Dispatched' AND o.approval<>'Rejected'`, scopeArgs(account)...).Scan(&unfinished); err != nil {
			return nil, err
		}
		if unfinished > 0 {
			return nil, bad("Replace this employee in Access & new users first to transfer unfinished work automatically.")
		}
	}
	_, e = tx.Exec(ctx, `UPDATE users SET active=false,deleted_at=now(),session_version=session_version+1 WHERE id=$1`, setupID(r))
	if e != nil {
		return nil, e
	}
	if e = accessAudit(ctx, tx, current(r).ID, "User deleted; history retained", "User", setupID(r), M{"username": u["username"]}); e != nil {
		return nil, e
	}
	return M{"ok": true}, tx.Commit(ctx)
}
func (a *App) developerIssue(w http.ResponseWriter, r *http.Request) (any, error) {
	if e := developerRequired(r); e != nil {
		return nil, e
	}
	d, e := readBody(w, r)
	if e != nil {
		return nil, e
	}
	if e = allowedFields(d, "status", "resolution", "version"); e != nil {
		return nil, e
	}
	status, note := str(d, "status"), str(d, "resolution")
	if !slices.Contains([]string{"Open", "Investigating", "Resolved"}, status) || len(note) > 2000 || (status == "Resolved" && note == "") {
		return nil, bad("Choose an issue status and enter a resolution note when resolving it.")
	}
	ctx := r.Context()
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	tag, e := tx.Exec(ctx, `UPDATE system_issues SET status=$1,resolution=$2,version=version+1 WHERE id=$3 AND version=$4`, status, note, setupID(r), id(d, "version"))
	if e != nil {
		return nil, e
	}
	if tag.RowsAffected() != 1 {
		return nil, failure{409, "Issue changed. Refresh before saving."}
	}
	if e = accessAudit(ctx, tx, current(r).ID, "Issue status updated", "Issue", setupID(r), M{"status": status, "resolution": note}); e != nil {
		return nil, e
	}
	return M{"ok": true}, tx.Commit(ctx)
}
func (a *App) developerNotifications(w http.ResponseWriter, r *http.Request) (any, error) {
	if e := developerRequired(r); e != nil {
		return nil, e
	}
	d, e := readBody(w, r)
	if e != nil {
		return nil, e
	}
	if e = allowedFields(d, "company_id", "recipients", "version"); e != nil {
		return nil, e
	}
	values, ok := d["recipients"].([]any)
	if !ok || len(values) > 10 {
		return nil, bad("Use at most ten reminder email addresses.")
	}
	recipients := []string{}
	for _, v := range values {
		s, ok := v.(string)
		s = strings.ToLower(strings.TrimSpace(s))
		if !ok || !validEmail(s) || slices.Contains(recipients, s) {
			return nil, bad("Enter unique valid email addresses.")
		}
		recipients = append(recipients, s)
	}
	ctx := r.Context()
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	cid := id(d, "company_id")
	version := id(d, "version")
	if cid == 0 {
		tag, err := tx.Exec(ctx, `UPDATE notification_settings SET recipients=$1,version=version+1 WHERE id=1 AND version=$2`, recipients, version)
		e = err
		if e == nil && tag.RowsAffected() != 1 {
			e = failure{409, "Reminder settings changed. Refresh first."}
		}
	} else {
		var exists bool
		e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM company_setups WHERE id=$1 AND deleted_at IS NULL)`, cid).Scan(&exists)
		if e != nil {
			return nil, e
		}
		if !exists {
			return nil, missing()
		}
		tag, err := tx.Exec(ctx, `INSERT INTO company_notification_settings(company_id,recipients) SELECT $1,$2 WHERE $3=0 ON CONFLICT(company_id) DO NOTHING`, cid, recipients, version)
		e = err
		if version > 0 {
			tag, e = tx.Exec(ctx, `UPDATE company_notification_settings SET recipients=$1,version=version+1 WHERE company_id=$2 AND version=$3`, recipients, cid, version)
		}
		if e == nil && tag.RowsAffected() != 1 {
			e = failure{409, "Company reminder settings changed. Refresh first."}
		}
	}
	if e != nil {
		return nil, e
	}
	if e = accessAudit(ctx, tx, current(r).ID, "Reminder recipients updated", "Company", cid, M{"recipients": recipients}); e != nil {
		return nil, e
	}
	return M{"ok": true}, tx.Commit(ctx)
}
