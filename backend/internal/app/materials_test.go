package app

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"strings"
	"testing"
)

func TestMaterialRedactionDoesNotLeakCommercialData(t *testing.T) {
	u := &User{Role: "Marketing"}
	original := M{"id": 1, "company_id": 1, "kind": "fabric", "code": "ESF-0000001", "supplier_setup_id": 99, "supplier_rating": "Good", "profile": M{"article": "SECRET-ART", "price": "123.50", "supplier": "SECRET-SUPPLIER", "category": 1, "weave": 2, "composition": 3, "color": 4, "measurements": []M{{"weight": 200, "width": 58, "weight_unit": "GSM", "width_unit": "inch", "actual_weight": 999, "inspection_notes": "SECRET"}}}}
	b, _ := json.Marshal(redactMaterial(u, original))
	for _, forbidden := range []string{"SECRET", "price", "supplier_setup_id", "supplier_rating", "actual_weight", "inspection_notes"} {
		if bytes.Contains(b, []byte(forbidden)) {
			t.Fatalf("redaction leaked %s", forbidden)
		}
	}
	if !bytes.Contains(b, []byte("ESF-0000001")) {
		t.Fatal("public reference missing")
	}
	admin := &User{Role: "Super Admin"}
	if redactMaterial(admin, original)["supplier_setup_id"] == nil {
		t.Fatal("commercial view unexpectedly redacted")
	}
}
func TestMaterialProfileRejectsInvalidMeasurements(t *testing.T) {
	app := &App{}
	for _, m := range []M{{"weight": -1, "width": 58, "weight_unit": "GSM", "width_unit": "inch"}, {"weight": 200, "weight_max": 100, "width": 58, "weight_unit": "GSM", "width_unit": "inch"}, {"weight": 200, "width": 58, "weight_unit": "bad", "width_unit": "inch"}} {
		_, e := app.validateMaterial(context.Background(), nil, 1, "trim", M{"received_on": "2026-09-26", "article": "TEST", "measurements": []M{m}})
		if e == nil {
			t.Fatal("invalid measurement accepted")
		}
	}
	p, e := app.validateMaterial(context.Background(), nil, 1, "trim", M{"received_on": "2026-09-26", "article": "TEST", "price": "2.50", "admin": true, "measurements": []M{{"weight": 200, "width": 58, "weight_unit": "GSM", "width_unit": "inch"}}})
	if e != nil {
		t.Fatal(e)
	}
	if p["admin"] != nil {
		t.Fatal("unknown field stored")
	}
}
func TestFabricPhotosAreImagesAndLoseMetadata(t *testing.T) {
	if _, _, e := cleanMaterialPhoto("bad.svg", []byte("<svg/>")); e == nil {
		t.Fatal("SVG accepted")
	}
	if _, _, e := cleanMaterialPhoto("bad.png", []byte("<script/>")); e == nil {
		t.Fatal("fake PNG accepted")
	}
	pic := image.NewRGBA(image.Rect(0, 0, 3, 3))
	pic.Set(1, 1, color.RGBA{20, 60, 80, 255})
	var b bytes.Buffer
	png.Encode(&b, pic)
	b.WriteString("PRIVATE CAMERA METADATA")
	clean, mime, e := cleanMaterialPhoto("sample.png", b.Bytes())
	if e != nil {
		t.Fatal(e)
	}
	if mime != "image/png" || bytes.Contains(clean, []byte("PRIVATE CAMERA")) {
		t.Fatal("photo did not discard trailing data")
	}
}
func TestWorkbookValuesAreLiteralAndEscaped(t *testing.T) {
	b, e := simpleWorkbook([]string{"Reference"}, [][]string{{"=HYPERLINK(\"x\")"}, {"<&>"}})
	if e != nil {
		t.Fatal(e)
	}
	z, e := zip.NewReader(bytes.NewReader(b), int64(len(b)))
	if e != nil {
		t.Fatal(e)
	}
	for _, f := range z.File {
		if f.Name == "xl/worksheets/sheet1.xml" {
			r, _ := f.Open()
			body, _ := io.ReadAll(r)
			r.Close()
			s := string(body)
			if strings.Contains(s, "<f>") || !strings.Contains(s, "&lt;&amp;&gt;") || !strings.Contains(s, `t="inlineStr"`) {
				t.Fatal("unsafe spreadsheet content")
			}
			return
		}
	}
	t.Fatal("sheet missing")
}
func TestEmptyMeasurementsExport(t *testing.T) {
	if got := materialMeasureText(M{}, "weight"); got != "" {
		t.Fatal(got)
	}
}
func TestMaterialProfileKeepsNumericReferences(t *testing.T) {
	p := materialProfile(M{"category": int64(42)})
	if id(p, "category") != 42 {
		t.Fatal("numeric reference lost in JSON conversion")
	}
}

func TestTrimPackQuantityValidation(t *testing.T) {
	a := &App{}
	for _, value := range []any{-1, "not a quantity"} {
		if _, e := a.validateMaterial(context.Background(), nil, 1, "trim", M{"received_on": "2026-09-26", "article": "TEST", "units_per_pack": value}); e == nil {
			t.Fatal("invalid pack quantity accepted")
		}
	}
	p, e := a.validateMaterial(context.Background(), nil, 1, "trim", M{"received_on": "2026-09-26", "article": "TEST", "units_per_pack": 5000})
	if e != nil || p["units_per_pack"] != "5000" {
		t.Fatalf("numeric pack quantity lost: %v %v", p, e)
	}
}
