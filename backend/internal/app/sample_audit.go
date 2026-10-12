package app

import (
	"encoding/json"
	"math"
	"slices"
	"strconv"
	"strings"
)

func sampleAudit(raw any) (M, error) {
	p := materialProfile(raw)
	out, e := sampleStrings(p, strings.Fields("fabric_used trims_used style_used submission_attempt special_remarks merchant_contact"))
	if e != nil {
		return nil, e
	}
	for _, key := range []string{"fabric_used", "trims_used", "style_used"} {
		if !slices.Contains([]string{"Actual", "Alternate"}, str(out, key)) {
			return nil, bad("Choose actual or alternate fabric, trims and style.")
		}
	}
	if !slices.Contains([]string{"1", "2", "3", "4"}, str(out, "submission_attempt")) {
		return nil, bad("Choose submission attempt 1 to 4.")
	}
	list, ok := p["measurements"].([]any)
	if !ok || len(list) < 1 || len(list) > 100 {
		return nil, bad("Add 1 to 100 measurement rows to the audit report.")
	}
	measurements := []M{}
	for _, raw := range list {
		m := materialProfile(raw)
		for _, key := range []string{"required", "actual", "tolerance"} {
			switch v := m[key].(type) {
			case json.Number:
				m[key] = v.String()
			case float64:
				m[key] = strconv.FormatFloat(v, 'f', -1, 64)
			}
		}
		v, e := sampleStrings(m, strings.Fields("point required actual unit tolerance remarks"))
		if e != nil {
			return nil, e
		}
		if str(v, "point") == "" || len(str(v, "point")) > 150 {
			return nil, bad("Name each measurement point (up to 150 characters).")
		}
		if !slices.Contains([]string{"cm", "mm", "in"}, str(v, "unit")) {
			return nil, bad("Choose cm, mm or inches for each measurement.")
		}
		for _, key := range []string{"required", "actual", "tolerance"} {
			if key == "tolerance" && str(v, key) == "" {
				continue
			}
			n, err := strconv.ParseFloat(str(v, key), 64)
			if err != nil || math.IsNaN(n) || math.IsInf(n, 0) || n < 0 || n > 10000 {
				return nil, bad("Enter valid non-negative specification and actual measurements.")
			}
		}
		measurements = append(measurements, v)
	}
	out["measurements"] = measurements
	return out, nil
}
