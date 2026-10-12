package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"
)

var permissionLabels = map[string]string{"view_orders": "View assigned orders and dashboard", "orders": "Create buyer orders", "approve": "Approve orders and purchases", "plan": "Planning and line allocation", "develop": "Development and CAD", "sample": "Sampling", "cut": "Cutting", "sew": "Sewing output", "finish": "Finishing", "pack": "Packing", "quality": "Quality inspection", "warehouse": "Warehouse and dispatch", "finance": "Invoicing and financial values", "suppliers": "Supplier management", "inventory": "Inventory receipts and issues", "purchases": "Purchase orders", "companies": "Manage operational resources", "audit": "Reports and audit history", "documents": "Order documents"}

func renewalRouteAllowed(r *http.Request, u *User) bool {
	path := r.URL.Path
	if path == "/api/logout" || path == "/api/password" {
		return true
	}
	if path == "/api/company-setup" {
		return r.Method == "GET"
	}
	// Address suggestions remain available while correcting an overdue company.
	// The endpoint independently enforces Finance view and create/edit grants.
	if path == "/api/company-postal-lookup" {
		return r.Method == "GET"
	}
	if strings.HasPrefix(path, "/api/company-setup/") {
		cid := setupID(r)
		for _, b := range u.RenewalBlocks {
			if id(b, "id") == cid {
				return true
			}
		}
		return false
	}
	return strings.HasPrefix(path, "/api/company-files/") || path == "/api/company-ports"
}
func accessAudit(ctx context.Context, q queryer, actor int64, action, kind string, subject int64, detail any) error {
	b, e := json.Marshal(detail)
	if e != nil {
		return e
	}
	_, e = q.Exec(ctx, `INSERT INTO access_history(actor_id,action,subject_type,subject_id,detail) VALUES($1,$2,$3,$4,$5)`, actor, action, kind, subject, b)
	return e
}
func (a *App) accessMetadata(ctx context.Context) (any, error) {
	out := M{"local": !a.Production, "roles": Roles, "capabilities": setupCaps, "labels": setupLabels, "permission_capabilities": permissionCapabilityMap(), "permission_labels": permissionLabels, "products": Products, "access_codes": accessCodes}
	queries := map[string]string{
		"users":            `SELECT u.id,u.username,u.name,u.email,u.phone,u.role,u.active,u.is_developer,u.setup_features,u.setup_custom_access,u.permissions,u.custom_permissions,u.scope,u.companies,u.teams,u.lines,u.orders,u.finishing_units,COALESCE(u.reports_to_id,0) AS reports_to_id,u.territory,u.restrict_buyers,u.buyer_ids,u.production_areas,u.session_version AS version,u.designation_id,u.inherit_designation,d.name AS designation,d.department FROM users u LEFT JOIN access_designations d ON d.id=u.designation_id WHERE u.deleted_at IS NULL ORDER BY u.active DESC,u.name`,
		"designations":     `SELECT * FROM access_designations ORDER BY department,name`,
		"companies":        `SELECT c.id,c.name,c.setup_id,c.status FROM companies c LEFT JOIN company_setups s ON s.id=c.setup_id WHERE c.status='Approved' AND (c.setup_id IS NULL OR (s.deleted_at IS NULL AND s.status='Approved')) ORDER BY c.name`,
		"buyers":           `SELECT * FROM buyers ORDER BY company_id,name`,
		"production_areas": `SELECT * FROM production_areas ORDER BY company_id,kind,name`,
		"teams":            `SELECT * FROM teams ORDER BY company_id,name`, "lines": `SELECT * FROM lines ORDER BY company_id,name`, "orders": `SELECT id,company_id,team_id,line_id,po,stage,finishing_unit,version FROM orders ORDER BY id DESC`,
		"history": `SELECT h.*,u.username AS actor FROM access_history h JOIN users u ON u.id=h.actor_id ORDER BY h.id DESC LIMIT 60`,
	}
	for key, query := range queries {
		v, e := rows(ctx, a.DB, query)
		if e != nil {
			return nil, e
		}
		out[key] = v
	}
	for _, entry := range out["users"].([]M) {
		u, e := a.loadUser(ctx, id(entry, "id"))
		if e != nil {
			return nil, e
		}
		entry["portal"] = "/" + u.Portal
		entry["setup_features"] = u.SetupGrants
		entry["effective_permissions"] = u.Grants
		entry["employee_code"] = fmt.Sprintf("ES-%05d", u.ID)
	}
	return out, nil
}
func stringChoices(v any, caps []string) ([]string, error) {
	out := []string{}
	list, ok := v.([]any)
	if !ok {
		return nil, bad("Choose allowed features.")
	}
	for _, value := range list {
		s, ok := value.(string)
		if !ok || !slices.Contains(caps, s) || slices.Contains(out, s) {
			return nil, bad("A permission is not available for this designation.")
		}
		out = append(out, s)
	}
	return out, nil
}
func toStrings(v any) []string {
	b, _ := json.Marshal(v)
	out := []string{}
	_ = json.Unmarshal(b, &out)
	return out
}
func toIDs(v any) []int64 {
	b, _ := json.Marshal(v)
	out := []int64{}
	_ = json.Unmarshal(b, &out)
	return out
}
func (a *App) designationSave(w http.ResponseWriter, r *http.Request) (any, error) {
	actor := current(r)
	if actor.Role != "Super Admin" {
		return nil, forbidden()
	}
	d, e := readBody(w, r)
	if e != nil {
		return nil, e
	}
	name, e := text(d, "name", 100)
	if e != nil {
		return nil, e
	}
	department, e := text(d, "department", 100)
	if e != nil {
		return nil, e
	}
	role := str(d, "role")
	if !slices.Contains(Roles, role) {
		return nil, bad("Choose a valid department role.")
	}
	features, e := setupFeatureInput(d, role)
	if e != nil {
		return nil, e
	}
	perms, e := stringChoices(d["permissions"], rolePermissionCapabilities(role))
	if e != nil {
		return nil, e
	}
	if needsOrderVisibility(perms) && !slices.Contains(perms, "view_orders") {
		return nil, bad("Enable order visibility before operational actions.")
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
	did := setupID(r)
	before := M{}
	if did == 0 {
		e = tx.QueryRow(ctx, `INSERT INTO access_designations(name,department,role,permissions,features) VALUES($1,$2,$3,$4,$5) RETURNING id`, name, department, role, perms, features).Scan(&did)
	} else {
		before, e = row(ctx, tx, `SELECT * FROM access_designations WHERE id=$1 FOR UPDATE`, did)
		if e != nil {
			return nil, e
		}
		if id(before, "version") != id(d, "version") {
			return nil, failure{409, "Designation changed. Reload before saving."}
		}
		if str(before, "role") == "Super Admin" {
			return nil, bad("Super Admin permissions are fixed.")
		}
		if role != str(before, "role") {
			return nil, bad("Create a new designation to use a different department role.")
		}
		_, e = tx.Exec(ctx, `UPDATE access_designations SET name=$1,department=$2,permissions=$3,features=$4,version=version+1 WHERE id=$5`, name, department, perms, features, did)
	}
	if e != nil {
		return nil, e
	}
	if _, e = tx.Exec(ctx, `UPDATE users SET session_version=session_version+1 WHERE designation_id=$1`, did); e != nil {
		return nil, e
	}
	if e = accessAudit(ctx, tx, actor.ID, "Designation saved", "Designation", did, M{"before": before, "after": M{"name": name, "department": department, "role": role, "permissions": perms, "features": features}}); e != nil {
		return nil, e
	}
	return M{"id": did}, tx.Commit(ctx)
}
func validateAccessScope(ctx context.Context, q queryer, d M, role string) (*User, error) {
	p := &User{Role: role, Scope: str(d, "scope"), Companies: []int64{}, Teams: []int64{}, Lines: []int64{}, Orders: []int64{}, Units: []string{}}
	if p.Scope == "" {
		p.Scope = "company"
	}
	if !slices.Contains([]string{"company", "team", "line", "po", "cutting_table", "finishing_line"}, p.Scope) {
		return nil, bad("Choose a visibility scope.")
	}
	for key, dest := range map[string]*[]int64{"companies": &p.Companies, "teams": &p.Teams, "lines": &p.Lines, "orders": &p.Orders} {
		v := d[key]
		if v == nil {
			v = []any{}
		}
		values, e := ids(v)
		if e != nil {
			return nil, e
		}
		*dest = values
	}
	if p.Scope != "po" {
		p.Orders = []int64{}
	}
	var count int
	if e := q.QueryRow(ctx, `SELECT count(*) FROM companies c LEFT JOIN company_setups s ON s.id=c.setup_id WHERE c.id=ANY($1) AND c.status='Approved' AND (c.setup_id IS NULL OR(s.deleted_at IS NULL AND s.status='Approved'))`, p.Companies).Scan(&count); e != nil {
		return nil, e
	}
	if count != len(p.Companies) {
		return nil, bad("Only approved internal companies can be assigned.")
	}
	for key, values := range map[string][]int64{"teams": p.Teams, "lines": p.Lines, "orders": p.Orders} {
		if e := q.QueryRow(ctx, `SELECT count(*) FROM `+key+` WHERE id=ANY($1) AND company_id=ANY($2)`, values, p.Companies).Scan(&count); e != nil {
			return nil, e
		}
		if count != len(values) {
			return nil, bad("Assignments must belong to the selected companies.")
		}
	}
	if len(p.Companies) > 0 && (p.Scope == "team" && len(p.Teams) == 0 || p.Scope == "line" && len(p.Lines) == 0 || p.Scope == "po" && len(p.Orders) == 0) {
		return nil, bad("Choose assignments for the selected visibility scope.")
	}
	v := d["finishing_units"]
	if v == nil {
		v = []any{}
	}
	var e error
	p.Units, e = stringChoices(v, Products)
	if e != nil {
		return nil, e
	}
	if role == "Finishing" && len(p.Companies) > 0 && len(p.Units) == 0 {
		return nil, bad("Choose a finishing unit.")
	}
	return p, nil
}

// Current account forms always send confirmation. Omission stays compatible with
// existing clients; a supplied confirmation must match exactly and is never stored.
func validateInitialPasswordConfirmation(d M, password string) error {
	value, supplied := d["password_confirmation"]
	if !supplied {
		return nil
	}
	confirmation, ok := value.(string)
	if !ok || confirmation == "" {
		return bad("Confirm the initial password.")
	}
	if confirmation != password {
		return bad("Passwords do not match. Enter the same password in both fields.")
	}
	return nil
}

// Existing account edits never supply or return the stored password. Omitted
// credentials preserve old clients; a requested reset requires confirmation.
func existingUserCredentialInput(d M, currentUsername string) (string, string, error) {
	username := currentUsername
	if value, supplied := d["username"]; supplied {
		requested, ok := value.(string)
		if !ok {
			return "", "", bad("Enter a valid username.")
		}
		username = strings.ToLower(strings.TrimSpace(requested))
		if !regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,79}$`).MatchString(username) {
			return "", "", bad("Use 2–80 letters, numbers, dots, underscores or hyphens for the username, starting with a letter or number.")
		}
	}
	password := ""
	if value, supplied := d["new_password"]; supplied {
		var ok bool
		password, ok = value.(string)
		if !ok {
			return "", "", bad("Enter a valid new password or leave it blank.")
		}
	}
	confirmation := ""
	if value, supplied := d["new_password_confirmation"]; supplied {
		var ok bool
		confirmation, ok = value.(string)
		if !ok {
			return "", "", bad("Confirm the new password.")
		}
	}
	if password == "" {
		if confirmation != "" {
			return "", "", bad("Enter a new password before confirming it, or leave both password fields blank.")
		}
		return username, "", nil
	}
	if !validPassword(password) {
		return "", "", bad("Use 12–128 characters for the new password.")
	}
	if confirmation == "" {
		return "", "", bad("Confirm the new password.")
	}
	if confirmation != password {
		return "", "", bad("Passwords do not match. Enter the same password in both fields.")
	}
	return username, password, nil
}

func (a *App) setupUserSave(w http.ResponseWriter, r *http.Request) (any, error) {
	actor := current(r)
	if actor.Role != "Super Admin" {
		return nil, forbidden()
	}
	d, e := readBody(w, r)
	if e != nil {
		return nil, e
	}
	uid := setupID(r)
	isNew := uid == 0
	if uid == actor.ID {
		return nil, bad("Use another Super Admin to edit your account.")
	}
	name, e := text(d, "name", 120)
	if e != nil {
		return nil, e
	}
	active, ok := d["active"].(bool)
	if !ok {
		return nil, bad("Choose active status.")
	}
	username := strings.ToLower(strings.TrimSpace(str(d, "username")))
	password, _ := d["password"].(string)
	if isNew && password == "" && os.Getenv("APP_ENV") == "local" {
		password = "1234"
	}
	if uid == 0 && (!regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,79}$`).MatchString(username) || !validPassword(password)) {
		return nil, bad("Enter a unique username and a valid initial password.")
	}
	if isNew {
		if e = validateInitialPasswordConfirmation(d, password); e != nil {
			return nil, e
		}
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
	if uid > 0 {
		var protected bool
		if e = tx.QueryRow(ctx, `SELECT is_developer OR deleted_at IS NOT NULL FROM users WHERE id=$1`, uid).Scan(&protected); e != nil {
			return nil, e
		}
		if protected {
			return nil, forbidden()
		}
	}
	did := id(d, "designation_id")
	if did == 0 {
		if e = tx.QueryRow(ctx, `SELECT id FROM access_designations WHERE role=$1 ORDER BY id LIMIT 1`, str(d, "role")).Scan(&did); e != nil {
			return nil, bad("Choose a designation.")
		}
	}
	designation, e := row(ctx, tx, `SELECT * FROM access_designations WHERE id=$1 FOR UPDATE`, did)
	if e != nil {
		return nil, e
	}
	role := str(designation, "role")
	inherit := d["inherit_designation"] == true
	if d["features"] == nil {
		d["features"] = []any{}
	}
	features, e := setupFeatureInput(d, role)
	if e != nil {
		return nil, e
	}
	perms := []string{}
	if d["permissions"] != nil {
		perms, e = stringChoices(d["permissions"], rolePermissionCapabilities(role))
		if e != nil {
			return nil, e
		}
	}
	if inherit {
		features = toStrings(designation["features"])
		perms = toStrings(designation["permissions"])
	} else {
		for _, f := range features {
			if !slices.Contains(toStrings(designation["features"]), f) {
				return nil, bad("This company feature exceeds the designation.")
			}
		}
		for _, p := range perms {
			if !slices.Contains(toStrings(designation["permissions"]), p) {
				return nil, bad("This action exceeds the designation.")
			}
		}
	}
	if needsOrderVisibility(perms) && !slices.Contains(perms, "view_orders") {
		return nil, bad("Enable order visibility before operational actions.")
	}
	profile, e := validateAccessScope(ctx, tx, d, role)
	if e != nil {
		return nil, e
	}
	before := M{}
	newPassword := ""
	action := "User created"
	if uid > 0 {
		before, e = row(ctx, tx, `SELECT id,username,name,email,phone,mfa_enabled,role,active,is_developer,deleted_at,designation_id,inherit_designation,setup_features,permissions,scope,companies,teams,lines,orders,finishing_units,reports_to_id,territory,restrict_buyers,buyer_ids,production_areas,session_version FROM users WHERE id=$1 FOR UPDATE`, uid)
		if e != nil {
			return nil, e
		}
		if before["is_developer"] == true || before["deleted_at"] != nil {
			return nil, forbidden()
		}
		if id(before, "session_version") != id(d, "version") {
			return nil, failure{409, "This user changed. Reload access before saving."}
		}
		if str(before, "role") == "Super Admin" && (!active || role != "Super Admin") {
			var n int
			if e = tx.QueryRow(ctx, `SELECT count(*) FROM users WHERE role='Super Admin' AND active AND id<>$1`, uid).Scan(&n); e != nil {
				return nil, e
			}
			if n == 0 {
				return nil, bad("Keep at least one active Super Admin.")
			}
		}
		username, newPassword, e = existingUserCredentialInput(d, str(before, "username"))
		if e != nil {
			return nil, e
		}
		action = "User access updated"
		if username != str(before, "username") || newPassword != "" {
			action = "User credentials and access updated"
		}
	}
	if isNew || username != str(before, "username") {
		var usernameExists bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE username=$1 AND id<>$2)`, username, uid).Scan(&usernameExists); e != nil {
			return nil, e
		}
		if usernameExists {
			return nil, failure{409, "This username is already in use. Choose a different username."}
		}
	}
	email, phone, e := userContactInput(d, before)
	if e != nil {
		return nil, e
	}
	if isNew && email == "" {
		return nil, bad("Enter the new user email address for their welcome message.")
	}
	org, e := userOrganization(ctx, tx, uid, d, before, profile, active)
	if e != nil {
		return nil, e
	}
	if uid == 0 {
		e = tx.QueryRow(ctx, `INSERT INTO users(username,name,password_hash,role,scope,companies,active) VALUES($1,$2,$3,$4,'company','{}',$5) RETURNING id`, username, name, passwordHash(password), role, active).Scan(&uid)
		if e != nil {
			return nil, e
		}
	}
	newHash := ""
	if newPassword != "" {
		newHash = passwordHash(newPassword)
	}
	_, e = tx.Exec(ctx, `UPDATE users SET name=$1,role=$2,active=$3,designation_id=$4,inherit_designation=$5,setup_features=$6,setup_custom_access=true,permissions=$7,custom_permissions=true,scope=$8,companies=$9,teams=$10,lines=$11,orders=$12,finishing_units=$13,email=$15,phone=$16,username=$17,password_hash=CASE WHEN $18='' THEN password_hash ELSE $18 END,session_version=session_version+1 WHERE id=$14`, name, role, active, did, inherit, features, perms, profile.Scope, profile.Companies, profile.Teams, profile.Lines, profile.Orders, profile.Units, uid, email, phone, username, newHash)
	if e != nil {
		return nil, e
	}
	if e = saveUserOrganization(ctx, tx, uid, org); e != nil {
		return nil, e
	}
	after := M{"username": username, "name": name, "email": email, "phone": phone, "role": role, "active": active, "designation_id": did, "inherit_designation": inherit, "features": features, "permissions": perms, "scope": profile.Scope, "companies": profile.Companies, "teams": profile.Teams, "lines": profile.Lines, "orders": profile.Orders, "finishing_units": profile.Units}
	for key, value := range org {
		after[key] = value
	}
	detail := M{"before": before, "after": after}
	if newPassword != "" {
		detail["password_reset"] = true
	}
	if e = accessAudit(ctx, tx, actor.ID, action, "User", uid, detail); e != nil {
		return nil, e
	}
	b, _ := json.Marshal(detail)
	if _, e = tx.Exec(ctx, `INSERT INTO setup_access_history(actor_id,user_id,action,detail) VALUES($1,$2,$3,$4)`, actor.ID, uid, action, b); e != nil {
		return nil, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, e
	}
	result := M{"id": uid}
	if isNew {
		result["welcome_email"] = a.welcomeNewUser(ctx, actor.ID, uid, password)
	}
	return result, nil
}

// A replacement is a new identity. Historical actors and entries are never rewritten.
func (a *App) replaceUser(w http.ResponseWriter, r *http.Request) (any, error) {
	actor := current(r)
	if actor.Role != "Super Admin" {
		return nil, forbidden()
	}
	d, e := readBody(w, r)
	if e != nil {
		return nil, e
	}
	sourceID := setupID(r)
	if sourceID == actor.ID {
		return nil, bad("Another Super Admin must replace your account.")
	}
	name, e := text(d, "name", 120)
	if e != nil {
		return nil, e
	}
	username := strings.ToLower(str(d, "username"))
	password, _ := d["password"].(string)
	if password == "" && os.Getenv("APP_ENV") == "local" {
		password = "1234"
	}
	if !regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,79}$`).MatchString(username) || !validPassword(password) {
		return nil, bad("Enter a username and valid initial password for the replacement.")
	}
	if e = validateInitialPasswordConfirmation(d, password); e != nil {
		return nil, e
	}
	email, phone, e := userContactInput(d, M{})
	if e != nil {
		return nil, e
	}
	if email == "" {
		return nil, bad("Enter the replacement user email address for their welcome message.")
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
	source, e := row(ctx, tx, `SELECT * FROM users WHERE id=$1 FOR UPDATE`, sourceID)
	if e != nil {
		return nil, e
	}
	if source["is_developer"] == true || source["deleted_at"] != nil {
		return nil, forbidden()
	}
	if source["active"] != true {
		return nil, bad("This user is already inactive.")
	}
	if id(d, "version") != id(source, "session_version") {
		return nil, failure{409, "User changed. Reload before replacing."}
	}
	// Serialize with order writers before taking the progress snapshot.
	u, e := a.loadUser(ctx, sourceID)
	if e != nil {
		return nil, e
	}
	orders, e := rows(ctx, tx, `SELECT o.id,o.po,o.stage,o.version FROM orders o WHERE `+scopeSQL+` AND o.stage<>'Dispatched' AND o.approval<>'Rejected' ORDER BY o.id FOR UPDATE OF o`, scopeArgs(u)...)
	if e != nil {
		return nil, e
	}
	for _, o := range orders {
		for _, table := range []string{"work_items", "milestones", "entries", "quality", "materials"} {
			snapshot, err := rows(ctx, tx, `SELECT * FROM `+table+` WHERE order_id=$1 ORDER BY id`, id(o, "id"))
			if err != nil {
				return nil, err
			}
			o[table] = snapshot
		}
	}
	oid := []int64{}
	for _, o := range orders {
		oid = append(oid, id(o, "id"))
	}
	handoverOrders := []int64{}
	if u.Scope == "po" {
		handoverOrders = oid
	}
	var uid int64
	e = tx.QueryRow(ctx, `INSERT INTO users(username,name,password_hash,role,scope,companies,teams,lines,orders,finishing_units,permissions,custom_permissions,setup_features,setup_custom_access,designation_id,inherit_designation,email,phone) SELECT $1,$2,$3,role,scope,companies,teams,lines,$4,finishing_units,permissions,custom_permissions,setup_features,setup_custom_access,designation_id,inherit_designation,$6,$7 FROM users WHERE id=$5 RETURNING id`, username, name, passwordHash(password), handoverOrders, sourceID, email, phone).Scan(&uid)
	if e != nil {
		return nil, e
	}
	if _, e = tx.Exec(ctx, `UPDATE users SET reports_to_id=(SELECT reports_to_id FROM users WHERE id=$1),territory=(SELECT territory FROM users WHERE id=$1),restrict_buyers=(SELECT restrict_buyers FROM users WHERE id=$1),buyer_ids=(SELECT buyer_ids FROM users WHERE id=$1),production_areas=(SELECT production_areas FROM users WHERE id=$1) WHERE id=$2`, sourceID, uid); e != nil {
		return nil, e
	}
	directReports, e := rows(ctx, tx, `UPDATE users SET reports_to_id=$2,session_version=session_version+1 WHERE reports_to_id=$1 AND deleted_at IS NULL RETURNING id`, sourceID, uid)
	if e != nil {
		return nil, e
	}
	companyIDs, e := rows(ctx, tx, `SELECT id,legal_name,version,revision FROM company_setups WHERE deleted_at IS NULL AND (assigned_to=$1 OR responsible_user_id=$1) ORDER BY id FOR UPDATE`, sourceID)
	if e != nil {
		return nil, e
	}
	for _, c := range companyIDs {
		if _, e = tx.Exec(ctx, `UPDATE company_setups SET assigned_to=CASE WHEN assigned_to=$1 THEN $2 ELSE assigned_to END,responsible_user_id=CASE WHEN responsible_user_id=$1 THEN $2 ELSE responsible_user_id END,version=version+1 WHERE id=$3`, sourceID, uid, id(c, "id")); e != nil {
			return nil, e
		}
		if e = setupHistory(ctx, tx, actor, M{"id": id(c, "id"), "revision": id(c, "revision")}, "Responsibility transferred", fmt.Sprintf("Employee ES-%05d replaced by ES-%05d", sourceID, uid)); e != nil {
			return nil, e
		}
	}
	if _, e = tx.Exec(ctx, `UPDATE users SET active=false,session_version=session_version+1 WHERE id=$1`, sourceID); e != nil {
		return nil, e
	}
	snapshot := M{"from_user_id": sourceID, "from_name": str(source, "name"), "to_user_id": uid, "to_name": name, "orders": orders, "company_records": companyIDs, "direct_reports": directReports, "previous_work": "Original entries, documents and audit actors retained"}
	if e = accessAudit(ctx, tx, actor.ID, "Employee replaced; unfinished work transferred", "User", sourceID, snapshot); e != nil {
		return nil, e
	}
	for _, o := range orders {
		if e = accessAudit(ctx, tx, actor.ID, "Work transferred", "Order", id(o, "id"), M{"from_user_id": sourceID, "to_user_id": uid, "stage": o["stage"], "version": o["version"]}); e != nil {
			return nil, e
		}
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, e
	}
	return M{"id": uid, "orders_transferred": len(orders), "companies_transferred": len(companyIDs), "reports_transferred": len(directReports), "welcome_email": a.welcomeNewUser(ctx, actor.ID, uid, password)}, nil
}
func (a *App) accessResourceSave(w http.ResponseWriter, r *http.Request) (any, error) {
	if current(r).Role != "Super Admin" {
		return nil, forbidden()
	}
	d, e := readBody(w, r)
	if e != nil {
		return nil, e
	}
	name, e := text(d, "name", 120)
	if e != nil {
		return nil, e
	}
	kind := str(d, "kind")
	if kind != "teams" && kind != "lines" && kind != "cutting_table" && kind != "finishing_line" && kind != "buyers" {
		return nil, bad("Choose a buyer / brand, team, sewing line, cutting table or finishing sub-line.")
	}
	cid := id(d, "company_id")
	var n int
	e = a.DB.QueryRow(r.Context(), `SELECT count(*) FROM companies c JOIN company_setups s ON s.id=c.setup_id WHERE c.id=$1 AND s.status='Approved' AND s.deleted_at IS NULL`, cid).Scan(&n)
	if e != nil {
		return nil, e
	}
	if n != 1 {
		return nil, bad("Choose an approved internal company.")
	}
	tx, e := a.DB.Begin(r.Context())
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(r.Context())
	var rid int64
	if kind == "buyers" {
		var team int64
		if err := tx.QueryRow(r.Context(), `SELECT id FROM teams WHERE id=$1 AND company_id=$2`, id(d, "team_id"), cid).Scan(&team); err != nil {
			return nil, bad("Choose a buyer team within the selected company.")
		}
		e = tx.QueryRow(r.Context(), `INSERT INTO buyers(company_id,team_id,name) VALUES($1,$2,$3) RETURNING id`, cid, team, name).Scan(&rid)
	} else if kind == "cutting_table" || kind == "finishing_line" {
		e = tx.QueryRow(r.Context(), `INSERT INTO production_areas(company_id,kind,name) VALUES($1,$2,$3) RETURNING id`, cid, kind, name).Scan(&rid)
	} else {
		e = tx.QueryRow(r.Context(), `INSERT INTO `+kind+`(company_id,name) VALUES($1,$2) RETURNING id`, cid, name).Scan(&rid)
	}
	if e != nil {
		return nil, e
	}
	if e = accessAudit(r.Context(), tx, current(r).ID, "Resource created", kind, rid, M{"company_id": cid, "name": name, "team_id": id(d, "team_id")}); e != nil {
		return nil, e
	}
	return M{"id": rid}, tx.Commit(r.Context())
}
func (a *App) companyResponsibility(w http.ResponseWriter, r *http.Request) (any, error) {
	actor := current(r)
	if actor.Role != "Super Admin" {
		return nil, forbidden()
	}
	d, e := readBody(w, r)
	if e != nil {
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
	c, e := a.setupRow(ctx, tx, setupID(r), true)
	if e != nil {
		return nil, e
	}
	if id(d, "version") != id(c, "version") {
		return nil, failure{409, "Company changed. Reload before assigning responsibility."}
	}
	uid := id(d, "user_id")
	var n int
	e = tx.QueryRow(ctx, `SELECT count(*) FROM users WHERE id=$1 AND active AND role<>'Super Admin'`, uid).Scan(&n)
	if e != nil {
		return nil, e
	}
	if n != 1 {
		return nil, bad("Choose an active responsible user.")
	}
	_, e = tx.Exec(ctx, `UPDATE company_setups SET responsible_user_id=$1,version=version+1 WHERE id=$2`, uid, id(c, "id"))
	if e != nil {
		return nil, e
	}
	if e = setupHistory(ctx, tx, actor, c, "Renewal responsibility assigned", fmt.Sprintf("Responsible employee ES-%05d", uid)); e != nil {
		return nil, e
	}
	return M{"ok": true}, tx.Commit(ctx)
}

// Contact details are stored for future notifications. Omitted fields retain
// existing values so older clients cannot accidentally erase contact details.
func userContactInput(d, before M) (string, string, error) {
	email, phone := str(before, "email"), str(before, "phone")
	if value, exists := d["email"]; exists {
		raw, ok := value.(string)
		if !ok {
			return "", "", bad("Enter a valid email address.")
		}
		email = strings.ToLower(strings.TrimSpace(raw))
		if email != "" && !validEmail(email) {
			return "", "", bad("Enter a valid email address.")
		}
	}
	if before["mfa_enabled"] == true && email == "" {
		return "", "", bad("An email address is required while email verification is enabled.")
	}
	if value, exists := d["phone"]; exists {
		raw, ok := value.(string)
		if !ok || len(raw) > 50 {
			return "", "", bad("Enter a mobile number with country code, for example +919876543210.")
		}
		phone = strings.NewReplacer(" ", "", "-", "", "(", "", ")", "").Replace(strings.TrimSpace(raw))
		if phone == "+91" {
			phone = ""
		}
		if regexp.MustCompile(`^[0-9]{10}$`).MatchString(phone) {
			phone = "+91" + phone
		}
		if phone != "" && !regexp.MustCompile(`^\+[1-9][0-9]{6,14}$`).MatchString(phone) {
			return "", "", bad("Enter a mobile number with country code, for example +919876543210.")
		}
	}
	return email, phone, nil
}
