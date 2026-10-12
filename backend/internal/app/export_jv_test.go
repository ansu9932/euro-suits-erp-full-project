package app

import (
	"reflect"
	"strings"
	"testing"
)

func exportJVTestBody() M {
	return M{"record_kind": "jv", "party_type": "manual", "invoice_number": "JV/2026/001", "currency": "USD", "invoice_amount": "1234.56", "profile": M{"export_date": "2026-10-10", "jv_by_date": "Finance / 10 Oct 2026", "scrip_tally_reference": "000017"}}
}

func TestExportJVDefaultsAndEightFields(t *testing.T) {
	body := exportJVTestBody()
	prepared := exportJVCreateDefaults(body)
	out, err := exportValidateBody(prepared)
	if err != nil {
		t.Fatal(err)
	}
	if body["party_name"] != nil || materialProfile(body["profile"])["shipping_mode"] != nil {
		t.Fatal("creation defaults mutated input")
	}
	if str(out, "party_name") != "EURO SUITS MFG" || str(materialProfile(out["profile"]), "shipping_mode") != "HANDO" {
		t.Fatal("JV defaults missing")
	}
	if str(materialProfile(out["profile"]), "jv_by_date") != "Finance / 10 Oct 2026" {
		t.Fatal("combined By / date changed")
	}
	for _, patch := range []M{
		{"currency": ""}, {"invoice_amount": "1.001"}, {"invoice_amount": 12.34}, {"party_name": ""},
		{"company_id": int64(1)}, {"party_id": int64(1)}, {"order_id": int64(1)}, {"party_type": "buyer"},
		{"record_kind": "JV"}, {"record_kind": nil}, {"receipts": []M{{"payment_received": "1.00", "currency": "USD"}}},
		{"ebrcs": []M{{"ebrc_number": "injected"}}}, {"invoice_number": "JV\nREF"},
	} {
		candidate := exportJVCreateDefaults(exportJVTestBody())
		for key, value := range patch {
			candidate[key] = value
		}
		if _, err := exportValidateBody(candidate); err == nil {
			t.Fatalf("accepted invalid JV field: %v", patch)
		}
	}
	for _, profile := range []M{
		{"shipping_bill_number": "not a JV field"}, {"jv_by_date": "x", "statuses": M{"jv_by_date": "Pending"}},
		{"shipping_mode": strings.Repeat("x", 161)}, {"shipping_mode": "HANDO\nAIR"},
		{"export_date": "2026-02-30"}, {"unknown": "injected"},
	} {
		candidate := exportJVCreateDefaults(exportJVTestBody())
		candidate["profile"] = profile
		if _, err := exportValidateBody(candidate); err == nil {
			t.Fatalf("accepted unrelated JV profile: %v", profile)
		}
	}
	custom := exportJVTestBody()
	custom["party_name"] = "Another company"
	custom["profile"] = M{"shipping_mode": "Courier"}
	actual, err := exportValidateBody(exportJVCreateDefaults(custom))
	if err != nil || str(actual, "party_name") != "Another company" || str(materialProfile(actual["profile"]), "shipping_mode") != "Courier" {
		t.Fatal("explicit JV values replaced by defaults")
	}
	noCurrency := exportJVTestBody()
	delete(noCurrency, "currency")
	if _, err := exportValidateBody(exportJVCreateDefaults(noCurrency)); err == nil {
		t.Fatal("currency defaulted without authorization")
	}
}

func TestExportJVDocumentAndFinancialIsolation(t *testing.T) {
	if len(exportDocumentKindsFor("jv")) != 1 || str(exportDocumentKindsFor("jv")[0], "key") != "jv_supporting" {
		t.Fatal("JV should offer only supporting document")
	}
	for _, kind := range exportDocumentKinds {
		key := str(kind, "key")
		if exportDocumentKindAllowed("jv", key) || !exportDocumentKindAllowed("export", key) {
			t.Fatal("existing export document category crossed into JV")
		}
	}
	if !exportDocumentKindAllowed("jv", "jv_supporting") || exportDocumentKindAllowed("export", "jv_supporting") {
		t.Fatal("supporting JV document crossed registers")
	}
	jv := M{"record_kind": "jv", "invoice_amount": "999999.99", "currency": "USD"}
	if exportJVFinancialAction(jv) == nil || exportJVFinancialAction(M{}) != nil {
		t.Fatal("JV financial actions not isolated")
	}
	for _, role := range []string{"Finance", "Finance Head", "MD", "Super Admin"} {
		permissions := exportJVPermissions(&User{Role: role, Scope: "company"}, jv)
		for _, key := range []string{"finance_check", "audit_check", "discount_first", "discount_final"} {
			if permissions[key] != false {
				t.Fatalf("JV exposes financial capability %s to %s", key, role)
			}
		}
	}
	invoice := M{"invoice_amount": "100.00", "currency": "USD", "receipts": []M{{"payment_received": "90.00", "currency": "USD"}}}
	totals := exportTotals([]M{invoice, jv})
	if len(totals) != 1 || str(totals[0], "invoice_amount") != "100.00" || str(totals[0], "outstanding") != "10.00" {
		t.Fatalf("JV polluted export balances: %v", totals)
	}
	if len(exportTotals([]M{jv})) != 0 {
		t.Fatal("JV register has export totals")
	}
}

func TestExportJVWorkbookColumns(t *testing.T) {
	body := exportJVCreateDefaults(exportJVTestBody())
	sheets := exportJVSheets([]M{body})
	wantHeaders := []string{"JV REF", "DATE", "PARTY NAME", "CURR", "AMOUNT", "MODE", "BY / DATE", "TALLY REF"}
	wantRow := []string{"JV/2026/001", "2026-10-10", "EURO SUITS MFG", "USD", "1234.56", "HANDO", "Finance / 10 Oct 2026", "000017"}
	if len(sheets) != 1 || sheets[0].Name != "JV" || !reflect.DeepEqual(sheets[0].Headers, wantHeaders) || !reflect.DeepEqual(sheets[0].Rows, [][]string{wantRow}) {
		t.Fatalf("JV workbook must have only eight highlighted fields: %v", sheets)
	}
	for _, value := range []string{"", "export", "jv"} {
		if _, err := exportKind(value); err != nil {
			t.Fatal(err)
		}
	}
	for _, value := range []string{"all", "JV", " export ", "invoice"} {
		if _, err := exportKind(value); err == nil {
			t.Fatalf("accepted invalid register kind %q", value)
		}
	}
}
