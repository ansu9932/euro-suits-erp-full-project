package app

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

var exportDocumentKinds = []M{{"key": "invoice", "label": "Invoice"}, {"key": "packing_list", "label": "Packing list"}, {"key": "shipping_bill", "label": "Shipping bill"}, {"key": "delivery_challan", "label": "Delivery challan"}, {"key": "eway_bill", "label": "E-way bill"}, {"key": "bill_of_lading", "label": "Bill of lading / AWB"}, {"key": "advance_license", "label": "Advance licence"}, {"key": "payment_details", "label": "Payment details"}, {"key": "ebrc", "label": "eBRC"}, {"key": "bank_advice", "label": "Bank advice"}, {"key": "clearing_invoice", "label": "Clearing invoice"}, {"key": "other", "label": "Other"}}

func exportDocumentKindsFor(recordKind string) []M {
	if recordKind == "jv" {
		return []M{{"key": "jv_supporting", "label": "Supporting JV document"}}
	}
	return exportDocumentKinds
}

func exportDocumentKindAllowed(recordKind, kind string) bool {
	for _, option := range exportDocumentKindsFor(recordKind) {
		if str(option, "key") == kind {
			return true
		}
	}
	return false
}

func exportUploadParameter(r *http.Request, key string) (string, error) {
	query := r.URL.Query()[key]
	form := r.MultipartForm.Value[key]
	if len(query) > 1 || len(form) > 1 {
		return "", bad("Duplicate upload parameter.")
	}
	q, f := "", ""
	if len(query) > 0 {
		q = query[0]
	}
	if len(form) > 0 {
		f = form[0]
	}
	if q != "" && f != "" && q != f {
		return "", bad("Conflicting upload parameters.")
	}
	if f != "" {
		return f, nil
	}
	return q, nil
}
func (a *App) exportFileUpload(w http.ResponseWriter, r *http.Request) (any, error) {
	return a.exportUploadFile(w, r, false)
}
func (a *App) exportReconciliationFileUpload(w http.ResponseWriter, r *http.Request) (any, error) {
	return a.exportUploadFile(w, r, true)
}
func (a *App) exportUploadFile(w http.ResponseWriter, r *http.Request, reconciliation bool) (any, error) {
	u := current(r)
	if !exportCan(u, "files") || (reconciliation && !exportEditor(u)) {
		return nil, forbidden()
	}
	ctx := r.Context()
	if e := a.limit(ctx, fmt.Sprintf("export-upload:%d", u.ID), 20, time.Minute); e != nil {
		return nil, e
	}
	select {
	case a.UploadSlots <- struct{}{}:
		defer func() { <-a.UploadSlots }()
	default:
		return nil, failure{429, "Uploads are busy. Retry shortly."}
	}
	r.Body = http.MaxBytesReader(w, r.Body, 11<<20)
	if e := r.ParseMultipartForm(1 << 20); e != nil {
		return nil, bad("Choose a document up to 10 MB.")
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	if reconciliation {
		for key := range r.URL.Query() {
			if key != "kind" && key != "version" {
				return nil, bad("Reconciliation uploads accept only eBRC files and the invoice version.")
			}
		}
		for key := range r.MultipartForm.Value {
			if key != "kind" && key != "version" {
				return nil, bad("Reconciliation uploads accept only eBRC files and the invoice version.")
			}
		}
	}
	kind, e := exportUploadParameter(r, "kind")
	if e != nil {
		return nil, e
	}
	if !exportDocumentKindAllowed("export", kind) && !exportDocumentKindAllowed("jv", kind) {
		return nil, bad("Choose a document category.")
	}
	receiptKey, e := exportUploadParameter(r, "receipt_key")
	if e != nil {
		return nil, e
	}
	fieldKey, e := exportUploadParameter(r, "field_key")
	if e != nil {
		return nil, e
	}
	if receiptKey != "" {
		receiptKey, e = exportReceiptKey(receiptKey)
		if e != nil {
			return nil, e
		}
	}
	if reconciliation && (kind != "ebrc" || receiptKey != "" || fieldKey != "") {
		return nil, bad("Reconciliation accepts only shared eBRC documents.")
	}
	versionText, e := exportUploadParameter(r, "version")
	if e != nil {
		return nil, e
	}
	version, e := strconv.ParseInt(versionText, 10, 64)
	if e != nil || version < 1 {
		return nil, bad("Refresh the invoice before attaching a document.")
	}
	f, h, e := r.FormFile("file")
	if e != nil {
		return nil, bad("Choose a file.")
	}
	defer f.Close()
	raw, e := io.ReadAll(io.LimitReader(f, (10<<20)+1))
	if e != nil {
		return nil, e
	}
	if len(raw) == 0 || len(raw) > 10<<20 || len(h.Filename) > 180 {
		return nil, bad("Choose a nonempty document up to 10 MB with a filename up to 180 characters.")
	}
	filename := filepath.Base(strings.ReplaceAll(h.Filename, "\\", "/"))
	if filename == "." || strings.ContainsAny(filename, "\x00\r\n") {
		return nil, bad("Invalid filename.")
	}
	if e = validateUpload(filename, raw); e != nil {
		return nil, e
	}
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	rid := materialID(r)
	parent, e := a.exportRecord(ctx, tx, u, rid, true)
	if e != nil {
		return nil, e
	}
	if !exportDocumentKindAllowed(exportRecordKind(parent), kind) {
		return nil, bad("Choose a document category for this register.")
	}
	if id(parent, "version") != version {
		return nil, exportConflict()
	}
	if reconciliation {
		if e = exportReconciliationAccess(u, parent, true); e != nil {
			return nil, e
		}
	} else if e = exportContentEditable(parent); e != nil {
		return nil, e
	}
	if e = a.exportWritableCompany(ctx, tx, u, parent); e != nil {
		return nil, e
	}
	if e = exportValidateFileContext(parent, kind, receiptKey, fieldKey); e != nil {
		return nil, e
	}
	if reconciliation {
		if e = exportReconciliationScope(ctx, tx, rid); e != nil {
			return nil, e
		}
	}
	var count int
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM export_files f WHERE invoice_id=$1 AND NOT EXISTS(SELECT 1 FROM export_file_removals x WHERE x.file_id=f.id)`, rid).Scan(&count); e != nil {
		return nil, e
	}
	if count >= 100 {
		return nil, bad("An invoice supports up to 100 documents.")
	}
	cipher, e := companyCipher()
	if e != nil {
		return nil, e
	}
	nonce := make([]byte, cipher.NonceSize())
	if _, e = rand.Read(nonce); e != nil {
		return nil, e
	}
	encrypted := cipher.Seal(nonce, nonce, raw, []byte(fmt.Sprintf("export:%d", rid)))
	contentType := http.DetectContentType(raw)
	if strings.EqualFold(filepath.Ext(filename), ".pdf") && bytes.HasPrefix(raw, []byte("%PDF-")) {
		contentType = "application/pdf"
	}
	hash := fmt.Sprintf("%x", sha256.Sum256(raw))
	var fid int64
	e = tx.QueryRow(ctx, `INSERT INTO export_files(invoice_id,kind,filename,mime,size,sha256,ciphertext,created_by,receipt_key,field_key) VALUES($1,$2,$3,$4,$5,$6,$7,$8,NULLIF($9,'')::uuid,NULLIF($10,'')) RETURNING id`, rid, kind, filename, contentType, len(raw), hash, encrypted, u.ID, receiptKey, fieldKey).Scan(&fid)
	if e != nil {
		return nil, e
	}
	updateSQL := `UPDATE export_invoices SET version=version+1,content_revision=content_revision+1,workflow_status='Draft',finance_check='{}',audit_check='{}',updated_at=now() WHERE id=$1 RETURNING version`
	if reconciliation {
		updateSQL = `UPDATE export_invoices SET version=version+1,documentation_cleared='{}',updated_at=now() WHERE id=$1 RETURNING version`
	}
	if e = tx.QueryRow(ctx, updateSQL, rid).Scan(&version); e != nil {
		return nil, e
	}
	event := "Document attached: " + kind
	if reconciliation {
		event = "Reconciliation eBRC document attached: " + filename
	}
	if fieldKey != "" {
		event += " / " + exportReceiptDocumentFields[fieldKey].label
	}
	if e = a.exportHistory(ctx, tx, u, rid, event); e != nil {
		return nil, e
	}
	return M{"id": fid, "version": version, "receipt_key": receiptKey, "field_key": fieldKey}, tx.Commit(ctx)
}

func (a *App) exportFileRemove(w http.ResponseWriter, r *http.Request) (any, error) {
	return a.exportRemoveFile(w, r, false)
}
func (a *App) exportReconciliationFileRemove(w http.ResponseWriter, r *http.Request) (any, error) {
	return a.exportRemoveFile(w, r, true)
}
func (a *App) exportRemoveFile(w http.ResponseWriter, r *http.Request, reconciliation bool) (any, error) {
	u := current(r)
	if !exportCan(u, "files") || (reconciliation && !exportEditor(u)) {
		return nil, forbidden()
	}
	d, e := readBody(w, r)
	if e != nil {
		return nil, e
	}
	if e = allowedFields(d, "version"); e != nil {
		return nil, e
	}
	version, e := integer(d["version"])
	if e != nil || version < 1 {
		return nil, bad("Refresh the invoice before removing a document.")
	}
	fid, e := strconv.ParseInt(chi.URLParam(r, "file_id"), 10, 64)
	if e != nil || fid < 1 {
		return nil, missing()
	}
	ctx := r.Context()
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	rid := materialID(r)
	parent, e := a.exportRecord(ctx, tx, u, rid, true)
	if e != nil {
		return nil, e
	}
	if id(parent, "version") != version {
		return nil, exportConflict()
	}
	if reconciliation {
		if e = exportReconciliationAccess(u, parent, true); e != nil {
			return nil, e
		}
	} else if e = exportContentEditable(parent); e != nil {
		return nil, e
	}
	if e = a.exportWritableCompany(ctx, tx, u, parent); e != nil {
		return nil, e
	}
	file, e := row(ctx, tx, `SELECT f.id,f.kind,f.filename FROM export_files f WHERE f.id=$1 AND f.invoice_id=$2 AND NOT EXISTS(SELECT 1 FROM export_file_removals x WHERE x.file_id=f.id)`, fid, rid)
	if e != nil {
		return nil, e
	}
	if reconciliation {
		if str(file, "kind") != "ebrc" {
			return nil, bad("Only eBRC documents can be removed in reconciliation.")
		}
		if e = exportReconciliationScope(ctx, tx, rid); e != nil {
			return nil, e
		}
	}
	if _, e = tx.Exec(ctx, `INSERT INTO export_file_removals(file_id,removed_by) VALUES($1,$2)`, fid, u.ID); e != nil {
		return nil, e
	}
	updateSQL := `UPDATE export_invoices SET version=version+1,content_revision=content_revision+1,workflow_status='Draft',approved_revision=NULL,finance_check='{}',audit_check='{}',updated_at=now() WHERE id=$1 RETURNING version`
	if reconciliation {
		updateSQL = `UPDATE export_invoices SET version=version+1,documentation_cleared='{}',updated_at=now() WHERE id=$1 RETURNING version`
	}
	if e = tx.QueryRow(ctx, updateSQL, rid).Scan(&version); e != nil {
		return nil, e
	}
	event := "Document removed: " + str(file, "kind") + " — " + str(file, "filename")
	if reconciliation {
		event = "Reconciliation eBRC document removed: " + str(file, "filename")
	}
	if e = a.exportHistory(ctx, tx, u, rid, event); e != nil {
		return nil, e
	}
	return M{"id": fid, "invoice_id": rid, "version": version, "removed": true}, tx.Commit(ctx)
}

func (a *App) exportFileGet(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if !exportCan(u, "download") {
		return nil, forbidden()
	}
	ctx := r.Context()
	p, e := row(ctx, a.DB, `SELECT f.* FROM export_files f WHERE id=$1 AND NOT EXISTS(SELECT 1 FROM export_file_removals x WHERE x.file_id=f.id)`, materialID(r))
	if e != nil {
		return nil, e
	}
	if _, e = a.exportRecord(ctx, a.DB, u, id(p, "invoice_id"), false); e != nil {
		return nil, e
	}
	cipher, e := companyCipher()
	if e != nil {
		return nil, e
	}
	raw, ok := p["ciphertext"].([]byte)
	if !ok || len(raw) < cipher.NonceSize() {
		return nil, bad("Document unavailable.")
	}
	plain, e := cipher.Open(nil, raw[:cipher.NonceSize()], raw[cipher.NonceSize():], []byte(fmt.Sprintf("export:%d", id(p, "invoice_id"))))
	if e != nil {
		return nil, e
	}
	name := str(p, "filename")
	preview := r.URL.Query().Get("preview") == "1"
	// Retain the existing direct PDF-open behavior; explicit download always
	// takes precedence. Raster previews are opt-in and Office stays download-only.
	preview = (preview || str(p, "mime") == "application/pdf") && r.URL.Query().Get("download") != "1"
	disposition, contentType := exportDocumentPresentation(name, plain, preview)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": name}))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Security-Policy", "sandbox")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(plain))
	return nil, nil
}

// Never trust the stored MIME label for inline rendering. Only a supported
// filename plus validated PDF/raster contents can enter the browser preview.
func exportDocumentPresentation(name string, plain []byte, preview bool) (string, string) {
	if preview {
		contentType := ""
		switch strings.ToLower(filepath.Ext(name)) {
		case ".pdf":
			contentType = "application/pdf"
		case ".png":
			contentType = "image/png"
		case ".jpg", ".jpeg":
			contentType = "image/jpeg"
		}
		if contentType != "" && validateUpload(name, plain) == nil {
			return "inline", contentType
		}
	}
	return "attachment", "application/octet-stream"
}
