package app

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/mail"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

func enquiryDocuments(u *User) bool {
	return enquiryRead(u) && (u.Role == "Super Admin" || u.Can("documents"))
}
func (a *App) enquiryFileUpload(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if !enquiryDocuments(u) {
		return nil, forbidden()
	}
	ctx := r.Context()
	rid := materialID(r)
	if e := a.limit(ctx, fmt.Sprintf("enquiry-upload:%d", u.ID), 20, time.Minute); e != nil {
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
		return nil, bad("Choose a nonempty file up to 10 MB with a filename up to 180 characters.")
	}
	if strings.EqualFold(filepath.Ext(h.Filename), ".eml") {
		m, err := mail.ReadMessage(bytes.NewReader(raw))
		if err != nil || m.Header.Get("From") == "" {
			return nil, bad("Choose a valid email message file.")
		}
	} else if e = validateUpload(h.Filename, raw); e != nil {
		return nil, e
	}
	category := r.URL.Query().Get("category")
	if !slices.Contains([]string{"Buyer email", "CAD", "Specification", "Supplier quotation", "Buyer confirmation", "Purchase proof", "Other"}, category) {
		return nil, bad("Choose a document category.")
	}
	if (category == "Supplier quotation" || category == "Purchase proof") && (!materialCommercial(u) || !u.Can("enquiry_fabric")) {
		return nil, forbidden()
	}
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	parent, e := a.enquiryRecord(ctx, tx, u, rid, true)
	if e != nil {
		return nil, e
	}
	if category != "Purchase proof" && !slices.Contains([]string{"Draft", "Fabric requested", "Costing", "Changes requested"}, str(parent, "status")) {
		return nil, bad("Documents are locked during and after review. Request an amendment first.")
	}
	var orderID any
	if category == "Purchase proof" {
		if str(parent, "status") != "Booked" {
			return nil, bad("Purchase proof belongs to a confirmed booking.")
		}
		oid, err := strconv.ParseInt(r.URL.Query().Get("order_id"), 10, 64)
		if err != nil {
			return nil, bad("Choose the linked production PO for this proof.")
		}
		if _, err = row(ctx, tx, `SELECT id FROM orders WHERE id=$1 AND enquiry_id=$2`, oid, rid); err != nil {
			return nil, bad("Choose a production PO from this booking.")
		}
		orderID = oid
	} else if category == "Supplier quotation" {
		if str(parent, "status") == "Draft" {
			return nil, forbidden()
		}
	} else if !enquiryWrite(u) {
		return nil, forbidden()
	}
	var count int
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM enquiry_files WHERE enquiry_id=$1`, rid).Scan(&count); e != nil {
		return nil, e
	}
	if count >= 30 {
		return nil, bad("Each enquiry supports up to 30 documents.")
	}
	cipher, e := companyCipher()
	if e != nil {
		return nil, e
	}
	nonce := make([]byte, cipher.NonceSize())
	if _, e = rand.Read(nonce); e != nil {
		return nil, e
	}
	encrypted := cipher.Seal(nonce, nonce, raw, []byte(fmt.Sprintf("enquiry:%d", rid)))
	var fid int64
	e = tx.QueryRow(ctx, `INSERT INTO enquiry_files(enquiry_id,category,name,ciphertext,created_by,order_id) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, rid, category, h.Filename, encrypted, u.ID, orderID).Scan(&fid)
	if e != nil {
		return nil, e
	}
	if _, e = tx.Exec(ctx, `UPDATE enquiries SET version=version+1,updated_at=now() WHERE id=$1`, rid); e != nil {
		return nil, e
	}
	if e = a.enquiryHistory(ctx, tx, u, rid, "Document attached: "+category); e != nil {
		return nil, e
	}
	return M{"id": fid}, tx.Commit(ctx)
}
func (a *App) enquiryFileGet(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if !enquiryDocuments(u) {
		return nil, forbidden()
	}
	p, e := row(r.Context(), a.DB, `SELECT * FROM enquiry_files WHERE id=$1`, materialID(r))
	if e != nil {
		return nil, e
	}
	if _, e = a.enquiryRecord(r.Context(), a.DB, u, id(p, "enquiry_id"), false); e != nil {
		return nil, e
	}
	if slices.Contains([]string{"Supplier quotation", "Purchase proof"}, str(p, "category")) && !materialCommercial(u) {
		return nil, forbidden()
	}
	cipher, e := companyCipher()
	if e != nil {
		return nil, e
	}
	raw, ok := p["ciphertext"].([]byte)
	if !ok || len(raw) < cipher.NonceSize() {
		return nil, bad("Document unavailable.")
	}
	plain, e := cipher.Open(nil, raw[:cipher.NonceSize()], raw[cipher.NonceSize():], []byte(fmt.Sprintf("enquiry:%d", id(p, "enquiry_id"))))
	if e != nil {
		return nil, e
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": str(p, "name")}))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Security-Policy", "sandbox")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Write(plain)
	return nil, nil
}
