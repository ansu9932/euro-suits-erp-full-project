package app

import (
	"context"
	"encoding/json"
	"github.com/xuri/excelize/v2"
	"reflect"
	"testing"
	"time"
)

func TestSampleCapacityDates(t *testing.T) {
	start, _ := time.Parse("2006-01-02", "2026-10-03") // Saturday
	rules := []sampleCapacityRule{{"2026-10-01", 2, []int{0}, []string{"2026-10-05"}}, {"2026-10-07", 1, []int{0}, nil}}
	actual, e := sampleDates(start, []string{"2026-10-03", "2026-10-03", "2026-10-06", "2026-10-07", "2026-10-07"}, rules, map[string]int{"2026-10-03": 1})
	want := []string{"2026-10-03", "2026-10-06", "2026-10-06", "2026-10-07", "2026-10-08"}
	if e != nil || !reflect.DeepEqual(actual, want) {
		t.Fatal(actual, e)
	}
	if _, e = sampleDates(start, []string{"2026-10-03"}, nil, map[string]int{}); e == nil {
		t.Fatal("scheduled with no capacity")
	}
}
func TestSampleAuditNumbersAndTampering(t *testing.T) {
	report := M{"fabric_used": "Actual", "trims_used": "Actual", "style_used": "Actual", "submission_attempt": "1", "auditor": "FORGED", "measurements": []any{M{"point": "Chest", "required": json.Number("100"), "actual": json.Number("100.5"), "unit": "cm"}}}
	p, e := sampleAudit(report)
	if e != nil {
		t.Fatal(e)
	}
	if p["auditor"] != nil {
		t.Fatal("accepted forged actor")
	}
	report["measurements"].([]any)[0].(M)["actual"] = "NaN"
	if _, e = sampleAudit(report); e == nil {
		t.Fatal("accepted NaN")
	}
}
func TestSampleStageGuards(t *testing.T) {
	producer := &User{Role: "Sampling", Name: "Coordinator"}
	merchant := &User{Role: "Marketing"}
	p := M{}
	if _, e := sampleAdvance(producer, "Ready", "sew", M{}, p, "now"); e == nil {
		t.Fatal("skipped cutting")
	}
	if _, e := sampleAdvance(merchant, "Pattern ready", "cut", M{}, p, "now"); e == nil {
		t.Fatal("merchant changed production")
	}
	if _, e := sampleAdvance(producer, "Awaiting audit", "audit", M{}, p, "now"); e == nil {
		t.Fatal("coordinator approved QA")
	}
	if _, e := sampleAdvance(producer, "Handed over", "plan", M{}, p, "now"); e == nil {
		t.Fatal("edited closed indent")
	}
}
func TestSampleSpecPreview(t *testing.T) {
	f := excelize.NewFile()
	defer f.Close()
	f.SetCellValue("Sheet1", "A1", "Measurement")
	f.SetCellValue("Sheet1", "B1", "M")
	f.SetCellValue("Sheet1", "A2", "Chest")
	f.SetCellValue("Sheet1", "B2", 100)
	raw, e := f.WriteToBuffer()
	if e != nil {
		t.Fatal(e)
	}
	tables, e := sampleSpecTables(context.Background(), "buyer.xlsx", raw.Bytes())
	if e != nil || len(tables) != 1 {
		t.Fatal(tables, e)
	}
	if tables[0]["rows"].([][]string)[1][1] != "100" {
		t.Fatal(tables)
	}
	tables, e = sampleSpecTables(context.Background(), "buyer.csv", []byte("Point,Required\nChest,100\n"))
	if e != nil || len(tables) != 1 {
		t.Fatal(e)
	}
	if _, e = sampleSpecTables(context.Background(), "buyer.exe", []byte("x")); e == nil {
		t.Fatal("accepted unsupported file")
	}
	if _, e = sampleSpecTables(context.Background(), "buyer.xlsx", []byte("invalid")); e == nil {
		t.Fatal("accepted corrupt workbook")
	}
}
