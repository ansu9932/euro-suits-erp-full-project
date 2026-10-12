package app

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
)

// Reconciliation is a separate documentary stage. It can add or correct only
// eBRC evidence, never an invoice field, settlement or approval revision.
func exportReconciliationAccess(u *User, parent M, files bool) error {
	if !exportEditor(u) || (files && !exportCan(u, "files")) {
		return forbidden()
	}
	if exportRecordKind(parent) != "export" {
		return bad("Reconciliation evidence is available for export invoices only.")
	}
	return nil
}

func exportReconciliationScope(ctx context.Context, q queryer, rid int64) error {
	_, err := q.Exec(ctx, `SELECT set_config('erp.export_reconciliation',$1,true)`, strconv.FormatInt(rid, 10))
	return err
}

func exportEBRCIdentity(s string) string {
	return strings.ToUpper(strings.Join(strings.Fields(s), " "))
}

func exportReconciliationEBRCValues(parent M, d M) ([]M, bool, error) {
	if err := allowedFields(d, "version", "ebrc_number", "previous_ebrc_number"); err != nil {
		return nil, false, err
	}
	number, ok := d["ebrc_number"].(string)
	number = strings.TrimSpace(number)
	if !ok || number == "" || len(number) > 160 || strings.ContainsAny(number, "\r\n\t\x00") {
		return nil, false, bad("Enter an eBRC number on one line, up to 160 characters.")
	}
	previous := ""
	if value, present := d["previous_ebrc_number"]; present {
		previous, ok = value.(string)
		if !ok || strings.TrimSpace(previous) == "" || len(previous) > 2000 {
			return nil, false, bad("Choose the saved eBRC number to correct.")
		}
	}
	target, duplicate := -1, -1
	originals := enquiryRows(parent["ebrcs"])
	out := make([]M, 0, len(originals)+1)
	for i, item := range originals {
		out = append(out, materialProfile(item))
		if exportEBRCIdentity(str(item, "ebrc_number")) == exportEBRCIdentity(number) {
			duplicate = i
		}
		if previous != "" && exportEBRCIdentity(str(item, "ebrc_number")) == exportEBRCIdentity(previous) {
			target = i
		}
	}
	if previous != "" {
		if target < 0 {
			return nil, false, bad("The eBRC number being corrected is no longer saved. Refresh reconciliation.")
		}
		if duplicate >= 0 && duplicate != target {
			return nil, false, bad("This eBRC number is already saved on the invoice.")
		}
		if str(out[target], "ebrc_number") == number {
			return out, false, nil
		}
		out[target]["ebrc_number"] = number
		return out, true, nil
	}
	if duplicate >= 0 {
		return out, false, nil
	}
	if len(out) >= 100 {
		return nil, false, bad("An invoice supports up to 100 eBRC entries.")
	}
	return append(out, M{"ebrc_number": number, "statuses": M{}}), true, nil
}

func (a *App) exportReconciliationEBRC(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if !exportEditor(u) {
		return nil, forbidden()
	}
	d, err := readBody(w, r)
	if err != nil {
		return nil, err
	}
	if err = allowedFields(d, "version", "ebrc_number", "previous_ebrc_number"); err != nil {
		return nil, err
	}
	version, err := integer(d["version"])
	if err != nil || version < 1 {
		return nil, bad("Refresh reconciliation before saving eBRC evidence.")
	}
	ctx := r.Context()
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rid := materialID(r)
	parent, err := a.exportRecord(ctx, tx, u, rid, true)
	if err != nil {
		return nil, err
	}
	if err = exportReconciliationAccess(u, parent, false); err != nil {
		return nil, err
	}
	if id(parent, "version") != version {
		return nil, exportConflict()
	}
	if err = a.exportWritableCompany(ctx, tx, u, parent); err != nil {
		return nil, err
	}
	entries, changed, err := exportReconciliationEBRCValues(parent, d)
	if err != nil {
		return nil, err
	}
	if !changed {
		return M{"id": rid, "version": version, "ebrcs": entries, "documentation_cleared": parent["documentation_cleared"]}, tx.Commit(ctx)
	}
	if err = exportReconciliationScope(ctx, tx, rid); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(entries)
	if err != nil {
		return nil, err
	}
	if err = tx.QueryRow(ctx, `UPDATE export_invoices SET ebrcs=$2,documentation_cleared='{}',version=version+1,updated_at=now() WHERE id=$1 RETURNING version`, rid, raw).Scan(&version); err != nil {
		return nil, err
	}
	event := "Reconciliation eBRC number added: " + str(d, "ebrc_number")
	if str(d, "previous_ebrc_number") != "" {
		event = "Reconciliation eBRC number corrected: " + str(d, "previous_ebrc_number") + " → " + str(d, "ebrc_number")
	}
	if err = a.exportHistory(ctx, tx, u, rid, event); err != nil {
		return nil, err
	}
	return M{"id": rid, "version": version, "ebrcs": entries, "documentation_cleared": M{}}, tx.Commit(ctx)
}
