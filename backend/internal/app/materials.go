package app

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"github.com/go-chi/chi/v5"
	"math"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"
)

//go:embed material_fields.json
var materialFieldsJSON []byte

type materialField struct {
	Key      string `json:"key"`
	Label    string `json:"label"`
	Section  string `json:"section"`
	Type     string `json:"type"`
	Options  any    `json:"options,omitempty"`
	Required bool   `json:"required"`
}

var materialFields []materialField
var materialOptionKinds = []string{"category", "weave", "sub_item", "composition", "product", "color", "swatch_type", "finish_type", "season", "trim_type", "unit", "currency", "weight_unit", "width_unit", "development_type", "fit", "feature"}

func init() {
	if e := json.Unmarshal(materialFieldsJSON, &materialFields); e != nil {
		panic(e)
	}
	for _, role := range []string{"Super Admin", "MD", "CEO", "Marketing", "Development", "Sampling", "Sourcing", "Stores", "Quality"} {
		roleGrants[role] = append(roleGrants[role], "material_view")
	}
	for _, role := range []string{"Super Admin", "MD", "CEO", "Sourcing"} {
		roleGrants[role] = append(roleGrants[role], "material_commercial")
	}
	for _, role := range []string{"Super Admin", "Sourcing"} {
		roleGrants[role] = append(roleGrants[role], "fabric_edit", "trim_edit", "material_masters")
	}
	// Merchant writes remain disabled to preserve the seven-field privacy boundary.
	for key, label := range map[string]string{"material_view": "View fabric / trim library", "material_commercial": "View supplier and fabric commercial details", "fabric_edit": "Create / update fabric samples and photos", "trim_edit": "Create / update trim samples", "material_masters": "Maintain material master lists"} {
		permissionLabels[key] = label
	}
}
func materialCan(u *User, g string) bool { return u.Role == "Super Admin" || u.Can(g) }
func materialEditor(u *User, kind string) bool {
	return materialCommercial(u) && materialCan(u, map[string]string{"fabric": "fabric_edit", "trim": "trim_edit"}[kind])
}
func materialCommercial(u *User) bool { return materialCan(u, "material_commercial") }
func materialProfile(v any) M {
	b, _ := json.Marshal(v)
	m := M{}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.UseNumber()
	_ = dec.Decode(&m)
	return m
}
func materialID(r *http.Request) int64 {
	n, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	return n
}
func materialNumber(v any) (float64, error) {
	s := fmt.Sprint(v)
	if v == nil || s == "" {
		return 0, nil
	}
	f, e := strconv.ParseFloat(s, 64)
	if e != nil || math.IsNaN(f) || math.IsInf(f, 0) || f < 0 || f > 1e9 {
		return 0, bad("Enter a non-negative number below one billion.")
	}
	return f, nil
}
func (a *App) materialSample(ctx context.Context, q queryer, u *User, id int64, lock bool) (M, error) {
	if !materialCan(u, "material_view") {
		return nil, forbidden()
	}
	sql := `SELECT * FROM material_samples WHERE id=$1 AND company_id=ANY($2)`
	if lock {
		sql += ` FOR UPDATE`
	}
	return row(ctx, q, sql, id, u.Companies)
}
func redactMaterial(u *User, m M) M {
	p := materialProfile(m["profile"])
	if !materialCommercial(u) {
		keep := M{}
		for _, k := range []string{"category", "weave", "composition", "color", "measurements"} {
			keep[k] = p[k]
		}
		// Only the two public measurement values are included; actual inspection values stay private.
		if ms, ok := p["measurements"].([]any); ok {
			clean := []M{}
			for _, v := range ms {
				x := materialProfile(v)
				n := M{}
				for _, k := range []string{"weight", "weight_max", "weight_unit", "width", "width_max", "width_unit"} {
					n[k] = x[k]
				}
				clean = append(clean, n)
			}
			keep["measurements"] = clean
		}
		return M{"id": m["id"], "company_id": m["company_id"], "kind": m["kind"], "code": m["code"], "profile": keep, "active": m["active"], "supplier_blocked": m["supplier_blocked"]}
	}
	return m
}
func (a *App) materialList(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if !materialCan(u, "material_view") {
		return nil, forbidden()
	}
	ctx := r.Context()
	out := M{"can_development": developmentRead(u), "fields": materialFields, "option_kinds": materialOptionKinds, "can_edit_fabric": materialEditor(u, "fabric"), "can_edit_trim": materialEditor(u, "trim"), "can_masters": materialCan(u, "material_masters"), "commercial": materialCommercial(u), "can_rate": u.Role == "MD" || u.Role == "Super Admin"}
	cs, e := rows(ctx, a.DB, `SELECT id,name,status FROM companies WHERE id=ANY($1) ORDER BY name`, u.Companies)
	if e != nil {
		return nil, e
	}
	out["companies"] = cs
	opts, e := rows(ctx, a.DB, `SELECT * FROM material_options WHERE company_id=ANY($1) ORDER BY kind,label`, u.Companies)
	if e != nil {
		return nil, e
	}
	out["options"] = opts
	all, e := rows(ctx, a.DB, `SELECT m.*,COALESCE(s.rating,'Not rated') AS supplier_rating,COALESCE(s.rating IN ('Blocked','Delisted'),false) AS supplier_blocked FROM material_samples m LEFT JOIN supplier_assessments s ON s.supplier_setup_id=m.supplier_setup_id WHERE m.company_id=ANY($1) ORDER BY m.id DESC LIMIT 10001`, u.Companies)
	if e != nil {
		return nil, e
	}
	out["truncated"] = len(all) > 10000
	if len(all) > 10000 {
		all = all[:10000]
	}
	list := []M{}
	for _, m := range all {
		list = append(list, redactMaterial(u, m))
	}
	out["samples"] = list
	out["suppliers"] = []M{}
	if materialCommercial(u) {
		ss, e := rows(ctx, a.DB, `SELECT d.*,COALESCE(a.rating,'Not rated') AS rating,COALESCE(a.reason,'') AS rating_reason,COALESCE(a.version,0) AS rating_version FROM erp_material_suppliers() d LEFT JOIN supplier_assessments a ON a.supplier_setup_id=d.id ORDER BY d.legal_name`)
		if e != nil {
			return nil, e
		}
		out["suppliers"] = ss
	}
	return out, nil
}
func (a *App) materialDetail(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	m, e := a.materialSample(r.Context(), a.DB, u, materialID(r), false)
	if e != nil {
		return nil, e
	}
	var blocked bool
	if e = a.DB.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM supplier_assessments WHERE supplier_setup_id=$1 AND rating IN ('Blocked','Delisted'))`, id(m, "supplier_setup_id")).Scan(&blocked); e != nil {
		return nil, e
	}
	m["supplier_blocked"] = blocked
	m = redactMaterial(u, m)
	m["photos"] = []M{}
	m["history"] = []M{}
	if materialCommercial(u) {
		p, e := rows(r.Context(), a.DB, `SELECT id,name,mime,size,created_at FROM material_photos WHERE sample_id=$1 ORDER BY id`, id(m, "id"))
		if e != nil {
			return nil, e
		}
		m["photos"] = p
		h, e := rows(r.Context(), a.DB, `SELECT h.id,h.action,h.snapshot,h.created_at,u.name AS actor FROM material_history h JOIN users u ON u.id=h.actor_id WHERE h.sample_id=$1 ORDER BY h.id DESC LIMIT 200`, id(m, "id"))
		if e != nil {
			return nil, e
		}
		m["history"] = h
	}
	return m, nil
}
func materialCustomChoice(ctx context.Context, q queryer, company int64, kind, value string) bool {
	if q == nil || value == "" {
		return false
	}
	var found bool
	err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM material_options WHERE company_id=$1 AND kind=$2 AND label=$3 AND active)`, company, kind, value).Scan(&found)
	return err == nil && found
}
func (a *App) validateMaterial(ctx context.Context, q queryer, company int64, kind string, raw any) (M, error) {
	p := materialProfile(raw)
	clean := M{}
	for _, f := range materialFields {
		value := strings.TrimSpace(fmt.Sprint(p[f.Key]))
		if p[f.Key] == nil {
			value = ""
		}
		required := f.Required
		if kind == "trim" && (f.Key == "category" || f.Key == "weave") {
			required = false
		}
		if required && value == "" {
			return nil, bad(f.Label + " is required.")
		}
		if len(value) > 2000 {
			return nil, bad(f.Label + " is too long.")
		}
		switch f.Type {
		case "number":
			if _, e := materialNumber(value); e != nil {
				return nil, bad(f.Label + ": enter a valid non-negative number.")
			}
		case "date":
			if value != "" {
				if _, e := time.Parse("2006-01-02", value); e != nil {
					return nil, bad(f.Label + ": use a valid date.")
				}
			}
		case "select":
			if value != "" {
				found := false
				for _, v := range f.Options.([]any) {
					if v == value {
						found = true
					}
				}
				choiceKind := "unit"
				if f.Key == "currency" {
					choiceKind = "currency"
				}
				if !found && !materialCustomChoice(ctx, q, company, choiceKind, value) {
					return nil, bad("Choose a valid " + f.Label)
				}
			}
		case "option":
			if value != "" {
				oid, e := strconv.ParseInt(value, 10, 64)
				if e != nil {
					return nil, bad("Choose " + f.Label + " from its master list.")
				}
				o, e := row(ctx, q, `SELECT id,parent_id FROM material_options WHERE id=$1 AND company_id=$2 AND kind=$3 AND active`, oid, company, f.Options)
				if e != nil {
					return nil, bad("Choose an active " + f.Label + " from this company's masters.")
				}
				parent := ""
				if f.Key == "weave" {
					parent = "category"
				}
				if f.Key == "sub_item" {
					parent = "weave"
				}
				if parent != "" && id(o, "parent_id") != id(p, parent) {
					return nil, bad(f.Label + " does not belong to the selected parent.")
				}
				clean[f.Key] = oid
				continue
			}
		}
		clean[f.Key] = value
	}
	measurements := []M{}
	if raw, ok := p["measurements"].([]any); ok {
		if len(raw) > 20 {
			return nil, bad("Use at most 20 measurement rows.")
		}
		for _, v := range raw {
			x := materialProfile(v)
			n := M{}
			for _, k := range []string{"weight", "weight_max", "width", "width_max", "actual_weight", "actual_width"} {
				f, e := materialNumber(x[k])
				if e != nil {
					return nil, e
				}
				n[k] = f
			}
			wu := str(x, "weight_unit")
			du := str(x, "width_unit")
			if (!slices.Contains([]string{"GSM", "g/linear metre", "oz/yd²"}, wu) && !materialCustomChoice(ctx, q, company, "weight_unit", wu)) || (!slices.Contains([]string{"cm", "inch", "metre"}, du) && !materialCustomChoice(ctx, q, company, "width_unit", du)) {
				return nil, bad("Choose weight and width units.")
			}
			for _, k := range []string{"weight", "width"} {
				if n[k+"_max"].(float64) > 0 && n[k+"_max"].(float64) < n[k].(float64) {
					return nil, bad("Measurement maximum must not be below its minimum.")
				}
			}
			n["weight_unit"] = wu
			n["width_unit"] = du
			measurements = append(measurements, n)
		}
	}
	clean["measurements"] = measurements
	for _, k := range []string{"trim_type", "size", "unit", "units_per_pack", "pack_unit", "development_type", "fit", "style_features"} {
		v := str(p, k)
		if k == "units_per_pack" && p[k] != nil {
			v = fmt.Sprint(p[k])
			if _, e := materialNumber(v); e != nil {
				return nil, bad("Units per pack must be a non-negative number.")
			}
		}
		if len(v) > 1000 {
			return nil, bad("Trim detail is too long.")
		}
		clean[k] = v
	}
	extras := []M{}
	if xs, ok := p["additional"].([]any); ok {
		if len(xs) > 30 {
			return nil, bad("Use at most 30 additional details.")
		}
		for _, v := range xs {
			x := materialProfile(v)
			k, val := str(x, "label"), str(x, "value")
			if k == "" || len(k) > 100 || len(val) > 2000 {
				return nil, bad("Additional details need a short label and value.")
			}
			extras = append(extras, M{"label": k, "value": val})
		}
	}
	clean["additional"] = extras
	return clean, nil
}
func (a *App) materialSave(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	d, e := readBody(w, r)
	if e != nil {
		return nil, e
	}
	kind := str(d, "kind")
	if !slices.Contains([]string{"fabric", "trim"}, kind) || !materialEditor(u, kind) || !materialCan(u, "material_view") {
		return nil, forbidden()
	}
	ctx := r.Context()
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	cid, sid, rid := id(d, "company_id"), id(d, "supplier_setup_id"), materialID(r)
	if e = a.company(ctx, tx, u, cid, true); e != nil {
		return nil, e
	}
	var before M
	if rid > 0 {
		before, e = a.materialSample(ctx, tx, u, rid, true)
		if e != nil {
			return nil, e
		}
		if id(before, "version") != id(d, "version") {
			return nil, failure{409, "This sample changed. Reopen it before saving."}
		}
		if cid != id(before, "company_id") || sid != id(before, "supplier_setup_id") || kind != str(before, "kind") {
			return nil, bad("Company, supplier and material type cannot change on an existing reference.")
		}
	}
	var supplierExists bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM erp_material_suppliers() WHERE id=$1)`, sid).Scan(&supplierExists); e != nil {
		return nil, e
	}
	if !supplierExists {
		return nil, bad("Finance must register the supplier and obtain approval first.")
	}
	// A shared supplier lock prevents a concurrent block and new sample crossing.
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, sid+824740000); e != nil {
		return nil, e
	}
	var blocked bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM supplier_assessments WHERE supplier_setup_id=$1 AND rating IN ('Blocked','Delisted'))`, sid).Scan(&blocked); e != nil {
		return nil, e
	}
	if blocked {
		return nil, bad("Supplier is blocked or delisted. New or changed samples are not allowed.")
	}
	p, e := a.validateMaterial(ctx, tx, cid, kind, d["profile"])
	if e != nil {
		return nil, e
	}
	quoteChanged := rid == 0
	for _, key := range []string{"price", "currency", "price_unit", "price_date", "price_code", "price_note"} {
		quoteChanged = quoteChanged || fmt.Sprint(materialProfile(before["profile"])[key]) != fmt.Sprint(p[key])
	}
	if quoteChanged {
		p["price_recorded_at"] = time.Now().UTC().Format(time.RFC3339)
	} else {
		p["price_recorded_at"] = materialProfile(before["profile"])["price_recorded_at"]
	}
	oldETF, err := normalizeOldETF(str(p, "old_etf"))
	if err != nil {
		return nil, err
	}
	if kind != "fabric" && oldETF != "" {
		return nil, bad("Old ETF is only available for fabric references.")
	}
	if rid > 0 && oldETF != str(materialProfile(before["profile"]), "old_etf") {
		return nil, bad("Old ETF cannot change after registration. Use Legacy reference for additional information.")
	}
	p["old_etf"] = oldETF
	b, _ := json.Marshal(p)
	code := ""
	active := d["active"] != false
	if rid == 0 {
		origin := str(d, "origin")
		if !slices.Contains([]string{"India", "Dubai"}, origin) {
			return nil, bad("Choose India or Dubai as the reference origin.")
		}
		if e = tx.QueryRow(ctx, `SELECT nextval('material_samples_id_seq')`).Scan(&rid); e != nil {
			return nil, e
		}
		prefix := "ESF"
		if kind == "trim" {
			prefix = "ESTR"
		}
		if origin == "Dubai" {
			prefix = "ETF"
			if kind == "trim" {
				prefix = "ETTR"
			}
		}
		for {
			code = fmt.Sprintf("%s-%07d", prefix, rid)
			if oldETF != "" {
				code = oldETF
			}
			inserted, insertErr := tx.Exec(ctx, `INSERT INTO material_samples(id,company_id,supplier_setup_id,kind,code,profile,active,created_by,updated_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$8) ON CONFLICT(code) DO NOTHING`, rid, cid, sid, kind, code, b, active, u.ID)
			if insertErr != nil {
				return nil, insertErr
			}
			if inserted.RowsAffected() == 1 {
				break
			}
			if oldETF != "" {
				return nil, failure{409, "This Old ETF reference is already registered. Open the existing record instead."}
			}
			if e = tx.QueryRow(ctx, `SELECT nextval('material_samples_id_seq')`).Scan(&rid); e != nil {
				return nil, e
			}
		}
	} else {
		code = str(before, "code")
		_, e = tx.Exec(ctx, `UPDATE material_samples SET profile=$1,active=$2,version=version+1,updated_by=$3,updated_at=now() WHERE id=$4`, b, active, u.ID, rid)
	}
	if e != nil {
		return nil, e
	}
	action := "Sample created"
	if before != nil {
		action = "Sample updated"
	}
	snapshot, _ := json.Marshal(M{"profile": p, "active": active, "code": code})
	if _, e = tx.Exec(ctx, `INSERT INTO material_history(sample_id,actor_id,action,snapshot) VALUES($1,$2,$3,$4)`, rid, u.ID, action, snapshot); e != nil {
		return nil, e
	}
	if e = audit(ctx, tx, u, cid, action, "Material", rid, code); e != nil {
		return nil, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, e
	}
	return M{"id": rid, "code": code}, nil
}
func (a *App) materialOptionSave(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if !materialCan(u, "material_masters") {
		return nil, forbidden()
	}
	d, e := readBody(w, r)
	if e != nil {
		return nil, e
	}
	cid := id(d, "company_id")
	kind, label := str(d, "kind"), str(d, "label")
	if !slices.Contains(materialOptionKinds, kind) || label == "" || len(label) > 150 {
		return nil, bad("Choose a master type and enter a name of up to 150 characters.")
	}
	ctx := r.Context()
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	if e = a.company(ctx, tx, u, cid, true); e != nil {
		return nil, e
	}
	var parent any
	if kind == "weave" || kind == "sub_item" {
		pk := "category"
		if kind == "sub_item" {
			pk = "weave"
		}
		p, e := row(ctx, tx, `SELECT id FROM material_options WHERE id=$1 AND company_id=$2 AND kind=$3 AND active`, id(d, "parent_id"), cid, pk)
		if e != nil {
			return nil, bad("Choose an active parent from the same company.")
		}
		parent = id(p, "id")
	}
	rid := materialID(r)
	if rid == 0 {
		e = tx.QueryRow(ctx, `INSERT INTO material_options(company_id,kind,label,parent_id,created_by) VALUES($1,$2,$3,$4,$5) RETURNING id`, cid, kind, label, parent, u.ID).Scan(&rid)
	} else {
		old, e2 := row(ctx, tx, `SELECT * FROM material_options WHERE id=$1 AND company_id=$2 FOR UPDATE`, rid, cid)
		if e2 != nil {
			return nil, e2
		}
		if id(old, "version") != id(d, "version") {
			return nil, failure{409, "Master changed. Refresh before editing."}
		}
		if str(old, "kind") != kind || id(old, "parent_id") != id(d, "parent_id") {
			return nil, bad("A master cannot be moved to another type or parent. Create a new entry instead.")
		}
		_, e = tx.Exec(ctx, `UPDATE material_options SET label=$1,active=$2,version=version+1 WHERE id=$3`, label, d["active"] != false, rid)
	}
	if e != nil {
		return nil, e
	}
	if e = audit(ctx, tx, u, cid, "Master saved", "Material master", rid, kind+": "+label); e != nil {
		return nil, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, e
	}
	return M{"id": rid}, nil
}
func (a *App) materialSupplierRate(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if u.Role != "MD" && u.Role != "Super Admin" {
		return nil, forbidden()
	}
	d, e := readBody(w, r)
	if e != nil {
		return nil, e
	}
	rating, reason := str(d, "rating"), str(d, "reason")
	if !slices.Contains([]string{"Not rated", "Good", "Acceptable", "Avoid", "Blocked", "Delisted"}, rating) || len(reason) > 2000 || (slices.Contains([]string{"Avoid", "Blocked", "Delisted"}, rating) && reason == "") {
		return nil, bad("Choose a rating and provide a reason for restrictions.")
	}
	ctx := r.Context()
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	sid := materialID(r)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, sid+824740000); e != nil {
		return nil, e
	}
	var exists bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM erp_material_suppliers() WHERE id=$1)`, sid).Scan(&exists); e != nil {
		return nil, e
	}
	if !exists {
		return nil, missing()
	}
	var version int64
	if e = tx.QueryRow(ctx, `SELECT COALESCE((SELECT version FROM supplier_assessments WHERE supplier_setup_id=$1),0)`, sid).Scan(&version); e != nil {
		return nil, e
	}
	if version != id(d, "version") {
		return nil, failure{409, "Supplier rating changed. Refresh first."}
	}
	_, e = tx.Exec(ctx, `INSERT INTO supplier_assessments(supplier_setup_id,rating,reason,updated_by) VALUES($1,$2,$3,$4) ON CONFLICT(supplier_setup_id) DO UPDATE SET rating=EXCLUDED.rating,reason=EXCLUDED.reason,updated_by=EXCLUDED.updated_by,updated_at=now(),version=supplier_assessments.version+1`, sid, rating, reason, u.ID)
	if e != nil {
		return nil, e
	}
	if e = accessAudit(ctx, tx, u.ID, "Supplier rating updated", "Supplier", sid, M{"rating": rating, "reason": reason}); e != nil {
		return nil, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, e
	}
	return M{"ok": true}, nil
}

// Serialize checks against changes to the supplier restriction.
func materialSupplierAvailable(ctx context.Context, q queryer, sid int64) error {
	if _, e := q.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, sid+824740000); e != nil {
		return e
	}
	var blocked bool
	if e := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM supplier_assessments WHERE supplier_setup_id=$1 AND rating IN ('Blocked','Delisted'))`, sid).Scan(&blocked); e != nil {
		return e
	}
	if blocked {
		return bad("Supplier is blocked or delisted. Request changes or contact MD.")
	}
	return nil
}

// Historical references remain exact, printable CODE128 values.
func normalizeOldETF(value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) > 80 {
		return "", bad("Old ETF must be at most 80 characters.")
	}
	for _, c := range value {
		if c < 32 || c > 126 {
			return "", bad("Old ETF must use printable English letters, numbers or punctuation.")
		}
	}
	return value, nil
}
