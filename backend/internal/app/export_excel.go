package app

import (
	"encoding/json"
	"fmt"
	"github.com/xuri/excelize/v2"
	"math/big"
	"net/http"
	"sort"
	"strings"
	"unicode/utf8"
)

func exportSignedCents(s string) *big.Int {
	n, ok := new(big.Rat).SetString(s)
	if !ok {
		return new(big.Int)
	}
	n.Mul(n, big.NewRat(100, 1))
	return new(big.Int).Quo(n.Num(), n.Denom())
}
func exportTotals(records []M) []M {
	by := map[string]map[string]*big.Int{}
	keys := strings.Fields("invoice_amount payment_received foreign_bank_charges bank_charges approved_discount outstanding")
	for _, m := range records {
		if exportRecordKind(m) == "jv" {
			continue
		}
		cur := str(m, "currency")
		if by[cur] == nil {
			by[cur] = map[string]*big.Int{}
			for _, k := range keys {
				by[cur][k] = new(big.Int)
			}
		}
		b := exportBalance(m)
		for _, k := range keys {
			by[cur][k].Add(by[cur][k], exportSignedCents(str(b, k)))
		}
	}
	currencies := []string{}
	for c := range by {
		currencies = append(currencies, c)
	}
	sort.Strings(currencies)
	out := []M{}
	for _, cur := range currencies {
		m := M{"currency": cur}
		for _, key := range keys {
			m[key] = exportDecimal(by[cur][key])
		}
		out = append(out, m)
	}
	return out
}

type exportSheet struct {
	Name    string
	Headers []string
	Rows    [][]string
}

func exportWorkbook(sheets []exportSheet) ([]byte, error) {
	cells := 0
	for _, sheet := range sheets {
		cells += (len(sheet.Rows) + 1) * len(sheet.Headers)
	}
	if cells > 250000 {
		return nil, bad("This workbook is too large for one download. Select a company, party, currency or accounting year to narrow it.")
	}
	book := excelize.NewFile()
	defer book.Close()
	for i, sheet := range sheets {
		if i == 0 {
			if e := book.SetSheetName("Sheet1", sheet.Name); e != nil {
				return nil, e
			}
		} else {
			if _, e := book.NewSheet(sheet.Name); e != nil {
				return nil, e
			}
		}
		for row, line := range append([][]string{sheet.Headers}, sheet.Rows...) {
			for col, value := range line {
				if utf8.RuneCountInString(value) > 32767 {
					return nil, bad("An export value exceeds the spreadsheet cell limit; split the source entry before downloading.")
				}
				cell, e := excelize.CoordinatesToCellName(col+1, row+1)
				if e != nil {
					return nil, e
				}
				if e = book.SetCellStr(sheet.Name, cell, value); e != nil {
					return nil, e
				}
			}
		}
	}
	raw, e := book.WriteToBuffer()
	if e != nil {
		return nil, e
	}
	return raw.Bytes(), nil
}
func exportWriteSheets(w http.ResponseWriter, name string, sheets []exportSheet) (any, error) {
	raw, e := exportWorkbook(sheets)
	if e != nil {
		return nil, e
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`.xlsx"`)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Write(raw)
	return nil, nil
}
func exportWriteExcel(w http.ResponseWriter, name string, headers []string, body [][]string) (any, error) {
	return exportWriteSheets(w, name, []exportSheet{{"Export", headers, body}})
}

func exportJSON(v any) string { b, _ := json.Marshal(v); return string(b) }
func exportFlatten(body *[][]string, prefix string, m M) {
	keys := []string{}
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		v := m[k]
		switch x := v.(type) {
		case M:
			exportFlatten(body, prefix+k+" / ", x)
		default:
			*body = append(*body, []string{prefix + strings.ReplaceAll(k, "_", " "), exportString(v)})
		}
	}
}
func (a *App) exportDetailExcel(w http.ResponseWriter, r *http.Request) (any, error) {
	if !exportCan(current(r), "download") {
		return nil, forbidden()
	}
	result, e := a.exportDetail(w, r)
	if e != nil {
		return nil, e
	}
	d := result.(M)
	m := d["record"].(M)
	if exportRecordKind(m) == "jv" {
		return exportWriteSheets(w, fmt.Sprintf("JV-%d", id(m, "id")), exportJVSheets([]M{m}))
	}
	body := [][]string{}
	for _, key := range strings.Fields("invoice_number party_name currency invoice_amount bill_amount workflow_status content_revision submitted_by_name submitted_at reviewed_by_name reviewed_at review_note approved_revision created_at updated_at version") {
		body = append(body, []string{strings.ReplaceAll(key, "_", " "), exportString(m[key])})
	}
	exportFlatten(&body, "Shipment / ", materialProfile(m["profile"]))
	exportFlatten(&body, "Balance / ", materialProfile(m["balance"]))
	exportFlatten(&body, "Documentation clearance / ", materialProfile(m["documentation_cleared"]))
	for _, kind := range []string{"receipts", "ebrcs"} {
		for i, child := range enquiryRows(m[kind]) {
			exportFlatten(&body, fmt.Sprintf("%s %d / ", kind, i+1), child)
		}
	}
	exportFlatten(&body, "Finance check / ", materialProfile(m["finance_check"]))
	exportFlatten(&body, "Audit check / ", materialProfile(m["audit_check"]))
	for _, kind := range exportDocumentKinds {
		names := []string{}
		for _, file := range enquiryRows(d["files"]) {
			if str(file, "kind") == str(kind, "key") {
				name := str(file, "filename")
				if str(file, "receipt_key") != "" {
					name += " [" + str(file, "context_label") + "]"
				}
				names = append(names, name)
			}
		}
		status := "No document attached"
		if len(names) > 0 {
			status = strings.Join(names, "; ")
		}
		body = append(body, []string{"Document checklist / " + str(kind, "label"), status})
	}
	for i, discount := range enquiryRows(d["discounts"]) {
		exportFlatten(&body, fmt.Sprintf("Discount %d / ", i+1), discount)
	}
	for i, comment := range enquiryRows(d["comments"]) {
		exportFlatten(&body, fmt.Sprintf("Comment %d / ", i+1), comment)
	}
	return exportWriteExcel(w, fmt.Sprintf("Export-%d", id(m, "id")), []string{"Field", "Value"}, body)
}
func exportColumnKeys(texts []string, money map[string]string, extra ...string) []string {
	keys := append(append([]string{}, texts...), extra...)
	for key, currency := range money {
		keys = append(keys, key, currency)
	}
	sort.Strings(keys)
	out := []string{}
	for _, key := range keys {
		if len(out) == 0 || out[len(out)-1] != key {
			out = append(out, key)
		}
	}
	return out
}
func exportColumns(keys []string) []string {
	out := []string{}
	for _, key := range keys {
		out = append(out, strings.ReplaceAll(key, "_", " "))
	}
	return out
}
func exportCell(m M, key string) string {
	if key == "statuses" {
		return exportJSON(m[key])
	}
	return exportString(m[key])
}
func exportRegisterSheets(records []M) []exportSheet {
	pkeys := exportColumnKeys(exportProfileText, exportProfileMoney, "scrip_sale_percentage", "statuses")
	rkeys := exportColumnKeys(exportReceiptText, exportReceiptMoney, "exchange_rate", "inr_conversion_rate", "statuses")
	ekeys := exportColumnKeys(exportEBRCText, exportEBRCMoney, "statuses")
	balanceKeys := strings.Fields("invoice_amount payment_received foreign_bank_charges bank_charges approved_discount outstanding")
	header := append([]string{"Invoice number", "Party", "Currency"}, exportColumns(balanceKeys)...)
	header = append(header, exportColumns(pkeys)...)
	header = append(header, "Receipt count", "eBRC count", "Finance check", "Audit check", "Documentation cleared", "Documentation clearance")
	sheets := []exportSheet{{Name: "Register", Headers: header}, {Name: "Receipts", Headers: append([]string{"Invoice ID", "Invoice number", "Party", "Invoice currency", "Receipt sequence"}, exportColumns(rkeys)...)}, {Name: "eBRC", Headers: append([]string{"Invoice ID", "Invoice number", "Party", "eBRC sequence"}, exportColumns(ekeys)...)}, {Name: "Currency totals", Headers: append([]string{"Currency"}, exportColumns(balanceKeys)...)}}
	for _, m := range records {
		b := exportBalance(m)
		line := []string{str(m, "invoice_number"), str(m, "party_name"), str(m, "currency")}
		for _, key := range balanceKeys {
			line = append(line, str(b, key))
		}
		p := materialProfile(m["profile"])
		for _, key := range pkeys {
			line = append(line, exportCell(p, key))
		}
		receipts, ebrcs := enquiryRows(m["receipts"]), enquiryRows(m["ebrcs"])
		clearanceStatus := "Pending"
		if len(materialProfile(m["documentation_cleared"])) > 0 {
			clearanceStatus = "Cleared"
		}
		line = append(line, fmt.Sprint(len(receipts)), fmt.Sprint(len(ebrcs)), exportJSON(m["finance_check"]), exportJSON(m["audit_check"]), clearanceStatus, exportJSON(m["documentation_cleared"]))
		sheets[0].Rows = append(sheets[0].Rows, line)
		for i, p := range receipts {
			line := []string{fmt.Sprint(id(m, "id")), str(m, "invoice_number"), str(m, "party_name"), str(m, "currency"), fmt.Sprint(i + 1)}
			for _, key := range rkeys {
				line = append(line, exportCell(p, key))
			}
			sheets[1].Rows = append(sheets[1].Rows, line)
		}
		for i, p := range ebrcs {
			line := []string{fmt.Sprint(id(m, "id")), str(m, "invoice_number"), str(m, "party_name"), fmt.Sprint(i + 1)}
			for _, key := range ekeys {
				line = append(line, exportCell(p, key))
			}
			sheets[2].Rows = append(sheets[2].Rows, line)
		}
	}
	for _, total := range exportTotals(records) {
		line := []string{str(total, "currency")}
		for _, key := range balanceKeys {
			line = append(line, str(total, key))
		}
		sheets[3].Rows = append(sheets[3].Rows, line)
	}
	return sheets
}
func (a *App) exportRegisterExcel(w http.ResponseWriter, r *http.Request) (any, error) {
	if !exportCan(current(r), "download") {
		return nil, forbidden()
	}
	records, e := a.exportRecords(r)
	if e != nil {
		return nil, e
	}
	if len(records) > 10000 {
		return nil, bad("Select a company or party before exporting this register.")
	}
	if r.URL.Query().Get("record_kind") == "jv" {
		return exportWriteSheets(w, "JV-register", exportJVSheets(records))
	}
	return exportWriteSheets(w, "Export-register", exportRegisterSheets(records))
}

func (a *App) exportStatementExcel(w http.ResponseWriter, r *http.Request) (any, error) {
	if !exportCan(current(r), "download") {
		return nil, forbidden()
	}
	kind, err := exportKind(r.URL.Query().Get("record_kind"))
	if err != nil {
		return nil, err
	}
	if kind == "jv" {
		return nil, bad("JV entries do not have export outstanding statements.")
	}
	if r.URL.Query().Get("party_id") == "" && !(r.URL.Query().Get("party_type") == "manual" && strings.TrimSpace(r.URL.Query().Get("party_name")) != "") {
		return nil, bad("Choose a party for its outstanding statement.")
	}
	records, e := a.exportRecords(r)
	if e != nil {
		return nil, e
	}
	if len(records) > 10000 {
		return nil, bad("Select a company to narrow this statement.")
	}
	body := [][]string{}
	open := []M{}
	for _, m := range records {
		b := exportBalance(m)
		if str(b, "outstanding") == "0.00" {
			continue
		}
		open = append(open, m)
		body = append(body, []string{str(m, "party_name"), str(m, "invoice_number"), str(materialProfile(m["profile"]), "export_date"), str(m, "currency"), str(b, "invoice_amount"), str(b, "payment_received"), str(b, "foreign_bank_charges"), str(b, "bank_charges"), str(b, "approved_discount"), str(b, "outstanding")})
	}
	body = append(body, []string{"Totals by currency (credit balances remain negative)"})
	for _, t := range exportTotals(open) {
		body = append(body, []string{"", "", "", str(t, "currency"), str(t, "invoice_amount"), str(t, "payment_received"), str(t, "foreign_bank_charges"), str(t, "bank_charges"), str(t, "approved_discount"), str(t, "outstanding")})
	}
	return exportWriteExcel(w, "Party-outstanding", []string{"Party", "Invoice number", "Export date", "Currency", "Invoice amount", "Receipts", "Foreign bank charges", "Bank charges", "Approved discount", "Outstanding / credit"}, body)
}
