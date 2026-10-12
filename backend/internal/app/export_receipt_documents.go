package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"
)

var exportReceiptKeyPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)
var exportReceiptDocumentFields = map[string]struct{ kind, label string }{
	"inward_remittance":    {"payment_details", "Inward remittance"},
	"disposal_instruction": {"payment_details", "Disposal instruction"},
	"credit_advice":        {"bank_advice", "Credit advice"},
}

func exportReceiptKey(value any) (string, error) {
	s, ok := value.(string)
	s = strings.ToLower(strings.TrimSpace(s))
	if !ok || !exportReceiptKeyPattern.MatchString(s) || s == "00000000-0000-0000-0000-000000000000" {
		return "", bad("Each receipt needs a valid unique receipt key. Refresh the invoice and retry.")
	}
	return s, nil
}

func exportReceiptPresent(parent M, key string) bool {
	for _, receipt := range enquiryRows(parent["receipts"]) {
		if str(receipt, "receipt_key") == key {
			return true
		}
	}
	return false
}

func exportValidateFileContext(parent M, kind, receiptKey, fieldKey string) error {
	if receiptKey == "" && fieldKey == "" {
		return nil
	}
	if receiptKey == "" || fieldKey == "" {
		return bad("Choose both the receipt and its document field.")
	}
	field, ok := exportReceiptDocumentFields[fieldKey]
	if !ok || field.kind != kind || exportRecordKind(parent) != "export" {
		return bad("Choose the matching document category for this receipt field.")
	}
	if !exportReceiptPresent(parent, receiptKey) {
		return bad("Save the receipt before attaching its document.")
	}
	return nil
}

// Keep legacy invoice-wide files visible without guessing which receipt owns
// them. Labels travel with immutable review/history snapshots as well as detail.
func exportLabelReceiptFiles(parent M, files []M) {
	for _, file := range files {
		file["context_label"] = "Shared invoice document"
		if key := str(file, "receipt_key"); key != "" {
			field := exportReceiptDocumentFields[str(file, "field_key")]
			label := "Receipt " + key
			for i, receipt := range enquiryRows(parent["receipts"]) {
				if str(receipt, "receipt_key") == key {
					label = fmt.Sprintf("Receipt %d", i+1)
					if reference := str(receipt, "reference"); reference != "" {
						label += " · " + reference
					}
					break
				}
			}
			file["field_label"], file["receipt_label"] = field.label, label
			file["context_label"] = label + " / " + field.label
		}
	}
}

func exportKeepAttachedReceipts(ctx context.Context, q queryer, rid int64, receipts any) error {
	files, err := rows(ctx, q, `SELECT DISTINCT f.receipt_key::text AS receipt_key FROM export_files f WHERE f.invoice_id=$1 AND f.receipt_key IS NOT NULL AND NOT EXISTS(SELECT 1 FROM export_file_removals x WHERE x.file_id=f.id)`, rid)
	if err != nil {
		return err
	}
	next := M{"receipts": receipts}
	for _, file := range files {
		if !exportReceiptPresent(next, str(file, "receipt_key")) {
			return bad("Remove this receipt's attached documents before removing or replacing the receipt.")
		}
	}
	return nil
}

func exportSavedEBRC(parent M, number string) (string, error) {
	normalized := strings.ToUpper(strings.Join(strings.Fields(number), " "))
	if normalized == "" {
		return "", bad("Save an eBRC number before clearing the documentation.")
	}
	for _, certificate := range enquiryRows(parent["ebrcs"]) {
		saved := str(certificate, "ebrc_number")
		if strings.ToUpper(strings.Join(strings.Fields(saved), " ")) == normalized {
			return saved, nil
		}
	}
	return "", bad("Choose an eBRC number already saved on this invoice.")
}

func (a *App) exportReconcile(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if !exportEditor(u) {
		return nil, forbidden()
	}
	d, err := readBody(w, r)
	if err != nil {
		return nil, err
	}
	if err = allowedFields(d, "version", "ebrc_number", "file_id"); err != nil {
		return nil, err
	}
	version, err := integer(d["version"])
	if err != nil || version < 1 {
		return nil, bad("Refresh the invoice before clearing the documentation.")
	}
	fid, err := integer(d["file_id"])
	if err != nil || fid < 1 {
		return nil, bad("Choose an attached eBRC document.")
	}
	number, ok := d["ebrc_number"].(string)
	if !ok || len(number) > 2000 {
		return nil, bad("Choose a saved eBRC number.")
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
	if exportRecordKind(parent) != "export" {
		return nil, bad("Documentation clearance is available for export invoices only.")
	}
	if id(parent, "version") != version {
		return nil, exportConflict()
	}
	if err = a.exportWritableCompany(ctx, tx, u, parent); err != nil {
		return nil, err
	}
	number, err = exportSavedEBRC(parent, number)
	if err != nil {
		return nil, err
	}
	var attached bool
	if err = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM export_files f WHERE id=$1 AND invoice_id=$2 AND kind='ebrc' AND NOT EXISTS(SELECT 1 FROM export_file_removals x WHERE x.file_id=f.id))`, fid, rid).Scan(&attached); err != nil {
		return nil, err
	}
	if !attached {
		return nil, bad("Choose an active eBRC document attached to this invoice.")
	}
	clearance := M{"by": u.ID, "at": time.Now().UTC().Format(time.RFC3339Nano), "ebrc_number": number, "file_id": fid, "content_revision": id(parent, "content_revision")}
	raw, err := json.Marshal(clearance)
	if err != nil {
		return nil, err
	}
	if err = tx.QueryRow(ctx, `UPDATE export_invoices SET documentation_cleared=$2,version=version+1,updated_at=now() WHERE id=$1 RETURNING version`, rid, raw).Scan(&version); err != nil {
		return nil, err
	}
	if err = a.exportHistory(ctx, tx, u, rid, "Export documentation cleared: eBRC "+number); err != nil {
		return nil, err
	}
	return M{"id": rid, "version": version, "documentation_cleared": clearance}, tx.Commit(ctx)
}
