package app

import (
	"reflect"
	"strings"
	"testing"
)

const receiptTestKey = "7e5a1111-a6d4-46d4-8935-0a7491fd2541"

func TestExportReceiptStableKeysAndLegacy(t *testing.T) {
	for _, raw := range []any{nil, "", 0, "not-a-uuid", "00000000-0000-0000-0000-000000000000", "7e5a1111-a6d4-46d4-8935-0a7491fd254", true} {
		if _, err := exportValidateArrays([]M{{"receipt_key": raw}}, "receipts", "USD"); err == nil {
			t.Fatalf("accepted invalid receipt key %v", raw)
		}
	}
	keys := []M{{"receipt_key": receiptTestKey}, {"receipt_key": strings.ToUpper(receiptTestKey)}}
	if _, err := exportValidateArrays(keys, "receipts", "USD"); err == nil {
		t.Fatal("duplicate receipt keys accepted")
	}
	input := []M{{"reference": "Legacy receipt"}, {"receipt_key": strings.ToUpper(receiptTestKey), "reference": "Foreign reference"}}
	out, err := exportValidateArrays(input, "receipts", "USD")
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := out[0]["receipt_key"]; exists {
		t.Fatal("assigned legacy receipt by guesswork")
	}
	if str(out[1], "receipt_key") != receiptTestKey || str(input[1], "receipt_key") != strings.ToUpper(receiptTestKey) {
		t.Fatal("key normalization mutated input or was lost")
	}
}

func TestExportReceiptDocumentExactContext(t *testing.T) {
	parent := M{"record_kind": "export", "receipts": []M{{"receipt_key": receiptTestKey, "reference": "BOB-34"}}}
	for field, expected := range exportReceiptDocumentFields {
		if err := exportValidateFileContext(parent, expected.kind, receiptTestKey, field); err != nil {
			t.Fatal(err)
		}
		for _, kind := range []string{"invoice", "ebrc", "jv_supporting", "other"} {
			if exportValidateFileContext(parent, kind, receiptTestKey, field) == nil {
				t.Fatalf("wrong kind %s accepted for %s", kind, field)
			}
		}
	}
	for _, args := range [][3]string{{"payment_details", "", "inward_remittance"}, {"payment_details", receiptTestKey, ""}, {"payment_details", receiptTestKey, "unknown"}, {"payment_details", "4e5a1111-a6d4-46d4-8935-0a7491fd2541", "inward_remittance"}, {"payment_details", receiptTestKey, "credit_advice"}, {"bank_advice", receiptTestKey, "disposal_instruction"}} {
		if exportValidateFileContext(parent, args[0], args[1], args[2]) == nil {
			t.Fatalf("accepted invalid file context %v", args)
		}
	}
	parent["record_kind"] = "jv"
	if exportValidateFileContext(parent, "payment_details", receiptTestKey, "inward_remittance") == nil {
		t.Fatal("JV accepted contextual document")
	}
	if err := exportValidateFileContext(parent, "jv_supporting", "", ""); err != nil {
		t.Fatal("legacy shared attachment rejected")
	}
}

func TestExportReceiptFieldsPreserveFinancialSettlement(t *testing.T) {
	old := M{"currency": "USD", "gross_payment_received": "1000.00", "payment_received": "990.00", "foreign_bank_charges": "10.00", "foreign_bank_charges_currency": "USD"}
	next := materialProfile(old)
	for key, value := range (M{"receipt_key": receiptTestKey, "inward_reference": "IN-7", "inward_due_date": "2026-10-31", "disposal_credit_account": "CC-344", "disposal_debit_account": "FOREX-017", "credit_advice_bill_number": "BILL-42", "credit_advice_credit_account": "344", "credit_advice_debit_account": "017", "inr_received_amount": "822690.00", "inr_bank_charges": "100.00", "inr_currency": "INR"}) {
		next[key] = value
	}
	out, err := exportValidateArrays([]M{next}, "receipts", "USD")
	if err != nil {
		t.Fatal(err)
	}
	baseline := exportBalance(M{"currency": "USD", "invoice_amount": "1100.00", "receipts": []M{old}})
	revised := exportBalance(M{"currency": "USD", "invoice_amount": "1100.00", "receipts": out})
	if !reflect.DeepEqual(baseline, revised) {
		t.Fatalf("new documentary fields changed settlement: %v -> %v", baseline, revised)
	}
	for _, patch := range []M{{"disposal_amount": "2000.00"}, {"disposal_currency": "GBP"}, {"inward_due_date": "2026-02-30"}, {"disposal_credit_account": "unsafe\naccount"}, {"credit_advice_bill_number": strings.Repeat("x", 161)}} {
		candidate := materialProfile(next)
		for key, value := range patch {
			candidate[key] = value
		}
		if _, err := exportValidateArrays([]M{candidate}, "receipts", "USD"); err == nil {
			t.Fatalf("invalid or duplicate derived field accepted: %v", patch)
		}
	}
	profile := M{"invoice_date": "2026-10-10", "export_date": "2026-10-11", "transport_document_date": "2026-10-12", "port_of_loading": "Mumbai", "port_of_departure": "Chennai"}
	p, err := exportValidateObject(profile, exportProfileText, exportProfileMoney, []string{"scrip_sale_percentage"})
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range profile {
		if p[key] != value {
			t.Fatalf("shipment field %s was lost", key)
		}
	}
	profile["invoice_date"] = "2026-02-30"
	if _, err := exportValidateObject(profile, exportProfileText, exportProfileMoney, nil); err == nil {
		t.Fatal("invalid invoice date accepted")
	}
}

func TestExportDocumentationEvidenceAndContextLabels(t *testing.T) {
	parent := M{"record_kind": "export", "workflow_status": "Draft", "invoice_amount": "1000.00", "receipts": []M{{"receipt_key": receiptTestKey, "reference": "FR-42"}}, "ebrcs": []M{{"ebrc_number": "EBRC / 42"}}}
	for _, status := range []string{"Draft", "Submitted", "Approved", "Changes requested"} {
		parent["workflow_status"] = status
		actual, err := exportSavedEBRC(parent, " ebrc  /  42 ")
		if err != nil || actual != "EBRC / 42" {
			t.Fatalf("evidence lookup incorrectly gated by workflow: %s %v", status, err)
		}
	}
	for _, number := range []string{"", "missing"} {
		if _, err := exportSavedEBRC(parent, number); err == nil {
			t.Fatal("unsaved eBRC accepted")
		}
	}
	files := []M{{"filename": "shared.pdf"}, {"receipt_key": receiptTestKey, "field_key": "disposal_instruction", "filename": "dispose.pdf"}}
	exportLabelReceiptFiles(parent, files)
	if str(files[0], "context_label") != "Shared invoice document" || str(files[1], "field_label") != "Disposal instruction" || !strings.Contains(str(files[1], "context_label"), "FR-42") {
		t.Fatalf("incorrect document scope labels %v", files)
	}
}

func TestExportReconciliationWorkbookFields(t *testing.T) {
	invoice := M{"id": 1, "invoice_number": "INV/42", "currency": "USD", "invoice_amount": "1000.00", "profile": M{"invoice_date": "2026-10-10", "port_of_loading": "Mumbai"}, "receipts": []M{{"receipt_key": receiptTestKey, "inward_reference": "IN-42", "gross_payment_received": "200.00", "currency": "USD"}}, "documentation_cleared": M{"ebrc_number": "EBRC/42", "file_id": 7}}
	sheets := exportRegisterSheets([]M{invoice})
	for _, tc := range []struct {
		sheet         int
		header, value string
	}{{0, "invoice date", "2026-10-10"}, {0, "port of loading", "Mumbai"}, {0, "Documentation cleared", "Cleared"}, {0, "Documentation clearance", `{"ebrc_number":"EBRC/42","file_id":7}`}, {1, "receipt key", receiptTestKey}, {1, "inward reference", "IN-42"}} {
		found := false
		for i, header := range sheets[tc.sheet].Headers {
			if header == tc.header {
				found = true
				if sheets[tc.sheet].Rows[0][i] != tc.value {
					t.Errorf("%s lost in workbook", header)
				}
			}
		}
		if !found {
			t.Errorf("%s missing from workbook", tc.header)
		}
	}
}
