package app

import (
	"context"
	"fmt"
	"github.com/go-chi/chi/v5"
	"net/http"
	"slices"
	"strconv"
	"strings"
)

func (a *App) createRecord(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	d, e := readBody(w, r)
	if e != nil {
		return nil, e
	}
	kind := chi.URLParam(r, "kind")
	if kind == "companies" {
		return nil, bad("Use the Company Setup approval workflow.")
	}
	grant := map[string]string{"companies": "companies", "teams": "users", "buyers": "users", "lines": "users", "suppliers": "suppliers", "inventory": "inventory", "purchases": "purchases"}[kind]
	if grant == "" {
		return nil, missing()
	}
	if e = u.Require(grant); e != nil {
		return nil, e
	}
	ctx := r.Context()
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	cid := id(d, "company_id")
	if kind != "companies" {
		if e = a.company(ctx, tx, u, cid, true); e != nil {
			return nil, e
		}
	}
	name := str(d, "name")
	if kind != "purchases" && (name == "" || len(name) > 150) {
		return nil, bad("Enter a name of up to 150 characters.")
	}
	var rid int64
	switch kind {
	case "companies":
		e = tx.QueryRow(ctx, `INSERT INTO companies(name,kind) VALUES($1,$2) RETURNING id`, name, str(d, "kind")).Scan(&rid)
		if e != nil {
			return nil, e
		}
		cid = rid
		_, e = tx.Exec(ctx, `UPDATE users SET companies=array_append(companies,$1::bigint) WHERE id=$2 OR (role='MD' AND companies && $3::bigint[])`, rid, u.ID, u.Companies)
	case "teams":
		e = tx.QueryRow(ctx, `INSERT INTO teams(company_id,name) VALUES($1,$2) RETURNING id`, cid, name).Scan(&rid)
	case "buyers":
		e = tx.QueryRow(ctx, `INSERT INTO buyers(company_id,team_id,name) VALUES($1,$2,$3) RETURNING id`, cid, id(d, "team_id"), name).Scan(&rid)
	case "lines":
		n, e2 := integer(d["capacity"])
		if e2 != nil || n < 1 {
			return nil, bad("Enter a positive daily capacity.")
		}
		e = tx.QueryRow(ctx, `INSERT INTO lines(company_id,name,capacity) VALUES($1,$2,$3) RETURNING id`, cid, name, n).Scan(&rid)
	case "suppliers":
		if !slices.Contains(Categories, str(d, "category")) {
			return nil, bad("Choose a supplier category.")
		}
		var buyer any
		if id(d, "buyer_id") > 0 {
			buyer = id(d, "buyer_id")
		}
		e = tx.QueryRow(ctx, `INSERT INTO suppliers(company_id,name,category,designation,buyer_id,contact) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, cid, name, str(d, "category"), str(d, "designation"), buyer, str(d, "contact")).Scan(&rid)
	case "inventory":
		n, e2 := minor(d["reorder_level"])
		if e2 != nil {
			return nil, e2
		}
		sku, e2 := text(d, "sku", 50)
		if e2 != nil {
			return nil, e2
		}
		e = tx.QueryRow(ctx, `INSERT INTO inventory(company_id,sku,name,store,unit,reorder_hundredths) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, cid, sku, name, str(d, "store"), str(d, "unit"), n).Scan(&rid)
	case "purchases":
		var order any
		if id(d, "order_id") > 0 {
			o, e2 := a.order(ctx, tx, u, id(d, "order_id"), true)
			if e2 != nil {
				return nil, e2
			}
			if str(o, "stage") == "Dispatched" {
				return nil, bad("Dispatched orders are locked.")
			}
			if id(o, "company_id") != cid {
				return nil, bad("Order belongs to another company.")
			}
			order = id(o, "id")
		} else if u.Scope != "company" || u.RestrictBuyers {
			return nil, bad("Select an assigned order.")
		}
		qty, e2 := minor(d["quantity"])
		if e2 != nil || qty <= 0 {
			return nil, bad("Enter a positive purchase quantity.")
		}
		cost, e2 := minor(d["unit_cost"])
		if e2 != nil {
			return nil, e2
		}
		due, e2 := date(str(d, "due_date"))
		if e2 != nil {
			return nil, e2
		}
		ref, e2 := text(d, "reference", 60)
		if e2 != nil {
			return nil, e2
		}
		e = tx.QueryRow(ctx, `INSERT INTO purchases(company_id,reference,supplier_id,item_id,order_id,quantity_hundredths,unit_cost_minor,due_date,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING id`, cid, ref, id(d, "supplier_id"), id(d, "item_id"), order, qty, cost, due, u.ID).Scan(&rid)
		name = ref
	}
	if e != nil {
		return nil, e
	}
	if e = audit(ctx, tx, u, cid, "Created", kind, rid, name); e != nil {
		return nil, e
	}
	return M{"id": rid}, tx.Commit(ctx)
}
func (a *App) recordAction(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	d, e := readBody(w, r)
	if e != nil {
		return nil, e
	}
	kind := chi.URLParam(r, "kind")
	if kind == "companies" {
		return nil, bad("Use the Company Setup approval workflow.")
	}
	rid, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	ctx := r.Context()
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	action := str(d, "action")
	cid := int64(0)
	detail := ""
	switch kind {
	case "companies":
		if u.Role != "MD" || !u.Can("approve") || action != "approve" {
			return nil, forbidden()
		}
		if e = a.company(ctx, tx, u, rid, false); e != nil {
			return nil, e
		}
		c, e2 := row(ctx, tx, `SELECT * FROM companies WHERE id=$1 FOR UPDATE`, rid)
		if e2 != nil {
			return nil, e2
		}
		if str(c, "status") != "Pending" {
			return nil, bad("This company has already been reviewed.")
		}
		_, e = tx.Exec(ctx, `UPDATE companies SET status='Approved' WHERE id=$1`, rid)
		cid = rid
		detail = str(c, "name")
	case "purchases":
		p, e2 := row(ctx, tx, `SELECT * FROM purchases WHERE id=$1 AND company_id=ANY($2) FOR UPDATE`, rid, u.Companies)
		if e2 != nil {
			return nil, e2
		}
		cid = id(p, "company_id")
		detail = str(p, "reference")
		if id(p, "order_id") > 0 {
			o, err := a.order(ctx, tx, u, id(p, "order_id"), true)
			if err != nil {
				return nil, err
			}
			if str(o, "stage") == "Dispatched" {
				return nil, bad("Dispatched orders are locked.")
			}
		}
		if u.Scope != "company" || u.RestrictBuyers {
			if _, e2 = a.order(ctx, tx, u, id(p, "order_id"), false); e2 != nil {
				return nil, e2
			}
		}
		if action == "approve" || action == "reject" {
			if e = u.Require("approve"); e != nil {
				return nil, e
			}
			if str(p, "status") != "Pending" {
				return nil, bad("Purchase has already been reviewed.")
			}
			status := "Approved"
			if action == "reject" {
				status = "Rejected"
			}
			_, e = tx.Exec(ctx, `UPDATE purchases SET status=$1 WHERE id=$2`, status, rid)
		} else if action == "receive" {
			if e = u.Require("inventory"); e != nil {
				return nil, e
			}
			if str(p, "status") != "Approved" {
				return nil, bad("Only an approved, unreceived purchase can be received.")
			}
			if _, e2 = row(ctx, tx, `SELECT id FROM inventory WHERE id=$1 FOR UPDATE`, id(p, "item_id")); e2 != nil {
				return nil, e2
			}
			if _, e = tx.Exec(ctx, `INSERT INTO movements(item_id,quantity_hundredths,kind,order_id,reference,created_by) VALUES($1,$2,'Receipt',$3,$4,$5)`, id(p, "item_id"), id(p, "quantity_hundredths"), p["order_id"], detail, u.ID); e != nil {
				return nil, e
			}
			_, e = tx.Exec(ctx, `UPDATE purchases SET status='Received' WHERE id=$1`, rid)
		} else {
			return nil, bad("Unknown purchase action.")
		}
	case "work_items":
		item, e2 := row(ctx, tx, `SELECT * FROM work_items WHERE id=$1 FOR UPDATE`, rid)
		if e2 != nil {
			return nil, e2
		}
		o, e2 := a.order(ctx, tx, u, id(item, "order_id"), true)
		if e2 != nil {
			return nil, e2
		}
		cid = id(o, "company_id")
		expectedStage := "Development"
		grant := "develop"
		if strings.Contains(str(item, "kind"), "sample") {
			expectedStage = "Sampling"
			grant = "sample"
		}
		if e = u.Require(grant); e != nil {
			return nil, e
		}
		if str(o, "stage") != expectedStage {
			return nil, bad("This department handoff is closed. Work records can only change in their active stage.")
		}
		_, e = tx.Exec(ctx, `UPDATE work_items SET status=$1,notes=$2 WHERE id=$3`, str(d, "status"), str(d, "notes"), rid)
		if e == nil {
			_, e = tx.Exec(ctx, `UPDATE orders SET version=version+1,updated_at=now() WHERE id=$1`, id(o, "id"))
		}
		detail = str(item, "title")
	default:
		return nil, missing()
	}
	if e != nil {
		return nil, e
	}
	if e = audit(ctx, tx, u, cid, action, kind, rid, detail); e != nil {
		return nil, e
	}
	return M{"ok": true}, tx.Commit(ctx)
}
func (a *App) inventoryHistory(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if e := u.Require("inventory"); e != nil {
		return nil, e
	}
	iid, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if _, e := row(r.Context(), a.DB, `SELECT id FROM inventory WHERE id=$1 AND company_id=ANY($2)`, iid, u.Companies); e != nil {
		return nil, e
	}
	sql := `SELECT m.* FROM movements m WHERE m.item_id=$11 AND (($2='company' AND NOT $8) OR m.order_id IN (SELECT o.id FROM orders o WHERE ` + scopeSQL + `)) ORDER BY m.id DESC`
	return rows(r.Context(), a.DB, sql, append(scopeArgs(u), iid)...)
}
func (a *App) stockMovement(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if e := u.Require("inventory"); e != nil {
		return nil, e
	}
	d, e := readBody(w, r)
	if e != nil {
		return nil, e
	}
	iid, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	ctx := r.Context()
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	item, e := row(ctx, tx, `SELECT * FROM inventory WHERE id=$1 AND company_id=ANY($2) FOR UPDATE`, iid, u.Companies)
	if e != nil {
		return nil, e
	}
	qty, e := minor(d["quantity"])
	if e != nil || qty <= 0 {
		return nil, bad("Enter a positive stock quantity.")
	}
	kind := str(d, "kind")
	var order any
	if id(d, "order_id") > 0 {
		o, e2 := a.order(ctx, tx, u, id(d, "order_id"), true)
		if e2 != nil {
			return nil, e2
		}
		if str(o, "stage") == "Dispatched" {
			return nil, bad("Dispatched orders are locked.")
		}
		if id(o, "company_id") != id(item, "company_id") {
			return nil, bad("Material and order must belong to the same company.")
		}
		if str(o, "approval") != "Approved" {
			return nil, bad("Approve the order before moving stock against it.")
		}
		order = id(o, "id")
	}
	if kind == "Issue" {
		if order == nil {
			return nil, bad("Select the order receiving this issue.")
		}
		var stock int64
		if e = tx.QueryRow(ctx, `SELECT COALESCE(sum(quantity_hundredths),0)::bigint FROM movements WHERE item_id=$1`, iid).Scan(&stock); e != nil {
			return nil, e
		}
		if qty > stock {
			return nil, bad("Insufficient stock. Negative balances are not allowed.")
		}
		qty = -qty
	} else if kind != "Receipt" {
		return nil, bad("Choose receipt or issue.")
	}
	if (u.Scope != "company" || u.RestrictBuyers) && order == nil {
		return nil, bad("A scoped stock movement requires an assigned order.")
	}
	ref, e := text(d, "reference", 120)
	if e != nil {
		return nil, e
	}
	if _, e = tx.Exec(ctx, `INSERT INTO movements(item_id,quantity_hundredths,kind,order_id,reference,created_by) VALUES($1,$2,$3,$4,$5,$6)`, iid, qty, kind, order, ref, u.ID); e != nil {
		return nil, e
	}
	if e = audit(ctx, tx, u, id(item, "company_id"), "Stock "+kind, "Inventory", iid, str(item, "sku")+" · "+fmt.Sprint(float64(qty)/100)); e != nil {
		return nil, e
	}
	return M{"ok": true}, tx.Commit(ctx)
}

func validateProfile(ctx context.Context, q queryer, actor *User, d M) (*User, error) {
	p := &User{Role: str(d, "role"), Scope: str(d, "scope"), Units: []string{}}
	if !slices.Contains(Roles, p.Role) || !slices.Contains([]string{"company", "team", "line", "po"}, p.Scope) {
		return nil, bad("Choose a valid role and scope.")
	}
	var e error
	p.Companies, e = ids(d["companies"])
	if e != nil || len(p.Companies) == 0 {
		return nil, bad("Assign at least one company.")
	}
	for _, cid := range p.Companies {
		if !slices.Contains(actor.Companies, cid) {
			return nil, forbidden()
		}
	}
	for key, dest := range map[string]*[]int64{"teams": &p.Teams, "lines": &p.Lines, "orders": &p.Orders} {
		v := d[key]
		if v == nil {
			v = []any{}
		}
		list, e := ids(v)
		if e != nil {
			return nil, e
		}
		var count int
		if e = q.QueryRow(ctx, `SELECT count(*) FROM `+key+` WHERE id=ANY($1) AND company_id=ANY($2)`, list, p.Companies).Scan(&count); e != nil {
			return nil, e
		}
		if count != len(list) {
			return nil, bad("Assignments must belong to the selected companies.")
		}
		*dest = list
	}
	if p.Scope == "team" && len(p.Teams) == 0 || p.Scope == "line" && len(p.Lines) == 0 || p.Scope == "po" && len(p.Orders) == 0 {
		return nil, bad("Choose at least one assignment for the selected scope.")
	}
	if v, ok := d["finishing_units"].([]any); ok {
		for _, unit := range v {
			s, ok := unit.(string)
			if !ok || !slices.Contains(Products, s) {
				return nil, bad("Invalid finishing unit.")
			}
			p.Units = append(p.Units, s)
		}
	}
	if p.Role == "Finishing" && len(p.Units) == 0 {
		return nil, bad("Assign at least one finishing unit.")
	}
	if v, exists := d["permissions"]; exists {
		p.CustomPermissions = true
		p.Permissions = []string{}
		list, ok := v.([]any)
		if !ok {
			return nil, bad("Invalid permissions.")
		}
		for _, v := range list {
			s, ok := v.(string)
			if !ok || !slices.Contains(roleGrants[p.Role], s) || slices.Contains(p.Permissions, s) {
				return nil, bad("Permissions must belong to the selected department role.")
			}
			p.Permissions = append(p.Permissions, s)
		}
	}
	return p, nil
}
func (a *App) createUser(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if e := u.Require("users"); e != nil {
		return nil, e
	}
	d, e := readBody(w, r)
	if e != nil {
		return nil, e
	}
	password, _ := d["password"].(string)
	if !validPassword(password) {
		return nil, bad("Use a password between 12 and 128 characters.")
	}
	username, e := text(d, "username", 80)
	if e != nil {
		return nil, e
	}
	username = strings.ToLower(username)
	if strings.ContainsAny(username, " \t\n\r") {
		return nil, bad("Username cannot contain spaces.")
	}
	name, e := text(d, "name", 120)
	if e != nil {
		return nil, e
	}
	ctx := r.Context()
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	p, e := validateProfile(ctx, tx, u, d)
	if e != nil {
		return nil, e
	}
	var uid int64
	e = tx.QueryRow(ctx, `INSERT INTO users(username,name,password_hash,role,scope,companies,teams,lines,orders,finishing_units) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`, username, name, passwordHash(password), p.Role, p.Scope, p.Companies, p.Teams, p.Lines, p.Orders, p.Units).Scan(&uid)
	if e != nil {
		return nil, e
	}
	if p.CustomPermissions {
		if _, e = tx.Exec(ctx, `UPDATE users SET permissions=$1,custom_permissions=true WHERE id=$2`, p.Permissions, uid); e != nil {
			return nil, e
		}
	}
	for _, cid := range p.Companies {
		if e = audit(ctx, tx, u, cid, "User created", "User", uid, username+" · "+p.Role+" · "+p.Scope); e != nil {
			return nil, e
		}
	}
	return M{"id": uid}, tx.Commit(ctx)
}
func (a *App) updateUser(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if e := u.Require("users"); e != nil {
		return nil, e
	}
	uid, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if uid == u.ID {
		return nil, bad("Use another Super Admin account to change your own permissions.")
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
	target, e := row(ctx, tx, `SELECT id,username,companies FROM users WHERE id=$1 AND companies <@ $2::bigint[] AND cardinality(companies)>0 FOR UPDATE`, uid, u.Companies)
	if e != nil {
		return nil, e
	}
	companies := []int64{}
	switch v := target["companies"].(type) {
	case []int64:
		companies = v
	case []any:
		for _, x := range v {
			n, _ := integer(x)
			companies = append(companies, n)
		}
	}
	if active, ok := d["active"].(bool); ok {
		_, e = tx.Exec(ctx, `UPDATE users SET active=$1,session_version=session_version+1 WHERE id=$2`, active, uid)
	} else {
		p, e2 := validateProfile(ctx, tx, u, d)
		if e2 != nil {
			return nil, e2
		}
		companies = p.Companies
		_, e = tx.Exec(ctx, `UPDATE users SET role=$1,scope=$2,companies=$3,teams=$4,lines=$5,orders=$6,finishing_units=$7,session_version=session_version+1 WHERE id=$8`, p.Role, p.Scope, p.Companies, p.Teams, p.Lines, p.Orders, p.Units, uid)
		if e == nil && p.CustomPermissions {
			_, e = tx.Exec(ctx, `UPDATE users SET permissions=$1,custom_permissions=true WHERE id=$2`, p.Permissions, uid)
		} else if e == nil {
			_, e = tx.Exec(ctx, `UPDATE users SET permissions='{}',custom_permissions=false WHERE id=$1`, uid)
		}
	}
	if e != nil {
		return nil, e
	}
	for _, cid := range companies {
		if e = audit(ctx, tx, u, cid, "User access updated", "User", uid, str(target, "username")); e != nil {
			return nil, e
		}
	}
	return M{"ok": true}, tx.Commit(ctx)
}
