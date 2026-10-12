package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

var exportDefaults = map[string][]string{
	"Super Admin":  {"export_view", "export_edit", "export_files", "export_download", "export_finance_check", "export_audit_check", "export_discount_final", "export_approve"},
	"Finance Head": {"export_view", "export_edit", "export_files", "export_download", "export_finance_check", "export_discount_first"},
	"Finance":      {"export_view", "export_edit", "export_files", "export_download"},
	"MD":           {"export_view", "export_download", "export_discount_final", "export_approve"},
}

func init() {
	for role, caps := range exportDefaults {
		roleGrants[role] = append(roleGrants[role], caps...)
	}
	for key, label := range map[string]string{"export_view": "View export documentation", "export_edit": "Create and edit export invoices and receipts", "export_files": "Attach export documents", "export_download": "Download export documents and statements", "export_finance_check": "Finance review of export documents", "export_audit_check": "Audit review of export documents (explicit assignment)", "export_discount_first": "First discount approval (Finance Head)", "export_discount_final": "Final discount approval (MD / Super Admin)"} {
		permissionLabels[key] = label
	}
	permissionLabels["export_approve"] = "Approve export invoices (MD / Super Admin)"
}

// An auditor is an explicitly assigned capability, not a broader default role.
func rolePermissionCapabilities(role string) []string {
	out := append([]string{}, roleGrants[role]...)
	if slices.Contains([]string{"Finance", "Finance Head"}, role) {
		out = append(out, "export_audit_check")
	}
	return out
}
func permissionCapabilityMap() map[string][]string {
	out := map[string][]string{}
	for role := range roleGrants {
		out[role] = rolePermissionCapabilities(role)
	}
	return out
}
func exportExplicitAudit(u *User) bool {
	if !slices.Contains([]string{"Finance", "Finance Head"}, u.Role) {
		return false
	}
	if u.DesignationID != 0 && !slices.Contains(u.DesignationPermissions, "export_audit_check") {
		return false
	}
	if u.InheritDesignation {
		return slices.Contains(u.DesignationPermissions, "export_audit_check")
	}
	if u.CustomPermissions {
		return slices.Contains(u.Permissions, "export_audit_check")
	}
	return u.DesignationID != 0 && slices.Contains(u.DesignationPermissions, "export_audit_check")
}
func needsOrderVisibility(perms []string) bool {
	for _, p := range perms {
		if !strings.HasPrefix(p, "export_") {
			return true
		}
	}
	return false
}
func exportRead(u *User) bool { return u.Can("export_view") }
func exportCan(u *User, key string) bool {
	if !exportRead(u) || !u.Can("export_"+key) {
		return false
	}
	if key == "discount_first" {
		return u.Role == "Finance Head"
	}
	if key == "discount_final" || key == "approve" {
		return slices.Contains([]string{"MD", "Super Admin"}, u.Role)
	}
	return true
}
func exportPermissions(u *User) M {
	out := M{}
	for _, k := range strings.Fields("view edit files download finance_check audit_check discount_first discount_final approve") {
		out[k] = exportCan(u, k)
	}
	out["requires_order"] = u.Role != "Super Admin" && !slices.Contains([]string{"company", "team"}, u.Scope)
	out["standalone"] = exportStandalone(u)
	return out
}

// The explicit SQL boundary also protects non-production owner connections.
const exportCompanyScope = `($4::bigint[] IS NOT NULL OR $4 IS NULL) AND ($5::bigint[] IS NOT NULL OR $5 IS NULL) AND ($6::text[] IS NOT NULL OR $6 IS NULL) AND ($10::bigint[] IS NOT NULL OR $10 IS NULL) AND (d.party_setup_id IS NULL OR EXISTS(SELECT 1 FROM company_setups s WHERE s.id=d.party_setup_id AND s.category='Buyer' AND s.status='Approved' AND s.deleted_at IS NULL AND ($7='Super Admin' OR ($11::boolean AND ($7='MD' OR s.assigned_to=$12))))) AND d.company_id=ANY($1::bigint[]) AND ($7='Super Admin' OR ((NOT $8::boolean OR d.buyer_id=ANY($9::bigint[])) AND (($2='company') OR ($2='team' AND EXISTS(SELECT 1 FROM buyers b WHERE ($4::bigint[] IS NOT NULL OR $4 IS NULL) AND ($5::bigint[] IS NOT NULL OR $5 IS NULL) AND ($6::text[] IS NOT NULL OR $6 IS NULL) AND ($10::bigint[] IS NOT NULL OR $10 IS NULL) AND b.id=d.buyer_id AND b.team_id=ANY($3::bigint[]))) OR ($2 NOT IN ('company','team') AND d.order_id IN (SELECT o.id FROM erp_export_orders(d.company_id) o WHERE o.buyer_id=d.buyer_id)))))`
const exportScope = `((` + exportCompanyScope + `) OR (d.company_id IS NULL AND d.party_type='manual' AND $7 IN ('Super Admin','MD','Finance','Finance Head')))`

func exportArgs(u *User) []any { return append(scopeArgs(u), u.SetupCan("view"), u.ID) }

const exportSelect = `SELECT d.*,d.invoice_amount::text AS invoice_amount,(SELECT name FROM users WHERE id=d.submitted_by) AS submitted_by_name,(SELECT name FROM users WHERE id=d.reviewed_by) AS reviewed_by_name,COALESCE((SELECT sum(x.amount) FROM export_discounts x WHERE x.invoice_id=d.id AND x.status='Approved'),0)::text AS approved_discount FROM export_invoices d WHERE `

func (a *App) exportRecord(ctx context.Context, q queryer, u *User, rid int64, lock bool) (M, error) {
	if !exportRead(u) {
		return nil, forbidden()
	}
	sql := exportSelect + exportScope + ` AND d.id=$13`
	if lock {
		sql += ` FOR UPDATE OF d`
	}
	m, e := row(ctx, q, sql, append(exportArgs(u), rid)...)
	if e == nil {
		if exportRecordKind(m) != "jv" {
			m["balance"] = exportBalance(m)
			m["bill_amount"] = m["invoice_amount"]
		}
	}
	return m, e
}
func (a *App) exportHistory(ctx context.Context, q queryer, u *User, rid int64, action string) error {
	m, e := a.exportRecord(ctx, q, u, rid, false)
	if e != nil {
		return e
	}
	discounts, e := rows(ctx, q, `SELECT id,amount::text,currency,reason,status,requested_by,first_by,first_at,final_by,final_at,review_note,created_at FROM export_discounts WHERE invoice_id=$1 ORDER BY id`, rid)
	if e != nil {
		return e
	}
	files, e := rows(ctx, q, `SELECT id,kind,filename,mime,size,sha256,created_by,created_at,receipt_key::text AS receipt_key,field_key FROM export_files f WHERE invoice_id=$1 AND NOT EXISTS(SELECT 1 FROM export_file_removals x WHERE x.file_id=f.id) ORDER BY id`, rid)
	if e != nil {
		return e
	}
	exportLabelReceiptFiles(m, files)
	m["discounts"], m["files"] = discounts, files
	removedFiles, e := rows(ctx, q, `SELECT f.id,f.kind,f.filename,f.mime,f.size,f.sha256,f.created_by,f.created_at,f.receipt_key::text AS receipt_key,f.field_key,x.removed_by,x.removed_at FROM export_files f JOIN export_file_removals x ON x.file_id=f.id WHERE f.invoice_id=$1 ORDER BY f.id`, rid)
	if e != nil {
		return e
	}
	exportLabelReceiptFiles(m, removedFiles)
	m["removed_files"] = removedFiles
	comments, e := exportComments(ctx, q, rid)
	if e != nil {
		return e
	}
	m["comments"] = comments
	raw, e := json.Marshal(m)
	if e != nil {
		return e
	}
	if _, e = q.Exec(ctx, `INSERT INTO export_history(invoice_id,actor_id,action,snapshot) VALUES($1,$2,$3,$4)`, rid, u.ID, action, raw); e != nil {
		return e
	}
	entity := "Export invoice"
	if exportRecordKind(m) == "jv" {
		entity = "JV entry"
	}
	if id(m, "company_id") == 0 {
		_, e = q.Exec(ctx, `INSERT INTO audit(company_id,actor_id,action,entity,entity_id,detail) VALUES(NULL,$1,$2,$3,$4,$5)`, u.ID, action, entity, rid, str(m, "invoice_number"))
		return e
	}
	return audit(ctx, q, u, id(m, "company_id"), action, entity, rid, str(m, "invoice_number"))
}
func (a *App) exportOptions(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if !exportRead(u) {
		return nil, forbidden()
	}
	kind, e := exportKind(r.URL.Query().Get("record_kind"))
	if e != nil {
		return nil, e
	}
	if kind == "jv" && r.URL.Query().Get("company_id") != "" {
		return nil, bad("JV entries use the standalone register.")
	}
	ctx := r.Context()
	companies, e := rows(ctx, a.DB, `SELECT id,name,status FROM companies WHERE id=ANY($1) ORDER BY name`, u.Companies)
	if e != nil {
		return nil, e
	}
	cid, _ := strconv.ParseInt(r.URL.Query().Get("company_id"), 10, 64)
	parties, orders := []M{}, []M{}
	if cid > 0 {
		if e = a.company(ctx, a.DB, u, cid, false); e != nil {
			return nil, e
		}
		parties, e = rows(ctx, a.DB, `SELECT b.id,b.name,b.company_id,'buyer' AS party_type FROM buyers b WHERE ($4::bigint[] IS NOT NULL OR $4 IS NULL) AND ($5::bigint[] IS NOT NULL OR $5 IS NULL) AND ($6::text[] IS NOT NULL OR $6 IS NULL) AND ($10::bigint[] IS NOT NULL OR $10 IS NULL) AND b.company_id=$11 AND b.company_id=ANY($1) AND ($7='Super Admin' OR (NOT $8 OR b.id=ANY($9)) AND ($2='company' OR ($2='team' AND b.team_id=ANY($3)) OR ($2 NOT IN ('company','team') AND b.id IN (SELECT o.buyer_id FROM erp_export_orders(b.company_id) o)))) ORDER BY b.name`, append(scopeArgs(u), cid)...)
		if e != nil {
			return nil, e
		}
		if (u.Role == "Super Admin" || u.SetupCan("view")) && (u.Role == "Super Admin" || (u.Scope == "company" && !u.RestrictBuyers)) {
			masters, err := rows(ctx, a.DB, `SELECT id,name,'company_setup' AS party_type,$1::bigint AS company_id FROM erp_export_buyers($1) ORDER BY name`, cid)
			if err != nil {
				return nil, err
			}
			parties = append(parties, masters...)
		}
		orders, e = rows(ctx, a.DB, `SELECT * FROM erp_export_orders($1) ORDER BY id DESC LIMIT 2000`, cid)
		if e != nil {
			return nil, e
		}
	}
	standaloneParties, e := exportPartyOptions(ctx, a.DB, u)
	if e != nil {
		return nil, e
	}
	return M{"record_kind": kind, "companies": companies, "parties": parties, "standalone_parties": standaloneParties, "orders": orders, "currencies": exportCurrencies, "permissions": exportJVPermissions(u, M{"record_kind": kind}), "document_kinds": exportDocumentKindsFor(kind)}, nil
}
func (a *App) exportRecords(r *http.Request) ([]M, error) {
	u := current(r)
	if !exportRead(u) {
		return nil, forbidden()
	}
	query := r.URL.Query()
	kind, e := exportKind(query.Get("record_kind"))
	if e != nil {
		return nil, e
	}
	parseFilter := func(key string) (int64, error) {
		value := query.Get(key)
		if value == "" {
			return 0, nil
		}
		n, e := strconv.ParseInt(value, 10, 64)
		if e != nil || n < 1 {
			return 0, bad("Invalid export filter.")
		}
		return n, nil
	}
	cid, e := parseFilter("company_id")
	if e != nil {
		return nil, e
	}
	pid, e := parseFilter("party_id")
	if e != nil {
		return nil, e
	}
	typ := query.Get("party_type")
	if pid > 0 && !slices.Contains([]string{"buyer", "company_setup"}, typ) {
		return nil, bad("Choose a party type for the statement.")
	}
	partyName := strings.TrimSpace(query.Get("party_name"))
	if partyName != "" && (typ != "manual" || len(partyName) > 160 || strings.ContainsAny(partyName, "\r\n\t\x00")) {
		return nil, bad("Use a standalone customer name up to 160 characters.")
	}
	if typ != "" && !slices.Contains([]string{"buyer", "company_setup", "manual"}, typ) {
		return nil, bad("Choose a valid invoice party type.")
	}
	invoiceScope := query.Get("scope")
	if !slices.Contains([]string{"", "standalone", "company"}, invoiceScope) {
		return nil, bad("Choose all, standalone or company-linked invoices.")
	}
	status := query.Get("status")
	if !slices.Contains([]string{"", "Draft", "Submitted", "Approved", "Changes requested"}, status) {
		return nil, bad("Choose a valid invoice review status.")
	}
	cur := query.Get("currency")
	if cur != "" && !exportCurrency(cur) {
		return nil, bad("Choose a supported currency filter.")
	}
	search := strings.TrimSpace(query.Get("search"))
	if len(search) > 160 {
		return nil, bad("Use a search up to 160 characters.")
	}
	from, to := "", ""
	if fy := query.Get("fy"); fy != "" {
		parts := strings.Split(fy, "-")
		year, err := strconv.Atoi(parts[0])
		if len(parts) != 2 || err != nil || year < 1900 || year > 2200 || len(parts[0]) != 4 || parts[1] != fmt.Sprintf("%02d", (year+1)%100) {
			return nil, bad("Use an accounting year such as 2026-27.")
		}
		from = fmt.Sprintf("%04d-04-01", year)
		to = fmt.Sprintf("%04d-04-01", year+1)
	}
	// strpos is literal: % and _ in a user's search are not SQL wildcards.
	return rows(r.Context(), a.DB, exportSelect+exportScope+` AND ($13::bigint=0 OR d.company_id=$13) AND ($14::bigint=0 OR (d.party_id=$14 AND d.party_type=$15)) AND ($16='' OR d.currency=$16) AND ($17='' OR strpos(lower(CASE d.record_kind WHEN 'jv' THEN 'JV-' ELSE 'EX-' END||lpad(d.id::text,greatest(6,length(d.id::text)),'0')||' '||d.invoice_number||' '||d.party_name||' '||COALESCE(d.profile->>'export_reference','')||' '||COALESCE(d.profile->>'shipping_bill_number','')),lower($17))>0) AND ($18='' OR (COALESCE(NULLIF(d.profile->>'export_date',''),to_char(d.created_at AT TIME ZONE 'Asia/Kolkata','YYYY-MM-DD')) >= $18 AND COALESCE(NULLIF(d.profile->>'export_date',''),to_char(d.created_at AT TIME ZONE 'Asia/Kolkata','YYYY-MM-DD')) < $19)) AND ($20='' OR ($20='standalone' AND d.company_id IS NULL) OR ($20='company' AND d.company_id IS NOT NULL)) AND ($21='' OR d.workflow_status=$21) AND ($15<>'manual' OR d.party_type='manual') AND ($22='' OR (d.party_type='manual' AND lower(btrim(d.party_name))=lower($22))) AND d.record_kind=$23 ORDER BY d.id DESC LIMIT 10001`, append(exportArgs(u), cid, pid, typ, cur, search, from, to, invoiceScope, status, partyName, kind)...)
}

func (a *App) exportList(w http.ResponseWriter, r *http.Request) (any, error) {
	records, e := a.exportRecords(r)
	if e != nil {
		return nil, e
	}
	if len(records) > 10000 {
		return nil, bad("Select a company or party to reduce this register to 10,000 invoices.")
	}
	for _, m := range records {
		if exportRecordKind(m) != "jv" {
			m["balance"] = exportBalance(m)
			m["bill_amount"] = m["invoice_amount"]
		}
	}
	kind, _ := exportKind(r.URL.Query().Get("record_kind"))
	return M{"record_kind": kind, "records": records, "totals": exportTotals(records), "permissions": exportJVPermissions(current(r), M{"record_kind": kind})}, nil
}
func (a *App) exportDetail(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	ctx := r.Context()
	rid := materialID(r)
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	m, e := a.exportRecord(ctx, tx, u, rid, false)
	if e != nil {
		return nil, e
	}
	files, e := rows(ctx, tx, `SELECT id,kind,filename,mime,size,created_by,created_at,receipt_key::text AS receipt_key,field_key FROM export_files f WHERE invoice_id=$1 AND NOT EXISTS(SELECT 1 FROM export_file_removals x WHERE x.file_id=f.id) ORDER BY id`, rid)
	if e != nil {
		return nil, e
	}
	history, e := rows(ctx, tx, `SELECT h.id,h.action,h.created_at,u.name AS actor_name FROM export_history h LEFT JOIN users u ON u.id=h.actor_id WHERE invoice_id=$1 ORDER BY h.id DESC LIMIT 500`, rid)
	if e != nil {
		return nil, e
	}
	discounts, e := rows(ctx, tx, `SELECT x.*,x.amount::text AS amount,(SELECT name FROM users WHERE id=x.requested_by) AS requested_by_name,(SELECT name FROM users WHERE id=x.first_by) AS first_by_name,(SELECT name FROM users WHERE id=x.final_by) AS final_by_name FROM export_discounts x WHERE invoice_id=$1 ORDER BY id DESC`, rid)
	if e != nil {
		return nil, e
	}
	comments, e := exportComments(ctx, tx, rid)
	if e != nil {
		return nil, e
	}
	exportLabelReceiptFiles(m, files)
	return M{"record": m, "files": files, "history": history, "discounts": discounts, "comments": comments, "actions": exportWorkflowActions(u, m), "permissions": exportJVPermissions(u, m)}, tx.Commit(ctx)
}
func (a *App) exportHistoryDetail(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	ctx := r.Context()
	rid := materialID(r)
	if _, e := a.exportRecord(ctx, a.DB, u, rid, false); e != nil {
		return nil, e
	}
	hid, _ := strconv.ParseInt(chi.URLParam(r, "history_id"), 10, 64)
	return row(ctx, a.DB, `SELECT h.snapshot,h.action,h.created_at,u.name AS actor_name FROM export_history h LEFT JOIN users u ON u.id=h.actor_id WHERE h.id=$1 AND h.invoice_id=$2`, hid, rid)
}
func (a *App) exportParty(ctx context.Context, q queryer, u *User, m M) (string, any, any, error) {
	cid, pid, oid := id(m, "company_id"), id(m, "party_id"), id(m, "order_id")
	if str(m, "party_type") == "manual" {
		if !exportStandalone(u) || cid != 0 || pid != 0 || oid != 0 {
			return "", nil, nil, forbidden()
		}
		return str(m, "party_name"), nil, nil, nil
	}
	if e := a.company(ctx, q, u, cid, true); e != nil {
		return "", nil, nil, e
	}
	var name string
	var bid, sid any
	switch str(m, "party_type") {
	case "buyer":
		b, e := row(ctx, q, `SELECT b.name FROM buyers b WHERE ($4::bigint[] IS NOT NULL OR $4 IS NULL) AND ($5::bigint[] IS NOT NULL OR $5 IS NULL) AND ($6::text[] IS NOT NULL OR $6 IS NULL) AND ($10::bigint[] IS NOT NULL OR $10 IS NULL) AND b.id=$11 AND b.company_id=$12 AND b.company_id=ANY($1) AND ($7='Super Admin' OR (NOT $8 OR b.id=ANY($9)) AND ($2='company' OR ($2='team' AND b.team_id=ANY($3)) OR ($2 NOT IN ('company','team') AND b.id IN (SELECT o.buyer_id FROM erp_export_orders(b.company_id) o))))`, append(scopeArgs(u), pid, cid)...)
		if e != nil {
			return "", nil, nil, e
		}
		name, bid = str(b, "name"), pid
	case "company_setup":
		if u.Role != "Super Admin" && (!u.SetupCan("view") || u.Scope != "company" || u.RestrictBuyers) {
			return "", nil, nil, forbidden()
		}
		b, e := row(ctx, q, `SELECT name FROM erp_export_buyers($1) WHERE id=$2`, cid, pid)
		if e != nil {
			return "", nil, nil, e
		}
		name, sid = str(b, "name"), pid
	default:
		return "", nil, nil, bad("Choose an existing buyer party.")
	}
	if oid > 0 {
		if bid == nil {
			return "", nil, nil, bad("A linked PO requires its operational buyer party.")
		}
		order, e := row(ctx, q, `SELECT * FROM erp_export_orders($1) WHERE id=$2`, cid, oid)
		if e != nil {
			return "", nil, nil, e
		}
		if id(order, "company_id") != cid || id(order, "buyer_id") != pid {
			return "", nil, nil, bad("The PO must belong to this company and buyer.")
		}
	} else if u.Role != "Super Admin" && !slices.Contains([]string{"company", "team"}, u.Scope) {
		return "", nil, nil, bad("Select an accessible production PO for this invoice.")
	}
	return name, bid, sid, nil
}
func (a *App) exportSave(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if !exportCan(u, "edit") {
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
	var old M
	if rid > 0 {
		old, e = a.exportRecord(ctx, tx, u, rid, true)
		if e != nil {
			return nil, e
		}
		if id(old, "version") != id(d, "version") {
			return nil, exportConflict()
		}
		if e = exportContentEditable(old); e != nil {
			return nil, e
		}
		if _, supplied := d["record_kind"]; !supplied {
			d["record_kind"] = exportRecordKind(old)
		}
		if str(d, "record_kind") != exportRecordKind(old) {
			return nil, bad("Export and JV classification cannot change after creation.")
		}
	} else {
		d = exportJVCreateDefaults(d)
	}
	m, e := exportValidateBody(d)
	if e != nil {
		return nil, e
	}
	if old != nil {
		if e = exportKeepAttachedReceipts(ctx, tx, rid, m["receipts"]); e != nil {
			return nil, e
		}
		for _, k := range []string{"company_id", "party_id", "order_id"} {
			if id(old, k) != id(m, k) {
				return nil, bad("Company, party and linked PO cannot change after creation.")
			}
		}
		if str(old, "party_type") != str(m, "party_type") {
			return nil, bad("The invoice party cannot change after creation.")
		}
		if str(old, "currency") != str(m, "currency") && str(old, "approved_discount") != "0.00" && str(old, "approved_discount") != "0" {
			return nil, bad("Currency cannot change after a discount has been approved.")
		}
	}
	if old != nil {
		m["approved_discount"] = old["approved_discount"]
		approved, _ := exportCents(old["approved_discount"], false)
		if approved != nil && approved.Sign() > 0 && exportSignedCents(str(exportBalance(m), "outstanding")).Sign() < 0 {
			return nil, bad("The revised amounts would leave less balance than the approved discount. Keep approved financial entries intact.")
		}
	}
	name, bid, sid, e := a.exportParty(ctx, tx, u, m)
	if e != nil {
		return nil, e
	}
	p, _ := json.Marshal(m["profile"])
	receipts, _ := json.Marshal(m["receipts"])
	ebrcs, _ := json.Marshal(m["ebrcs"])
	var oid any
	if id(m, "order_id") > 0 {
		oid = id(m, "order_id")
	}
	var version int64
	if rid == 0 {
		var cid, pid any
		if id(m, "company_id") > 0 {
			cid, pid = id(m, "company_id"), id(m, "party_id")
		}
		e = tx.QueryRow(ctx, `INSERT INTO export_invoices(company_id,party_type,party_id,buyer_id,party_setup_id,party_name,order_id,invoice_number,currency,invoice_amount,profile,receipts,ebrcs,created_by,record_kind) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15) RETURNING id,version`, cid, str(m, "party_type"), pid, bid, sid, name, oid, str(m, "invoice_number"), str(m, "currency"), str(m, "invoice_amount"), p, receipts, ebrcs, u.ID, str(m, "record_kind")).Scan(&rid, &version)
	} else {
		_, e = tx.Exec(ctx, `UPDATE export_discounts SET status='Rejected',review_note='Invoice edited; a fresh approval request is required.' WHERE invoice_id=$1 AND status IN ('Pending finance','Pending final')`, rid)
		if e == nil {
			e = tx.QueryRow(ctx, `UPDATE export_invoices SET invoice_number=$1,currency=$2,invoice_amount=$3,profile=$4,receipts=$5,ebrcs=$6,finance_check='{}',audit_check='{}',version=version+1,content_revision=content_revision+1,workflow_status='Draft',approved_revision=NULL,party_name=$8,updated_at=now() WHERE id=$7 RETURNING version`, str(m, "invoice_number"), str(m, "currency"), str(m, "invoice_amount"), p, receipts, ebrcs, rid, name).Scan(&version)
		}
	}
	if e != nil {
		return nil, e
	}
	event := "Export invoice saved"
	if exportRecordKind(m) == "jv" {
		event = "JV entry saved"
	}
	if e = a.exportHistory(ctx, tx, u, rid, event); e != nil {
		return nil, e
	}
	return M{"id": rid, "version": version}, tx.Commit(ctx)
}
