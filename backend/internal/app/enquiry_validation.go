package app

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"
)

var enquiryDefaults = map[string][]string{
	"enquiry_type": {"Development", "Fabric enquiry", "Booking enquiry", "Query costing", "CM - Production", "Production"},
	"incoterm":     {"FOB", "CNF", "CIF", "EXWORK", "DDU", "DDP", "DDP PNPK"},
	"currency":     {"INR", "USD", "EUR", "GBP", "AED"},
	"product":      {"Trousers", "Suits", "Shirts", "Blazers", "Waistcoats", "General"},
	"season":       {"Summer", "Winter", "Spring", "Autumn", "S/S", "AW"},
	"brand":        {},
}
var enquiryProfileKeys = strings.Fields(`enquiry_type sale_type brand season year buyer_style product initial_costing_reference et_reference consumption garment_value garment_currency garment_delivery_date fabric_sailing_date fabric_status description packing washing embroidery incoterm lpo_number lut_number remarks quotation_date quotation_email quoted_price final_price negotiation_note sale_sgst_percent sale_cgst_percent`)
var enquiryFabricKeys = strings.Fields(`price currency incoterm unit gsm width_metres exchange_rate freight_metre duty_metre advance_license_metre sgst_percent cgst_percent purchase_type advance_percent moq_order moq_colour ex_mill_days transit_days available_metres response_date order_confirmed_date clearance_mode supplier_reference payment_terms gst_paid_reference gst_claim_reference remarks`)
var enquiryCostKeys = strings.Fields(`currency fabric_exchange_rate cm trims special_trims labels pocketing fusing general_trims washing embroidery development testing fob_other rejection_percent finance_percent overhead_percent margin_percent commission_percent remarks`)

func enquiryStrings(raw any, keys []string) (M, error) {
	p := materialProfile(raw)
	out := M{}
	for _, k := range keys {
		value := p[k]
		s := ""
		if value != nil {
			s = strings.TrimSpace(fmt.Sprint(value))
		}
		if len(s) > 2000 {
			return nil, bad("Enquiry field is too long: " + strings.ReplaceAll(k, "_", " "))
		}
		if strings.HasSuffix(k, "_date") && s != "" {
			if _, e := time.Parse("2006-01-02", s); e != nil {
				return nil, bad("Enter a valid " + strings.ReplaceAll(k, "_", " "))
			}
		}
		out[k] = s
	}
	return out, nil
}
func enquiryCurrency(p M, key string) error {
	s := strings.ToUpper(str(p, key))
	p[key] = s
	if s != "" && (len(s) != 3 || strings.IndexFunc(s, func(r rune) bool { return r < 'A' || r > 'Z' }) >= 0) {
		return bad("Currency must be a three-letter code.")
	}
	return nil
}
func enquiryValidateProfile(raw any) (M, error) {
	p, e := enquiryStrings(raw, enquiryProfileKeys)
	if e != nil {
		return nil, e
	}
	if !slices.Contains([]string{"Export", "Local"}, str(p, "sale_type")) {
		return nil, bad("Choose export or local sale.")
	}
	if !slices.Contains([]string{"", "Stock", "To be purchased", "None"}, str(p, "fabric_status")) {
		return nil, bad("Choose a valid fabric status.")
	}
	for _, key := range []string{"consumption", "garment_value", "quoted_price", "final_price", "sale_sgst_percent", "sale_cgst_percent"} {
		if _, e = costDecimal(p[key], key); e != nil {
			return nil, e
		}
	}
	for _, k := range []string{"sale_sgst_percent", "sale_cgst_percent"} {
		n, _ := costDecimal(p[k], k)
		f, _ := n.Float64()
		if f > 100 {
			return nil, bad("Sales tax percentages cannot exceed 100.")
		}
	}
	if str(p, "sale_type") == "Export" && (positiveCost(p, "sale_sgst_percent") || positiveCost(p, "sale_cgst_percent")) {
		return nil, bad("Export enquiries use the LUT reference. Remove local sales tax rates.")
	}
	if e = enquiryCurrency(p, "garment_currency"); e != nil {
		return nil, e
	}
	return p, nil
}
func enquiryColours(raw any) ([]M, int64, error) {
	list, ok := raw.([]any)
	if !ok || len(list) < 1 || len(list) > 100 {
		return nil, 0, bad("Add between 1 and 100 colour rows.")
	}
	out := []M{}
	total := int64(0)
	seen := map[string]bool{}
	for _, value := range list {
		m := materialProfile(value)
		colour, code := str(m, "colour"), str(m, "buyer_colour_code")
		q, e := integer(m["quantity"])
		if e != nil || q < 1 || q > 10000000 {
			return nil, 0, bad("Every colour needs a whole quantity from 1 to 10,000,000.")
		}
		if colour == "" || len(colour) > 150 || len(code) > 80 {
			return nil, 0, bad("Enter a colour (up to 150 characters) and an optional buyer colour code (up to 80).")
		}
		key := strings.ToLower(colour + "|" + code)
		if seen[key] {
			return nil, 0, bad("This colour and buyer code already has a quantity row.")
		}
		seen[key] = true
		total += q
		if total > 10000000 {
			return nil, 0, bad("Total quantity cannot exceed 10,000,000 pieces.")
		}
		out = append(out, M{"colour": colour, "buyer_colour_code": code, "quantity": q})
	}
	return out, total, nil
}
func enquiryQuantity(v any) int64 {
	var total int64
	for _, m := range enquiryRows(v) {
		total += id(m, "quantity")
	}
	return total
}

func enquiryValidateFabric(raw any) (M, error) {
	p, e := enquiryStrings(raw, enquiryFabricKeys)
	if e != nil {
		return nil, e
	}
	if !slices.Contains([]string{"", "None", "Advance licence", "Duty"}, str(p, "clearance_mode")) {
		return nil, bad("Choose the import clearance method.")
	}
	if positiveCost(p, "duty_metre") && positiveCost(p, "advance_license_metre") {
		return nil, bad("Use either duty or advance licence cost, not both.")
	}
	if positiveCost(p, "duty_metre") && str(p, "clearance_mode") != "Duty" || positiveCost(p, "advance_license_metre") && str(p, "clearance_mode") != "Advance licence" {
		return nil, bad("The import cost must match the clearance method.")
	}
	p["tax_recoverable"] = materialProfile(raw)["tax_recoverable"] == true
	if !slices.Contains([]string{"", "Local", "Import"}, str(p, "purchase_type")) {
		return nil, bad("Choose a local or import purchase.")
	}
	if !slices.Contains([]string{"MTR", "YDS", "KGS"}, str(p, "unit")) {
		return nil, bad("Choose the fabric price unit.")
	}
	if e = enquiryCurrency(p, "currency"); e != nil {
		return nil, e
	}
	for _, k := range strings.Fields("price gsm width_metres exchange_rate freight_metre duty_metre advance_license_metre sgst_percent cgst_percent advance_percent moq_order moq_colour available_metres") {
		n, err := costDecimal(p[k], k)
		if err != nil {
			return nil, err
		}
		if strings.HasSuffix(k, "_percent") {
			f, _ := n.Float64()
			if f > 100 {
				return nil, bad("Percentages cannot exceed 100.")
			}
		}
	}
	for _, k := range []string{"ex_mill_days", "transit_days"} {
		if str(p, k) != "" {
			n, err := integer(p[k])
			if err != nil || n < 0 || n > 3650 {
				return nil, bad("Lead-time days must be a whole number from 0 to 3,650.")
			}
		}
	}
	return p, nil
}
func enquiryValidateCost(raw any) (M, error) {
	p, e := enquiryStrings(raw, enquiryCostKeys)
	if e != nil {
		return nil, e
	}
	if e = enquiryCurrency(p, "currency"); e != nil {
		return nil, e
	}
	for _, k := range enquiryCostKeys {
		if k != "currency" && k != "remarks" {
			n, err := costDecimal(p[k], k)
			if err != nil {
				return nil, err
			}
			f, _ := n.Float64()
			if strings.HasSuffix(k, "_percent") && f > 100 {
				return nil, bad("Percentages cannot exceed 100.")
			}
		}
	}
	componentInput := materialProfile(raw)["components"]
	list := enquiryRows(componentInput)
	if componentInput != nil {
		data, err := json.Marshal(componentInput)
		var rows []M
		if err != nil {
			return nil, bad("Components must be a list.")
		}
		if err = json.Unmarshal(data, &rows); err != nil {
			return nil, bad("Components must be a list of items.")
		}
	}

	if len(list) > 100 {
		return nil, bad("A costing supports up to 100 itemised components.")
	}
	components := []M{}
	for _, row := range list {
		component, err := enquiryStrings(row, []string{"category", "description", "unit_price", "quantity", "currency", "exchange_rate"})
		if err != nil {
			return nil, err
		}
		if !slices.Contains([]string{"Special trims", "Labels", "Pocketing", "Fusing / lining", "General trims", "Washing", "Embroidery", "Other"}, str(component, "category")) || str(component, "description") == "" {
			return nil, bad("Enter a category and description for every component.")
		}
		if err = enquiryCurrency(component, "currency"); err != nil {
			return nil, err
		}
		if str(component, "currency") == "" {
			return nil, bad("Choose a currency for each component.")
		}
		for _, key := range []string{"unit_price", "quantity", "exchange_rate"} {
			if _, err = costDecimal(component[key], key); err != nil {
				return nil, err
			}
		}
		components = append(components, component)
	}
	p["components"] = components

	return p, nil
}

func enquiryRows(v any) []M {
	b, _ := json.Marshal(v)
	out := []M{}
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.UseNumber()
	_ = dec.Decode(&out)
	return out
}
