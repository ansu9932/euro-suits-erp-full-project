package app

import (
	"reflect"
	"strings"
	"testing"
)

func TestExportBankingExactDifferenceAndSettlement(t *testing.T) {
	for _, tc := range []struct{ fcr, credit, charge string }{
		{"1000.00", "990.00", "10.00"}, {"0.03", "0.02", "0.01"},
		{"999999999999999.99", "999999999999999.98", "0.01"}, {"0", "0", "0.00"},
	} {
		input := M{"currency": "USD", "fcr_amount": tc.fcr, "bank_advice_amount": tc.credit, "payment_received": tc.credit,
			"foreign_bank_charges": "999.00", "foreign_bank_charges_currency": "INR",
			"inr_received_amount": "82000.00", "inr_bank_charges": "100.00", "inr_currency": "INR",
			"gst_amount": "18.00", "gst_currency": "INR", "inr_conversion_rate": "82.123456"}
		before := materialProfile(input)
		rows, err := exportValidateArrays([]M{input}, "receipts", "USD")
		if err != nil {
			t.Fatal(err)
		}
		if got := rows[0]; str(got, "foreign_bank_charges") != tc.charge || str(got, "foreign_bank_charges_currency") != "USD" {
			t.Fatalf("wrong authoritative charge: %v", got)
		}
		balance := exportBalance(M{"invoice_amount": tc.fcr, "currency": "USD", "receipts": rows})
		if str(balance, "outstanding") != "0.00" {
			t.Fatalf("documentary values double-counted: %v", balance)
		}
		if !reflect.DeepEqual(input, before) {
			t.Fatal("receipt validation mutated caller input")
		}
	}
}

func TestExportBankingCrossCurrencyExplicitEquivalents(t *testing.T) {
	base := M{"currency": "GBP", "fcr_amount": "100.00", "bank_advice_amount": "98.00", "payment_received": "98.00", "invoice_currency_amount": "122.50"}
	if _, err := exportValidateArrays([]M{base}, "receipts", "USD"); err == nil {
		t.Fatal("derived foreign charges need an explicit invoice equivalent")
	}
	base["foreign_bank_charges_invoice_amount"] = "2.50"
	rows, err := exportValidateArrays([]M{base}, "receipts", "USD")
	if err != nil {
		t.Fatal(err)
	}
	if str(exportBalance(M{"invoice_amount": "125.00", "currency": "USD", "receipts": rows}), "outstanding") != "0.00" {
		t.Fatal("cross-currency settlement inferred or doubled")
	}
	base["currency"] = "USD"
	base["invoice_currency_amount"] = "98.00"
	if _, err = exportValidateArrays([]M{base}, "receipts", "USD"); err == nil {
		t.Fatal("same-currency equivalent must match the derived charge")
	}
}

func TestExportBankingPartialDraftAndLegacyPreservation(t *testing.T) {
	for _, input := range []M{
		{"payment_received": "10.00", "currency": "USD", "foreign_bank_charges": "2.00", "foreign_bank_charges_currency": "USD"},
		{"fcr_amount": "20.00", "currency": "USD"}, {"bank_advice_amount": "15.00", "currency": "USD"},
		{"instruction_action": "Credit FC account", "fc_account_currency": "GBP", "forex_account": "BOB FOREX ACCOUNT"},
		{"instruction_action": "Conversion", "disposal_credit_target": "CC", "conversion_debit_account": "D/ACCOUNT"},
	} {
		rows, err := exportValidateArrays([]M{input}, "receipts", "USD")
		if err != nil {
			t.Fatal(err)
		}
		if str(rows[0], "foreign_bank_charges") != str(input, "foreign_bank_charges") {
			t.Fatalf("partial or legacy charges were changed: %v", rows[0])
		}
		if str(rows[0], "payment_received") != str(input, "payment_received") {
			t.Fatal("bank advice implicitly changed settlement")
		}
	}
}

func TestExportBankingValidationRejectsInvalidValues(t *testing.T) {
	for _, input := range []M{
		{"currency": "USD", "fcr_amount": "10.00", "bank_advice_amount": "10.01"},
		{"fcr_amount": "10.00", "bank_advice_amount": "9.00"},
		{"currency": "USD", "fcr_amount": 10}, {"currency": "USD", "bank_advice_amount": "1e3"},
		{"currency": "USD", "fcr_amount": "-1"}, {"currency": "USD", "fcr_amount": "1.001"},
		{"instruction_action": "Send money"}, {"fc_account_currency": "EUR"}, {"disposal_credit_target": "Unknown"},
		{"inr_received_amount": "10.00", "inr_currency": "USD"}, {"inr_bank_charges": "1.00"},
		{"inr_conversion_rate": "0"}, {"inr_conversion_rate": "-1"}, {"inr_conversion_rate": "1.1234567"},
		{"inr_advice_date": "2026-02-30"}, {"gst_amount": "10.00"}, {"credit_bank_344_amount": "10.00"},
		{"forex_account": "a\nb"}, {"bank_instruction_reference": strings.Repeat("x", 161)},
		{"bank_advice_currency": "USD"}, {"automatic_bank_send": true},
	} {
		if _, err := exportValidateArrays([]M{input}, "receipts", "USD"); err == nil {
			t.Fatalf("accepted invalid banking data: %v", input)
		}
	}
}
