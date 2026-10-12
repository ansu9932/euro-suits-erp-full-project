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

func (a *App) sampleFileUpload(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if !sampleRead(u) || !u.Can("documents") {
		return nil, forbidden()
	}
	rid := materialID(r)
	ctx := r.Context()
	if e := a.limit(ctx, fmt.Sprintf("sample-upload:%d", u.ID), 20, time.Minute); e != nil {
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
	if len(raw) > 10<<20 {
		return nil, bad("File must be 10 MB or smaller.")
	}
	if strings.EqualFold(filepath.Ext(h.Filename), ".eml") {
		if len(h.Filename) > 180 {
			return nil, bad("Filename is too long.")
		}
		message, err := mail.ReadMessage(bytes.NewReader(raw))
		if err != nil || message.Header.Get("From") == "" {
			return nil, bad("Choose a valid email message file.")
		}
	} else if e = validateUpload(h.Filename, raw); e != nil {
		return nil, e
	}
	category := r.URL.Query().Get("category")
	if !slices.Contains([]string{"Buyer email", "Buyer specification", "Sample to copy", "CAD", "Labels", "Audit sheet", "Fabric swatch", "Trim card", "Sample images", "Dispatch", "Buyer feedback", "Other"}, category) {
		return nil, bad("Choose a document category.")
	}
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	parent, e := a.sampleRecord(ctx, tx, u, rid, true)
	if e != nil {
		return nil, e
	}
	if u.Role == "Sampling" && str(parent, "status") != "Approved" && id(parent, "created_by") != u.ID {
		return nil, forbidden()
	}
	if str(parent, "status") == "Submitted" {
		return nil, bad("Wait for MD review before adding documents.")
	}
	if str(parent, "status") != "Approved" && !sampleWriter(u) {
		return nil, forbidden()
	}
	if str(parent, "status") == "Approved" && !(sampleProduction(u) || u.Role == "Development" && u.Can("develop") && category == "CAD" || u.Role == "Quality" && u.Can("quality") && slices.Contains([]string{"Audit sheet", "Fabric swatch", "Trim card", "Sample images"}, category) || u.Role == "Marketing" && slices.Contains([]string{"Dispatch", "Buyer feedback"}, category)) {
		return nil, forbidden()
	}

	var indent any
	if value := r.URL.Query().Get("indent_id"); value != "" {
		iid, err := strconv.ParseInt(value, 10, 64)
		if err != nil || iid < 1 {
			return nil, bad("Choose a valid indent.")
		}
		if _, err = row(ctx, tx, `SELECT id FROM sample_indents WHERE id=$1 AND request_id=$2`, iid, rid); err != nil {
			return nil, bad("Indent does not belong to this request.")
		}
		indent = iid
	}
	var count int
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM sample_request_files WHERE request_id=$1 AND indent_id IS NOT DISTINCT FROM $2::bigint`, rid, indent).Scan(&count); e != nil {
		return nil, e
	}
	if count >= 30 {
		return nil, bad("Each request or indent supports up to 30 documents.")
	}
	cipher, e := companyCipher()
	if e != nil {
		return nil, e
	}
	nonce := make([]byte, cipher.NonceSize())
	if _, e = rand.Read(nonce); e != nil {
		return nil, e
	}
	encrypted := cipher.Seal(nonce, nonce, raw, []byte(fmt.Sprintf("sample-request:%d", rid)))
	var fid int64
	e = tx.QueryRow(ctx, `INSERT INTO sample_request_files(request_id,category,name,ciphertext,created_by,indent_id) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, rid, category, h.Filename, encrypted, u.ID, indent).Scan(&fid)
	if e != nil {
		return nil, e
	}
	if e = a.sampleHistory(ctx, tx, u, rid, "Document uploaded", M{"id": fid, "name": h.Filename, "category": category, "indent_id": indent}); e != nil {
		return nil, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, e
	}
	return M{"id": fid}, nil
}
func (a *App) sampleFileGet(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if !sampleRead(u) || !u.Can("documents") {
		return nil, forbidden()
	}
	p, e := row(r.Context(), a.DB, `SELECT * FROM sample_request_files WHERE id=$1`, materialID(r))
	if e != nil {
		return nil, e
	}
	parent, e := a.sampleRecord(r.Context(), a.DB, u, id(p, "request_id"), false)
	if e != nil {
		return nil, e
	}
	if str(parent, "status") != "Approved" && !(u.Role == "Marketing" || u.Role == "Super Admin" || sampleApprove(u) || id(parent, "created_by") == u.ID) {
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
	plain, e := cipher.Open(nil, raw[:cipher.NonceSize()], raw[cipher.NonceSize():], []byte(fmt.Sprintf("sample-request:%d", id(p, "request_id"))))
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
