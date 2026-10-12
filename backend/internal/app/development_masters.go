package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"
)

var developmentTypes = []string{"Self development", "Buyer development", "Photo sample", "SMS", "Proto", "PP", "TOP"}
var developmentFields = []string{"style_name", "product", "season", "fit", "features", "waist", "pattern_reference", "development_type", "customer_reference", "request_email_reference", "request_notes", "charge_type", "charge_amount", "currency", "fabric_notes", "trim_notes", "remarks"}

func init() {
	for _, role := range []string{"Super Admin", "Marketing", "Development"} {
		roleGrants[role] = append(roleGrants[role], "development_master")
	}
	permissionLabels["development_master"] = "Create product-development masters and sample requests"
}
func developmentRead(u *User) bool {
	return materialCan(u, "material_view") && slices.Contains([]string{"Super Admin", "MD", "CEO", "Marketing", "Development", "Sampling"}, u.Role)
}
func developmentWrite(u *User) bool {
	return developmentRead(u) && (u.Role == "Super Admin" || u.Can("development_master"))
}

const developmentScope = `d.company_id=ANY($1) AND ($2<>'Marketing' OR (d.buyer_id IS NULL AND d.created_by=$3) OR EXISTS(SELECT 1 FROM buyers b WHERE b.id=d.buyer_id AND ($4='company' OR ($4='team' AND b.team_id=ANY($5))) AND (NOT $6 OR b.id=ANY($7))))`

func developmentArgs(u *User) []any {
	return []any{u.Companies, u.Role, u.ID, u.Scope, u.Teams, u.RestrictBuyers, u.BuyerIDs}
}
func (a *App) developmentRecord(ctx context.Context, q queryer, u *User, id int64, lock bool) (M, error) {
	sql := `SELECT d.* FROM development_masters d WHERE ` + developmentScope + ` AND d.id=$8`
	if lock {
		sql += ` FOR UPDATE OF d`
	}
	return row(ctx, q, sql, append(developmentArgs(u), id)...)
}
func (a *App) developmentList(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if !developmentRead(u) {
		return nil, forbidden()
	}
	ctx := r.Context()
	records, e := rows(ctx, a.DB, `SELECT d.* FROM development_masters d WHERE `+developmentScope+` ORDER BY d.id DESC LIMIT 5000`, developmentArgs(u)...)
	if e != nil {
		return nil, e
	}
	buyers, e := rows(ctx, a.DB, `SELECT id,name,company_id,team_id FROM buyers WHERE company_id=ANY($1) AND ($2<>'Marketing' OR $3='company' OR ($3='team' AND team_id=ANY($4))) AND (NOT $5 OR id=ANY($6)) ORDER BY name`, u.Companies, u.Role, u.Scope, u.Teams, u.RestrictBuyers, u.BuyerIDs)
	if e != nil {
		return nil, e
	}
	return M{"records": records, "buyers": buyers, "types": developmentTypes, "can_edit": developmentWrite(u), "can_approve": u.Role == "Super Admin" || (u.Role == "MD" && (u.Can("approve") || u.SetupCan("approve")))}, nil
}
func (a *App) developmentSave(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	d, e := readBody(w, r)
	if e != nil {
		return nil, e
	}
	action := str(d, "action")
	if action == "" {
		action = "save"
	}
	approval := action == "approve" || action == "return"
	if !developmentRead(u) {
		return nil, forbidden()
	}
	if approval {
		if u.Role != "Super Admin" && (u.Role != "MD" || (!u.Can("approve") && !u.SetupCan("approve"))) {
			return nil, forbidden()
		}
	} else if !developmentWrite(u) {
		return nil, forbidden()
	}
	ctx := r.Context()
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	rid := materialID(r)
	var old M
	if rid > 0 {
		old, e = a.developmentRecord(ctx, tx, u, rid, true)
		if e != nil {
			return nil, e
		}
		if id(old, "version") != id(d, "version") {
			return nil, failure{409, "Development record changed. Refresh before saving."}
		}
	}
	if approval {
		if old == nil || str(old, "status") != "Submitted" {
			return nil, bad("Only submitted requests can be reviewed.")
		}
		note := str(d, "note")
		if len(note) > 2000 || (action == "return" && note == "") {
			return nil, bad("Enter a review note of up to 2,000 characters.")
		}
		if action == "approve" {
			linked, err := rows(ctx, tx, `SELECT m.supplier_setup_id,m.active FROM material_samples m WHERE m.id=ANY(SELECT unnest(material_ids) FROM development_masters WHERE id=$1) ORDER BY m.supplier_setup_id FOR SHARE`, rid)
			if err != nil {
				return nil, err
			}
			for _, m := range linked {
				if m["active"] == false {
					return nil, bad("A linked material is archived. Request changes before approval.")
				}
				if err = materialSupplierAvailable(ctx, tx, id(m, "supplier_setup_id")); err != nil {
					return nil, err
				}
			}
		}
		status := "Approved"
		if action == "return" {
			status = "Changes requested"
		}
		_, e = tx.Exec(ctx, `UPDATE development_masters SET status=$1,review_note=$2,reviewed_by=$3,version=version+1,updated_at=now() WHERE id=$4`, status, note, u.ID, rid)
		if e != nil {
			return nil, e
		}
	} else {
		if !slices.Contains([]string{"save", "submit"}, action) {
			return nil, bad("Choose a valid action.")
		}
		if old != nil && slices.Contains([]string{"Approved", "Submitted"}, str(old, "status")) {
			return nil, bad("Submitted or approved development records are locked. Request changes before editing.")
		}
		cid := id(d, "company_id")
		if old != nil && cid != id(old, "company_id") {
			return nil, bad("The internal company cannot change.")
		}
		if e = a.company(ctx, tx, u, cid, true); e != nil {
			return nil, e
		}
		bid := id(d, "buyer_id")
		var buyer any
		if bid > 0 {
			b, e := row(ctx, tx, `SELECT id FROM buyers WHERE id=$1 AND company_id=$2 AND ($3<>'Marketing' OR $4='company' OR ($4='team' AND team_id=ANY($5))) AND (NOT $6 OR id=ANY($7))`, bid, cid, u.Role, u.Scope, u.Teams, u.RestrictBuyers, u.BuyerIDs)
			if e != nil {
				return nil, bad("Select a buyer within your assigned company and team.")
			}
			buyer = b["id"]
		}
		p := M{}
		raw := materialProfile(d["profile"])
		for _, key := range developmentFields {
			v := str(raw, key)
			if key == "charge_amount" && raw[key] != nil {
				v = fmt.Sprint(raw[key])
			}
			if len(v) > 2000 {
				return nil, bad("Development detail is too long.")
			}
			p[key] = v
		}
		if str(p, "style_name") == "" || !slices.Contains(developmentTypes, str(p, "development_type")) {
			return nil, bad("Enter a style name and choose a development type.")
		}
		if str(p, "development_type") != "Self development" && buyer == nil {
			return nil, bad("Buyer development and sample requests need an assigned buyer / brand.")
		}
		if !slices.Contains([]string{"Chargeable", "Free of charge"}, str(p, "charge_type")) {
			return nil, bad("Choose Chargeable or Free of charge.")
		}
		if !slices.Contains([]string{"USD", "INR", "AED", "EUR", "GBP", "CNY"}, str(p, "currency")) {
			return nil, bad("Choose a supported currency.")
		}
		if _, e = materialNumber(p["charge_amount"]); e != nil {
			return nil, e
		}
		mids := []int64{}
		if values, ok := d["material_ids"].([]any); ok {
			if len(values) > 100 {
				return nil, bad("Select at most 100 materials.")
			}
			for _, v := range values {
				mid := id(M{"id": v}, "id")
				if slices.Contains(mids, mid) {
					continue
				}
				m, e := a.materialSample(ctx, tx, u, mid, false)
				if e != nil || id(m, "company_id") != cid || m["active"] == false {
					return nil, bad("Choose active materials in this company.")
				}
				sid := id(m, "supplier_setup_id")
				if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, sid+824740000); e != nil {
					return nil, e
				}
				var blocked bool
				if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM supplier_assessments WHERE supplier_setup_id=$1 AND rating IN ('Blocked','Delisted'))`, sid).Scan(&blocked); e != nil {
					return nil, e
				}
				if blocked {
					return nil, bad("A selected material's supplier is blocked or delisted.")
				}
				mids = append(mids, mid)
			}
		}
		status := "Draft"
		if action == "submit" {
			if len(mids) == 0 {
				return nil, bad("Select materials before submitting.")
			}
			if str(p, "development_type") != "Self development" && strings.TrimSpace(str(p, "request_email_reference")) == "" {
				return nil, bad("Add the buyer request email reference before submitting.")
			}
			status = "Submitted"
		}
		b, _ := json.Marshal(p)
		if old == nil {
			e = tx.QueryRow(ctx, `SELECT nextval('development_masters_id_seq')`).Scan(&rid)
			if e != nil {
				return nil, e
			}
			_, e = tx.Exec(ctx, `INSERT INTO development_masters(id,company_id,buyer_id,code,profile,material_ids,status,created_by) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, rid, cid, buyer, fmt.Sprintf("EST-%07d", rid), b, mids, status, u.ID)
		} else {
			_, e = tx.Exec(ctx, `UPDATE development_masters SET buyer_id=$1,profile=$2,material_ids=$3,status=$4,version=version+1,updated_at=now() WHERE id=$5`, buyer, b, mids, status, rid)
		}
		if e != nil {
			return nil, e
		}
	}
	saved, e := a.developmentRecord(ctx, tx, u, rid, false)
	if e != nil {
		return nil, e
	}
	snapshot, _ := json.Marshal(saved)
	if _, e = tx.Exec(ctx, `INSERT INTO development_history(development_id,actor_id,action,snapshot) VALUES($1,$2,$3,$4)`, rid, u.ID, action, snapshot); e != nil {
		return nil, e
	}
	if e = audit(ctx, tx, u, id(saved, "company_id"), "Development "+action, "Development master", rid, str(saved, "code")); e != nil {
		return nil, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, e
	}
	return M{"id": rid, "code": saved["code"]}, nil
}
