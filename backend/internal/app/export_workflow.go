package app

import (
	"context"
	"net/http"
	"slices"
	"strings"
)

// Unassigned invoices are a shared finance workspace. They cannot contain any
// linked company/buyer/PO IDs; company-bound records retain their prior scope.
func exportStandalone(u *User) bool {
	return exportRead(u) && slices.Contains([]string{"Finance", "Finance Head", "MD", "Super Admin"}, u.Role)
}

func exportEditor(u *User) bool {
	return exportCan(u, "edit") && slices.Contains([]string{"Finance", "Finance Head", "Super Admin"}, u.Role)
}

func exportContentEditable(m M) error {
	if !slices.Contains([]string{"Draft", "Changes requested"}, str(m, "workflow_status")) {
		return bad("Submitted invoices cannot be edited. Request corrections, or reopen an approved invoice before changing its fields or documents.")
	}
	return nil
}

func exportWorkflowActions(u *User, m M) M {
	editable := exportContentEditable(m) == nil
	reviewer := exportCan(u, "approve")
	independent := id(m, "created_by") != u.ID && id(m, "submitted_by") != u.ID
	return M{
		"edit":                    exportEditor(u) && editable,
		"reconcile":               exportRecordKind(m) == "export" && exportEditor(u),
		"reconciliation_evidence": exportRecordKind(m) == "export" && exportEditor(u),
		"reconciliation_attach":   exportRecordKind(m) == "export" && exportEditor(u) && exportCan(u, "files"),
		"reconciliation_remove":   exportRecordKind(m) == "export" && exportEditor(u) && exportCan(u, "files"),
		"attach":                  exportCan(u, "files") && editable,
		"remove":                  exportCan(u, "files") && editable,
		"submit":                  exportEditor(u) && editable,
		"approve":                 reviewer && independent && str(m, "workflow_status") == "Submitted",
		"request_changes":         reviewer && independent && str(m, "workflow_status") == "Submitted",
		"reopen":                  exportEditor(u) && str(m, "workflow_status") == "Approved",
		"comment":                 exportEditor(u) || reviewer,
	}
}

func (a *App) exportWritableCompany(ctx context.Context, q queryer, u *User, m M) error {
	if id(m, "company_id") == 0 {
		if str(m, "party_type") != "manual" || !exportStandalone(u) {
			return forbidden()
		}
		return nil
	}
	return a.company(ctx, q, u, id(m, "company_id"), true)
}

func exportComments(ctx context.Context, q queryer, rid int64) ([]M, error) {
	return rows(ctx, q, `SELECT c.id,c.author_id,u.name AS author_name,c.note,c.created_at FROM export_comments c JOIN users u ON u.id=c.author_id WHERE c.invoice_id=$1 ORDER BY c.id`, rid)
}

func exportRequireInvoiceDocument(ctx context.Context, q queryer, rid int64, recordKind string) error {
	kind, label := "invoice", "invoice document"
	if recordKind == "jv" {
		kind, label = "jv_supporting", "supporting JV document"
	}
	var attached bool
	if err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM export_files f WHERE invoice_id=$1 AND kind=$2 AND NOT EXISTS(SELECT 1 FROM export_file_removals x WHERE x.file_id=f.id))`, rid, kind).Scan(&attached); err != nil {
		return err
	}
	if !attached {
		return bad("Attach at least one " + label + " before submitting for approval.")
	}
	return nil
}

func exportWorkflowNote(d M, required bool) (string, error) {
	value, exists := d["note"]
	if !exists && !required {
		return "", nil
	}
	note, ok := value.(string)
	note = strings.TrimSpace(note)
	if !ok || len(note) > 2000 || strings.ContainsRune(note, 0) || (required && note == "") {
		return "", bad("Enter a comment up to 2,000 characters.")
	}
	return note, nil
}

func (a *App) exportWorkflow(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	d, err := readBody(w, r)
	if err != nil {
		return nil, err
	}
	if err = allowedFields(d, "version", "action", "note"); err != nil {
		return nil, err
	}
	action := str(d, "action")
	switch action {
	case "submit", "reopen":
		if !exportEditor(u) {
			return nil, forbidden()
		}
	case "approve", "request_changes":
		if !exportCan(u, "approve") {
			return nil, forbidden()
		}
	default:
		return nil, bad("Choose a valid invoice review action.")
	}
	note, err := exportWorkflowNote(d, action == "request_changes" || action == "reopen")
	if err != nil {
		return nil, err
	}
	ctx := r.Context()
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rid := materialID(r)
	m, err := a.exportRecord(ctx, tx, u, rid, true)
	if err != nil {
		return nil, err
	}
	if id(m, "version") != id(d, "version") {
		return nil, exportConflict()
	}
	if err = a.exportWritableCompany(ctx, tx, u, m); err != nil {
		return nil, err
	}
	status, event := "", ""
	var version int64
	switch action {
	case "submit":
		if err = exportContentEditable(m); err != nil {
			return nil, err
		}
		if err = exportRequireInvoiceDocument(ctx, tx, rid, exportRecordKind(m)); err != nil {
			return nil, err
		}
		status, event = "Submitted", "Export invoice submitted for approval"
		err = tx.QueryRow(ctx, `UPDATE export_invoices SET workflow_status='Submitted',submitted_revision=content_revision,submitted_by=$1,submitted_at=now(),reviewed_by=NULL,reviewed_at=NULL,review_note='',approved_revision=NULL,version=version+1,updated_at=now() WHERE id=$2 RETURNING version`, u.ID, rid).Scan(&version)
	case "approve", "request_changes":
		if str(m, "workflow_status") != "Submitted" || id(m, "submitted_revision") != id(m, "content_revision") {
			return nil, bad("Only the current submitted invoice revision can be reviewed.")
		}
		if id(m, "created_by") == u.ID || id(m, "submitted_by") == u.ID {
			return nil, bad("A different MD or Super Admin must review an invoice you created or submitted.")
		}
		status, event = "Approved", "Export invoice approved"
		var approved any = id(m, "content_revision")
		if action == "request_changes" {
			status, event, approved = "Changes requested", "Export invoice corrections requested", nil
		}
		err = tx.QueryRow(ctx, `UPDATE export_invoices SET workflow_status=$1,reviewed_by=$2,reviewed_at=now(),review_note=$3,approved_revision=$4,version=version+1,updated_at=now() WHERE id=$5 RETURNING version`, status, u.ID, note, approved, rid).Scan(&version)
	case "reopen":
		if str(m, "workflow_status") != "Approved" {
			return nil, bad("Only an approved invoice can be reopened for editing.")
		}
		status, event = "Draft", "Export invoice reopened; approval required again"
		err = tx.QueryRow(ctx, `UPDATE export_invoices SET workflow_status='Draft',approved_revision=NULL,version=version+1,updated_at=now() WHERE id=$1 RETURNING version`, rid).Scan(&version)
	}
	if err != nil {
		return nil, err
	}
	if note != "" {
		if _, err = tx.Exec(ctx, `INSERT INTO export_comments(invoice_id,author_id,note) VALUES($1,$2,$3)`, rid, u.ID, note); err != nil {
			return nil, err
		}
	}
	if exportRecordKind(m) == "jv" {
		event = strings.Replace(event, "Export invoice", "JV entry", 1)
	}
	if err = a.exportHistory(ctx, tx, u, rid, event); err != nil {
		return nil, err
	}
	return M{"id": rid, "version": version, "workflow_status": status}, tx.Commit(ctx)
}

func (a *App) exportComment(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if !exportEditor(u) && !exportCan(u, "approve") {
		return nil, forbidden()
	}
	d, err := readBody(w, r)
	if err != nil {
		return nil, err
	}
	if err = allowedFields(d, "version", "note"); err != nil {
		return nil, err
	}
	note, err := exportWorkflowNote(d, true)
	if err != nil {
		return nil, err
	}
	ctx := r.Context()
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rid := materialID(r)
	m, err := a.exportRecord(ctx, tx, u, rid, true)
	if err != nil {
		return nil, err
	}
	if id(m, "version") != id(d, "version") {
		return nil, exportConflict()
	}
	if err = a.exportWritableCompany(ctx, tx, u, m); err != nil {
		return nil, err
	}
	var commentID, version int64
	if err = tx.QueryRow(ctx, `INSERT INTO export_comments(invoice_id,author_id,note) VALUES($1,$2,$3) RETURNING id`, rid, u.ID, note).Scan(&commentID); err != nil {
		return nil, err
	}
	if err = tx.QueryRow(ctx, `UPDATE export_invoices SET version=version+1,updated_at=now() WHERE id=$1 RETURNING version`, rid).Scan(&version); err != nil {
		return nil, err
	}
	event := "Export invoice comment added"
	if exportRecordKind(m) == "jv" {
		event = "JV entry comment added"
	}
	if err = a.exportHistory(ctx, tx, u, rid, event); err != nil {
		return nil, err
	}
	return M{"id": rid, "version": version, "comment_id": commentID}, tx.Commit(ctx)
}
