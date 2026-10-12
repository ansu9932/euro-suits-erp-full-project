package app

import (
	"math/big"
	"slices"
	"strings"
)

// FCR and the credited bank-advice figure use the receipt currency. Calculate
// their difference exactly once in the existing foreign-charge settlement slot.
// A partial pair is a documentary draft and never invents a zero or an FX rate.
func exportPrepareBankingReceipt(p M) (M, error) {
	out := materialProfile(p)
	fcr, err := exportCents(out["fcr_amount"], false)
	if err != nil {
		return nil, bad("FCR amount: " + err.Error())
	}
	credit, err := exportCents(out["bank_advice_amount"], false)
	if err != nil {
		return nil, bad("Bank advice amount: " + err.Error())
	}
	if fcr == nil || credit == nil {
		return out, nil
	}
	if !exportCurrency(str(out, "currency")) {
		return nil, bad("Choose the receipt currency for FCR and bank advice amounts.")
	}
	if credit.Cmp(fcr) > 0 {
		return nil, bad("The bank advice amount cannot exceed FCR. Check the amounts before saving.")
	}
	out["foreign_bank_charges"] = exportDecimal(new(big.Int).Sub(fcr, credit))
	out["foreign_bank_charges_currency"] = str(out, "currency")
	return out, nil
}

func exportValidateBankingReceipt(p M) error {
	for _, field := range []struct {
		key     string
		label   string
		choices []string
	}{
		{"instruction_action", "bank instruction", []string{"Credit FC account", "Disposal", "Conversion"}},
		{"fc_account_currency", "FC account currency", []string{"USD", "GBP"}},
		{"disposal_credit_target", "disposal credit account", []string{"PC", "CC"}},
	} {
		if value := str(p, field.key); value != "" && !slices.Contains(field.choices, value) {
			return bad("Choose a valid " + field.label + ".")
		}
	}
	if currency := str(p, "inr_currency"); currency != "" && currency != "INR" {
		return bad("INR advice amounts must use INR.")
	}
	if rate := str(p, "inr_conversion_rate"); rate != "" {
		n, ok := new(big.Rat).SetString(rate)
		if !ok || n.Sign() <= 0 {
			return bad("INR conversion rate must be positive.")
		}
	}
	for _, key := range []string{"forex_account", "bank_instruction_reference", "inr_advice_reference", "credit_bank_344", "conversion_debit_account", "inward_reference", "disposal_credit_account", "disposal_debit_account", "credit_advice_bill_number", "credit_advice_credit_account", "credit_advice_debit_account"} {
		if value := str(p, key); len(value) > 160 || strings.ContainsAny(value, "\r\n\t\x00") {
			return bad("Enter " + strings.ReplaceAll(key, "_", " ") + " on one line, up to 160 characters.")
		}
	}
	return nil
}
