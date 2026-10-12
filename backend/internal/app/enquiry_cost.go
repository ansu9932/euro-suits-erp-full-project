package app

import (
	"fmt"
	"math/big"
	"regexp"
	"strings"
)

// Financial calculations use exact decimal rationals. Round only the displayed
// result; never feed a rounded intermediate back into a later costing stage.
var enquiryDecimal = regexp.MustCompile(`^(\d{1,10}(\.\d{0,6})?|\.\d{1,6})$`)

func costDecimal(v any, label string) (*big.Rat, error) {
	s := ""
	if v != nil {
		s = strings.TrimSpace(fmt.Sprint(v))
	}
	if s == "" {
		s = "0"
	}
	if !enquiryDecimal.MatchString(s) {
		return nil, bad(label + ": enter a positive number with up to six decimal places.")
	}
	r, ok := new(big.Rat).SetString(s)
	if !ok {
		return nil, bad("Invalid " + label)
	}
	return r, nil
}

type enquiryCostInput struct {
	Consumption, Quantity any
	Fabric                M
	Cost                  M
}

func enquiryCosting(in enquiryCostInput) (M, error) {
	values := map[string]*big.Rat{}
	for _, group := range []struct {
		data M
		keys string
	}{
		{in.Fabric, "price gsm width_metres exchange_rate freight_metre duty_metre advance_license_metre sgst_percent cgst_percent"},
		{in.Cost, "cm trims special_trims labels pocketing fusing general_trims washing embroidery development testing fob_other rejection_percent finance_percent overhead_percent margin_percent commission_percent"},
		{M{"consumption": in.Consumption, "quantity": in.Quantity}, "consumption quantity"},
	} {
		for _, key := range strings.Fields(group.keys) {
			n, err := costDecimal(group.data[key], strings.ReplaceAll(key, "_", " "))
			if err != nil {
				return nil, err
			}
			if strings.HasSuffix(key, "_percent") && n.Cmp(big.NewRat(100, 1)) > 0 {
				return nil, bad("Percentages must be between 0 and 100.")
			}
			values[key] = n
		}
	}
	v := func(k string) *big.Rat { return new(big.Rat).Set(values[k]) }
	add := func(ns ...*big.Rat) *big.Rat {
		r := new(big.Rat)
		for _, n := range ns {
			r.Add(r, n)
		}
		return r
	}
	mul := func(a, b *big.Rat) *big.Rat { return new(big.Rat).Mul(a, b) }
	pct := func(base, p *big.Rat) *big.Rat { return new(big.Rat).Quo(mul(base, p), big.NewRat(100, 1)) }
	unit := str(in.Fabric, "unit")
	if unit == "" {
		unit = "MTR"
	}
	price := v("price")
	kgMetre := new(big.Rat).Quo(mul(v("gsm"), v("width_metres")), big.NewRat(1000, 1))
	switch unit {
	case "MTR":
	case "YDS":
		price.Quo(price, big.NewRat(9144, 10000))
	case "KGS":
		if kgMetre.Sign() == 0 && price.Sign() > 0 {
			return nil, bad("Enter GSM and width in metres to convert a price per kg.")
		}
		price.Mul(price, kgMetre)
	default:
		return nil, bad("Choose MTR, YDS or KGS for the fabric price unit.")
	}
	fx := v("exchange_rate")
	if str(in.Cost, "fabric_exchange_rate") != "" {
		var err error
		fx, err = costDecimal(in.Cost["fabric_exchange_rate"], "fabric exchange rate")
		if err != nil {
			return nil, err
		}
	}
	if str(in.Fabric, "currency") == str(in.Cost, "currency") || str(in.Fabric, "currency") == "" {
		fx = big.NewRat(1, 1)
	}
	if fx.Sign() == 0 && price.Sign() > 0 {
		return nil, bad("Enter the fabric-to-costing exchange rate.")
	}
	price.Mul(price, fx)
	sgst, cgst := pct(price, v("sgst_percent")), pct(price, v("cgst_percent"))
	// Recoverable input tax is disclosed separately and excluded from landed cost.
	tax := new(big.Rat)
	if in.Fabric["tax_recoverable"] != true {
		tax = add(sgst, cgst)
	}
	landed := add(price, v("freight_metre"), v("duty_metre"), v("advance_license_metre"), tax)
	fabric := mul(landed, v("consumption"))
	base := new(big.Rat).Set(fabric)
	itemTotal := new(big.Rat)
	for _, component := range enquiryRows(in.Cost["components"]) {
		amount, err := costDecimal(component["unit_price"], "component unit price")
		if err != nil {
			return nil, err
		}
		quantity, err := costDecimal(component["quantity"], "component quantity")
		if err != nil || quantity.Sign() <= 0 {
			return nil, bad("Component quantity must be greater than zero.")
		}
		rate := big.NewRat(1, 1)
		if str(component, "currency") != str(in.Cost, "currency") {
			rate, err = costDecimal(component["exchange_rate"], "component exchange rate")
			if err != nil || rate.Sign() <= 0 {
				return nil, bad("Enter an exchange rate for each component in a different currency.")
			}
		}
		itemTotal.Add(itemTotal, mul(mul(amount, quantity), rate))
	}
	base.Add(base, itemTotal)
	for _, key := range strings.Fields("cm trims special_trims labels pocketing fusing general_trims washing embroidery development testing fob_other") {
		base.Add(base, v(key))
	}
	out := M{"fabric_price_metre": price.FloatString(4), "kg_metre": kgMetre.FloatString(6), "sgst_metre": sgst.FloatString(4), "cgst_metre": cgst.FloatString(4), "landed_metre": landed.FloatString(4), "fabric_per_piece": fabric.FloatString(4), "required_metres": mul(v("consumption"), v("quantity")).FloatString(3), "target_consumption": mul(v("consumption"), big.NewRat(95, 100)).FloatString(4), "itemised_cost": itemTotal.FloatString(4), "base_cost": base.FloatString(4)}
	subtotal := new(big.Rat).Set(base)
	for _, key := range []string{"rejection", "finance", "overhead", "margin"} {
		n := pct(base, v(key+"_percent"))
		out[key] = n.FloatString(4)
		subtotal.Add(subtotal, n)
	}
	commission := pct(subtotal, v("commission_percent"))
	final := add(subtotal, commission)
	out["before_commission"] = subtotal.FloatString(4)
	out["commission"] = commission.FloatString(4)
	out["cost_per_piece"] = final.FloatString(4)
	out["total_cost"] = mul(final, v("quantity")).FloatString(2)
	if final.Cmp(big.NewRat(10000000000, 1)) >= 0 {
		return nil, bad("Calculated cost exceeds the supported range.")
	}
	return out, nil
}

// Agreed sale values are separate from the cost build-up. Local rates are entered
// explicitly; export quotations remain exclusive of local sales tax.
func enquirySaleCalculation(p M, quantity int64) (M, error) {
	price, e := costDecimal(p["final_price"], "final agreed price")
	if e != nil {
		return nil, e
	}
	sg, e := costDecimal(p["sale_sgst_percent"], "sales SGST")
	if e != nil {
		return nil, e
	}
	cg, e := costDecimal(p["sale_cgst_percent"], "sales CGST")
	if e != nil {
		return nil, e
	}
	if str(p, "sale_type") != "Local" {
		sg = new(big.Rat)
		cg = new(big.Rat)
	}
	base := new(big.Rat).Mul(price, big.NewRat(quantity, 1))
	sg.Mul(sg, base).Quo(sg, big.NewRat(100, 1))
	cg.Mul(cg, base).Quo(cg, big.NewRat(100, 1))
	gross := new(big.Rat).Add(base, sg)
	gross.Add(gross, cg)
	return M{"agreed_sale_net": base.FloatString(2), "sale_sgst": sg.FloatString(2), "sale_cgst": cg.FloatString(2), "agreed_sale_gross": gross.FloatString(2)}, nil
}
