package app

import (
	"strings"
	"testing"
)

func TestExportPartyOptionName(t *testing.T) {
	for _, tc := range []struct {
		name    string
		value   any
		want    string
		invalid bool
	}{
		{"keeps exact company punctuation", "EURO TROUSERS MFG,CO, (FZC)", "EURO TROUSERS MFG,CO, (FZC)", false},
		{"trims and collapses spaces", "  P & C  RETAIL UK LTD  ", "P & C RETAIL UK LTD", false},
		{"keeps user spelling", "ERRY ELLIS EUROPE LIMITED", "ERRY ELLIS EUROPE LIMITED", false},
		{"keeps case", "New Customer Ltd", "New Customer Ltd", false},
		{"normalizes unicode spaces", "Customer\u00a0Name", "Customer Name", false},
		{"empty", "  ", "", true},
		{"missing", nil, "", true},
		{"not text", 1, "", true},
		{"line break", "Customer\nName", "", true},
		{"tab", "Customer\tName", "", true},
		{"control", "Customer\x00Name", "", true},
		{"maximum", strings.Repeat("A", 160), strings.Repeat("A", 160), false},
		{"too long", strings.Repeat("A", 161), "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := exportPartyOptionName(tc.value)
			if (err != nil) != tc.invalid || got != tc.want {
				t.Fatalf("got %q, err=%v; want %q, invalid=%v", got, err, tc.want, tc.invalid)
			}
		})
	}
}
