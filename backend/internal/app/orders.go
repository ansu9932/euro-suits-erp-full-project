package app

import (
	"context"
	"encoding/csv"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

const scopeSQL = `o.company_id=ANY($1::bigint[]) AND ($2='company' OR ($2='team' AND o.team_id=ANY($3::bigint[])) OR ($2='line' AND o.line_id=ANY($4::bigint[])) OR ($2='po' AND o.id=ANY($5::bigint[])) OR ($2 IN ('cutting_table','finishing_line') AND EXISTS(SELECT 1 FROM production_areas a WHERE a.id=ANY($10::bigint[]) AND a.company_id=o.company_id AND a.kind=$2 AND a.name=CASE $2 WHEN 'cutting_table' THEN o.cutting_table ELSE o.sub_line END))) AND ($7<>'Finishing' OR o.finishing_unit=ANY($6::text[])) AND (NOT $8::boolean OR o.buyer_id=ANY($9::bigint[]))`

func scopeArgs(u *User) []any {
	return []any{u.Companies, u.Scope, u.Teams, u.Lines, u.Orders, u.Units, u.Role, u.RestrictBuyers, u.BuyerIDs, u.ProductionAreas}
}
func (a *App) order(ctx context.Context, q queryer, u *User, oid int64, lock bool) (M, error) {
	if !u.Can("view_orders") {
		return nil, forbidden()
	}
	sql := `SELECT o.*,o.due_date::text AS due_date FROM orders o WHERE ` + scopeSQL + ` AND o.id=$11`
	if lock {
		sql += ` FOR UPDATE OF o`
	}
	return row(ctx, q, sql, append(scopeArgs(u), oid)...)
}
func audit(ctx context.Context, q queryer, u *User, cid int64, action, entity string, entityID int64, detail string) error {
	_, e := q.Exec(ctx, `INSERT INTO audit(company_id,actor_id,action,entity,entity_id,detail) VALUES($1,$2,$3,$4,$5,$6)`, cid, u.ID, action, entity, entityID, detail)
	return e
}
func (a *App) company(ctx context.Context, q queryer, u *User, cid int64, approved bool) error {
	if !slices.Contains(u.Companies, cid) {
		return missing()
	}
	var status string
	if e := q.QueryRow(ctx, `SELECT status FROM companies WHERE id=$1`, cid).Scan(&status); e != nil {
		return e
	}
	if approved && status != "Approved" {
		return bad("Company setup needs MD approval first.")
	}
	return nil
}
func (a *App) data(w http.ResponseWriter, r *http.Request) (any, error) {
	ctx := r.Context()
	u := current(r)
	if !u.Can("view_orders") {
		return nil, forbidden()
	}
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	out := M{"user": u, "local": !a.Production, "categories": Categories, "roles": Roles, "stages": Stages, "products": Products, "role_grants": roleGrants}
	fetch := func(key, sql string, args ...any) error {
		v, e := rows(ctx, tx, sql, args...)
		out[key] = v
		return e
	}
	if e := fetch("companies", `SELECT * FROM companies WHERE id=ANY($1) ORDER BY id`, u.Companies); e != nil {
		return nil, e
	}
	if e := fetch("teams", `SELECT * FROM teams WHERE company_id=ANY($1) AND ($2='company' OR ($2='team' AND id=ANY($3))) ORDER BY id`, u.Companies, u.Scope, u.Teams); e != nil {
		return nil, e
	}
	if e := fetch("buyers", `SELECT * FROM buyers WHERE company_id=ANY($1) AND ($2='company' OR ($2='team' AND team_id=ANY($3))) AND (NOT $4::boolean OR id=ANY($5::bigint[])) ORDER BY name`, u.Companies, u.Scope, u.Teams, u.RestrictBuyers, u.BuyerIDs); e != nil {
		return nil, e
	}
	if e := fetch("lines", `SELECT * FROM lines WHERE company_id=ANY($1) AND ($2<>'line' OR id=ANY($3)) ORDER BY id`, u.Companies, u.Scope, u.Lines); e != nil {
		return nil, e
	}
	if e := fetch("production_areas", `SELECT * FROM production_areas WHERE company_id=ANY($1) AND ($2 NOT IN ('cutting_table','finishing_line') OR id=ANY($3)) ORDER BY kind,name`, u.Companies, u.Scope, u.ProductionAreas); e != nil {
		return nil, e
	}
	sql := `SELECT o.*,o.due_date::text AS due_date,c.name AS company,b.name AS buyer,t.name AS team,l.name AS line,
 COALESCE((SELECT jsonb_object_agg(s.stage,s.total) FROM (SELECT e.stage,sum(good) AS total FROM entries e WHERE e.order_id=o.id GROUP BY e.stage)s),'{}') AS totals,
 COALESCE((SELECT jsonb_agg(jsonb_build_object('id',m.id,'title',m.title,'due_date',m.due_date,'done',m.done) ORDER BY m.due_date) FROM milestones m WHERE m.order_id=o.id),'[]') AS milestones
 FROM orders o JOIN companies c ON c.id=o.company_id JOIN buyers b ON b.id=o.buyer_id JOIN teams t ON t.id=o.team_id LEFT JOIN lines l ON l.id=o.line_id WHERE ` + scopeSQL + ` ORDER BY o.due_date,o.id`
	list, e := rows(ctx, tx, sql, scopeArgs(u)...)
	if e != nil {
		return nil, e
	}
	for _, o := range list {
		if !u.Can("finance") && !u.Can("orders") {
			delete(o, "unit_price_minor")
			delete(o, "currency")
		}
	}
	out["orders"] = list
	for _, key := range []string{"suppliers", "inventory", "purchases", "work_items", "entries", "quality", "audit", "users", "reminders"} {
		out[key] = []M{}
	}
	if u.Can("suppliers") || u.Can("purchases") || u.Can("inventory") {
		if e = fetch("suppliers", `SELECT * FROM suppliers WHERE company_id=ANY($1) ORDER BY name`, u.Companies); e != nil {
			return nil, e
		}
	}
	if u.Can("inventory") || u.Can("purchases") || u.Can("plan") {
		if e = fetch("inventory", `SELECT i.*,COALESCE((SELECT sum(quantity_hundredths)::bigint FROM movements m WHERE m.item_id=i.id),0) AS stock_hundredths FROM inventory i WHERE company_id=ANY($1) ORDER BY sku`, u.Companies); e != nil {
			return nil, e
		}
	}
	if u.Can("purchases") || u.Can("inventory") || u.Can("finance") || u.Can("approve") {
		if e = fetch("purchases", `SELECT p.*,p.due_date::text AS due_date,s.name AS supplier_name,i.name AS item_name FROM purchases p JOIN suppliers s ON s.id=p.supplier_id JOIN inventory i ON i.id=p.item_id WHERE p.company_id=ANY($1) AND (($2='company' AND NOT $8) OR p.order_id IN (SELECT o.id FROM orders o WHERE `+scopeSQL+`)) ORDER BY p.id DESC`, scopeArgs(u)...); e != nil {
			return nil, e
		}
	}
	if !u.Can("finance") && !u.Can("purchases") {
		if purchases, ok := out["purchases"].([]M); ok {
			for _, purchase := range purchases {
				delete(purchase, "unit_cost_minor")
			}
		}
	}
	for _, kind := range []string{"work_items", "entries", "quality", "reminders"} {
		if e = fetch(kind, `SELECT x.* FROM `+kind+` x JOIN orders o ON o.id=x.order_id WHERE `+scopeSQL, scopeArgs(u)...); e != nil {
			return nil, e
		}
	}
	if u.Can("audit") {
		if e = fetch("audit", `SELECT a.*,u.name AS actor FROM audit a LEFT JOIN users u ON u.id=a.actor_id WHERE a.company_id=ANY($1) AND (($2='company' AND NOT $8) OR (a.entity='Order' AND a.entity_id IN (SELECT o.id FROM orders o WHERE `+scopeSQL+`))) ORDER BY a.id DESC LIMIT 100`, scopeArgs(u)...); e != nil {
			return nil, e
		}
	}
	if u.Can("users") {
		us, e := rows(ctx, tx, `SELECT id,username,name,role,scope,companies,teams,lines,orders,finishing_units,active,permissions,custom_permissions FROM users WHERE companies <@ $1::bigint[] AND cardinality(companies)>0 ORDER BY id`, u.Companies)
		if e != nil {
			return nil, e
		}
		out["users"] = us
	}
	return out, tx.Commit(ctx)
}
func (a *App) createOrder(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if e := u.Require("orders"); e != nil {
		return nil, e
	}
	if u.Scope != "company" && u.Scope != "team" {
		return nil, forbidden()
	}
	d, e := readBody(w, r)
	if e != nil {
		return nil, e
	}
	po, e := text(d, "po", 60)
	if e != nil {
		return nil, e
	}
	style, e := text(d, "style", 150)
	if e != nil {
		return nil, e
	}
	qty, e := integer(d["quantity"])
	if e != nil || qty < 1 || qty > 10000000 {
		return nil, bad("Enter an order quantity from 1 to 10,000,000.")
	}
	due, e := date(str(d, "due_date"))
	if e != nil {
		return nil, e
	}
	product := str(d, "product")
	if !slices.Contains(Products, product) {
		return nil, bad("Choose a product.")
	}
	price := int64(0)
	if d["unit_price"] != nil {
		price, e = minor(d["unit_price"])
		if e != nil {
			return nil, e
		}
	}
	currency := str(d, "currency")
	if currency == "" {
		currency = "USD"
	}
	cid := id(d, "company_id")
	ctx := r.Context()
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	if e = a.company(ctx, tx, u, cid, true); e != nil {
		return nil, e
	}
	buyer, e := row(ctx, tx, `SELECT id,team_id FROM buyers WHERE id=$1 AND company_id=$2`, id(d, "buyer_id"), cid)
	if e != nil {
		return nil, e
	}
	if u.RestrictBuyers && !slices.Contains(u.BuyerIDs, id(buyer, "id")) {
		return nil, forbidden()
	}
	team := id(buyer, "team_id")
	if u.Scope == "team" && !slices.Contains(u.Teams, team) {
		return nil, forbidden()
	}
	var oid int64
	e = tx.QueryRow(ctx, `INSERT INTO orders(company_id,buyer_id,team_id,po,style,product,quantity,due_date,unit_price_minor,currency,notes,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12) RETURNING id`, cid, id(buyer, "id"), team, po, style, product, qty, due, price, currency, str(d, "notes"), u.ID).Scan(&oid)
	if e != nil {
		return nil, e
	}
	for _, m := range []struct {
		title string
		days  int
	}{{"Sample approval", 21}, {"Material readiness", 14}, {"Production complete", 3}, {"Ex-factory", 0}} {
		if _, e = tx.Exec(ctx, `INSERT INTO milestones(order_id,title,due_date) VALUES($1,$2,$3)`, oid, m.title, due.AddDate(0, 0, -m.days)); e != nil {
			return nil, e
		}
	}
	if e = audit(ctx, tx, u, cid, "Order created", "Order", oid, po+" · "+style); e != nil {
		return nil, e
	}
	return M{"id": oid}, tx.Commit(ctx)
}
func (a *App) orderDetail(w http.ResponseWriter, r *http.Request) (any, error) {
	oid, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	u := current(r)
	tx, err := a.DB.Begin(r.Context())
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(r.Context())
	o, e := a.order(r.Context(), tx, u, oid, false)
	if e != nil {
		return nil, e
	}
	if !u.Can("finance") && !u.Can("orders") {
		delete(o, "unit_price_minor")
		delete(o, "currency")
	}
	for key, sql := range map[string]string{"documents": `SELECT id,order_id,name,size,sha256,created_at FROM documents WHERE order_id=$1 ORDER BY id DESC`, "materials": `SELECT m.*,i.name AS item,i.unit FROM materials m JOIN inventory i ON i.id=m.item_id WHERE order_id=$1`, "movements": `SELECT m.*,i.name AS item FROM movements m JOIN inventory i ON i.id=m.item_id WHERE order_id=$1 ORDER BY id DESC`} {
		v, e := rows(r.Context(), tx, sql, oid)
		if e != nil {
			return nil, e
		}
		o[key] = v
	}
	return o, tx.Commit(r.Context())
}
func goodTotal(ctx context.Context, q queryer, oid int64, stage string) (int64, error) {
	var n int64
	e := q.QueryRow(ctx, `SELECT COALESCE(sum(good),0)::bigint FROM entries WHERE order_id=$1 AND stage=$2`, oid, stage).Scan(&n)
	return n, e
}
func ensureWorkItems(ctx context.Context, q queryer, oid int64, stage, style string) error {
	kinds := []string{}
	if stage == "Development" {
		kinds = []string{"Style development", "CAD / pattern"}
	} else if stage == "Sampling" {
		kinds = []string{"Pre-production sample"}
	}
	for _, kind := range kinds {
		if _, e := q.Exec(ctx, `INSERT INTO work_items(order_id,kind,title,status) SELECT $1,$2,$3,'Open' WHERE NOT EXISTS(SELECT 1 FROM work_items WHERE order_id=$1 AND kind=$2)`, oid, kind, style); e != nil {
			return e
		}
	}
	return nil
}
func (a *App) orderAction(w http.ResponseWriter, r *http.Request) (any, error) {
	d, e := readBody(w, r)
	if e != nil {
		return nil, e
	}
	oid, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	ctx := r.Context()
	u := current(r)
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	o, e := a.order(ctx, tx, u, oid, true)
	if e != nil {
		return nil, e
	}
	if id(d, "version") != id(o, "version") {
		return nil, failure{409, "This order changed. Refresh and try again."}
	}
	action := str(d, "action")
	stage := str(o, "stage")
	if stage == "Dispatched" {
		return nil, bad("Dispatched orders are locked. Historical work cannot be changed.")
	}
	cid := id(o, "company_id")
	qty := id(o, "quantity")
	switch action {
	case "approve", "reject":
		if e = u.Require("approve"); e != nil {
			return nil, e
		}
		if str(o, "approval") != "Pending" {
			return nil, bad("This order has already been reviewed.")
		}
		status := "Approved"
		unit := str(o, "product")
		if action == "reject" {
			status = "Rejected"
			unit = ""
		}
		_, e = tx.Exec(ctx, `UPDATE orders SET approval=$1,finishing_unit=$2 WHERE id=$3`, status, unit, oid)
	case "allocate":
		if e = u.Require("plan"); e != nil {
			return nil, e
		}
		if slices.Index(Stages, stage) > 4 {
			return nil, bad("Allocation is closed after sewing.")
		}
		line, e2 := row(ctx, tx, `SELECT id FROM lines WHERE id=$1 AND company_id=$2`, id(d, "line_id"), cid)
		if e2 != nil {
			return nil, e2
		}
		if u.Scope == "line" && !slices.Contains(u.Lines, id(line, "id")) {
			return nil, forbidden()
		}
		for _, key := range []string{"cutting_table", "sub_line", "marker"} {
			if len(str(d, key)) > 120 {
				return nil, bad("Allocation names must be 120 characters or less.")
			}
		}
		_, e = tx.Exec(ctx, `UPDATE orders SET line_id=$1,cutting_table=$2,marker=$3,sub_line=$4 WHERE id=$5`, id(line, "id"), str(d, "cutting_table"), str(d, "marker"), str(d, "sub_line"), oid)
	case "advance":
		if str(o, "approval") != "Approved" {
			return nil, bad("Approve this order before production.")
		}
		stageGrant := map[string]string{"Development": "develop", "Sampling": "sample", "Planning": "plan", "Cutting": "cut", "Sewing": "sew", "Finishing": "finish", "Packing": "pack", "Warehouse": "finance", "Invoiced": "warehouse"}
		grant, ok := stageGrant[stage]
		if !ok {
			return nil, bad("This order is already dispatched.")
		}
		if e = u.Require(grant); e != nil {
			return nil, e
		}
		if stage == "Development" || stage == "Sampling" {
			kinds := []string{"Style development", "CAD / pattern"}
			if stage == "Sampling" {
				kinds = []string{"Pre-production sample"}
			}
			for _, kind := range kinds {
				var ok bool
				if e = tx.QueryRow(ctx, `SELECT COALESCE((SELECT status='Approved' FROM work_items WHERE order_id=$1 AND kind=$2 ORDER BY id DESC LIMIT 1),false)`, oid, kind).Scan(&ok); e != nil {
					return nil, e
				}
				if !ok {
					return nil, bad("Approve the " + strings.ToLower(kind) + " record first.")
				}
			}
		}
		if stage == "Planning" && o["line_id"] == nil {
			return nil, bad("Assign a sewing line first.")
		}
		if slices.Contains([]string{"Cutting", "Sewing", "Finishing", "Packing"}, stage) {
			good, e2 := goodTotal(ctx, tx, oid, stage)
			if e2 != nil {
				return nil, e2
			}
			if good < qty {
				return nil, bad("Complete good output for this stage before moving forward.")
			}
			check, e2 := row(ctx, tx, `SELECT result,created_at >= COALESCE((SELECT max(created_at) FROM entries WHERE order_id=$1 AND stage=$2),'-infinity'::timestamptz) AS current FROM quality WHERE order_id=$1 AND stage=$2 ORDER BY id DESC LIMIT 1`, oid, stage)
			if e2 != nil || str(check, "result") != "Pass" || check["current"] != true {
				return nil, bad("Record a passing quality check after the latest output for this stage.")
			}
		}
		invoice := str(o, "invoice_number")
		dispatch := str(o, "dispatch_reference")
		if stage == "Warehouse" {
			invoice, e = text(d, "invoice_number", 80)
			if e != nil {
				return nil, e
			}
		}
		if stage == "Invoiced" {
			dispatch, e = text(d, "dispatch_reference", 100)
			if e != nil {
				return nil, e
			}
		}
		next := Stages[slices.Index(Stages, stage)+1]
		_, e = tx.Exec(ctx, `UPDATE orders SET stage=$1,invoice_number=$2,dispatch_reference=$3,finishing_unit=product WHERE id=$4`, next, invoice, dispatch, oid)
		if e == nil {
			e = ensureWorkItems(ctx, tx, oid, next, str(o, "style"))
		}
		stage = next
	case "output":
		grant := map[string]string{"Cutting": "cut", "Sewing": "sew", "Finishing": "finish", "Packing": "pack"}[stage]
		if grant == "" || str(o, "approval") != "Approved" {
			return nil, bad("Output requires an approved order in an active production stage.")
		}
		if e = u.Require(grant); e != nil {
			return nil, e
		}
		good, e2 := integer(d["good"])
		if e2 != nil {
			return nil, e2
		}
		rework, e2 := integer(d["rework"])
		if e2 != nil {
			return nil, e2
		}
		if good+rework == 0 {
			return nil, bad("Enter good output or rework quantity.")
		}
		total, e2 := goodTotal(ctx, tx, oid, stage)
		if e2 != nil {
			return nil, e2
		}
		if good+total > qty {
			return nil, bad("Good output cannot exceed the order quantity.")
		}
		_, e = tx.Exec(ctx, `INSERT INTO entries(order_id,stage,good,rework,note,created_by) VALUES($1,$2,$3,$4,$5,$6)`, oid, stage, good, rework, str(d, "note"), u.ID)
	case "quality":
		if e = u.Require("quality"); e != nil {
			return nil, e
		}
		inspected, e2 := integer(d["inspected"])
		if e2 != nil {
			return nil, e2
		}
		defects, e2 := integer(d["defects"])
		if e2 != nil {
			return nil, e2
		}
		if inspected < 1 || inspected > qty || defects > inspected || slices.Index(Stages, stage) > 7 {
			return nil, bad("Check inspection quantity and order stage.")
		}
		_, e = tx.Exec(ctx, `INSERT INTO quality(order_id,stage,inspected,defects,result,notes,created_by) VALUES($1,$2,$3,$4,$5,$6,$7)`, oid, stage, inspected, defects, str(d, "result"), str(d, "notes"), u.ID)
	case "milestone":
		if e = u.Require("plan"); e != nil {
			return nil, e
		}
		tag, e2 := tx.Exec(ctx, `UPDATE milestones SET done=NOT done WHERE id=$1 AND order_id=$2`, id(d, "milestone_id"), oid)
		if e2 != nil {
			return nil, e2
		}
		if tag.RowsAffected() != 1 {
			return nil, missing()
		}
	case "add_milestone":
		if e = u.Require("plan"); e != nil {
			return nil, e
		}
		due, e2 := date(str(d, "due_date"))
		if e2 != nil {
			return nil, e2
		}
		title, e2 := text(d, "title", 120)
		if e2 != nil {
			return nil, e2
		}
		_, e = tx.Exec(ctx, `INSERT INTO milestones(order_id,title,due_date) VALUES($1,$2,$3)`, oid, title, due)
	case "work_item":
		kind := str(d, "kind")
		expectedStage := "Development"
		grant := "develop"
		if strings.Contains(kind, "sample") {
			expectedStage = "Sampling"
			grant = "sample"
		}
		if e = u.Require(grant); e != nil {
			return nil, e
		}
		if stage != expectedStage {
			return nil, bad("Work records can only be added in their active department stage.")
		}
		title, e2 := text(d, "title", 120)
		if e2 != nil {
			return nil, e2
		}
		_, e = tx.Exec(ctx, `INSERT INTO work_items(order_id,kind,title,status,notes) VALUES($1,$2,$3,$4,$5)`, oid, kind, title, str(d, "status"), str(d, "notes"))
	case "material":
		if e = u.Require("plan"); e != nil {
			return nil, e
		}
		item, e2 := row(ctx, tx, `SELECT id FROM inventory WHERE id=$1 AND company_id=$2`, id(d, "item_id"), cid)
		if e2 != nil {
			return nil, e2
		}
		n, e2 := minor(d["quantity"])
		if e2 != nil || n <= 0 {
			return nil, bad("Enter a positive material quantity.")
		}
		_, e = tx.Exec(ctx, `INSERT INTO materials(order_id,item_id,quantity_hundredths) VALUES($1,$2,$3)`, oid, id(item, "id"), n)
	default:
		return nil, bad("Unknown order action.")
	}
	if e != nil {
		return nil, e
	}
	if _, e = tx.Exec(ctx, `UPDATE orders SET version=version+1,updated_at=now() WHERE id=$1`, oid); e != nil {
		return nil, e
	}
	if e = audit(ctx, tx, u, cid, action, "Order", oid, str(o, "po")+" · "+stage); e != nil {
		return nil, e
	}
	return M{"ok": true}, tx.Commit(ctx)
}
func (a *App) exportOrders(w http.ResponseWriter, r *http.Request) (any, error) {
	if !current(r).Can("view_orders") {
		return nil, forbidden()
	}
	u := current(r)
	sql := `SELECT o.po,c.name AS company,b.name AS buyer,o.style,o.product,o.quantity,o.stage,o.approval,o.due_date::text AS due_date,COALESCE(l.name,'') AS line,o.finishing_unit FROM orders o JOIN companies c ON c.id=o.company_id JOIN buyers b ON b.id=o.buyer_id LEFT JOIN lines l ON l.id=o.line_id WHERE ` + scopeSQL
	args := scopeArgs(u)
	if q := r.URL.Query().Get("company"); q != "" {
		cid, e := strconv.ParseInt(q, 10, 64)
		if e != nil {
			return nil, bad("Invalid company.")
		}
		sql += ` AND o.company_id=$11`
		args = append(args, cid)
	}
	data, e := rows(r.Context(), a.DB, sql, args...)
	if e != nil {
		return nil, e
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="euro-suits-orders.csv"`)
	writer := csv.NewWriter(w)
	keys := []string{"po", "company", "buyer", "style", "product", "quantity", "stage", "approval", "due_date", "line", "finishing_unit"}
	writer.Write(keys)
	for _, o := range data {
		v := []string{}
		for _, k := range keys {
			s := fmt.Sprint(o[k])
			if strings.ContainsAny(strings.TrimLeft(s, " \t\r\n")[:min(1, len(strings.TrimLeft(s, " \t\r\n")))], "=+-@") {
				s = "'" + s
			}
			v = append(v, s)
		}
		writer.Write(v)
	}
	writer.Flush()
	return nil, nil
}

var _ pgx.Tx
var _ = time.Second
