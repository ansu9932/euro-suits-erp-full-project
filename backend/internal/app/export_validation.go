package app

import (
	"fmt"
	"math/big"
	"regexp"
	"slices"
	"strings"
	"time"
)

var exportCurrencies = strings.Fields("AED AFN ALL AMD ANG AOA ARS AUD AWG AZN BAM BBD BDT BGN BHD BIF BMD BND BOB BRL BSD BTN BWP BYN BZD CAD CDF CHF CLP CNY COP CRC CUP CVE CZK DJF DKK DOP DZD EGP ERN ETB EUR FJD FKP GBP GEL GHS GIP GMD GNF GTQ GYD HKD HNL HTG HUF IDR ILS INR IQD IRR ISK JMD JOD JPY KES KGS KHR KMF KPW KRW KWD KYD KZT LAK LBP LKR LRD LSL LYD MAD MDL MGA MKD MMK MNT MOP MRU MUR MVR MWK MXN MYR MZN NAD NGN NIO NOK NPR NZD OMR PAB PEN PGK PHP PKR PLN PYG QAR RON RSD RUB RWF SAR SBD SCR SDG SEK SGD SHP SLE SOS SRD SSP STN SVC SYP SZL THB TJS TMT TND TOP TRY TTD TWD TZS UAH UGX USD UYU UZS VES VND VUV WST XAF XCD XOF XPF YER ZAR ZMW ZWG")
var exportMoneyPattern = regexp.MustCompile(`^(0|[1-9][0-9]{0,14})(\.[0-9]{1,2})?$`)
var exportRatePattern = regexp.MustCompile(`^(0|[1-9][0-9]{0,14})(\.[0-9]{1,6})?$`)
var exportProfileText = strings.Fields("export_reference export_date invoice_date transport_document_date port_of_loading port_of_departure shipping_bill_number shipping_bill_date eway_bill_number shipping_mode transport_document_number advance_license_number payment_details adjustment_note customs_house_agent freight_forwarder clearing_invoice port_code linked_bank scrip_reference scrip_tally_reference scrip_bank_credit shipment_status shipment_note incentive_status incentive_note scrip_note clearance_note")
var exportProfileMoney = map[string]string{"drawback_claim_amount": "drawback_claim_currency", "rosctl_amount": "rosctl_currency", "scrip_sale_amount": "scrip_sale_currency", "scrip_bank_credit_amount": "scrip_bank_credit_currency"}
var exportReceiptText = strings.Fields("receipt_key inward_reference inward_due_date disposal_credit_account disposal_debit_account credit_advice_bill_number credit_advice_credit_account credit_advice_debit_account reference receipt_date receipt_bank receipt_advice receipt_tally_reference credit_bank_017 packing_credit bank_advice_reference bank_advice_date receiving_bank note receipt_note exchange_rate_note conversion_note linked_bank forex_account instruction_action fc_account_currency disposal_credit_target bank_instruction_reference inr_advice_reference inr_advice_date credit_bank_344 conversion_debit_account")
var exportReceiptMoney = map[string]string{"fcr_amount": "currency", "bank_advice_amount": "currency", "inr_received_amount": "inr_currency", "inr_bank_charges": "inr_currency", "credit_bank_344_amount": "credit_bank_344_currency", "gst_amount": "gst_currency", "gross_payment_received": "currency", "net_payment_received": "currency", "payment_received": "currency", "invoice_currency_amount": "invoice_currency", "foreign_bank_charges": "foreign_bank_charges_currency", "foreign_bank_charges_invoice_amount": "invoice_currency", "bank_charges": "bank_charges_currency", "bank_charges_invoice_amount": "invoice_currency", "converted_amount": "converted_currency", "conversion_amount": "conversion_currency", "credit_bank_017_amount": "credit_bank_017_currency", "packing_credit_amount": "packing_credit_currency"}
var exportEBRCText = strings.Fields("export_reference shipping_bill_number shipping_bill_date ebrc_file_number ebrc_number ebrc_note status")
var exportEBRCMoney = map[string]string{"ebrc_amount": "currency", "ebrc_balance_amount": "currency"}

// The scale is exactly two throughout settlement. No float64 is involved.
func exportCents(v any, required bool) (*big.Int, error) {
	if v == nil || v == "" {
		if required {
			return nil, bad("Enter the amount as a decimal string.")
		}
		return nil, nil
	}
	s, ok := v.(string)
	if !ok || !exportMoneyPattern.MatchString(s) {
		return nil, bad("Amounts must be non-negative decimal strings with at most two decimal places and fifteen whole digits.")
	}
	parts := strings.Split(s, ".")
	frac := "00"
	if len(parts) == 2 {
		frac = (parts[1] + "0")[:2]
	}
	n, _ := new(big.Int).SetString(parts[0]+frac, 10)
	return n, nil
}
func exportDecimal(n *big.Int) string {
	if n == nil {
		return ""
	}
	a := new(big.Int).Abs(n)
	s := a.String()
	for len(s) < 3 {
		s = "0" + s
	}
	sign := ""
	if n.Sign() < 0 {
		sign = "-"
	}
	return sign + s[:len(s)-2] + "." + s[len(s)-2:]
}
func exportObject(v any) (M, error) {
	if v == nil {
		return M{}, nil
	}
	switch p := v.(type) {
	case M:
		return p, nil
	default:
		return nil, bad("Enter an object of export fields.")
	}
}
func exportCurrency(v string) bool { return slices.Contains(exportCurrencies, v) }
func exportValidateObject(v any, texts []string, money map[string]string, rates []string) (M, error) {
	p, e := exportObject(v)
	if e != nil {
		return nil, e
	}
	out := M{}
	allowed := append(append([]string{}, texts...), rates...)
	allowed = append(allowed, "statuses")
	for key, currency := range money {
		allowed = append(allowed, key, currency)
	}
	if e = allowedFields(p, allowed...); e != nil {
		return nil, e
	}
	for key, value := range p {
		if value == nil || key == "statuses" {
			continue
		}
		s, ok := value.(string)
		if !ok {
			return nil, bad("Enter text for " + strings.ReplaceAll(key, "_", " ") + ".")
		}
		s = strings.TrimSpace(s)
		if len(s) > 2000 || strings.ContainsRune(s, 0) {
			return nil, bad("Export field is too long or contains invalid characters.")
		}
		if strings.HasSuffix(key, "_date") && key != "jv_by_date" && s != "" {
			d, err := time.Parse("2006-01-02", s)
			if err != nil || d.Format("2006-01-02") != s {
				return nil, bad("Use YYYY-MM-DD for export dates.")
			}
		}
		if strings.HasSuffix(key, "currency") && s != "" && !exportCurrency(s) {
			return nil, bad("Choose a supported currency.")
		}
		out[key] = s
	}
	for key, cur := range money {
		n, err := exportCents(out[key], false)
		if err != nil {
			return nil, bad(strings.ReplaceAll(key, "_", " ") + ": " + err.Error())
		}
		if n == nil {
			delete(out, key)
			continue
		}
		if !exportCurrency(str(out, cur)) {
			return nil, bad("Choose the currency for " + strings.ReplaceAll(key, "_", " ") + ".")
		}
		out[key] = exportDecimal(n)
	}
	for _, key := range rates {
		s := str(out, key)
		if s == "" {
			continue
		}
		if !exportRatePattern.MatchString(s) {
			return nil, bad("Enter a decimal value with up to six decimal places.")
		}
		n, _ := new(big.Rat).SetString(s)
		if key == "exchange_rate" && n.Sign() <= 0 {
			return nil, bad("Exchange rate must be positive.")
		}
		if key == "scrip_sale_percentage" && n.Cmp(big.NewRat(100, 1)) > 0 {
			return nil, bad("Percentage cannot exceed 100.")
		}
	}
	statuses, e := exportObject(p["statuses"])
	if e != nil {
		return nil, e
	}
	clean := M{}
	for key, value := range statuses {
		s, ok := value.(string)
		if !ok || key == "statuses" || !slices.Contains(allowed, key) || len(s) > 160 || strings.ContainsRune(s, 0) {
			return nil, bad("Invalid export field status.")
		}
		if strings.TrimSpace(s) != "" {
			clean[key] = strings.TrimSpace(s)
		}
	}
	out["statuses"] = clean
	return out, nil
}
func exportValidateArrays(v any, kind, invoiceCurrency string) ([]M, error) {
	if v == nil {
		return []M{}, nil
	}
	var input []any
	switch xs := v.(type) {
	case []any:
		input = xs
	case []M:
		for _, x := range xs {
			input = append(input, x)
		}
	default:
		return nil, bad("Enter a list of " + kind + ".")
	}
	if len(input) > 100 {
		return nil, bad("An invoice supports up to 100 " + kind + ".")
	}
	out := []M{}
	certificates := map[string]bool{}
	receiptKeys := map[string]bool{}
	for _, raw := range input {
		p, e := exportObject(raw)
		if e != nil {
			return nil, e
		}
		texts, money := exportEBRCText, exportEBRCMoney
		rates := []string{}
		if kind == "receipts" {
			texts, money, rates = exportReceiptText, exportReceiptMoney, []string{"exchange_rate", "inr_conversion_rate"}
			p = materialProfile(p)
			if rawKey, supplied := p["receipt_key"]; supplied {
				key, keyErr := exportReceiptKey(rawKey)
				if keyErr != nil {
					return nil, keyErr
				}
				if receiptKeys[key] {
					return nil, bad("Each receipt must have a different receipt key.")
				}
				receiptKeys[key], p["receipt_key"] = true, key
			}
			if s := str(p, "invoice_currency"); s != "" && s != invoiceCurrency {
				return nil, bad("The settlement currency must match the invoice.")
			}
			p["invoice_currency"] = invoiceCurrency
			p, e = exportPrepareBankingReceipt(p)
			if e != nil {
				return nil, e
			}
		}
		m, e := exportValidateObject(p, texts, money, rates)
		if e != nil {
			return nil, e
		}
		if kind == "receipts" {
			if e = exportValidateBankingReceipt(m); e != nil {
				return nil, e
			}
			for _, pair := range [][3]string{{"payment_received", "currency", "invoice_currency_amount"}, {"foreign_bank_charges", "foreign_bank_charges_currency", "foreign_bank_charges_invoice_amount"}, {"bank_charges", "bank_charges_currency", "bank_charges_invoice_amount"}} {
				n, _ := exportCents(m[pair[0]], false)
				equiv, _ := exportCents(m[pair[2]], false)
				if n != nil && str(m, pair[1]) != invoiceCurrency && equiv == nil {
					return nil, bad("Enter an invoice-currency equivalent for cross-currency " + strings.ReplaceAll(pair[0], "_", " ") + ".")
				}
				if equiv != nil && (n == nil || (str(m, pair[1]) == invoiceCurrency && n.Cmp(equiv) != 0)) {
					return nil, bad("Invoice-currency equivalents must match the original amount when currencies match.")
				}
			}
		}
		if kind != "receipts" {
			certificate := strings.ToUpper(strings.Join(strings.Fields(str(m, "ebrc_number")), " "))
			if certificate != "" {
				if certificates[certificate] {
					return nil, bad("This eBRC certificate number is already recorded on the invoice.")
				}
				certificates[certificate] = true
			}
			if certificate == "" && !slices.Contains([]string{"Pending", "Not required"}, str(m, "status")) {
				return nil, bad("Enter the eBRC number, or mark the record Pending or Not required.")
			}
		}
		out = append(out, m)
	}
	return out, nil
}
func exportSettlementValue(p M, amount, currency, equivalent, invoiceCurrency string) *big.Int {
	key := amount
	if str(p, currency) != invoiceCurrency {
		key = equivalent
	}
	n, _ := exportCents(p[key], false)
	if n == nil {
		return new(big.Int)
	}
	return n
}

// Approved business basis: receipts and each separate recorded charge reduce the
// balance once. Documentary gross/net, FX and eBRC amounts never enter this sum.
func exportBalance(m M) M {
	inv, _ := exportCents(m["invoice_amount"], false)
	if inv == nil {
		inv = new(big.Int)
	}
	paid, foreign, bank, discount := new(big.Int), new(big.Int), new(big.Int), new(big.Int)
	cur := str(m, "currency")
	for _, p := range enquiryRows(m["receipts"]) {
		paid.Add(paid, exportSettlementValue(p, "payment_received", "currency", "invoice_currency_amount", cur))
		foreign.Add(foreign, exportSettlementValue(p, "foreign_bank_charges", "foreign_bank_charges_currency", "foreign_bank_charges_invoice_amount", cur))
		bank.Add(bank, exportSettlementValue(p, "bank_charges", "bank_charges_currency", "bank_charges_invoice_amount", cur))
	}
	if n, _ := exportCents(m["approved_discount"], false); n != nil {
		discount = n
	}
	balance := new(big.Int).Set(inv)
	for _, n := range []*big.Int{paid, foreign, bank, discount} {
		balance.Sub(balance, n)
	}
	return M{"invoice_amount": exportDecimal(inv), "bill_amount": exportDecimal(inv), "payment_received": exportDecimal(paid), "foreign_bank_charges": exportDecimal(foreign), "bank_charges": exportDecimal(bank), "approved_discount": exportDecimal(discount), "outstanding": exportDecimal(balance), "currency": cur, "basis": "Invoice − receipts − foreign bank charges − bank charges − approved discounts", "overpaid": balance.Sign() < 0}
}
func exportDiscountFits(amount string, balance M) bool {
	n, e := exportCents(amount, true)
	if e != nil || n.Sign() <= 0 {
		return false
	}
	out, _ := new(big.Rat).SetString(str(balance, "outstanding"))
	if out == nil {
		return false
	}
	return new(big.Rat).SetFrac(n, big.NewInt(100)).Cmp(out) <= 0
}
func exportConflict() error {
	return failure{409, "This invoice changed. Refresh before saving or approving."}
}
func exportValidateBody(d M) (M, error) {
	if e := allowedFields(d, "record_kind", "company_id", "party_type", "party_id", "party_name", "order_id", "invoice_number", "currency", "invoice_amount", "profile", "receipts", "ebrcs", "version"); e != nil {
		return nil, e
	}
	kind := "export"
	if value, exists := d["record_kind"]; exists {
		var ok bool
		kind, ok = value.(string)
		if !ok || (kind != "export" && kind != "jv") {
			return nil, bad("Choose the Export or JV register.")
		}
	}
	if kind == "jv" {
		if str(d, "party_type") != "manual" {
			return nil, bad("JV entries belong to the standalone register.")
		}
		if e := exportJVFinancialFields(d); e != nil {
			return nil, e
		}
	}
	partyName := ""
	if str(d, "party_type") == "manual" {
		for _, key := range []string{"company_id", "party_id", "order_id"} {
			if d[key] != nil {
				return nil, bad("A standalone invoice cannot link a company, buyer or production PO.")
			}
		}
		var e error
		partyName, e = text(d, "party_name", 160)
		if e != nil || strings.ContainsAny(partyName, "\r\n\t\x00") {
			return nil, bad("Enter a customer name on one line, up to 160 characters.")
		}
	} else {
		for _, key := range []string{"company_id", "party_id"} {
			n, e := integer(d[key])
			if e != nil || n < 1 {
				return nil, bad("Choose the " + strings.ReplaceAll(key, "_id", "") + ".")
			}
		}
		if _, supplied := d["party_name"]; supplied {
			return nil, bad("Company-linked invoices use the selected buyer name.")
		}
	}
	if d["order_id"] != nil {
		if _, e := integer(d["order_id"]); e != nil {
			return nil, e
		}
	}
	number, e := text(d, "invoice_number", 100)
	if e != nil {
		return nil, e
	}
	if strings.ContainsAny(number, "\r\n\t\x00") {
		return nil, bad("Enter a single-line invoice number.")
	}
	cur := str(d, "currency")
	if !exportCurrency(cur) {
		return nil, bad("Choose the invoice currency.")
	}
	n, e := exportCents(d["invoice_amount"], true)
	if e != nil {
		return nil, e
	}
	texts, money, rates := exportProfileText, exportProfileMoney, []string{"scrip_sale_percentage"}
	if kind == "jv" {
		texts, money, rates = exportJVProfileText, map[string]string{}, nil
	}
	p, e := exportValidateObject(d["profile"], texts, money, rates)
	if e != nil {
		return nil, e
	}
	if kind == "jv" {
		if len(materialProfile(p["statuses"])) != 0 {
			return nil, bad("JV entries do not support unrelated field statuses.")
		}
		if e = exportJVMode(str(p, "shipping_mode")); e != nil {
			return nil, e
		}
	} else if mode := str(p, "shipping_mode"); mode != "" && !slices.Contains([]string{"Sea", "Air", "Courier", "Hand-carried"}, mode) {
		return nil, bad("Choose Sea, Air, Courier or Hand-carried.")
	}
	receipts, e := exportValidateArrays(d["receipts"], "receipts", cur)
	if e != nil {
		return nil, e
	}
	ebrcs, e := exportValidateArrays(d["ebrcs"], "eBRCs", cur)
	if e != nil {
		return nil, e
	}
	return M{"record_kind": kind, "company_id": id(d, "company_id"), "party_type": str(d, "party_type"), "party_id": id(d, "party_id"), "party_name": partyName, "order_id": id(d, "order_id"), "invoice_number": number, "currency": cur, "invoice_amount": exportDecimal(n), "profile": p, "receipts": receipts, "ebrcs": ebrcs}, nil
}
func exportString(v any) string {
	if v == nil {
		return ""
	}
	return fmt.Sprint(v)
}
