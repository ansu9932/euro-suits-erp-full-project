package app

import "strings"

var exportJVProfileText = []string{"export_date", "shipping_mode", "jv_by_date", "scrip_tally_reference"}
var exportJVHeaders = []string{"JV REF", "DATE", "PARTY NAME", "CURR", "AMOUNT", "MODE", "BY / DATE", "TALLY REF"}

func exportRecordKind(m M) string {
	if str(m, "record_kind") == "jv" {
		return "jv"
	}
	return "export"
}

func exportKind(value string) (string, error) {
	if value == "" {
		return "export", nil
	}
	if value != "export" && value != "jv" {
		return "", bad("Choose the Export or JV register.")
	}
	return value, nil
}

// Defaults belong only to creation, never to a saved record or linked master.
func exportJVCreateDefaults(d M) M {
	if str(d, "record_kind") != "jv" {
		return d
	}
	out := M{}
	for key, value := range d {
		out[key] = value
	}
	if _, exists := out["party_name"]; !exists {
		out["party_name"] = "EURO SUITS MFG"
	}
	if original, e := exportObject(d["profile"]); e == nil {
		profile := M{}
		for key, value := range original {
			profile[key] = value
		}
		if _, exists := profile["shipping_mode"]; !exists {
			profile["shipping_mode"] = "HANDO"
		}
		out["profile"] = profile
	}
	return out
}

func exportJVFinancialFields(d M) error {
	for _, key := range []string{"receipts", "ebrcs"} {
		if d[key] == nil {
			continue
		}
		switch value := d[key].(type) {
		case []any:
			if len(value) == 0 {
				continue
			}
		case []M:
			if len(value) == 0 {
				continue
			}
		}
		return bad("JV entries contain only the eight workbook fields; receipts and eBRC records are not applicable.")
	}
	return nil
}

func exportJVPermissions(u *User, m M) M {
	permissions := exportPermissions(u)
	if exportRecordKind(m) == "jv" {
		for _, key := range []string{"finance_check", "audit_check", "discount_first", "discount_final"} {
			permissions[key] = false
		}
	}
	return permissions
}

func exportJVFinancialAction(m M) error {
	if exportRecordKind(m) == "jv" {
		return bad("JV entries do not support export financial checks or discount adjustments.")
	}
	return nil
}

func exportJVSheets(records []M) []exportSheet {
	sheet := exportSheet{Name: "JV", Headers: append([]string{}, exportJVHeaders...)}
	for _, m := range records {
		p := materialProfile(m["profile"])
		sheet.Rows = append(sheet.Rows, []string{str(m, "invoice_number"), str(p, "export_date"), str(m, "party_name"), str(m, "currency"), str(m, "invoice_amount"), str(p, "shipping_mode"), str(p, "jv_by_date"), str(p, "scrip_tally_reference")})
	}
	return []exportSheet{sheet}
}

func exportJVMode(mode string) error {
	if len(mode) > 160 || strings.ContainsAny(mode, "\r\n\t\x00") {
		return bad("Enter the JV mode on one line, up to 160 characters.")
	}
	return nil
}
