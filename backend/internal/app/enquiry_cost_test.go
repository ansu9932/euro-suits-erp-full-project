package app

import "testing"

func TestEnquiryWorkbookCalculation(t *testing.T) {
	r, e := enquiryCosting(enquiryCostInput{Consumption: "1.35", Quantity: 4000, Fabric: M{"price": "1.75", "unit": "MTR", "currency": "USD", "freight_metre": ".25", "advance_license_metre": ".2"}, Cost: M{"currency": "USD", "trims": "2", "rejection_percent": "2", "finance_percent": "3", "overhead_percent": "5", "margin_percent": "8", "commission_percent": "5"}})
	if e != nil {
		t.Fatal(e)
	}
	for k, want := range map[string]string{"required_metres": "5400.000", "landed_metre": "2.2000", "fabric_per_piece": "2.9700", "base_cost": "4.9700", "cost_per_piece": "6.1578", "total_cost": "24631.32", "target_consumption": "1.2825"} {
		if r[k] != want {
			t.Fatalf("%s: got %v want %s", k, r[k], want)
		}
	}
}

func TestEnquiryUnitAndCurrencyConversion(t *testing.T) {
	for _, tc := range []struct {
		fabric M
		want   string
	}{
		{M{"price": 6, "unit": "KGS", "gsm": 300, "width_metres": "1.5"}, "2.7000"},
		{M{"price": "0.9144", "unit": "YDS"}, "1.0000"},
		{M{"price": 2, "unit": "MTR", "currency": "USD", "exchange_rate": 80}, "160.0000"},
	} {
		r, e := enquiryCosting(enquiryCostInput{Fabric: tc.fabric, Cost: M{"currency": "INR"}})
		if e != nil || r["fabric_price_metre"] != tc.want {
			t.Fatalf("conversion %v: %v %v", tc.fabric, r, e)
		}
	}
	for _, fabric := range []M{{"price": 6, "unit": "KGS"}, {"price": 2, "currency": "USD"}, {"price": -1}, {"price": "NaN"}, {"price": "1e8"}, {"sgst_percent": 101}} {
		if _, e := enquiryCosting(enquiryCostInput{Fabric: fabric, Cost: M{"currency": "INR"}}); e == nil {
			t.Fatalf("accepted invalid %v", fabric)
		}
	}
}

func TestEnquiryRecoverableTax(t *testing.T) {
	f := M{"price": 100, "sgst_percent": "2.5", "cgst_percent": "2.5", "tax_recoverable": true}
	r, e := enquiryCosting(enquiryCostInput{Fabric: f})
	if e != nil || r["landed_metre"] != "100.0000" || r["sgst_metre"] != "2.5000" {
		t.Fatal(r, e)
	}
	f["tax_recoverable"] = false
	r, e = enquiryCosting(enquiryCostInput{Fabric: f})
	if e != nil || r["landed_metre"] != "105.0000" {
		t.Fatal(r, e)
	}
}

func TestEnquiryItemisedCurrencyAndSales(t *testing.T) {
	in := enquiryCostInput{Consumption: "1", Quantity: 10, Fabric: M{"price": 2, "currency": "USD"}, Cost: M{"currency": "USD", "components": []M{{"unit_price": "80", "quantity": 2, "currency": "INR", "exchange_rate": ".0125"}, {"unit_price": ".5", "quantity": 1, "currency": "USD"}}}}
	r, e := enquiryCosting(in)
	if e != nil || r["itemised_cost"] != "2.5000" || r["cost_per_piece"] != "4.5000" {
		t.Fatal(r, e)
	}
	in.Cost["components"] = []M{{"unit_price": 10, "quantity": 1, "currency": "INR"}}
	if _, e = enquiryCosting(in); e == nil {
		t.Fatal("missing component FX accepted")
	}
	r, e = enquirySaleCalculation(M{"sale_type": "Local", "final_price": "100", "sale_sgst_percent": "2.5", "sale_cgst_percent": "2.5"}, 10)
	if e != nil || r["agreed_sale_gross"] != "1050.00" {
		t.Fatal(r, e)
	}
	for _, v := range []any{M{"currency": "USD", "components": M{"price": 10}}, M{"currency": "USD", "components": []any{1}}, M{"currency": "USD", "components": []M{{"category": "Other", "description": "", "currency": "USD"}}}} {
		if _, e = enquiryValidateCost(v); e == nil {
			t.Fatal("invalid component accepted", v)
		}
	}
}
