package app

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"io/fs"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	companyPreviewXMLLimit    = 2 << 20
	companyPreviewTextLimit   = 200 << 10
	companyPreviewSheetLimit  = 3
	companyPreviewRowLimit    = 100
	companyPreviewColumnLimit = 20
)

// Office previews are inert, bounded text extracts. Formatting, formulas, macros,
// linked resources and embedded objects are never rendered or executed.
func companyOfficeTextPreview(ctx context.Context, name string, plain []byte) (string, []byte, error) {
	ext := strings.ToLower(filepath.Ext(name))
	if ext != ".docx" && ext != ".xlsx" {
		return "", nil, bad("Text preview is available for DOCX and XLSX documents only.")
	}
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	files, err := companyPreviewXMLFiles(ctx, plain)
	if err != nil {
		return "", nil, err
	}
	if files["[Content_Types].xml"] == nil {
		return "", nil, bad("Cannot preview this Office document. Download the original file.")
	}
	out := &companyPreviewText{limit: companyPreviewTextLimit - 2048}
	if ext == ".docx" {
		err = companyPreviewWord(ctx, files["word/document.xml"], out)
	} else {
		err = companyPreviewWorkbook(ctx, files, out)
	}
	if err != nil {
		return "", nil, err
	}
	body := "DOCUMENT TEXT PREVIEW\nFormatting and images are omitted. Download the original for the complete document.\n"
	if ext == ".xlsx" {
		body += "Up to 3 sheets, 100 rows per sheet and columns A–T. Values are cached text; formulas are not calculated.\n"
	}
	body += "\n" + strings.TrimSpace(out.String())
	if out.truncated {
		body += "\n\n[Preview shortened. Download the original for all content.]"
	}
	if strings.TrimSpace(out.String()) == "" {
		body += "\nNo readable text was found. Download the original to view images or formatting."
	}
	return "text/plain; charset=utf-8", []byte(body + "\n"), nil
}

type companyPreviewContextReader struct {
	ctx    context.Context
	reader io.Reader
}

func (r companyPreviewContextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(p)
}

func companyPreviewXMLFiles(ctx context.Context, plain []byte) (map[string][]byte, error) {
	invalid := bad("Cannot preview this Office document safely. Download the original file.")
	if len(plain) == 0 || len(plain) > 20<<20 {
		return nil, invalid
	}
	archive, err := zip.NewReader(bytes.NewReader(plain), int64(len(plain)))
	if err != nil || len(archive.File) > 1000 {
		return nil, invalid
	}
	seen := map[string]bool{}
	var expanded, xmlSize uint64
	for _, file := range archive.File {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		clean := path.Clean(file.Name)
		if strings.ContainsAny(file.Name, "\\\x00") || clean != strings.TrimSuffix(file.Name, "/") || strings.HasPrefix(clean, "/") || clean == ".." || strings.HasPrefix(clean, "../") || strings.Contains(clean, ":") || seen[clean] || file.Flags&1 != 0 || file.Mode()&fs.ModeSymlink != 0 {
			return nil, invalid
		}
		seen[clean] = true
		if file.CompressedSize64 > uint64(len(plain)) || file.UncompressedSize64 > 20<<20 || file.UncompressedSize64 > 1000*(file.CompressedSize64+1) || file.UncompressedSize64 > (80<<20)-expanded {
			return nil, invalid
		}
		expanded += file.UncompressedSize64
		if !file.FileInfo().IsDir() && (strings.HasSuffix(file.Name, ".xml") || strings.HasSuffix(file.Name, ".rels")) {
			if file.UncompressedSize64 > companyPreviewXMLLimit-xmlSize {
				return nil, bad("This document is too large for text preview. Download the original file.")
			}
			xmlSize += file.UncompressedSize64
		}
	}
	files := map[string][]byte{}
	for _, file := range archive.File {
		if file.FileInfo().IsDir() || !(strings.HasSuffix(file.Name, ".xml") || strings.HasSuffix(file.Name, ".rels")) {
			continue
		}
		reader, err := file.Open()
		if err != nil {
			return nil, invalid
		}
		data, err := io.ReadAll(io.LimitReader(companyPreviewContextReader{ctx, reader}, int64(file.UncompressedSize64)+1))
		reader.Close()
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, invalid
		}
		if uint64(len(data)) != file.UncompressedSize64 {
			return nil, invalid
		}
		files[file.Name] = data
	}
	return files, nil
}

type companyPreviewText struct {
	strings.Builder
	limit     int
	truncated bool
}

func (out *companyPreviewText) add(value string) {
	remaining := out.limit - out.Len()
	if len(value) > remaining {
		out.truncated = true
		if remaining <= 0 {
			return
		}
		value = value[:remaining]
		for !utf8.ValidString(value) {
			value = value[:len(value)-1]
		}
	}
	out.WriteString(value)
}

func companyPreviewTokens(ctx context.Context, data []byte, visit func(xml.Token) error) error {
	if len(data) == 0 {
		return bad("Cannot read this document's text. Download the original file.")
	}
	decoder := xml.NewDecoder(bytes.NewReader(data))
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		token, err := decoder.Token()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return bad("Cannot read this document's text. Download the original file.")
		}
		if _, directive := token.(xml.Directive); directive {
			return bad("Document directives are not supported in text previews.")
		}
		if err := visit(token); err != nil {
			return err
		}
	}
}

func companyPreviewWord(ctx context.Context, data []byte, out *companyPreviewText) error {
	textDepth := 0
	return companyPreviewTokens(ctx, data, func(token xml.Token) error {
		switch token := token.(type) {
		case xml.StartElement:
			switch token.Name.Local {
			case "t":
				textDepth++
			case "tab":
				out.add("\t")
			case "br", "cr":
				out.add("\n")
			}
		case xml.EndElement:
			switch token.Name.Local {
			case "t":
				textDepth--
			case "p":
				out.add("\n")
			case "tc":
				out.add("\t")
			}
		case xml.CharData:
			if textDepth > 0 {
				out.add(string(token))
			}
		}
		return nil
	})
}

func companyPreviewAttribute(element xml.StartElement, key string) string {
	for _, attribute := range element.Attr {
		if attribute.Name.Local == key {
			return attribute.Value
		}
	}
	return ""
}

type companyPreviewSheet struct{ name, relation string }

func companyPreviewWorkbook(ctx context.Context, files map[string][]byte, out *companyPreviewText) error {
	sheets := []companyPreviewSheet{}
	if err := companyPreviewTokens(ctx, files["xl/workbook.xml"], func(token xml.Token) error {
		if element, ok := token.(xml.StartElement); ok && element.Name.Local == "sheet" {
			if len(sheets) < companyPreviewSheetLimit {
				sheets = append(sheets, companyPreviewSheet{companyPreviewAttribute(element, "name"), companyPreviewAttribute(element, "id")})
			} else {
				out.truncated = true
			}
		}
		return nil
	}); err != nil {
		return err
	}
	relations := map[string]string{}
	if err := companyPreviewTokens(ctx, files["xl/_rels/workbook.xml.rels"], func(token xml.Token) error {
		element, ok := token.(xml.StartElement)
		if !ok || element.Name.Local != "Relationship" {
			return nil
		}
		if strings.EqualFold(companyPreviewAttribute(element, "TargetMode"), "External") {
			return nil
		}
		target := companyPreviewAttribute(element, "Target")
		if strings.ContainsAny(target, "\\\x00") {
			return nil
		}
		if strings.HasPrefix(target, "/") {
			target = strings.TrimPrefix(target, "/")
		} else {
			target = path.Join("xl", target)
		}
		target = path.Clean(target)
		if strings.HasPrefix(target, "xl/worksheets/") && strings.HasSuffix(target, ".xml") {
			relations[companyPreviewAttribute(element, "Id")] = target
		}
		return nil
	}); err != nil {
		return err
	}
	shared := []string{}
	if raw, found := files["xl/sharedStrings.xml"]; found {
		var value strings.Builder
		insideItem, insideText := false, false
		if err := companyPreviewTokens(ctx, raw, func(token xml.Token) error {
			switch token := token.(type) {
			case xml.StartElement:
				if token.Name.Local == "si" {
					insideItem = true
					value.Reset()
				}
				if token.Name.Local == "t" {
					insideText = true
				}
			case xml.CharData:
				if insideItem && insideText {
					value.Write(token)
				}
			case xml.EndElement:
				if token.Name.Local == "t" {
					insideText = false
				}
				if token.Name.Local == "si" {
					if len(shared) >= 50000 {
						return bad("This workbook has too many text values for preview. Download the original file.")
					}
					shared = append(shared, value.String())
					insideItem = false
				}
			}
			return nil
		}); err != nil {
			return err
		}
	}
	for _, sheet := range sheets {
		target := relations[sheet.relation]
		if target == "" {
			return bad("Cannot preview a linked or unsupported worksheet. Download the original file.")
		}
		out.add("\nWORKSHEET: " + sheet.name + "\n")
		if err := companyPreviewWorksheet(ctx, files[target], shared, out); err != nil {
			return err
		}
	}
	return nil
}

func companyPreviewColumn(reference string, fallback int) int {
	column := 0
	for _, char := range reference {
		if char < 'A' || char > 'Z' {
			break
		}
		column = column*26 + int(char-'A'+1)
		if column > companyPreviewColumnLimit {
			return companyPreviewColumnLimit
		}
	}
	if column == 0 {
		return fallback
	}
	return column - 1
}

func companyPreviewWorksheet(ctx context.Context, data []byte, shared []string, out *companyPreviewText) error {
	rowCount, rowLabel, column, nextColumn, lastColumn := 0, 0, 0, 0, -1
	cells := make([]string, companyPreviewColumnLimit)
	inRow, inCell, inValue, inText, hasFormula := false, false, false, false, false
	var value, inline strings.Builder
	cellType := ""
	return companyPreviewTokens(ctx, data, func(token xml.Token) error {
		switch token := token.(type) {
		case xml.StartElement:
			switch token.Name.Local {
			case "row":
				rowCount++
				rowLabel = rowCount
				if parsed, err := strconv.Atoi(companyPreviewAttribute(token, "r")); err == nil && parsed > 0 && parsed <= 1048576 {
					rowLabel = parsed
				}
				inRow = rowCount <= companyPreviewRowLimit
				nextColumn = 0
				lastColumn = -1
				clear(cells)
				if !inRow {
					out.truncated = true
				}
			case "c":
				inCell = inRow
				column = companyPreviewColumn(companyPreviewAttribute(token, "r"), nextColumn)
				nextColumn = column + 1
				if inCell && column >= companyPreviewColumnLimit {
					out.truncated = true
					inCell = false
				}
				cellType = companyPreviewAttribute(token, "t")
				value.Reset()
				inline.Reset()
				hasFormula = false
			case "v":
				inValue = true
			case "t":
				inText = true
			case "f":
				hasFormula = true
			}
		case xml.CharData:
			if inCell && inValue {
				value.Write(token)
			}
			if inCell && inText {
				inline.Write(token)
			}
		case xml.EndElement:
			switch token.Name.Local {
			case "v":
				inValue = false
			case "t":
				inText = false
			case "c":
				if inCell {
					text := value.String()
					switch cellType {
					case "s":
						index, err := strconv.Atoi(text)
						if err != nil || index < 0 || index >= len(shared) {
							return bad("Cannot read a worksheet text value. Download the original file.")
						}
						text = shared[index]
					case "inlineStr":
						text = inline.String()
					case "b":
						if text == "1" {
							text = "TRUE"
						} else if text == "0" {
							text = "FALSE"
						}
					}
					if hasFormula && text == "" {
						text = "[Formula: no cached value]"
					}
					text = strings.NewReplacer("\t", " ", "\r", " ", "\n", " ").Replace(text)
					if len(text) > 4096 {
						text = text[:4096]
						for !utf8.ValidString(text) {
							text = text[:len(text)-1]
						}
						text += "…"
						out.truncated = true
					}
					cells[column] = text
					if column > lastColumn {
						lastColumn = column
					}
				}
				inCell = false
			case "row":
				if inRow {
					out.add(fmt.Sprintf("Row %d\t", rowLabel) + strings.Join(cells[:lastColumn+1], "\t") + "\n")
				}
				inRow = false
			}
		}
		return nil
	})
}
