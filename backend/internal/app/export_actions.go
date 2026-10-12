package app

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"time"
)

func (a *App) exportCheck(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	d, e := readBody(w, r)
	if e != nil {
		return nil, e
	}
	if e = allowedFields(d, "version", "kind", "status", "note"); e != nil {
		return nil, e
	}
	kind := str(d, "kind")
	if !slices.Contains([]string{"finance", "audit"}, kind) || !exportCan(u, kind+"_check") {
		return nil, forbidden()
	}
	status := str(d, "status")
	if !slices.Contains([]string{"Checked", "Needs correction"}, status) {
		return nil, bad("Choose Checked or Needs correction.")
	}
	note, e := text(d, "note", 2000)
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
	m, e := a.exportRecord(ctx, tx, u, rid, true)
	if e != nil {
		return nil, e
	}
	if id(m, "version") != id(d, "version") {
		return nil, exportConflict()
	}
	if e = exportJVFinancialAction(m); e != nil {
		return nil, e
	}
	if e = a.exportWritableCompany(ctx, tx, u, m); e != nil {
		return nil, e
	}
	check, _ := json.Marshal(M{"status": status, "note": note, "actor_id": u.ID, "actor_name": u.Name, "at": time.Now().UTC().Format(time.RFC3339)})
	var version int64
	e = tx.QueryRow(ctx, `UPDATE export_invoices SET `+kind+`_check=$1,version=version+1,updated_at=now() WHERE id=$2 RETURNING version`, check, rid).Scan(&version)
	if e != nil {
		return nil, e
	}
	if e = a.exportHistory(ctx, tx, u, rid, strings.Title(kind)+" file check: "+status); e != nil {
		return nil, e
	}
	return M{"id": rid, "version": version}, tx.Commit(ctx)
}
func (a *App) exportDiscount(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if !exportRead(u) {
		return nil, forbidden()
	}
	d, e := readBody(w, r)
	if e != nil {
		return nil, e
	}
	if e = allowedFields(d, "version", "action", "amount", "reason", "discount_id"); e != nil {
		return nil, e
	}
	action := str(d, "action")
	switch action {
	case "request":
		if !exportCan(u, "edit") {
			return nil, forbidden()
		}
	case "first_approve":
		if !exportCan(u, "discount_first") {
			return nil, forbidden()
		}
	case "final_approve":
		if !exportCan(u, "discount_final") {
			return nil, forbidden()
		}
	case "reject":
		if !exportCan(u, "discount_first") && !exportCan(u, "discount_final") {
			return nil, forbidden()
		}
	default:
		return nil, bad("Choose a discount action.")
	}
	reason, e := text(d, "reason", 2000)
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
	m, e := a.exportRecord(ctx, tx, u, rid, true)
	if e != nil {
		return nil, e
	}
	if id(m, "version") != id(d, "version") {
		return nil, exportConflict()
	}
	if e = exportJVFinancialAction(m); e != nil {
		return nil, e
	}
	if e = a.exportWritableCompany(ctx, tx, u, m); e != nil {
		return nil, e
	}
	did := id(d, "discount_id")
	event := "Discount requested"
	if action == "request" {
		amount, err := exportCents(d["amount"], true)
		if err != nil {
			return nil, err
		}
		if !exportDiscountFits(exportDecimal(amount), exportBalance(m)) {
			return nil, bad("Discount must be positive and cannot exceed the remaining outstanding balance.")
		}
		var pending bool
		if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM export_discounts WHERE invoice_id=$1 AND status IN ('Pending finance','Pending final'))`, rid).Scan(&pending); e != nil {
			return nil, e
		}
		if pending {
			return nil, bad("Review or reject the existing pending discount first.")
		}
		e = tx.QueryRow(ctx, `INSERT INTO export_discounts(invoice_id,amount,currency,reason,requested_by) VALUES($1,$2,$3,$4,$5) RETURNING id`, rid, exportDecimal(amount), str(m, "currency"), reason, u.ID).Scan(&did)
	} else {
		discount, err := row(ctx, tx, `SELECT *,amount::text AS amount FROM export_discounts WHERE id=$1 AND invoice_id=$2 FOR UPDATE`, did, rid)
		if err != nil {
			return nil, err
		}
		status := str(discount, "status")
		if !slices.Contains([]string{"Pending finance", "Pending final"}, status) {
			return nil, bad("This discount has already been decided.")
		}
		if action != "reject" && !exportDiscountFits(str(discount, "amount"), exportBalance(m)) {
			return nil, bad("Discount now exceeds the outstanding balance; reject it and submit a corrected amount.")
		}
		switch action {
		case "first_approve":
			if status != "Pending finance" {
				return nil, bad("Finance approval is already recorded.")
			}
			_, e = tx.Exec(ctx, `UPDATE export_discounts SET status='Pending final',first_by=$1,first_at=now(),review_note=$2 WHERE id=$3`, u.ID, reason, did)
			event = "Discount approved by Finance Head"
		case "final_approve":
			if status != "Pending final" || id(discount, "first_by") == 0 {
				return nil, bad("Finance Head approval is required first.")
			}
			if id(discount, "first_by") == u.ID {
				return nil, bad("The final approver must be a different person from the Finance Head approver.")
			}
			_, e = tx.Exec(ctx, `UPDATE export_discounts SET status='Approved',final_by=$1,final_at=now(),review_note=$2 WHERE id=$3`, u.ID, reason, did)
			event = "Discount approved finally"
		case "reject":
			_, e = tx.Exec(ctx, `UPDATE export_discounts SET status='Rejected',review_note=$1 WHERE id=$2`, reason, did)
			event = "Discount rejected"
		}
	}
	if e != nil {
		return nil, e
	}
	var version int64
	if e = tx.QueryRow(ctx, `UPDATE export_invoices SET version=version+1,finance_check='{}',audit_check='{}',updated_at=now() WHERE id=$1 RETURNING version`, rid).Scan(&version); e != nil {
		return nil, e
	}
	if e = a.exportHistory(ctx, tx, u, rid, event); e != nil {
		return nil, e
	}
	return M{"id": rid, "version": version, "discount_id": did}, tx.Commit(ctx)
}
