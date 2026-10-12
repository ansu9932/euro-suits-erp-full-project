package app

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/xuri/excelize/v2"
)

// Import is a read-only preview. The user maps columns and checks measurements
// before submitting an audit; formulas, macros and external links are not run.
func (a *App) sampleSpecImport(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if !sampleRead(u) || u.Role != "Quality" || !u.Can("quality") || !u.Can("documents") {
		return nil, forbidden()
	}
	ctx := r.Context()
	piece, e := row(ctx, a.DB, `SELECT request_id FROM sample_indents WHERE id=$1`, materialID(r))
	if e != nil {
		return nil, e
	}
	if _, e = a.sampleRecord(ctx, a.DB, u, id(piece, "request_id"), false); e != nil {
		return nil, e
	}
	if e = a.limit(ctx, fmt.Sprintf("sample-import:%d", u.ID), 10, time.Minute); e != nil {
		return nil, e
	}
	select {
	case a.UploadSlots <- struct{}{}:
		defer func() { <-a.UploadSlots }()
	default:
		return nil, failure{429, "Imports are busy. Retry shortly."}
	}
	r.Body = http.MaxBytesReader(w, r.Body, 6<<20)
	if e = r.ParseMultipartForm(1 << 20); e != nil {
		return nil, bad("Choose an XLSX, CSV or text PDF up to 5 MB.")
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	f, h, e := r.FormFile("file")
	if e != nil {
		return nil, bad("Choose a specification sheet.")
	}
	defer f.Close()
	raw, e := io.ReadAll(io.LimitReader(f, (5<<20)+1))
	if e != nil {
		return nil, e
	}
	if len(raw) > 5<<20 {
		return nil, bad("Specification file must be 5 MB or smaller.")
	}
	sheets, e := sampleSpecTables(ctx, h.Filename, raw)
	if e != nil {
		return nil, e
	}
	return M{"sheets": sheets, "note": "Preview only. Select the garment size column, map measurement points and required values, and check every row before importing. The source file is not stored by this preview."}, nil
}
func sampleSpecTables(ctx context.Context, name string, raw []byte) ([]M, error) {
	tables := []M{}
	clean := func(rows [][]string) [][]string {
		for i := range rows {
			if len(rows[i]) > 60 {
				rows[i] = rows[i][:60]
			}
			for j := range rows[i] {
				if len(rows[i][j]) > 500 {
					rows[i][j] = rows[i][j][:500]
				}
			}
		}
		return rows
	}
	switch strings.ToLower(filepath.Ext(name)) {
	case ".xlsx":
		f, e := excelize.OpenReader(bytes.NewReader(raw), excelize.Options{UnzipSizeLimit: 32 << 20, UnzipXMLSizeLimit: 8 << 20})
		if e != nil {
			return nil, bad("Cannot read this Excel file. Use an unencrypted XLSX under 32 MB when expanded.")
		}
		defer f.Close()
		for _, sheet := range f.GetSheetList() {
			if len(tables) >= 10 {
				break
			}
			it, e := f.Rows(sheet)
			if e != nil {
				continue
			}
			table := [][]string{}
			for it.Next() && len(table) < 500 {
				v, err := it.Columns()
				if err != nil {
					it.Close()
					return nil, bad("Cannot read worksheet rows.")
				}
				table = append(table, v)
			}
			it.Close()
			tables = append(tables, M{"name": sheet, "rows": clean(table)})
		}
	case ".csv":
		reader := csv.NewReader(bytes.NewReader(raw))
		reader.FieldsPerRecord = -1
		table := [][]string{}
		for len(table) < 500 {
			v, e := reader.Read()
			if e == io.EOF {
				break
			}
			if e != nil {
				return nil, bad("Cannot read this CSV. Use comma-separated columns with quoted text where needed.")
			}
			table = append(table, v)
		}
		tables = append(tables, M{"name": "CSV", "rows": clean(table)})
	case ".pdf":
		if !bytes.HasPrefix(raw, []byte("%PDF-")) {
			return nil, bad("Choose a valid PDF.")
		}
		dir, e := os.MkdirTemp("", "erp-spec-")
		if e != nil {
			return nil, e
		}
		defer os.RemoveAll(dir)
		source, output := filepath.Join(dir, "spec.pdf"), filepath.Join(dir, "spec.txt")
		if e = os.WriteFile(source, raw, 0600); e != nil {
			return nil, e
		}
		bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		if e = exec.CommandContext(bounded, "pdftotext", "-f", "1", "-l", "20", "-layout", "-enc", "UTF-8", source, output).Run(); e != nil {
			return nil, bad("PDF text extraction is unavailable or this PDF cannot be read. Use XLSX/CSV, or enter measurements manually.")
		}
		stat, e := os.Stat(output)
		if e != nil || stat.Size() > 2<<20 {
			return nil, bad("PDF contains too much text. Upload only the specification pages.")
		}
		text, e := os.ReadFile(output)
		if e != nil {
			return nil, e
		}
		split := regexp.MustCompile(`\s{2,}`)
		table := [][]string{}
		for _, line := range strings.Split(string(text), "\n") {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}
			table = append(table, split.Split(line, -1))
			if len(table) >= 500 {
				break
			}
		}
		if len(table) == 0 {
			return nil, bad("This PDF has no readable text (it may be a scanned image). Upload the original XLSX/CSV or enter measurements manually.")
		}
		tables = append(tables, M{"name": "PDF text — verify column alignment", "rows": clean(table)})
	default:
		return nil, bad("Use XLSX, CSV or a PDF with selectable text.")
	}
	return tables, nil
}
