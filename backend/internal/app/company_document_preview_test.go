package app

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"unicode/utf8"
)

type companyPreviewEntry struct{ name, body string }

func companyPreviewArchive(t *testing.T, entries ...companyPreviewEntry) []byte {
	t.Helper()
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	for _, entry := range entries {
		writer, err := archive.CreateHeader(&zip.FileHeader{Name: entry.name, Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write([]byte(entry.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
func companyPreviewWordArchive(t *testing.T, document string) []byte {
	return companyPreviewArchive(t, companyPreviewEntry{"[Content_Types].xml", "<Types/>"}, companyPreviewEntry{"word/document.xml", document})
}
func companyPreviewWorkbookEntries(sheets int, worksheet string) []companyPreviewEntry {
	workbook, relations := "<workbook><sheets>", "<Relationships>"
	entries := []companyPreviewEntry{{"[Content_Types].xml", "<Types/>"}, {"xl/sharedStrings.xml", "<sst><si><t>Buyer &amp; Co</t></si><si><r><t>Rich </t></r><r><t>text</t></r></si></sst>"}}
	for i := 1; i <= sheets; i++ {
		workbook += fmt.Sprintf(`<sheet name="Sheet %d" id="rId%d"/>`, i, i)
		relations += fmt.Sprintf(`<Relationship Id="rId%d" Target="worksheets/sheet%d.xml"/>`, i, i)
		entries = append(entries, companyPreviewEntry{fmt.Sprintf("xl/worksheets/sheet%d.xml", i), worksheet})
	}
	return append(entries, companyPreviewEntry{"xl/workbook.xml", workbook + "</sheets></workbook>"}, companyPreviewEntry{"xl/_rels/workbook.xml.rels", relations + "</Relationships>"})
}

func TestCompanyOfficePreviewWordText(t *testing.T) {
	data := companyPreviewWordArchive(t, `<w:document xmlns:w="urn:word"><w:body><w:p><w:r><w:t>GST &amp; address</w:t><w:tab/><w:t>বাংলা</w:t><w:br/><w:t>&lt;script&gt;alert(1)&lt;/script&gt;</w:t></w:r></w:p><w:p><w:r><w:t>Next paragraph</w:t></w:r></w:p></w:body></w:document>`)
	contentType, body, err := companyOfficeTextPreview(context.Background(), "proof.docx", data)
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "text/plain; charset=utf-8" || !bytes.Contains(body, []byte("GST & address\tবাংলা\n<script>alert(1)</script>\nNext paragraph")) || bytes.Contains(body, []byte("<w:")) {
		t.Fatalf("unexpected inert text preview: %s %s", contentType, body)
	}
}

func TestCompanyOfficePreviewWorkbookValues(t *testing.T) {
	worksheet := `<worksheet><sheetData><row r="42"><c r="A42" t="s"><v>0</v></c><c r="B42" t="inlineStr"><is><t>&lt;b&gt;plain&lt;/b&gt;</t></is></c><c r="D42"><f>2+3</f><v>5</v></c><c r="E42" t="b"><v>1</v></c><c r="F42"><f>WEBSERVICE("https://invalid.test")</f></c><c r="G42" t="s"><v>1</v></c></row></sheetData></worksheet>`
	data := companyPreviewArchive(t, companyPreviewWorkbookEntries(1, worksheet)...)
	contentType, body, err := companyOfficeTextPreview(context.Background(), "invoice.XLSX", data)
	if err != nil {
		t.Fatal(err)
	}
	if contentType != "text/plain; charset=utf-8" || !bytes.Contains(body, []byte("Row 42\tBuyer & Co\t<b>plain</b>\t\t5\tTRUE\t[Formula: no cached value]\tRich text")) || bytes.Contains(body, []byte("https://invalid.test")) {
		t.Fatalf("unexpected preview: %s", body)
	}
}

func TestCompanyOfficePreviewWorkbookLimits(t *testing.T) {
	worksheet := "<worksheet><sheetData>"
	for i := 1; i <= 101; i++ {
		worksheet += fmt.Sprintf(`<row r="%d"><c r="A%d" t="inlineStr"><is><t>VALUE_ROW_%d</t></is></c><c r="U%d" t="inlineStr"><is><t>OMIT_COLUMN_U</t></is></c></row>`, i, i, i, i)
	}
	worksheet += "</sheetData></worksheet>"
	_, body, err := companyOfficeTextPreview(context.Background(), "invoice.xlsx", companyPreviewArchive(t, companyPreviewWorkbookEntries(4, worksheet)...))
	if err != nil {
		t.Fatal(err)
	}
	for _, omitted := range []string{"VALUE_ROW_101", "OMIT_COLUMN_U", "WORKSHEET: Sheet 4"} {
		if bytes.Contains(body, []byte(omitted)) {
			t.Fatalf("preview exceeded bounds: %s", omitted)
		}
	}
	if !bytes.Contains(body, []byte("VALUE_ROW_100")) || !bytes.Contains(body, []byte("Preview shortened")) {
		t.Fatal("preview truncation not explicit")
	}
}

func TestCompanyOfficePreviewTextLimitAndUTF8(t *testing.T) {
	doc := `<document><p><t>` + strings.Repeat("界", 80000) + `</t></p></document>`
	_, body, err := companyOfficeTextPreview(context.Background(), "proof.docx", companyPreviewWordArchive(t, doc))
	if err != nil {
		t.Fatal(err)
	}
	if len(body) > companyPreviewTextLimit || !utf8.Valid(body) || !bytes.Contains(body, []byte("Preview shortened")) {
		t.Fatal("preview text was not bounded valid UTF-8")
	}
}

func TestCompanyOfficePreviewRejectsUnsafeOrMalformedInput(t *testing.T) {
	valid := `<document><p><t>safe</t></p></document>`
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"not zip", []byte("invalid")},
		{"traversal", companyPreviewArchive(t, companyPreviewEntry{"../word/document.xml", valid})},
		{"duplicate", companyPreviewArchive(t, companyPreviewEntry{"word/document.xml", valid}, companyPreviewEntry{"word/document.xml", valid})},
		{"backslash", companyPreviewArchive(t, companyPreviewEntry{`word\document.xml`, valid})},
		{"doctype", companyPreviewWordArchive(t, `<!DOCTYPE document [<!ENTITY private SYSTEM "file:///etc/passwd">]><document><p><t>&private;</t></p></document>`)},
		{"malformed xml", companyPreviewWordArchive(t, `<document><p><t>broken</p></document>`)},
		{"xml size", companyPreviewWordArchive(t, `<document><p><t>`+strings.Repeat("x", companyPreviewXMLLimit)+`</t></p></document>`)},
		{"missing document", companyPreviewArchive(t, companyPreviewEntry{"[Content_Types].xml", "<Types/>"})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := companyOfficeTextPreview(context.Background(), "proof.docx", tc.data); err == nil {
				t.Fatal("unsafe input accepted")
			}
		})
	}
	data := companyPreviewWordArchive(t, valid)
	index := bytes.Index(data, []byte("safe"))
	data[index] = 'X'
	if _, _, err := companyOfficeTextPreview(context.Background(), "proof.docx", data); err == nil {
		t.Fatal("corrupt CRC accepted")
	}
}

func TestCompanyOfficePreviewRejectsExternalWorksheetAndBadStringIndex(t *testing.T) {
	entries := companyPreviewWorkbookEntries(1, `<worksheet><sheetData><row><c r="A1" t="s"><v>999</v></c></row></sheetData></worksheet>`)
	if _, _, err := companyOfficeTextPreview(context.Background(), "proof.xlsx", companyPreviewArchive(t, entries...)); err == nil {
		t.Fatal("invalid shared-string reference accepted")
	}
	for i := range entries {
		if entries[i].name == "xl/_rels/workbook.xml.rels" {
			entries[i].body = `<Relationships><Relationship Id="rId1" Target="https://invalid.test/private" TargetMode="External"/></Relationships>`
		}
	}
	if _, _, err := companyOfficeTextPreview(context.Background(), "proof.xlsx", companyPreviewArchive(t, entries...)); err == nil {
		t.Fatal("external worksheet accepted")
	}
}

func TestCompanyOfficePreviewCancellationAndArchiveBudget(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := companyOfficeTextPreview(ctx, "proof.docx", companyPreviewWordArchive(t, `<document/>`)); err != context.Canceled {
		t.Fatalf("cancellation not honored: %v", err)
	}
	var buffer bytes.Buffer
	archive := zip.NewWriter(&buffer)
	writer, err := archive.Create("word/document.xml")
	if err != nil {
		t.Fatal(err)
	}
	writer.Write([]byte(strings.Repeat("A", 3<<20)))
	archive.Close()
	if _, _, err := companyOfficeTextPreview(context.Background(), "proof.docx", buffer.Bytes()); err == nil {
		t.Fatal("ZIP expansion bomb accepted")
	}
	entries := make([]companyPreviewEntry, 1001)
	for i := range entries {
		entries[i] = companyPreviewEntry{fmt.Sprintf("f%d.xml", i), "<x/>"}
	}
	if _, _, err := companyOfficeTextPreview(context.Background(), "proof.docx", companyPreviewArchive(t, entries...)); err == nil {
		t.Fatal("unbounded archive entry count accepted")
	}
}
