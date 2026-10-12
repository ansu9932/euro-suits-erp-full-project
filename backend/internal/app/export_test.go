package app

import (
	"archive/zip"
	"bytes"
	"github.com/xuri/excelize/v2"
	"io"
	"strings"
	"testing"
)

func TestExportExactSettlement(t *testing.T) {
	cases := []struct {
		name     string
		invoice  string
		receipts []M
		discount string
		out      string
	}{
		{"received plus separate charge closes invoice", "1000.00", []M{{"payment_received": "990.00", "currency": "USD", "bank_charges": "10.00", "bank_charges_currency": "USD"}}, "0.00", "0.00"},
		{"gross and net are not counted again", "1000.00", []M{{"payment_received": "970.00", "gross_payment_received": "1000.00", "net_payment_received": "970.00", "currency": "USD", "foreign_bank_charges": "10.00", "foreign_bank_charges_currency": "USD", "bank_charges": "20.00", "bank_charges_currency": "USD"}}, "0.00", "0.00"},
		{"partial receipts and approved discount", "100.00", []M{{"payment_received": "30.10", "currency": "USD"}, {"payment_received": "59.89", "currency": "USD"}}, "0.01", "10.00"},
		{"overpayment retained", "100.00", []M{{"payment_received": "101.20", "currency": "USD"}}, "0.00", "-1.20"},
		{"cross currency equivalents only", "100.00", []M{{"payment_received": "8100.00", "currency": "INR", "invoice_currency_amount": "90.00", "foreign_bank_charges": "270.00", "foreign_bank_charges_currency": "INR", "foreign_bank_charges_invoice_amount": "3.00", "bank_charges": "2.00", "bank_charges_currency": "USD"}}, "1.00", "4.00"},
		{"large exact cent values", "999999999999999.99", []M{{"payment_received": "999999999999999.98", "currency": "USD"}}, "0.00", "0.01"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			m := M{"invoice_amount": tc.invoice, "currency": "USD", "receipts": tc.receipts, "approved_discount": tc.discount}
			b := exportBalance(m)
			if got := str(b, "outstanding"); got != tc.out {
				t.Fatalf("outstanding %s; want %s", got, tc.out)
			}
		})
	}
}
func TestExportMoneyAndMissingValues(t *testing.T) {
	for _, value := range []any{1.25, "1e3", "NaN", "-1", "1.001", "01.00", "FOC", "9999999999999999.00"} {
		if _, e := exportCents(value, true); e == nil {
			t.Fatalf("accepted invalid amount %#v", value)
		}
	}
	for _, value := range []any{nil, ""} {
		n, e := exportCents(value, false)
		if e != nil || n != nil {
			t.Fatalf("missing became a numeric amount: %v %v", n, e)
		}
	}
	n, e := exportCents("0", true)
	if e != nil || n == nil || exportDecimal(n) != "0.00" {
		t.Fatal("explicit zero must be distinct from missing")
	}
	p, e := exportValidateObject(M{"drawback_claim_amount": nil, "statuses": M{"drawback_claim_amount": "FOC"}}, exportProfileText, exportProfileMoney, []string{"scrip_sale_percentage"})
	if e != nil || str(materialProfile(p["statuses"]), "drawback_claim_amount") != "FOC" || p["drawback_claim_amount"] != nil {
		t.Fatalf("special code corrupted %v %v", p, e)
	}
}
func TestExportCurrencyEquivalents(t *testing.T) {
	valid := M{"payment_received": "80.00", "currency": "EUR", "invoice_currency_amount": "90.00", "foreign_bank_charges": "2.00", "foreign_bank_charges_currency": "USD"}
	if _, e := exportValidateArrays([]M{valid}, "receipts", "USD"); e != nil {
		t.Fatal(e)
	}
	cases := []M{{"payment_received": "80.00", "currency": "EUR"}, {"payment_received": "80.00", "currency": "USD", "invoice_currency_amount": "90.00"}, {"invoice_currency_amount": "90.00"}, {"bank_charges": "5.00"}, {"bank_charges": "5.00", "bank_charges_currency": "INR"}, {"payment_received": "80.00", "currency": "USD", "invoice_currency": "EUR"}}
	for _, m := range cases {
		if _, e := exportValidateArrays([]M{m}, "receipts", "USD"); e == nil {
			t.Fatalf("accepted inconsistent receipt %v", m)
		}
	}
}
func TestExportAllFieldsStrictAndDates(t *testing.T) {
	p := M{"company_id": int64(1), "party_type": "buyer", "party_id": int64(1), "invoice_number": "EX/0001", "currency": "USD", "invoice_amount": "100.10", "profile": M{"export_date": "2026-10-08", "shipping_bill_date": "2026-10-09", "shipping_mode": "Courier", "statuses": M{"shipping_bill_number": "Pending"}, "drawback_claim_amount": "0.00", "drawback_claim_currency": "INR", "scrip_sale_percentage": "99.125"}, "receipts": []M{{"payment_received": "95.10", "currency": "USD", "gross_payment_received": "100.10", "net_payment_received": "95.10", "converted_amount": "9000.00", "converted_currency": "INR", "conversion_amount": "8000.00", "conversion_currency": "INR", "credit_bank_017": "manual allocation", "packing_credit_amount": "10.00", "packing_credit_currency": "USD"}}, "ebrcs": []M{{"ebrc_number": "BR-1", "ebrc_amount": "40.00", "currency": "USD", "ebrc_balance_amount": "60.10", "ebrc_note": "PRINT PENDING"}, {"ebrc_number": "BR-2", "ebrc_amount": "60.10", "currency": "USD"}}}
	out, e := exportValidateBody(p)
	if e != nil {
		t.Fatal(e)
	}
	if len(enquiryRows(out["ebrcs"])) != 2 || len(enquiryRows(out["receipts"])) != 1 {
		t.Fatal("child multiplicity lost")
	}
	for _, patch := range []M{{"approved_discount": "100.00"}, {"finance_check": M{}}, {"invoice_amount": 100.10}} {
		badBody := materialProfile(p)
		for k, v := range patch {
			badBody[k] = v
		}
		if _, e = exportValidateBody(badBody); e == nil {
			t.Fatalf("accepted protected/invalid fields %v", patch)
		}
	}
	for _, badProfile := range []M{{"export_date": "2026-02-30"}, {"unknown_column": "secret"}, {"shipping_mode": "Spaceship"}, {"scrip_sale_percentage": "101"}, {"statuses": M{"admin_override": "yes"}}} {
		badBody := materialProfile(p)
		badBody["profile"] = badProfile
		if _, e = exportValidateBody(badBody); e == nil {
			t.Fatalf("accepted invalid profile %v", badProfile)
		}
	}
}
func TestExportGrantsAndDesignationLimits(t *testing.T) {
	fh := &User{Role: "Finance Head", Scope: "company"}
	if !exportCan(fh, "edit") || !exportCan(fh, "discount_first") || fh.Can("view_orders") || exportCan(fh, "audit_check") || exportCan(fh, "discount_final") {
		t.Fatal("finance head defaults broadened")
	}
	finance := &User{Role: "Finance", Scope: "company"}
	if exportCan(finance, "audit_check") {
		t.Fatal("audit must require explicit assignment")
	}
	finance.CustomPermissions = true
	finance.Permissions = []string{"export_view", "export_audit_check"}
	if !exportCan(finance, "audit_check") || exportCan(finance, "edit") {
		t.Fatal("custom audit grant or restriction ignored")
	}
	finance.DesignationID = 1
	finance.DesignationPermissions = []string{"export_view"}
	if exportCan(finance, "audit_check") {
		t.Fatal("user audit exceeded designation")
	}
	finance.DesignationPermissions = append(finance.DesignationPermissions, "export_audit_check")
	if !exportCan(finance, "audit_check") {
		t.Fatal("explicit auditor designation rejected")
	}
	fh.DesignationID = 1
	fh.InheritDesignation = true
	fh.DesignationPermissions = []string{}
	if exportRead(fh) {
		t.Fatal("new default widened existing restricted designation")
	}
	sa := &User{Role: "Super Admin"}
	if exportCan(sa, "discount_first") || !exportCan(sa, "discount_final") {
		t.Fatal("two-stage approver separation broken")
	}
	sa.CustomPermissions = true
	sa.Permissions = []string{}
	if exportRead(sa) {
		t.Fatal("existing explicit admin limits ignored")
	}
	if needsOrderVisibility([]string{"export_view", "export_edit"}) || !needsOrderVisibility([]string{"export_view", "finance"}) {
		t.Fatal("standalone capability validation widened old grants")
	}
}
func TestExportDiscountBoundsAndCurrencyTotals(t *testing.T) {
	balance := M{"outstanding": "0.03"}
	for _, s := range []string{"0.00", "0.04", "-0.01", "0.001"} {
		if exportDiscountFits(s, balance) {
			t.Fatalf("invalid discount accepted %s", s)
		}
	}
	if !exportDiscountFits("0.03", balance) {
		t.Fatal("exact closing discount rejected")
	}
	if exportDiscountFits("0.01", M{"outstanding": "-0.01"}) {
		t.Fatal("discount applied to credit")
	}
	totals := exportTotals([]M{{"currency": "USD", "invoice_amount": "100.00", "receipts": []M{{"payment_received": "20.00", "currency": "USD"}}}, {"currency": "INR", "invoice_amount": "1000.00", "receipts": []M{{"payment_received": "1001.00", "currency": "INR"}}}, {"currency": "USD", "invoice_amount": "0.03"}})
	if len(totals) != 2 || str(totals[0], "currency") != "INR" || str(totals[0], "outstanding") != "-1.00" || str(totals[1], "outstanding") != "80.03" {
		t.Fatalf("currency totals mixed %v", totals)
	}
}
func TestExportWorkbookFormulaText(t *testing.T) {
	raw, e := simpleWorkbook([]string{"Invoice"}, [][]string{{"=HYPERLINK(\"https://invalid.example\",\"formula\")"}, {"+SUM(1,2)"}, {"<script>& text"}})
	if e != nil {
		t.Fatal(e)
	}
	z, e := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
	if e != nil {
		t.Fatal(e)
	}
	for _, f := range z.File {
		if f.Name != "xl/worksheets/sheet1.xml" {
			continue
		}
		r, e := f.Open()
		if e != nil {
			t.Fatal(e)
		}
		body, _ := io.ReadAll(r)
		r.Close()
		s := string(body)
		if strings.Contains(s, "<f>") || !strings.Contains(s, `t="inlineStr"`) || !strings.Contains(s, "&lt;script&gt;&amp; text") {
			t.Fatalf("unsafe worksheet: %s", s)
		}
	}
}

func TestExportEBRCDuplicateAndPending(t *testing.T) {
	for _, rows := range [][]M{{{"ebrc_number": "BR 123"}, {"ebrc_number": " br   123 "}}, {{"ebrc_number": ""}}} {
		if _, e := exportValidateArrays(rows, "eBRCs", "USD"); e == nil {
			t.Fatalf("accepted invalid certificates %v", rows)
		}
	}
	if _, e := exportValidateArrays([]M{{"status": "Pending", "ebrc_note": "Awaiting bank"}, {"status": "Not required"}, {"ebrc_number": "BR-1"}}, "eBRCs", "USD"); e != nil {
		t.Fatal(e)
	}
}

func TestExportRegisterLargeChildListsAndFormulaSafety(t *testing.T) {
	receipts, ebrcs := []M{}, []M{}
	note := strings.Repeat("x", 2000)
	for i := 0; i < 100; i++ {
		receipts = append(receipts, M{"payment_received": "0.01", "currency": "USD", "receipt_note": note})
		ebrcs = append(ebrcs, M{"ebrc_number": strings.Repeat("b", 10) + exportString(i), "ebrc_note": note})
	}
	invoice := M{"id": int64(1), "invoice_number": "=1+1", "party_name": "Literal buyer", "currency": "USD", "invoice_amount": "10.00", "receipts": receipts, "ebrcs": ebrcs}
	raw, e := exportWorkbook(exportRegisterSheets([]M{invoice}))
	if e != nil {
		t.Fatal(e)
	}
	book, e := excelize.OpenReader(bytes.NewReader(raw))
	if e != nil {
		t.Fatal(e)
	}
	defer book.Close()
	if len(book.GetSheetList()) != 4 {
		t.Fatal("missing child or currency sheets")
	}
	for _, sheet := range []string{"Receipts", "eBRC"} {
		rows, e := book.GetRows(sheet)
		if e != nil || len(rows) != 101 {
			t.Fatalf("child rows lost for %s: %d %v", sheet, len(rows), e)
		}
		found := false
		for _, v := range rows[100] {
			if v == note {
				found = true
			}
		}
		if !found {
			t.Fatalf("long child notes truncated in %s", sheet)
		}
	}
	value, e := book.GetCellValue("Register", "A2")
	if e != nil || value != "=1+1" {
		t.Fatalf("formula-like invoice changed: %q %v", value, e)
	}
	formula, e := book.GetCellFormula("Register", "A2")
	if e != nil || formula != "" {
		t.Fatal("export generated executable formula")
	}
}
