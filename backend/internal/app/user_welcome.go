package app

import (
	"context"
	"eurosuits/erp/internal/brandedmail"
	"strings"
	"time"
)

// Only called after a new account has committed. The initial password is used
// in memory for this one send, never written to audit records, jobs or logs.
func (a *App) welcomeNewUser(parent context.Context, actor, uid int64, password string) string {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(parent), 20*time.Second)
	defer cancel()
	status := "failed"
	defer func() {
		_ = accessAudit(ctx, a.DB, actor, "Welcome email: "+status, "User", uid, M{"status": status})
		if status == "failed" || status == "not_configured" {
			a.recordIssue(ctx, "Email", "Error", "New user welcome email was not sent", "/api/setup-users", 503, uid, "")
		}
	}()
	u, e := a.loadUser(ctx, uid)
	if e != nil {
		return status
	}
	if u.Email == "" {
		status = "no_email"
		return status
	}
	if !emailConfigured() {
		status = "not_configured"
		return status
	}
	info, e := row(ctx, a.DB, `SELECT COALESCE(d.name,u.role) AS designation,COALESCE(d.department,'') AS department,u.scope,u.active FROM users u LEFT JOIN access_designations d ON d.id=u.designation_id WHERE u.id=$1`, uid)
	if e != nil {
		return status
	}
	companies, e := rows(ctx, a.DB, `SELECT name FROM companies WHERE id=ANY($1) ORDER BY name`, u.Companies)
	if e != nil {
		return status
	}
	names := []string{}
	for _, c := range companies {
		names = append(names, str(c, "name"))
	}
	company := strings.Join(names, ", ")
	if company == "" {
		company = "Awaiting company assignment by your administrator"
	}
	responsibilities := []string{}
	for _, permission := range u.SetupGrants {
		if label := setupLabels[permission]; label != "" {
			responsibilities = append(responsibilities, "Company Setup: "+label)
		}
	}
	for _, permission := range u.Grants {
		if label := permissionLabels[permission]; label != "" {
			responsibilities = append(responsibilities, label)
		}
	}
	if len(responsibilities) == 0 {
		responsibilities = []string{"Your administrator has not assigned features yet. Contact them before starting work."}
	}
	scope := map[string]string{"company": "Assigned companies", "team": "Selected buyer teams", "line": "Selected sewing lines", "po": "Selected purchase orders", "cutting_table": "Selected cutting tables", "finishing_line": "Selected finishing sub-lines"}[u.Scope]
	if u.RestrictBuyers {
		scope += "; restricted to assigned buyers / brands"
	}
	message := brandedmail.Welcome(brandedmail.WelcomeData{ID: uid, Name: u.Name, Username: u.Username, Password: password, Designation: str(info, "designation"), Department: str(info, "department"), Company: company, Scope: scope, Dashboard: strings.TrimRight(a.Origin, "/") + "/" + u.Portal, Responsibilities: responsibilities, Active: info["active"] == true})
	if e = sendBrandedMail(u.Email, message); e != nil {
		return status
	}
	status = "sent"
	return status
}
