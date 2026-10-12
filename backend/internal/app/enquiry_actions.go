package app

import (
	"context"
	"encoding/json"
	"math/big"
	"slices"
	"strings"
)

func (a *App) enquiryAction(ctx context.Context, q queryer, u *User, old, d M, action string) (string, error) {
	status, rid := str(old, "status"), id(old, "id")
	p := materialProfile(old["profile"])
	event := ""
	next := status
	switch action {
	case "request_fabric":
		if !enquiryWrite(u) {
			return "", forbidden()
		}
		if !slices.Contains([]string{"Draft", "Costing", "Changes requested"}, status) {
			return "", bad("Fabric enquiries can be raised from a draft or a returned costing.")
		}
		n, e := costDecimal(p["consumption"], "enquiry consumption")
		if e != nil || n.Sign() <= 0 {
			return "", bad("Enter enquiry consumption per garment before requesting fabric.")
		}
		if id(old, "material_id") == 0 && str(p, "description") == "" {
			return "", bad("Select an ETF / ESF or describe the fabric required.")
		}
		next, event = "Fabric requested", "Fabric response requested"
	case "fabric":
		if !u.Can("enquiry_fabric") {
			return "", forbidden()
		}
		if !slices.Contains([]string{"Fabric requested", "Costing"}, status) {
			return "", bad("This enquiry is not waiting for a fabric response.")
		}
		f, e := enquiryValidateFabric(d["fabric"])
		if e != nil {
			return "", e
		}
		if str(f, "currency") == "" || str(f, "price") == "" {
			return "", bad("Enter fabric price and currency.")
		}
		if e = enquiryCheckOptions(ctx, q, id(old, "company_id"), f, map[string]string{"currency": "currency", "incoterm": "incoterm"}); e != nil {
			return "", e
		}
		b, _ := json.Marshal(f)
		if _, e = q.Exec(ctx, `UPDATE enquiries SET fabric=$1 WHERE id=$2`, b, rid); e != nil {
			return "", e
		}
		next, event = "Costing", "Fabric response recorded"
	case "costing":
		if !enquiryCostRead(u) || !enquiryWrite(u) {
			return "", forbidden()
		}
		if !slices.Contains([]string{"Draft", "Costing", "Changes requested"}, status) {
			return "", bad("Costing is locked during or after approval.")
		}
		c, e := enquiryValidateCost(d["costing"])
		if e != nil {
			return "", e
		}
		if str(c, "currency") == "" {
			return "", bad("Select the costing currency.")
		}
		if str(p, "garment_currency") != "" && str(c, "currency") != str(p, "garment_currency") {
			return "", bad("Costing and garment-value currencies must match. Update the enquiry currency first.")
		}
		if e = enquiryCheckOptions(ctx, q, id(old, "company_id"), c, map[string]string{"currency": "currency"}); e != nil {
			return "", e
		}
		for _, component := range enquiryRows(c["components"]) {
			if e = enquiryCheckOptions(ctx, q, id(old, "company_id"), component, map[string]string{"currency": "currency"}); e != nil {
				return "", e
			}
		}
		candidate := materialProfile(old)
		candidate["costing"] = c
		if _, e = enquiryCalculation(candidate); e != nil {
			return "", e
		}
		b, _ := json.Marshal(c)
		if _, e = q.Exec(ctx, `UPDATE enquiries SET costing=$1 WHERE id=$2`, b, rid); e != nil {
			return "", e
		}
		next, event = "Costing", "Costing saved"
	case "submit":
		if !enquiryWrite(u) {
			return "", forbidden()
		}
		if !slices.Contains([]string{"Draft", "Costing", "Changes requested"}, status) {
			return "", bad("Only a draft or completed costing can be submitted.")
		}
		if str(p, "garment_delivery_date") == "" || str(p, "garment_currency") == "" || str(p, "incoterm") == "" {
			return "", bad("Enter garment delivery date, currency and Incoterm before submitting.")
		}
		if str(p, "enquiry_type") != "CM - Production" && str(p, "fabric_status") != "None" {
			f := materialProfile(old["fabric"])
			if str(f, "price") == "" || str(f, "currency") == "" {
				return "", bad("Record the fabric response before submitting the costing.")
			}
		}
		if _, e := enquiryValidateProfile(old["profile"]); e != nil {
			return "", e
		}
		c := materialProfile(old["costing"])
		if str(c, "currency") == "" || str(c, "currency") != str(p, "garment_currency") {
			return "", bad("Save an open costing in the garment currency before MD review.")
		}
		if !positiveCost(p, "consumption") {
			return "", bad("Enter consumption per garment before MD review.")
		}
		if _, e := enquiryCalculation(old); e != nil {
			return "", e
		}
		next, event = "Submitted", "Submitted for MD review"
	case "review_edit":
		if !enquiryReview(u) {
			return "", forbidden()
		}
		if status != "Submitted" {
			return "", bad("Review corrections are allowed only while an enquiry is submitted.")
		}
		note := str(d, "note")
		if note == "" || len(note) > 2000 {
			return "", bad("Explain the review correction (up to 2,000 characters).")
		}
		changes := materialProfile(d["profile"])
		for _, key := range []string{"consumption", "garment_delivery_date", "fabric_sailing_date", "garment_value", "quoted_price", "final_price", "quotation_date", "quotation_email", "negotiation_note"} {
			if value, ok := changes[key]; ok {
				p[key] = value
			}
		}
		p, e := enquiryValidateProfile(p)
		if e != nil {
			return "", e
		}
		if !positiveCost(p, "consumption") {
			return "", bad("Enquiry consumption must be greater than zero.")
		}
		candidate := materialProfile(old)
		candidate["profile"] = p
		if _, e = enquiryCalculation(candidate); e != nil {
			return "", e
		}
		b, _ := json.Marshal(p)
		if _, e = q.Exec(ctx, `UPDATE enquiries SET profile=$1,review_note=$2 WHERE id=$3`, b, note, rid); e != nil {
			return "", e
		}
		event = "MD review correction: " + note
	case "approve", "return":
		if !enquiryReview(u) {
			return "", forbidden()
		}
		if status != "Submitted" {
			return "", bad("Only submitted enquiries can be reviewed.")
		}
		note := str(d, "note")
		if len(note) > 2000 || (action == "return" && note == "") {
			return "", bad("Enter a review note explaining the required changes.")
		}
		next, event = "Approved", "Enquiry approved"
		if action == "return" {
			next, event = "Changes requested", "Changes requested: "+note
		}
		if action == "approve" {
			if e := a.enquiryMaterialAvailable(ctx, q, old); e != nil {
				return "", e
			}
			if _, e := enquiryCalculation(old); e != nil {
				return "", e
			}
		}
		if _, e := q.Exec(ctx, `UPDATE enquiries SET review_note=$1,reviewed_by=$2 WHERE id=$3`, note, u.ID, rid); e != nil {
			return "", e
		}
	case "book":
		if !(enquiryWrite(u) || enquiryReview(u)) {
			return "", forbidden()
		}
		if status != "Approved" {
			return "", bad("MD approval is required before generating a booking.")
		}
		if id(old, "material_id") == 0 || (id(old, "development_id") == 0 && str(p, "et_reference") == "") {
			return "", bad("Link an ETF / ESF and an approved ET / EST or legacy style reference before booking.")
		}
		if e := a.enquiryMaterialAvailable(ctx, q, old); e != nil {
			return "", e
		}
		n, e := costDecimal(p["final_price"], "final agreed price")
		if e != nil || n.Sign() <= 0 {
			return "", bad("The final agreed price must be recorded and approved before booking.")
		}
		if _, e = q.Exec(ctx, `UPDATE enquiries SET booking_number=COALESCE(booking_number,nextval('enquiry_booking_number')) WHERE id=$1`, rid); e != nil {
			return "", e
		}
		next, event = "Booked", "Booking generated"
	case "amend":
		if !enquiryReview(u) {
			return "", forbidden()
		}
		if !slices.Contains([]string{"Approved", "Booked"}, status) {
			return "", bad("Only an approved enquiry or booking can be reopened.")
		}
		var hasPO bool
		if e := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM orders WHERE enquiry_id=$1)`, rid).Scan(&hasPO); e != nil {
			return "", e
		}
		if hasPO {
			return "", bad("This booking has production POs. Create a new enquiry for changed commercial terms.")
		}
		note := str(d, "note")
		if note == "" || len(note) > 2000 {
			return "", bad("Enter the amendment reason.")
		}
		next, event = "Changes requested", "Amendment opened: "+note
		if _, e := q.Exec(ctx, `UPDATE enquiries SET reviewed_by=NULL,review_note=$1 WHERE id=$2`, note, rid); e != nil {
			return "", e
		}
	default:
		return "", bad("Choose a valid enquiry action.")
	}
	_, e := q.Exec(ctx, `UPDATE enquiries SET status=$1,version=version+1,updated_at=now() WHERE id=$2`, next, rid)
	return event, e
}

func positiveCost(p M, key string) bool {
	n, e := costDecimal(p[key], strings.ReplaceAll(key, "_", " "))
	return e == nil && n.Cmp(big.NewRat(0, 1)) > 0
}
