package app

import (
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"slices"
)

// Booking is the parent; each child PO enters the existing approval/production
// workflow. Lock the parent to prevent two merchants over-allocating its quantity.
func (a *App) enquiryOrder(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if !enquiryRead(u) || !u.Can("orders") {
		return nil, forbidden()
	}
	d, e := readBody(w, r)
	if e != nil {
		return nil, e
	}
	ctx := r.Context()
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	m, e := a.enquiryRecord(ctx, tx, u, materialID(r), true)
	if e != nil {
		return nil, e
	}
	if str(m, "status") != "Booked" {
		return nil, bad("Generate the approved booking before creating a production PO.")
	}
	if id(d, "version") != id(m, "version") {
		return nil, failure{409, "This booking changed. Refresh before creating a PO."}
	}
	if e = a.company(ctx, tx, u, id(m, "company_id"), true); e != nil {
		return nil, e
	}
	po, e := text(d, "po", 60)
	if e != nil {
		return nil, e
	}
	due, e := date(str(d, "due_date"))
	if e != nil {
		return nil, e
	}
	qty, e := integer(d["quantity"])
	if e != nil || qty < 1 || qty > 10000000 {
		return nil, bad("Enter a valid PO quantity.")
	}
	var used int64
	if e = tx.QueryRow(ctx, `SELECT COALESCE(sum(quantity),0) FROM orders WHERE enquiry_id=$1 AND approval<>'Rejected'`, id(m, "id")).Scan(&used); e != nil {
		return nil, e
	}
	if qty+used > enquiryQuantity(m["colours"]) {
		return nil, bad("PO quantities exceed the remaining booking quantity.")
	}
	product := str(d, "product")
	if !slices.Contains(Products, product) {
		return nil, bad("Choose the production category.")
	}
	p := materialProfile(m["profile"])
	style := str(p, "buyer_style")
	if style == "" {
		style = str(p, "product")
	}
	amount, e := costDecimal(p["final_price"], "final agreed price")
	if e != nil || amount.Sign() <= 0 {
		return nil, bad("The booking has no agreed price.")
	}
	price, e := minor(amount.FloatString(2))
	if e != nil {
		return nil, e
	}
	_, e = enquiryCalculation(m)
	if e != nil {
		return nil, e
	}
	consumption := str(p, "consumption")
	if consumption == "" {
		consumption = "0"
	}
	target, _ := costDecimal(consumption, "consumption")
	target.Mul(target, big.NewRat(95, 100))
	if e = a.enquiryMaterialAvailable(ctx, tx, m); e != nil {
		return nil, e
	}
	terms, _ := json.Marshal(M{"sale_type": p["sale_type"], "incoterm": p["incoterm"], "lut_number": p["lut_number"], "sale_sgst_percent": p["sale_sgst_percent"], "sale_cgst_percent": p["sale_cgst_percent"]})
	var oid int64
	e = tx.QueryRow(ctx, `INSERT INTO orders(company_id,buyer_id,team_id,po,style,product,quantity,due_date,unit_price_minor,currency,notes,created_by,enquiry_id,enquiry_consumption,target_consumption,booking_terms) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16) RETURNING id`, id(m, "company_id"), id(m, "buyer_id"), id(m, "team_id"), po, style, product, qty, due, price, str(p, "garment_currency"), fmt.Sprintf("Booking BK-%06d · %s · %s", id(m, "booking_number"), enquiryReference(m), str(p, "sale_type")), u.ID, id(m, "id"), consumption, target.FloatString(6), terms).Scan(&oid)
	if e != nil {
		return nil, e
	}
	for _, step := range []struct {
		title string
		days  int
	}{{"Sample approval", 21}, {"Material readiness", 14}, {"Production complete", 3}, {"Ex-factory", 0}} {
		if _, e = tx.Exec(ctx, `INSERT INTO milestones(order_id,title,due_date) VALUES($1,$2,$3)`, oid, step.title, due.AddDate(0, 0, -step.days)); e != nil {
			return nil, e
		}
	}
	if e = audit(ctx, tx, u, id(m, "company_id"), "Order created", "Order", oid, po+" · "+enquiryReference(m)); e != nil {
		return nil, e
	}
	if _, e = tx.Exec(ctx, `UPDATE enquiries SET version=version+1,updated_at=now() WHERE id=$1`, id(m, "id")); e != nil {
		return nil, e
	}
	if e = a.enquiryHistory(ctx, tx, u, id(m, "id"), "Production PO created: "+po); e != nil {
		return nil, e
	}
	return M{"id": oid}, tx.Commit(ctx)
}

func (a *App) enquiryPreview(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if !enquiryCostRead(u) {
		return nil, forbidden()
	}
	d, e := readBody(w, r)
	if e != nil {
		return nil, e
	}
	record, e := a.enquiryRecord(r.Context(), a.DB, u, materialID(r), false)
	if e != nil {
		return nil, e
	}
	c, e := enquiryValidateCost(d["costing"])
	if e != nil {
		return nil, e
	}
	record["costing"] = c
	calc, e := enquiryCalculation(record)
	if e != nil {
		return nil, e
	}
	// A preview is a calculation only; it never persists an unreviewed amendment.
	return M{"calculation": calc}, nil
}
