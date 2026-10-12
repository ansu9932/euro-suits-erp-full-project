package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
)

func init() {
	for key, label := range map[string]string{"enquiries": "Enquiries and booking register", "enquiry_costing": "View and prepare enquiry costings", "enquiry_fabric": "Respond to fabric enquiries", "enquiry_approve": "Review and approve booking enquiries"} {
		permissionLabels[key] = label
	}
	for _, role := range []string{"Super Admin", "MD", "Marketing", "Sourcing", "CEO"} {
		roleGrants[role] = append(roleGrants[role], "enquiries")
	}
	for _, role := range []string{"Super Admin", "MD", "Marketing", "CEO"} {
		roleGrants[role] = append(roleGrants[role], "enquiry_costing")
	}
	for _, role := range []string{"Super Admin", "Sourcing"} {
		roleGrants[role] = append(roleGrants[role], "enquiry_fabric")
	}
	for _, role := range []string{"Super Admin", "MD"} {
		roleGrants[role] = append(roleGrants[role], "enquiry_approve")
	}
}
func enquiryRead(u *User) bool {
	return u.Can("enquiries") && (u.Role == "Super Admin" || u.Scope == "company" || u.Scope == "team")
}
func enquiryWrite(u *User) bool {
	return enquiryRead(u) && (u.Role == "Super Admin" || u.Role == "MD" || u.Role == "Marketing" && u.Can("orders"))
}
func enquiryCostRead(u *User) bool { return enquiryRead(u) && u.Can("enquiry_costing") }
func enquiryReview(u *User) bool {
	return enquiryRead(u) && u.Can("enquiry_approve") && slices.Contains([]string{"Super Admin", "MD"}, u.Role)
}

const enquiryScope = `d.company_id=ANY($1) AND ($2='Super Admin' OR ($3='company' OR ($3='team' AND b.team_id=ANY($4))) AND (NOT $5 OR d.buyer_id=ANY($6)))`

func enquiryArgs(u *User) []any {
	return []any{u.Companies, u.Role, u.Scope, u.Teams, u.RestrictBuyers, u.BuyerIDs}
}
func (a *App) enquiryRecord(ctx context.Context, q queryer, u *User, rid int64, lock bool) (M, error) {
	if !enquiryRead(u) {
		return nil, forbidden()
	}
	sql := `SELECT d.*,b.name AS buyer_name,b.team_id,(SELECT name FROM users WHERE id=d.created_by) AS merchant,(SELECT code FROM material_samples WHERE id=d.material_id) AS material_code,(SELECT code FROM development_masters WHERE id=d.development_id) AS development_code FROM enquiries d JOIN buyers b ON b.id=d.buyer_id WHERE ` + enquiryScope + ` AND d.id=$7`
	if lock {
		sql += ` FOR UPDATE OF d`
	}
	m, e := row(ctx, q, sql, append(enquiryArgs(u), rid)...)
	if e == nil && str(m, "status") == "Draft" && !enquiryWrite(u) && !enquiryReview(u) && id(m, "created_by") != u.ID {
		return nil, missing()
	}
	return m, e
}
func enquiryPublic(u *User, m M) M {
	// All returns, including historical snapshots, pass through this boundary.
	out := materialProfile(m)
	fabric := materialProfile(out["fabric"])
	if !materialCommercial(u) {
		for _, k := range []string{"supplier_reference", "payment_terms", "gst_paid_reference", "gst_claim_reference", "remarks"} {
			delete(fabric, k)
		}
	}
	if !enquiryCostRead(u) && !u.Can("enquiry_fabric") {
		for _, k := range strings.Fields("price currency exchange_rate freight_metre duty_metre advance_license_metre sgst_percent cgst_percent advance_percent") {
			delete(fabric, k)
		}
	}
	out["fabric"] = fabric
	if !enquiryCostRead(u) {
		delete(out, "costing")
		delete(out, "calculation")
	}
	return out
}
func enquiryCalculation(m M) (M, error) {
	calc, e := enquiryCosting(enquiryCostInput{Consumption: materialProfile(m["profile"])["consumption"], Quantity: enquiryQuantity(m["colours"]), Fabric: materialProfile(m["fabric"]), Cost: materialProfile(m["costing"])})
	if e != nil {
		return nil, e
	}
	sales, e := enquirySaleCalculation(materialProfile(m["profile"]), enquiryQuantity(m["colours"]))
	if e != nil {
		return nil, e
	}
	for k, v := range sales {
		calc[k] = v
	}
	return calc, nil
}
func (a *App) enquiryHistory(ctx context.Context, q queryer, u *User, rid int64, action string) error {
	m, e := a.enquiryRecord(ctx, q, u, rid, false)
	if e != nil {
		return e
	}
	b, e := json.Marshal(m)
	if e != nil {
		return e
	}
	_, e = q.Exec(ctx, `INSERT INTO enquiry_history(enquiry_id,actor_id,action,snapshot) VALUES($1,$2,$3,$4)`, rid, u.ID, action, b)
	return e
}
func (a *App) enquiryList(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if !enquiryRead(u) {
		return nil, forbidden()
	}
	ctx := r.Context()
	records, e := rows(ctx, a.DB, `SELECT d.id,d.company_id,d.buyer_id,d.created_by,d.status,d.booking_number,d.version,d.updated_at,d.profile,d.colours,b.name AS buyer_name,(SELECT name FROM users WHERE id=d.created_by) AS merchant FROM enquiries d JOIN buyers b ON b.id=d.buyer_id WHERE `+enquiryScope+` AND (d.status<>'Draft' OR $7 OR d.created_by=$8) ORDER BY d.id DESC LIMIT 2000`, append(enquiryArgs(u), enquiryWrite(u) || enquiryReview(u), u.ID)...)
	if e != nil {
		return nil, e
	}
	buyers, e := rows(ctx, a.DB, `SELECT b.id,b.name,b.company_id,b.team_id FROM buyers b WHERE b.company_id=ANY($1) AND ($2='Super Admin' OR ($3='company' OR ($3='team' AND b.team_id=ANY($4))) AND (NOT $5 OR b.id=ANY($6))) ORDER BY b.name`, enquiryArgs(u)...)
	if e != nil {
		return nil, e
	}
	styles, e := rows(ctx, a.DB, `SELECT d.id,d.company_id,d.buyer_id,d.code,d.profile,d.material_ids FROM development_masters d WHERE `+sampleScope+` AND d.status='Approved' ORDER BY d.id DESC LIMIT 2000`, developmentArgs(u)...)
	if e != nil {
		return nil, e
	}
	materials, e := rows(ctx, a.DB, `SELECT id,company_id,code FROM material_samples WHERE company_id=ANY($1) AND kind='fabric' AND active ORDER BY code`, u.Companies)
	if e != nil {
		return nil, e
	}
	options, e := rows(ctx, a.DB, `SELECT company_id,kind,label FROM enquiry_options WHERE company_id=ANY($1) ORDER BY label`, u.Companies)
	if e != nil {
		return nil, e
	}
	return M{"records": records, "buyers": buyers, "styles": styles, "materials": materials, "options": options, "defaults": enquiryDefaults, "can_create": enquiryWrite(u), "can_cost": enquiryCostRead(u), "can_edit_cost": enquiryCostRead(u) && enquiryWrite(u), "record_limit": 2000, "can_fabric": u.Can("enquiry_fabric"), "can_review": enquiryReview(u), "can_documents": u.Can("documents") || u.Role == "Super Admin", "can_orders": u.Can("orders"), "supplier_details": materialCommercial(u)}, nil
}
func (a *App) enquiryDetail(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	ctx := r.Context()
	m, e := a.enquiryRecord(ctx, a.DB, u, materialID(r), false)
	if e != nil {
		return nil, e
	}
	if enquiryCostRead(u) {
		if calc, err := enquiryCalculation(m); err == nil {
			m["calculation"] = calc
		} else {
			m["calculation_error"] = err.Error()
		}
	}
	files, e := rows(ctx, a.DB, `SELECT id,name,category,order_id,created_at FROM enquiry_files WHERE enquiry_id=$1 AND ($2 OR category NOT IN ('Supplier quotation','Purchase proof')) ORDER BY id`, id(m, "id"), materialCommercial(u))
	if e != nil {
		return nil, e
	}
	history, e := rows(ctx, a.DB, `SELECT h.id,h.action,h.created_at,u.name AS actor FROM enquiry_history h JOIN users u ON u.id=h.actor_id WHERE enquiry_id=$1 ORDER BY h.id DESC LIMIT 500`, id(m, "id"))
	if e != nil {
		return nil, e
	}
	orders, e := rows(ctx, a.DB, `SELECT id,po,quantity,stage,approval,due_date::text FROM orders WHERE enquiry_id=$1 ORDER BY id`, id(m, "id"))
	if e != nil {
		return nil, e
	}
	purchases, e := rows(ctx, a.DB, `SELECT p.id,p.reference,p.status,p.quantity_hundredths,o.po FROM purchases p JOIN orders o ON o.id=p.order_id WHERE o.enquiry_id=$1 ORDER BY p.id`, id(m, "id"))
	if e != nil {
		return nil, e
	}
	return M{"record": enquiryPublic(u, m), "files": files, "history": history, "orders": orders, "purchases": purchases}, nil
}
func (a *App) enquiryHistoryDetail(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	ctx := r.Context()
	h, e := row(ctx, a.DB, `SELECT * FROM enquiry_history WHERE id=$1`, materialID(r))
	if e != nil {
		return nil, e
	}
	if _, e = a.enquiryRecord(ctx, a.DB, u, id(h, "enquiry_id"), false); e != nil {
		return nil, e
	}
	h["snapshot"] = enquiryPublic(u, materialProfile(h["snapshot"]))
	return h, nil
}
func (a *App) enquiryOption(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if !enquiryRead(u) || !(enquiryWrite(u) || u.Can("enquiry_fabric") || enquiryReview(u)) {
		return nil, forbidden()
	}
	d, e := readBody(w, r)
	if e != nil {
		return nil, e
	}
	cid := id(d, "company_id")
	if e = a.company(r.Context(), a.DB, u, cid, true); e != nil {
		return nil, e
	}
	k, label := str(d, "kind"), strings.Join(strings.Fields(str(d, "label")), " ")
	defaults, ok := enquiryDefaults[k]
	if !ok || len(label) < 1 || len(label) > 100 {
		return nil, bad("Choose a supported dropdown and enter a value up to 100 characters.")
	}
	if k == "currency" {
		p := M{"currency": label}
		if e = enquiryCurrency(p, "currency"); e != nil {
			return nil, e
		}
		label = str(p, "currency")
	}
	for _, v := range defaults {
		if strings.EqualFold(v, label) {
			return nil, bad("This option already exists.")
		}
	}
	_, e = a.DB.Exec(r.Context(), `INSERT INTO enquiry_options(company_id,kind,label,created_by) VALUES($1,$2,$3,$4)`, cid, k, label, u.ID)
	return M{"ok": true, "label": label}, e
}

func (a *App) enquirySave(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if !enquiryRead(u) {
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
	rid := materialID(r)
	action := str(d, "action")
	if action == "" {
		action = "save"
	}
	var old M
	if rid > 0 {
		old, e = a.enquiryRecord(ctx, tx, u, rid, true)
		if e != nil {
			return nil, e
		}
		if id(old, "version") != id(d, "version") {
			return nil, failure{409, "This enquiry changed. Refresh before saving."}
		}
	}
	if old != nil {
		if e = a.company(ctx, tx, u, id(old, "company_id"), true); e != nil {
			return nil, e
		}
	}
	status := str(old, "status")
	event := "Enquiry saved"
	switch action {
	case "save":
		if !enquiryWrite(u) {
			return nil, forbidden()
		}
		if old != nil && !slices.Contains([]string{"Draft", "Costing", "Changes requested"}, status) {
			return nil, bad("This enquiry is locked. Request an amendment before changing it.")
		}
		cid, bid := id(d, "company_id"), id(d, "buyer_id")
		if e = a.company(ctx, tx, u, cid, true); e != nil {
			return nil, e
		}
		if old != nil && (cid != id(old, "company_id") || bid != id(old, "buyer_id")) {
			return nil, bad("Company and buyer cannot change after the enquiry is saved.")
		}
		if _, e = row(ctx, tx, `SELECT b.id FROM buyers b WHERE b.id=$7 AND b.company_id=ANY($1) AND b.company_id=$8 AND ($2='Super Admin' OR ($3='company' OR ($3='team' AND b.team_id=ANY($4))) AND (NOT $5 OR b.id=ANY($6)))`, append(enquiryArgs(u), bid, cid)...); e != nil {
			return nil, e
		}
		p, err := enquiryValidateProfile(d["profile"])
		if err != nil {
			return nil, err
		}
		if err = enquiryCheckOptions(ctx, tx, cid, p, map[string]string{"enquiry_type": "enquiry_type", "brand": "brand", "season": "season", "product": "product", "incoterm": "incoterm", "garment_currency": "currency"}); err != nil {
			return nil, err
		}
		for _, k := range []string{"enquiry_type", "brand", "product"} {
			if str(p, k) == "" {
				return nil, bad("Enter " + strings.ReplaceAll(k, "_", " ") + ".")
			}
		}
		if old != nil && str(materialProfile(old["costing"]), "currency") != "" && str(p, "garment_currency") != str(materialProfile(old["costing"]), "currency") {
			return nil, bad("The saved costing uses another currency. Keep that currency or create a new enquiry.")
		}
		colours, _, err := enquiryColours(d["colours"])
		if err != nil {
			return nil, err
		}
		var mid, did any
		if n := id(d, "material_id"); n > 0 {
			if _, err = row(ctx, tx, `SELECT id FROM material_samples WHERE id=$1 AND company_id=$2 AND kind='fabric' AND active`, n, cid); err != nil {
				return nil, bad("Choose an active ETF / ESF from this company.")
			}
			mid = n
		}
		if n := id(d, "development_id"); n > 0 {
			style, err := a.developmentRecord(ctx, tx, u, n, false)
			if err != nil {
				return nil, err
			}
			if id(style, "company_id") != cid || (id(style, "buyer_id") != 0 && id(style, "buyer_id") != bid) || str(style, "status") != "Approved" {
				return nil, bad("Choose an approved style belonging to this buyer.")
			}
			did = n
		}
		pb, _ := json.Marshal(p)
		cb, _ := json.Marshal(colours)
		if old == nil {
			e = tx.QueryRow(ctx, `INSERT INTO enquiries(company_id,buyer_id,created_by,development_id,material_id,profile,colours) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`, cid, bid, u.ID, did, mid, pb, cb).Scan(&rid)
		} else {
			_, e = tx.Exec(ctx, `UPDATE enquiries SET development_id=$1,material_id=$2,profile=$3,colours=$4,version=version+1,updated_at=now() WHERE id=$5`, did, mid, pb, cb, rid)
		}
	default:
		if old == nil {
			return nil, bad("Save the enquiry first.")
		}
		event, e = a.enquiryAction(ctx, tx, u, old, d, action)
	}
	if e != nil {
		return nil, e
	}
	if e = a.enquiryHistory(ctx, tx, u, rid, event); e != nil {
		return nil, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, e
	}
	return M{"id": rid}, nil
}

func enquiryReference(m M) string { return fmt.Sprintf("EQ-%06d", id(m, "id")) }
