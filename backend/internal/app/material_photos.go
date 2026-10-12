package app

import (
	"bytes"
	"crypto/rand"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"mime"
	"net/http"
	"path/filepath"
	"strings"
	"time"
)

func cleanMaterialPhoto(name string, data []byte) ([]byte, string, error) {
	if len(data) > 5<<20 {
		return nil, "", bad("Each fabric photo must be 5 MB or smaller.")
	}
	ext := strings.ToLower(filepath.Ext(name))
	if ext != ".png" && ext != ".jpg" && ext != ".jpeg" {
		return nil, "", bad("Choose a PNG or JPEG fabric photo.")
	}
	if e := validateUpload(name, data); e != nil {
		return nil, "", e
	}
	cfg, _, e := image.DecodeConfig(bytes.NewReader(data))
	if e != nil || int64(cfg.Width)*int64(cfg.Height) > 16000000 {
		return nil, "", bad("Use a photo of at most 16 megapixels.")
	}
	pic, format, e := image.Decode(bytes.NewReader(data))
	if e != nil {
		return nil, "", bad("This photo cannot be decoded.")
	}
	var b bytes.Buffer
	// Re-encode pixels to discard executable trailers and camera metadata.
	contentType := "image/jpeg"
	if format == "png" {
		e = png.Encode(&b, pic)
		contentType = "image/png"
	} else {
		e = jpeg.Encode(&b, pic, &jpeg.Options{Quality: 88})
	}
	if e != nil {
		return nil, "", e
	}
	if b.Len() > 10<<20 {
		return nil, "", bad("Photo is too large after processing. Resize it first.")
	}
	return b.Bytes(), contentType, nil
}
func (a *App) materialPhotoUpload(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if !materialEditor(u, "fabric") {
		return nil, forbidden()
	}
	sid := materialID(r)
	m, e := a.materialSample(r.Context(), a.DB, u, sid, false)
	if e != nil {
		return nil, e
	}
	if str(m, "kind") != "fabric" {
		return nil, bad("Photo uploads are available on fabric samples only.")
	}
	if e = a.limit(r.Context(), fmt.Sprintf("material-photo:%d", u.ID), 20, time.Minute); e != nil {
		return nil, e
	}
	select {
	case a.UploadSlots <- struct{}{}:
		defer func() { <-a.UploadSlots }()
	default:
		return nil, failure{429, "Uploads are busy. Retry shortly."}
	}
	r.Body = http.MaxBytesReader(w, r.Body, 6<<20)
	if e = r.ParseMultipartForm(1 << 20); e != nil {
		return nil, bad("Choose a PNG or JPEG up to 5 MB.")
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	f, h, e := r.FormFile("file")
	if e != nil {
		return nil, bad("Choose a fabric photo.")
	}
	defer f.Close()
	raw, e := io.ReadAll(io.LimitReader(f, (5<<20)+1))
	if e != nil {
		return nil, e
	}
	data, contentType, e := cleanMaterialPhoto(h.Filename, raw)
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
	encrypted := cipher.Seal(nonce, nonce, data, []byte(fmt.Sprintf("material:%d", sid)))
	ctx := r.Context()
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	m, e = a.materialSample(ctx, tx, u, sid, true)
	if e != nil {
		return nil, e
	}
	if m["active"] == false {
		return nil, bad("This sample is archived.")
	}
	if e = materialSupplierAvailable(ctx, tx, id(m, "supplier_setup_id")); e != nil {
		return nil, e
	}
	var count int
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM material_photos WHERE sample_id=$1`, sid).Scan(&count); e != nil {
		return nil, e
	}
	if count >= 12 {
		return nil, bad("A sample can have up to 12 photos.")
	}
	var pid int64
	e = tx.QueryRow(ctx, `INSERT INTO material_photos(sample_id,name,mime,size,ciphertext,created_by) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, sid, h.Filename, contentType, len(data), encrypted, u.ID).Scan(&pid)
	if e != nil {
		return nil, e
	}
	if e = audit(ctx, tx, u, id(m, "company_id"), "Fabric photo uploaded", "Material", sid, str(m, "code")); e != nil {
		return nil, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, e
	}
	return M{"id": pid}, nil
}
func (a *App) materialPhotoGet(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if !materialCommercial(u) {
		return nil, forbidden()
	}
	p, e := row(r.Context(), a.DB, `SELECT * FROM material_photos WHERE id=$1`, materialID(r))
	if e != nil {
		return nil, e
	}
	if _, e = a.materialSample(r.Context(), a.DB, u, id(p, "sample_id"), false); e != nil {
		return nil, e
	}
	cipher, e := companyCipher()
	if e != nil {
		return nil, e
	}
	encrypted, ok := p["ciphertext"].([]byte)
	if !ok || len(encrypted) < cipher.NonceSize() {
		return nil, bad("Photo could not be opened.")
	}
	plain, e := cipher.Open(nil, encrypted[:cipher.NonceSize()], encrypted[cipher.NonceSize():], []byte(fmt.Sprintf("material:%d", id(p, "sample_id"))))
	if e != nil {
		return nil, e
	}
	w.Header().Set("Content-Type", str(p, "mime"))
	w.Header().Set("Content-Disposition", mime.FormatMediaType("inline", map[string]string{"filename": str(p, "name")}))
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Write(plain)
	return nil, nil
}
