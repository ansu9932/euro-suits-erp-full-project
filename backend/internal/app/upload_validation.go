package app

import (
	"archive/zip"
	"bytes"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"io/fs"
	"path"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// Files are never executed or unpacked by the app and are always downloaded as
// attachments. Signature checks supplement (not replace) the extension list.
func validateUpload(name string, data []byte) error {
	if name == "" || len(name) > 180 || strings.ContainsAny(name, "\x00\r\n/\\") || strings.HasPrefix(name, ".") {
		return bad("Use a short, plain file name.")
	}
	if len(data) == 0 || len(data) > 20<<20 {
		return bad("File must be between 1 byte and 20 MB.")
	}
	ext := strings.ToLower(filepath.Ext(name))
	mismatch := bad("The file contents do not match its supported file type.")
	switch ext {
	case ".pdf":
		if !bytes.HasPrefix(data, []byte("%PDF-")) {
			return mismatch
		}
	case ".png", ".jpg", ".jpeg":
		config, format, e := image.DecodeConfig(bytes.NewReader(data))
		if e != nil || config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 40000000 {
			return mismatch
		}
		if ext == ".png" && format != "png" || ext != ".png" && format != "jpeg" {
			return mismatch
		}
	case ".docx", ".xlsx", ".zip":
		z, e := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if e != nil || len(z.File) > 1000 {
			return mismatch
		}
		var total uint64
		names := map[string]bool{}
		for _, f := range z.File {
			clean := path.Clean(strings.ReplaceAll(f.Name, "\\", "/"))
			if strings.HasPrefix(clean, "/") || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, ":") || names[clean] || f.Flags&1 != 0 || f.Mode()&fs.ModeSymlink != 0 {
				return bad("Archive contains unsafe or duplicate entries.")
			}
			names[clean] = true
			total += f.UncompressedSize64
			if total > 80<<20 || f.UncompressedSize64 > 20<<20 || f.UncompressedSize64 > 1000*(f.CompressedSize64+1) {
				return bad("Archive expands beyond the permitted size.")
			}
			lower := strings.ToLower(clean)
			entryExt := strings.ToLower(path.Ext(clean))
			if strings.Contains(lower, "vbaproject") || strings.Contains(lower, "/embeddings/") || strings.Contains("|.exe|.dll|.bat|.cmd|.com|.msi|.ps1|.sh|.js|.html|.svg|.jar|.zip|.xlsm|.docm|", "|"+entryExt+"|") {
				return bad("Executable, macro or nested archive content is not allowed.")
			}
			if !f.FileInfo().IsDir() {
				reader, e := f.Open()
				if e != nil {
					return mismatch
				}
				_, e = io.Copy(io.Discard, io.LimitReader(reader, int64(f.UncompressedSize64)+1))
				reader.Close()
				if e != nil {
					return mismatch
				}
			}
		}
		if ext == ".docx" && (!names["[Content_Types].xml"] || !names["word/document.xml"]) {
			return mismatch
		}
		if ext == ".xlsx" && (!names["[Content_Types].xml"] || !names["xl/workbook.xml"]) {
			return mismatch
		}
	case ".txt", ".csv", ".dxf", ".plt":
		if !utf8.Valid(data) || bytes.ContainsRune(data, 0) {
			return mismatch
		}
		lower := strings.ToLower(strings.TrimSpace(string(data[:min(len(data), 512)])))
		if strings.Contains(lower, "<html") || strings.Contains(lower, "<script") || strings.Contains(lower, "<!doctype html") {
			return mismatch
		}
	case ".dwg":
		if !bytes.HasPrefix(data, []byte("AC10")) {
			return mismatch
		}
	case ".ai":
		if !bytes.HasPrefix(data, []byte("%PDF-")) && !bytes.HasPrefix(data, []byte("%!PS-Adobe")) {
			return mismatch
		}
	default:
		return bad("This file type is not allowed.")
	}
	return nil
}
