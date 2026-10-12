package app

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
)

func (a *App) enquiryExport(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	m, e := a.enquiryRecord(r.Context(), a.DB, u, materialID(r), false)
	if e != nil {
		return nil, e
	}
	m = enquiryPublic(u, m)
	body := [][]string{{"Enquiry", enquiryReference(m)}, {"Status", str(m, "status")}, {"Buyer", str(m, "buyer_name")}, {"Merchant", str(m, "merchant")}}
	if id(m, "booking_number") > 0 {
		body = append(body, []string{"Booking", fmt.Sprintf("BK-%06d", id(m, "booking_number"))})
	}
	appendMap := func(section string, p M, keys []string) {
		body = append(body, []string{section, ""})
		for _, k := range keys {
			if v, ok := p[k]; ok && fmt.Sprint(v) != "" {
				body = append(body, []string{strings.ReplaceAll(k, "_", " "), fmt.Sprint(v)})
			}
		}
	}
	appendMap("Enquiry details", materialProfile(m["profile"]), enquiryProfileKeys)
	body = append(body, []string{"Colours and quantities", ""})
	for _, c := range enquiryRows(m["colours"]) {
		body = append(body, []string{str(c, "colour") + " · " + str(c, "buyer_colour_code"), fmt.Sprint(c["quantity"]) + " pieces"})
	}
	appendMap("Fabric response", materialProfile(m["fabric"]), enquiryFabricKeys)
	if enquiryCostRead(u) {
		appendMap("Costing", materialProfile(m["costing"]), enquiryCostKeys)
		for _, c := range enquiryRows(materialProfile(m["costing"])["components"]) {
			body = append(body, []string{str(c, "category") + " · " + str(c, "description"), fmt.Sprintf("%v × %v %s · FX %v", c["quantity"], c["unit_price"], str(c, "currency"), c["exchange_rate"])})
		}
		if calc, err := enquiryCalculation(m); err == nil {
			keys := []string{}
			for k := range calc {
				keys = append(keys, k)
			}
			sort.Strings(keys)
			appendMap("Calculated values", calc, keys)
		}
	}
	b, e := simpleWorkbook([]string{"Field", "Value"}, body)
	if e != nil {
		return nil, e
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="`+enquiryReference(m)+`.xlsx"`)
	w.Header().Set("Cache-Control", "private, no-store")
	w.Write(b)
	return nil, nil
}
