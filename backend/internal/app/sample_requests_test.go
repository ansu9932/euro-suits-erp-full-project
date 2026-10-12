package app

import (
	"encoding/json"
	"testing"
)

func TestSampleVariantsPerPiece(t *testing.T) {
	valid := []any{M{"size": "M", "colour": "Navy", "quantity": json.Number("2")}, M{"size": "XL", "colour": "Grey", "quantity": json.Number("1")}}
	rows, e := sampleVariants(valid)
	if e != nil || len(rows) != 2 || id(rows[0], "quantity")+id(rows[1], "quantity") != 3 {
		t.Fatal(rows, e)
	}
	for _, q := range []any{0, -1, 1.5, 201, "bad"} {
		if _, e = sampleVariants([]any{M{"size": "M", "colour": "Navy", "quantity": q}}); e == nil {
			t.Fatalf("accepted %v", q)
		}
	}
}
func TestSampleOnlyMDApproves(t *testing.T) {
	for _, r := range []string{"Super Admin", "Marketing", "Sampling", "Quality"} {
		u := &User{Role: r}
		if sampleApprove(u) {
			t.Fatalf("%s can approve", r)
		}
	}
}
func TestSampleDatesAndBOM(t *testing.T) {
	if _, e := sampleStrings(M{"plan_date": "2026-02-31"}, sampleFields); e == nil {
		t.Fatal("invalid date accepted")
	}
	if _, e := sampleBOM([]any{M{"group": "Sewing trims", "component": "Thread", "quantity": "NaN"}}); e == nil {
		t.Fatal("NaN accepted")
	}
}
