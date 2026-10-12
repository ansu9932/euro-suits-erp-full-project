package app

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

func init() {
	permissionLabels["sample_indents"] = "Sample indents, MD review and sample production"
	for _, role := range []string{"Super Admin", "MD", "Marketing", "Sampling", "Development", "Quality", "Stores"} {
		roleGrants[role] = append(roleGrants[role], "sample_indents")
	}
}
func sampleRead(u *User) bool { return u.Can("sample_indents") }
func sampleWriter(u *User) bool {
	return sampleRead(u) && slices.Contains([]string{"Super Admin", "Marketing", "Sampling"}, u.Role)
}
func sampleProduction(u *User) bool {
	return sampleRead(u) && (u.Role == "Super Admin" || u.Role == "Sampling" && u.Can("sample"))
}
func sampleApprove(u *User) bool {
	return sampleRead(u) && u.Role == "MD" && (u.Can("approve") || u.SetupCan("approve"))
}

var sampleFields = strings.Fields(`development_id booking_reference fabric_choice brand request_date buyer_requested_date buyer_required_date plan_date email_date email_subject email_message sample_type est buyer_style buyer_spec product description fit factory season year buyer_po order_id pattern_status material_id length_metres alternate_material_id alternate_reference fabric_in_date fabric_expected_date trims_expected_date copy_sample_id copy_purpose cad_reference spec_reference spec_date spec_signed special_remarks finishing_remarks embroidery washing remake_indent_id`)
var sampleProductionFields = strings.Fields(`pattern_by cut_by sample_maker pattern_plan_date allocation_plan_date sewing_plan_date fabric_update_date trims_update_date style_update_date readiness_note audit_remarks handover_dc enclosures fabric_expected_date trims_expected_date cutting_plan_date finishing_plan_date audit_plan_date handover_plan_date dispatch_to dispatch_from dispatch_contact awb handover_destination`)
var sampleDefaults = M{"pattern_status": []string{"Existing", "Modified — 30 minutes", "Modified — 1 hour", "Modified — 2 hours", "New"}, "fit": []string{"Slim", "Regular", "Comfort"}, "sample_type": []string{"Proto", "Fit", "PP", "SMS", "Photo", "TOP"}, "factory": []string{"ET Dubai", "ESSS", "Line 1", "Line 2", "Suit", "Line 3", "Line 4"}, "season": []string{"S/S", "Summer", "Autumn", "Winter", "Spring", "AW"}, "product": []string{"Trousers", "Suits", "Shirts"}}

func sampleStrings(raw any, keys []string) (M, error) {
	p := materialProfile(raw)
	out := M{}
	for _, k := range keys {
		v := str(p, k)
		limit := 2000
		if k == "email_message" {
			limit = 20000
		}
		if len(v) > limit {
			return nil, bad("A sample field is too long.")
		}
		if strings.HasSuffix(k, "date") && v != "" {
			if _, e := time.Parse("2006-01-02", v); e != nil {
				return nil, bad("Enter a valid date.")
			}
		}
		out[k] = v
	}
	return out, nil
}
func sampleVariants(raw any) ([]M, error) {
	list, ok := raw.([]any)
	if !ok || len(list) == 0 || len(list) > 100 {
		return nil, bad("Add size, colour and quantity rows.")
	}
	out := []M{}
	total := 0
	for _, v := range list {
		m := materialProfile(v)
		qty := id(m, "quantity")
		if fmt.Sprint(m["quantity"]) != fmt.Sprint(qty) || qty < 1 || qty > 100 {
			return nil, bad("Quantity must be a whole number from 1 to 100.")
		}
		size, colour := str(m, "size"), str(m, "colour")
		if size == "" || colour == "" || len(size) > 80 || len(colour) > 150 {
			return nil, bad("Enter a size and colour for every row.")
		}
		total += int(qty)
		out = append(out, M{"size": size, "colour": colour, "quantity": qty})
	}
	if total > 200 {
		return nil, bad("A request can contain up to 200 pieces.")
	}
	return out, nil
}
func sampleBOM(raw any) ([]M, error) {
	list, ok := raw.([]any)
	if !ok || len(list) > 200 {
		return nil, bad("Provide up to 200 BOM components.")
	}
	out := []M{}
	for _, v := range list {
		p, e := sampleStrings(v, strings.Fields("group component reference quantity unit colour size ticket thread_position shade placement remarks"))
		if e != nil {
			return nil, e
		}
		if str(p, "component") == "" {
			continue
		}
		if !slices.Contains([]string{"Pocketing", "Fusing", "Sewing trims", "Labels", "Finishing", "Value additions"}, str(p, "group")) {
			return nil, bad("Choose a valid BOM group.")
		}
		if str(p, "quantity") != "" {
			n, e := strconv.ParseFloat(str(p, "quantity"), 64)
			if e != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n > 100000 {
				return nil, bad("Enter a valid BOM quantity.")
			}
		}
		out = append(out, p)
	}
	return out, nil
}

const sampleScope = developmentScope + ` AND (NOT $6 OR d.buyer_id=ANY($7))`

func (a *App) sampleRecord(ctx context.Context, q queryer, u *User, rid int64, lock bool) (M, error) {
	sql := `SELECT d.*,(SELECT name FROM users WHERE id=d.created_by) AS merchant,(SELECT name FROM buyers WHERE id=d.buyer_id) AS buyer_name FROM sample_requests d WHERE ` + sampleScope + ` AND d.id=$8`
	if lock {
		sql += ` FOR UPDATE OF d`
	}
	return row(ctx, q, sql, append(developmentArgs(u), rid)...)
}
func (a *App) sampleHistory(ctx context.Context, q queryer, u *User, rid int64, action string, snapshot any) error {
	b, e := json.Marshal(snapshot)
	if e != nil {
		return e
	}
	_, e = q.Exec(ctx, `INSERT INTO sample_request_history(request_id,actor_id,action,snapshot) VALUES($1,$2,$3,$4)`, rid, u.ID, action, b)
	return e
}
func (a *App) sampleList(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if !sampleRead(u) {
		return nil, forbidden()
	}
	ctx := r.Context()
	records, e := rows(ctx, a.DB, `SELECT d.*,b.name AS buyer,(SELECT count(*) FROM sample_indents i WHERE i.request_id=d.id) AS pieces FROM sample_requests d JOIN buyers b ON b.id=d.buyer_id WHERE `+sampleScope+` AND ($8 OR d.status='Approved' OR d.created_by=$9) ORDER BY d.id DESC LIMIT 2000`, append(developmentArgs(u), u.Role == "Marketing" || u.Role == "Super Admin" || sampleApprove(u), u.ID)...)
	if e != nil {
		return nil, e
	}
	buyers, e := rows(ctx, a.DB, `SELECT id,name,company_id FROM buyers WHERE company_id=ANY($1) AND ($2<>'Marketing' OR $3='company' OR ($3='team' AND team_id=ANY($4))) AND (NOT $5 OR id=ANY($6)) ORDER BY name`, u.Companies, u.Role, u.Scope, u.Teams, u.RestrictBuyers, u.BuyerIDs)
	if e != nil {
		return nil, e
	}
	materials, e := rows(ctx, a.DB, `SELECT id,company_id,code FROM material_samples WHERE company_id=ANY($1) AND kind='fabric' AND active ORDER BY code`, u.Companies)
	if e != nil {
		return nil, e
	}
	options, e := rows(ctx, a.DB, `SELECT * FROM sample_options WHERE company_id=ANY($1) ORDER BY label`, u.Companies)
	if e != nil {
		return nil, e
	}
	stock, e := rows(ctx, a.DB, `SELECT s.*,m.code FROM sample_stock s JOIN material_samples m ON m.id=s.material_id WHERE s.company_id=ANY($1)`, u.Companies)
	if e != nil {
		return nil, e
	}
	styles, e := rows(ctx, a.DB, `SELECT d.id,d.company_id,d.buyer_id,d.code,d.profile,d.material_ids FROM development_masters d WHERE `+sampleScope+` AND d.status='Approved' ORDER BY d.id DESC LIMIT 2000`, developmentArgs(u)...)
	if e != nil {
		return nil, e
	}
	return M{"styles": styles, "records": records, "buyers": buyers, "materials": materials, "options": options, "defaults": sampleDefaults, "stock": stock, "can_edit": sampleWriter(u), "can_approve": sampleApprove(u), "can_produce": sampleProduction(u), "can_pattern": sampleProduction(u) || u.Role == "Development" && u.Can("develop"), "can_audit": u.Role == "Quality" && u.Can("quality"), "can_feedback": u.Role == "Marketing", "can_handover": sampleProduction(u) || u.Role == "Marketing", "can_stock": sampleProduction(u) || u.Role == "Stores" && u.Can("inventory")}, nil
}
func (a *App) sampleDetail(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if !sampleRead(u) {
		return nil, forbidden()
	}
	ctx := r.Context()
	d, e := a.sampleRecord(ctx, a.DB, u, materialID(r), false)
	if e != nil {
		return nil, e
	}
	if str(d, "status") != "Approved" && !(u.Role == "Marketing" || u.Role == "Super Admin" || sampleApprove(u) || id(d, "created_by") == u.ID) {
		return nil, forbidden()
	}
	pieces, e := rows(ctx, a.DB, `SELECT * FROM sample_indents WHERE request_id=$1 ORDER BY id`, id(d, "id"))
	if e != nil {
		return nil, e
	}
	files, e := rows(ctx, a.DB, `SELECT id,name,category,indent_id,created_at FROM sample_request_files WHERE request_id=$1 ORDER BY id`, id(d, "id"))
	if e != nil {
		return nil, e
	}
	history, e := rows(ctx, a.DB, `SELECT h.id,h.action,h.created_at,u.name AS actor FROM sample_request_history h JOIN users u ON u.id=h.actor_id WHERE request_id=$1 ORDER BY h.id DESC`, id(d, "id"))
	if e != nil {
		return nil, e
	}
	moves, e := rows(ctx, a.DB, `SELECT m.*,s.code FROM sample_stock_moves m JOIN material_samples s ON s.id=m.material_id JOIN sample_indents i ON i.id=m.indent_id WHERE i.request_id=$1 ORDER BY m.id`, id(d, "id"))
	if e != nil {
		return nil, e
	}
	return M{"record": d, "pieces": pieces, "files": files, "history": history, "issues": moves}, nil
}
func (a *App) sampleSave(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if !sampleRead(u) {
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
		old, e = a.sampleRecord(ctx, tx, u, rid, true)
		if e != nil {
			return nil, e
		}
		if u.Role == "Sampling" && str(old, "status") != "Approved" && id(old, "created_by") != u.ID {
			return nil, forbidden()
		}
		if id(d, "version") != id(old, "version") {
			return nil, failure{409, "This request changed. Refresh before saving."}
		}
	}
	if action == "approve" || action == "return" {
		if !sampleApprove(u) {
			return nil, forbidden()
		}
		if old == nil || str(old, "status") != "Submitted" {
			return nil, bad("Only submitted requests can be reviewed.")
		}
		note := str(d, "note")
		if len(note) > 2000 || action == "return" && note == "" {
			return nil, bad("Enter a review note (up to 2,000 characters).")
		}
		status := "Approved"
		if action == "return" {
			status = "Changes requested"
		}
		if action == "approve" {
			vs, e := sampleVariants(old["variants"])
			if e != nil {
				return nil, e
			}
			initialStatus := "Ready"
			initialProduction := []byte("{}")
			profile := materialProfile(old["profile"])
			if str(profile, "pattern_status") == "Existing" && str(profile, "cad_reference") != "" {
				initialStatus = "Pattern ready"
				initialProduction, _ = json.Marshal(M{"pattern_actual": time.Now().UTC().Format(time.RFC3339), "pattern_completed_by": "Existing pattern confirmed in approved request", "cad_reference": str(profile, "cad_reference")})
			}
			for _, v := range vs {
				for n := int64(0); n < id(v, "quantity"); n++ {
					if _, e = tx.Exec(ctx, `INSERT INTO sample_indents(request_id,size,colour,status,production) VALUES($1,$2,$3,$4,$5)`, rid, str(v, "size"), str(v, "colour"), initialStatus, initialProduction); e != nil {
						return nil, e
					}
				}
			}
		}
		_, e = tx.Exec(ctx, `UPDATE sample_requests SET status=$1,review_note=$2,reviewed_by=$3,version=version+1,updated_at=now() WHERE id=$4`, status, note, u.ID, rid)
	} else {
		if !sampleWriter(u) {
			return nil, forbidden()
		}
		if action != "save" && action != "submit" {
			return nil, bad("Invalid sample action.")
		}
		if old != nil && !slices.Contains([]string{"Draft", "Changes requested"}, str(old, "status")) {
			return nil, bad("Submitted requests are locked for MD review.")
		}
		cid, bid := id(d, "company_id"), id(d, "buyer_id")
		if old != nil && cid != id(old, "company_id") {
			return nil, bad("Company cannot change.")
		}
		if e = a.company(ctx, tx, u, cid, true); e != nil {
			return nil, e
		}
		_, e = row(ctx, tx, `SELECT id FROM buyers WHERE id=$1 AND company_id=$2 AND ($3<>'Marketing' OR $4='company' OR ($4='team' AND team_id=ANY($5))) AND (NOT $6 OR id=ANY($7))`, bid, cid, u.Role, u.Scope, u.Teams, u.RestrictBuyers, u.BuyerIDs)
		if e != nil {
			return nil, bad("Choose an assigned buyer in this company.")
		}
		p, e := sampleStrings(d["profile"], sampleFields)
		if e != nil {
			return nil, e
		}

		if old == nil {
			p["request_date"] = time.Now().In(time.FixedZone("IST", 19800)).Format("2006-01-02")
		} else {
			p["request_date"] = str(materialProfile(old["profile"]), "request_date")
		}
		if value := str(p, "development_id"); value != "" {
			sid, err := strconv.ParseInt(value, 10, 64)
			if err != nil {
				return nil, bad("Choose an approved EST master.")
			}
			style, err := row(ctx, tx, `SELECT d.* FROM development_masters d WHERE `+sampleScope+` AND d.id=$8 AND d.status='Approved'`, append(developmentArgs(u), sid)...)
			if err != nil || id(style, "company_id") != cid || (id(style, "buyer_id") != 0 && id(style, "buyer_id") != bid) {
				return nil, bad("Choose an approved EST master for this buyer and company.")
			}
			p["est"] = strings.TrimPrefix(str(style, "code"), "EST-")
			sp := materialProfile(style["profile"])
			for _, key := range []string{"product", "fit"} {
				if str(sp, key) != "" {
					p[key] = sp[key]
				}
			}
			if str(sp, "pattern_reference") != "" {
				p["cad_reference"] = sp["pattern_reference"]
			}
		}
		if str(p, "fabric_choice") != "" && !slices.Contains([]string{"Actual", "Alternate"}, str(p, "fabric_choice")) {
			return nil, bad("Choose actual or alternate fabric.")
		}
		if v := str(p, "est"); v != "" && !regexp.MustCompile(`^[0-9]{1,30}$`).MatchString(v) {
			return nil, bad("EST must contain numbers only.")
		}
		for _, key := range []string{"material_id", "alternate_material_id"} {
			if str(p, key) != "" {
				mid, err := strconv.ParseInt(str(p, key), 10, 64)
				if err != nil {
					return nil, bad("Choose a fabric reference.")
				}
				m, err := row(ctx, tx, `SELECT id FROM material_samples WHERE id=$1 AND company_id=$2 AND kind='fabric' AND active`, mid, cid)
				if err != nil || m == nil {
					return nil, bad("Choose an active fabric in this company.")
				}
			}
		}
		for _, key := range []string{"copy_sample_id", "remake_indent_id"} {
			if str(p, key) != "" {
				iid, err := strconv.ParseInt(str(p, key), 10, 64)
				if err != nil {
					return nil, bad("Enter a valid existing indent number.")
				}
				parent, err := row(ctx, tx, `SELECT request_id FROM sample_indents WHERE id=$1`, iid)
				if err != nil {
					return nil, bad("Original indent is unavailable.")
				}
				original, err := a.sampleRecord(ctx, tx, u, id(parent, "request_id"), false)
				if err != nil || id(original, "company_id") != cid {
					return nil, bad("Original indent is outside this company or your access.")
				}
			}
		}
		if v := str(p, "length_metres"); v != "" {
			n, err := strconv.ParseFloat(v, 64)
			if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n > 100000 {
				return nil, bad("Enter a valid fabric length.")
			}
		}
		if v := str(p, "order_id"); v != "" {
			oid, err := strconv.ParseInt(v, 10, 64)
			if err != nil {
				return nil, bad("Choose an existing buyer order.")
			}
			order, err := a.order(ctx, tx, u, oid, false)
			if err != nil || id(order, "company_id") != cid || id(order, "buyer_id") != bid {
				return nil, bad("Select an accessible order for this company and buyer.")
			}
			p["buyer_po"] = str(order, "po")
		}
		vs, e := sampleVariants(d["variants"])
		if e != nil {
			return nil, e
		}
		bom, e := sampleBOM(d["bom"])
		if e != nil {
			return nil, e
		}
		status := "Draft"
		if action == "submit" {
			for _, key := range []string{"buyer_style", "sample_type", "est", "plan_date", "buyer_required_date", "material_id"} {
				if str(p, key) == "" {
					return nil, bad("Complete buyer style, sample type, EST, fabric, plan date and buyer required date before submitting.")
				}
			}
			status = "Submitted"
		}
		pb, _ := json.Marshal(p)
		vb, _ := json.Marshal(vs)
		bb, _ := json.Marshal(bom)
		if old == nil {
			e = tx.QueryRow(ctx, `INSERT INTO sample_requests(company_id,buyer_id,created_by,profile,variants,bom,status) VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`, cid, bid, u.ID, pb, vb, bb, status).Scan(&rid)
		} else {
			_, e = tx.Exec(ctx, `UPDATE sample_requests SET buyer_id=$1,profile=$2,variants=$3,bom=$4,status=$5,version=version+1,updated_at=now() WHERE id=$6`, bid, pb, vb, bb, status, rid)
		}
	}
	if e != nil {
		return nil, e
	}
	saved, e := a.sampleRecord(ctx, tx, u, rid, false)
	if e != nil {
		return nil, e
	}
	if e = a.sampleHistory(ctx, tx, u, rid, action, saved); e != nil {
		return nil, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, e
	}
	return saved, nil
}
func (a *App) samplePiece(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if !sampleRead(u) {
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
	piece, e := row(ctx, tx, `SELECT * FROM sample_indents WHERE id=$1 FOR UPDATE`, materialID(r))
	if e != nil {
		return nil, e
	}
	parent, e := a.sampleRecord(ctx, tx, u, id(piece, "request_id"), false)
	if e != nil {
		return nil, e
	}
	if str(parent, "status") != "Approved" {
		return nil, bad("MD approval is required.")
	}
	if id(d, "version") != id(piece, "version") {
		return nil, failure{409, "This indent changed. Refresh first."}
	}
	p := materialProfile(piece["production"])
	action, status := str(d, "action"), str(piece, "status")
	next := status
	now := time.Now().UTC().Format(time.RFC3339)
	next, e = sampleAdvance(u, status, action, d, p, now)
	if e != nil {
		return nil, e
	}

	b, _ := json.Marshal(p)
	_, e = tx.Exec(ctx, `UPDATE sample_indents SET status=$1,production=$2,version=version+1 WHERE id=$3`, next, b, id(piece, "id"))
	if e != nil {
		return nil, e
	}
	if e = a.sampleHistory(ctx, tx, u, id(parent, "id"), fmt.Sprintf("Indent %d: %s", id(piece, "id"), action), M{"production": p, "status": next}); e != nil {
		return nil, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, e
	}
	return M{"ok": true}, nil
}
func (a *App) sampleOption(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if !sampleWriter(u) {
		return nil, forbidden()
	}
	d, e := readBody(w, r)
	if e != nil {
		return nil, e
	}
	kind, label := str(d, "kind"), str(d, "label")
	if _, ok := sampleDefaults[kind]; !ok || label == "" || len(label) > 100 {
		return nil, bad("Choose a sample option kind and a label up to 100 characters.")
	}
	cid := id(d, "company_id")
	if e = a.company(r.Context(), a.DB, u, cid, true); e != nil {
		return nil, e
	}
	_, e = a.DB.Exec(r.Context(), `INSERT INTO sample_options(company_id,kind,label) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, cid, kind, label)
	return M{"ok": true}, e
}
func (a *App) sampleStock(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if !sampleRead(u) || !(sampleProduction(u) || u.Role == "Stores" && u.Can("inventory")) {
		return nil, forbidden()
	}
	d, e := readBody(w, r)
	if e != nil {
		return nil, e
	}
	qty, e := strconv.ParseFloat(fmt.Sprint(d["metres"]), 64)
	if e != nil || qty <= 0 || qty > 100000 || !regexp.MustCompile(`^\d+(\.\d{1,3})?$`).MatchString(fmt.Sprint(d["metres"])) {
		return nil, bad("Enter positive metres with up to three decimal places.")
	}
	ctx := r.Context()
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	mid, cid := id(d, "material_id"), id(d, "company_id")
	if e = a.company(ctx, tx, u, cid, true); e != nil {
		return nil, e
	}
	if _, e = row(ctx, tx, `SELECT id FROM material_samples WHERE id=$1 AND company_id=$2 AND kind='fabric' AND active`, mid, cid); e != nil {
		return nil, bad("Choose an active company fabric.")
	}
	if _, e = tx.Exec(ctx, `INSERT INTO sample_stock(material_id,company_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, mid, cid); e != nil {
		return nil, e
	}
	stock, e := row(ctx, tx, `SELECT material_id,metres::text AS metres FROM sample_stock WHERE material_id=$1 FOR UPDATE`, mid)
	if e != nil {
		return nil, e
	}
	var iid any
	ref := str(d, "reference")
	if id(d, "indent_id") > 0 {
		piece, err := row(ctx, tx, `SELECT * FROM sample_indents WHERE id=$1`, id(d, "indent_id"))
		if err != nil {
			return nil, bad("Indent unavailable.")
		}
		parent, err := a.sampleRecord(ctx, tx, u, id(piece, "request_id"), false)
		if err != nil || id(parent, "company_id") != cid || str(parent, "status") != "Approved" {
			return nil, forbidden()
		}
		if slices.Contains([]string{"Handed over", "Rejected"}, str(piece, "status")) {
			return nil, bad("This indent is closed.")
		}
		balance, _ := strconv.ParseFloat(fmt.Sprint(stock["metres"]), 64)
		if balance < qty {
			return nil, bad("Insufficient sample fabric balance.")
		}
		qty = -qty
		iid = id(piece, "id")
		ref = "Sample issue"
	} else if ref == "" || len(ref) > 200 {
		return nil, bad("Enter a receipt reference (up to 200 characters).")
	}
	var move int64
	e = tx.QueryRow(ctx, `INSERT INTO sample_stock_moves(material_id,company_id,indent_id,metres,reference,actor_id) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, mid, cid, iid, qty, ref, u.ID).Scan(&move)
	if e != nil {
		return nil, e
	}
	_, e = tx.Exec(ctx, `UPDATE sample_stock SET metres=metres+$1 WHERE material_id=$2`, qty, mid)
	if e != nil {
		return nil, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, e
	}
	return M{"id": move, "dc": fmt.Sprintf("SDC-%07d", move)}, nil
}
