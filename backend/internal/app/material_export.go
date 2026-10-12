package app

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

func simpleWorkbook(headers []string, records [][]string) ([]byte, error) {
	var out bytes.Buffer
	z := zip.NewWriter(&out)
	parts := map[string]string{
		"[Content_Types].xml":        `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/></Types>`,
		"_rels/.rels":                `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`,
		"xl/workbook.xml":            `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Samples" sheetId="1" r:id="rId1"/></sheets></workbook>`,
		"xl/_rels/workbook.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`,
	}
	var sheet strings.Builder
	sheet.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetViews><sheetView workbookViewId="0"><pane ySplit="1" topLeftCell="A2" state="frozen"/></sheetView></sheetViews><sheetData>`)
	for i, row := range append([][]string{headers}, records...) {
		fmt.Fprintf(&sheet, `<row r="%d">`, i+1)
		for _, value := range row {
			sheet.WriteString(`<c t="inlineStr"><is><t xml:space="preserve">`)
			xml.EscapeText(&sheet, []byte(value))
			sheet.WriteString(`</t></is></c>`)
		}
		sheet.WriteString(`</row>`)
	}
	sheet.WriteString(`</sheetData></worksheet>`)
	parts["xl/worksheets/sheet1.xml"] = sheet.String()
	for name, body := range parts {
		f, e := z.Create(name)
		if e != nil {
			return nil, e
		}
		if _, e = f.Write([]byte(body)); e != nil {
			return nil, e
		}
	}
	if e := z.Close(); e != nil {
		return nil, e
	}
	return out.Bytes(), nil
}
func (a *App) materialExport(w http.ResponseWriter, r *http.Request) (any, error) {
	result, e := a.materialList(w, r)
	if e != nil {
		return nil, e
	}
	d := result.(M)
	u := current(r)
	labels := map[int64]string{}
	for _, o := range d["options"].([]M) {
		labels[id(o, "id")] = str(o, "label")
	}
	wanted := map[int64]bool{}
	query := r.URL.Query().Get("ids")
	if len(query) > 20000 {
		return nil, bad("Select fewer samples for one export.")
	}
	for _, s := range strings.Split(query, ",") {
		n, _ := strconv.ParseInt(s, 10, 64)
		if n > 0 {
			wanted[n] = true
		}
	}
	headers := []string{"Fabric reference", "Category", "Weave", "Composition", "Weight", "Width", "Colour"}
	if materialCommercial(u) {
		headers = []string{"Reference", "Type", "Supplier ID", "Supplier name"}
		for _, f := range materialFields {
			headers = append(headers, f.Label)
		}
		headers = append(headers, "Measurements", "Additional details", "Price recorded at", "Trim type", "Trim size", "Individual unit", "Units per pack", "Pack unit")
	}
	supplierNames := map[int64]string{}
	for _, supplier := range d["suppliers"].([]M) {
		supplierNames[id(supplier, "id")] = str(supplier, "legal_name")
	}
	records := [][]string{}
	for _, s := range d["samples"].([]M) {
		if len(wanted) > 0 && !wanted[id(s, "id")] {
			continue
		}
		p := materialProfile(s["profile"])
		row := []string{str(s, "code")}
		if materialCommercial(u) {
			row = append(row, str(s, "kind"), fmt.Sprint(s["supplier_setup_id"]), supplierNames[id(s, "supplier_setup_id")])
			for _, f := range materialFields {
				value := fmt.Sprint(p[f.Key])
				if f.Type == "option" {
					value = labels[id(p, f.Key)]
				}
				row = append(row, value)
			}
			row = append(row, fmt.Sprint(p["measurements"]), fmt.Sprint(p["additional"]), fmt.Sprint(p["price_recorded_at"]))
			for _, key := range []string{"trim_type", "size", "unit", "units_per_pack", "pack_unit"} {
				row = append(row, str(p, key))
			}
		} else {
			row = append(row, labels[id(p, "category")], labels[id(p, "weave")], labels[id(p, "composition")], materialMeasureText(p, "weight"), materialMeasureText(p, "width"), labels[id(p, "color")])
		}
		records = append(records, row)
	}
	b, e := simpleWorkbook(headers, records)
	if e != nil {
		return nil, e
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="material-samples.xlsx"`)
	w.Write(b)
	return nil, nil
}
func materialMeasureText(p M, key string) string {
	out := []string{}
	b := materialProfile(M{"rows": p["measurements"]})
	values, _ := b["rows"].([]any)
	for _, v := range values {
		m := materialProfile(v)
		s := fmt.Sprint(m[key])
		if n, _ := materialNumber(m[key+"_max"]); n > 0 {
			s += "–" + fmt.Sprint(n)
		}
		out = append(out, s+" "+str(m, key+"_unit"))
	}
	return strings.Join(out, "; ")
}
