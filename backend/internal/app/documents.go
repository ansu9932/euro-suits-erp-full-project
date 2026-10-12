package app

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"github.com/go-chi/chi/v5"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"
)

func (a *App) uploadDocument(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if e := u.Require("documents"); e != nil {
		return nil, e
	}
	oid, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	o, e := a.order(r.Context(), a.DB, u, oid, false)
	if e != nil {
		return nil, e
	}
	if e = a.limit(r.Context(), fmt.Sprintf("upload:%d", u.ID), 20, time.Minute); e != nil {
		return nil, e
	}
	select {
	case a.UploadSlots <- struct{}{}:
		defer func() { <-a.UploadSlots }()
	default:
		w.Header().Set("Retry-After", "5")
		return nil, failure{429, "Uploads are busy. Please retry shortly."}
	}
	r.Body = http.MaxBytesReader(w, r.Body, 21<<20)
	if e = r.ParseMultipartForm(1 << 20); e != nil {
		return nil, bad("Upload a file smaller than 20 MB.")
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	src, h, e := r.FormFile("file")
	if e != nil {
		return nil, bad("Choose a document.")
	}
	defer src.Close()
	name := filepath.Base(h.Filename)
	if len(name) > 180 {
		return nil, bad("Use a shorter file name.")
	}
	ext := strings.ToLower(filepath.Ext(name))
	if !slices.Contains([]string{".pdf", ".docx", ".xlsx", ".csv", ".txt", ".png", ".jpg", ".jpeg", ".dxf", ".dwg", ".plt", ".ai", ".zip"}, ext) {
		return nil, bad("Supported files: PDF, Office documents, images, CSV, TXT, ZIP and CAD files.")
	}
	plain, e := io.ReadAll(io.LimitReader(src, (20<<20)+1))
	if e != nil {
		return nil, e
	}
	if e = validateUpload(name, plain); e != nil {
		return nil, e
	}
	cipher, e := companyCipher()
	if e != nil {
		return nil, e
	}
	nonce := make([]byte, cipher.NonceSize())
	if _, e = rand.Read(nonce); e != nil {
		return nil, e
	}
	encrypted := cipher.Seal(nonce, nonce, plain, []byte(fmt.Sprintf("order:%d", oid)))
	key := token()
	path := filepath.Join(a.FileRoot, key)
	dst, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return nil, e
	}
	keep := false
	defer func() {
		dst.Close()
		if !keep {
			os.Remove(path)
		}
	}()
	hash := sha256.New()
	hash.Write(plain)
	size := int64(len(plain))
	_, e = dst.Write(encrypted)
	if e != nil {
		return nil, e
	}
	if size == 0 || size > 20<<20 {
		return nil, bad("File must be between 1 byte and 20 MB.")
	}
	if e = dst.Sync(); e != nil {
		return nil, e
	}
	if e = dst.Close(); e != nil {
		return nil, e
	}
	ctx := r.Context()
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	o, e = a.order(ctx, tx, u, oid, true)
	if e != nil {
		return nil, e
	}
	if str(o, "stage") == "Dispatched" {
		return nil, bad("Dispatched orders are locked.")
	}
	var docID int64
	e = tx.QueryRow(ctx, `INSERT INTO documents(order_id,name,storage_key,size,sha256,created_by,encryption_version) VALUES($1,$2,$3,$4,$5,$6,1) RETURNING id`, oid, name, key, size, hex.EncodeToString(hash.Sum(nil)), u.ID).Scan(&docID)
	if e != nil {
		return nil, e
	}
	if e = audit(ctx, tx, u, id(o, "company_id"), "Document uploaded", "Order", oid, str(o, "po")+" · "+name); e != nil {
		return nil, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, e
	}
	keep = true
	return M{"id": docID}, nil
}
func (a *App) downloadDocument(w http.ResponseWriter, r *http.Request) (any, error) {
	if !current(r).Can("documents") {
		return nil, forbidden()
	}
	did, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	doc, e := row(r.Context(), a.DB, `SELECT * FROM documents WHERE id=$1`, did)
	if e != nil {
		return nil, e
	}
	if _, e = a.order(r.Context(), a.DB, current(r), id(doc, "order_id"), false); e != nil {
		return nil, e
	}
	key := str(doc, "storage_key")
	if filepath.Base(key) != key || strings.ContainsAny(key, "/\\") {
		return nil, missing()
	}
	file, e := os.Open(filepath.Join(a.FileRoot, key))
	if e != nil {
		return nil, missing()
	}
	defer file.Close()
	info, e := file.Stat()
	if e != nil {
		return nil, e
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": str(doc, "name")}))
	w.Header().Set("Content-Security-Policy", "sandbox")
	if id(doc, "encryption_version") == 1 {
		cipher, e := companyCipher()
		if e != nil {
			return nil, e
		}
		encrypted, e := io.ReadAll(io.LimitReader(file, (21<<20)+1))
		if e != nil {
			return nil, e
		}
		if len(encrypted) < cipher.NonceSize() {
			return nil, missing()
		}
		plain, e := cipher.Open(nil, encrypted[:cipher.NonceSize()], encrypted[cipher.NonceSize():], []byte(fmt.Sprintf("order:%d", id(doc, "order_id"))))
		if e != nil {
			return nil, e
		}
		http.ServeContent(w, r, str(doc, "name"), info.ModTime(), bytes.NewReader(plain))
	} else {
		http.ServeContent(w, r, str(doc, "name"), info.ModTime(), file)
	}
	return nil, nil
}

func (a *App) companySetupUpload(w http.ResponseWriter, r *http.Request) (any, error) {
	if !current(r).SetupCan("documents") {
		return nil, forbidden()
	}
	u := current(r)
	if u.Role != "Finance Head" {
		return nil, forbidden()
	}
	ctx := r.Context()
	c, e := a.setupRow(ctx, a.DB, setupID(r), false)
	if e != nil {
		return nil, e
	}
	if !setupEditable(u, c) {
		return nil, forbidden()
	}
	if e = a.limit(r.Context(), fmt.Sprintf("upload:%d", u.ID), 20, time.Minute); e != nil {
		return nil, e
	}
	select {
	case a.UploadSlots <- struct{}{}:
		defer func() { <-a.UploadSlots }()
	default:
		w.Header().Set("Retry-After", "5")
		return nil, failure{429, "Uploads are busy. Please retry shortly."}
	}
	r.Body = http.MaxBytesReader(w, r.Body, 21<<20)
	if e = r.ParseMultipartForm(1 << 20); e != nil {
		return nil, bad("Choose a file smaller than 20 MB.")
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	src, h, e := r.FormFile("file")
	if e != nil {
		return nil, bad("Choose a document.")
	}
	defer src.Close()
	name := filepath.Base(h.Filename)
	if len(name) > 180 || !slices.Contains([]string{".pdf", ".png", ".jpg", ".jpeg", ".docx", ".xlsx"}, strings.ToLower(filepath.Ext(name))) {
		return nil, bad("Use a PDF, image, Word or Excel file with a short name.")
	}
	plain, e := io.ReadAll(io.LimitReader(src, (20<<20)+1))
	if e != nil {
		return nil, e
	}
	if len(plain) == 0 || len(plain) > 20<<20 {
		return nil, bad("File must be between 1 byte and 20 MB.")
	}
	if e = validateUpload(name, plain); e != nil {
		return nil, e
	}
	stored, compressed, e := prepareCompanyDocumentStorage(ctx, plain)
	if e != nil {
		return nil, e
	}
	cipher, e := companyCipher()
	if e != nil {
		return nil, e
	}
	nonce := make([]byte, cipher.NonceSize())
	if _, e = rand.Read(nonce); e != nil {
		return nil, e
	}
	encrypted := cipher.Seal(nonce, nonce, stored, []byte(fmt.Sprint(id(c, "id"))))
	key := "kyc-" + token()
	if compressed {
		key = companyCompressedKeyPrefix + token()
	}
	path := filepath.Join(a.FileRoot, key)
	if e = os.WriteFile(path, encrypted, 0600); e != nil {
		return nil, e
	}
	keep := false
	defer func() {
		if !keep {
			os.Remove(path)
		}
	}()
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	c, e = a.setupRow(ctx, tx, id(c, "id"), true)
	if e != nil {
		return nil, e
	}
	if !setupEditable(u, c) {
		return nil, forbidden()
	}
	hash := sha256.Sum256(plain)
	var fid int64
	e = tx.QueryRow(ctx, `INSERT INTO company_setup_files(company_id,name,storage_key,size,sha256,created_by) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, id(c, "id"), name, key, len(plain), hex.EncodeToString(hash[:]), u.ID).Scan(&fid)
	if e != nil {
		return nil, e
	}
	if e = setupHistory(ctx, tx, u, c, "Document uploaded", "Protected company document attached"); e != nil {
		return nil, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, e
	}
	keep = true
	result := companyStorageMetadata(int64(len(plain)), int64(len(encrypted)), compressed)
	result["id"] = fid
	return result, nil
}
func (a *App) companySetupDownload(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if !setupRole(u) || !u.SetupCan("documents") {
		return nil, forbidden()
	}
	doc, e := row(r.Context(), a.DB, `SELECT * FROM company_setup_files WHERE id=$1`, setupID(r))
	if e != nil {
		return nil, e
	}
	c, e := a.setupRow(r.Context(), a.DB, id(doc, "company_id"), false)
	if e != nil {
		return nil, e
	}
	if !setupPrivate(u, c) {
		return nil, forbidden()
	}
	key := str(doc, "storage_key")
	if filepath.Base(key) != key {
		return nil, missing()
	}
	encrypted, e := readCompanyEncryptedFile(filepath.Join(a.FileRoot, key))
	if e != nil {
		return nil, missing()
	}
	cipher, e := companyCipher()
	if e != nil {
		return nil, e
	}
	if len(encrypted) < cipher.NonceSize() {
		return nil, bad("Document is damaged.")
	}
	plain, e := cipher.Open(nil, encrypted[:cipher.NonceSize()], encrypted[cipher.NonceSize():], []byte(fmt.Sprint(id(c, "id"))))
	if e != nil {
		return nil, e
	}
	plain, e = restoreCompanyDocumentStorage(r.Context(), plain, id(doc, "size"), str(doc, "sha256"), strings.HasPrefix(key, companyCompressedKeyPrefix))
	if e != nil {
		return nil, e
	}
	name := str(doc, "name")
	preview := r.URL.Query().Get("preview") == "1"
	disposition, contentType := "attachment", "application/octet-stream"
	ext := strings.ToLower(filepath.Ext(name))
	if preview && slices.Contains([]string{".docx", ".xlsx"}, ext) {
		contentType, plain, e = companyOfficeTextPreview(r.Context(), name, plain)
		if e != nil {
			return nil, e
		}
		disposition, name = "inline", name+".preview.txt"
	} else {
		disposition, contentType = companyDocumentPresentation(name, plain, preview)
	}
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition", mime.FormatMediaType(disposition, map[string]string{"filename": name}))
	w.Header().Set("Content-Security-Policy", "sandbox")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, name, time.Time{}, bytes.NewReader(plain))
	return nil, nil
}

// Only previously accepted, content-validated PDF and raster formats render
// inline. Original Office bytes and unexpected content are download-only;
// safe Office text extraction is handled separately before this helper.
func companyDocumentPresentation(name string, plain []byte, preview bool) (string, string) {
	if preview && validateUpload(name, plain) == nil {
		switch strings.ToLower(filepath.Ext(name)) {
		case ".pdf":
			return "inline", "application/pdf"
		case ".png":
			return "inline", "image/png"
		case ".jpg", ".jpeg":
			return "inline", "image/jpeg"
		}
	}
	return "attachment", "application/octet-stream"
}
