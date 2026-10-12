package app

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"time"
)

func (a *App) sampleCapacityList(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if !sampleRead(u) {
		return nil, forbidden()
	}
	records, e := rows(r.Context(), a.DB, `SELECT id,company_id,resource,effective_date::text,tailors,pieces_per_tailor,weekly_off,holidays::text[] AS holidays,version FROM sample_capacities WHERE company_id=ANY($1) ORDER BY resource,effective_date`, u.Companies)
	return M{"records": records, "can_edit": u.Role == "Super Admin"}, e
}
func (a *App) sampleCapacitySave(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if u.Role != "Super Admin" || !sampleRead(u) {
		return nil, forbidden()
	}
	d, e := readBody(w, r)
	if e != nil {
		return nil, e
	}
	cid := id(d, "company_id")
	tx, e := a.DB.Begin(r.Context())
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(r.Context())
	if e = a.company(r.Context(), tx, u, cid, true); e != nil {
		return nil, e
	}
	// Serialize capacity changes and plan freezing for the company.
	if _, e = tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(816,$1::integer)`, cid); e != nil {
		return nil, e
	}
	resource := str(d, "resource")
	if resource == "" || len(resource) > 80 {
		return nil, bad("Enter a resource name up to 80 characters.")
	}
	date, e := time.Parse("2006-01-02", str(d, "effective_date"))
	if e != nil {
		return nil, bad("Enter a valid effective date.")
	}
	tailors, per := id(d, "tailors"), id(d, "pieces_per_tailor")
	if tailors < 0 || tailors > 500 || per < 1 || per > 200 || fmt.Sprint(d["tailors"]) != fmt.Sprint(tailors) || fmt.Sprint(d["pieces_per_tailor"]) != fmt.Sprint(per) {
		return nil, bad("Enter whole numbers: 0–500 tailors and 1–200 pieces per tailor per day.")
	}
	off := []int{}
	if list, ok := d["weekly_off"].([]any); ok {
		for _, v := range list {
			n, e := json.Number(fmt.Sprint(v)).Int64()
			if e != nil || n < 0 || n > 6 {
				return nil, bad("Choose valid weekly off days.")
			}
			if !slices.Contains(off, int(n)) {
				off = append(off, int(n))
			}
		}
	} else {
		return nil, bad("Choose the weekly off days.")
	}
	if len(off) == 7 {
		return nil, bad("Keep at least one working day per week; use zero tailors for a closed resource.")
	}
	holidays := []string{}
	if list, ok := d["holidays"].([]any); ok && len(list) <= 366 {
		for _, v := range list {
			value := fmt.Sprint(v)
			if _, e = time.Parse("2006-01-02", value); e != nil {
				return nil, bad("Enter valid holiday dates.")
			}
			holidays = append(holidays, value)
		}
	} else {
		return nil, bad("Provide up to 366 holiday dates.")
	}
	var result M
	if id(d, "id") > 0 {
		result, e = row(r.Context(), tx, `UPDATE sample_capacities SET tailors=$1,pieces_per_tailor=$2,weekly_off=$3,holidays=$4::date[],version=version+1 WHERE id=$5 AND company_id=$6 AND resource=$7 AND effective_date=$8 AND version=$9 RETURNING id`, tailors, per, off, holidays, id(d, "id"), cid, resource, date, id(d, "version"))
		if e != nil {
			return nil, failure{409, "Capacity changed. Refresh before saving."}
		}
	} else {
		result, e = row(r.Context(), tx, `INSERT INTO sample_capacities(company_id,resource,effective_date,tailors,pieces_per_tailor,weekly_off,holidays,created_by) VALUES($1,$2,$3,$4,$5,$6,$7::date[],$8) ON CONFLICT(company_id,resource,effective_date) DO NOTHING RETURNING id`, cid, resource, date, tailors, per, off, holidays, u.ID)
		if e != nil {
			return nil, bad("A capacity record already exists for this resource and date.")
		}
	}
	if e = audit(r.Context(), tx, u, cid, "Sampling capacity saved", "Sample capacity", id(result, "id"), resource); e != nil {
		return nil, e
	}
	if e = tx.Commit(r.Context()); e != nil {
		return nil, e
	}
	return result, nil
}

type sampleCapacityRule struct {
	date     string
	capacity int
	off      []int
	holidays []string
}

func sampleRule(day time.Time, rules []sampleCapacityRule) int {
	date := day.Format("2006-01-02")
	var rule *sampleCapacityRule
	for i := range rules {
		if rules[i].date <= date {
			rule = &rules[i]
		}
	}
	if rule == nil || slices.Contains(rule.off, int(day.Weekday())) || slices.Contains(rule.holidays, date) {
		return 0
	}
	return rule.capacity
}

// Priority is the input order. A later item may use an earlier free day when
// the higher-priority garment is waiting for fabric or trims.
func sampleDates(start time.Time, earliest []string, rules []sampleCapacityRule, used map[string]int) ([]string, error) {
	out := []string{}
	for _, ready := range earliest {
		day := start
		if ready > day.Format("2006-01-02") {
			day, _ = time.Parse("2006-01-02", ready)
		}
		found := false
		for n := 0; n < 1096; n++ {
			key := day.Format("2006-01-02")
			if sampleRule(day, rules) > used[key] {
				used[key]++
				out = append(out, key)
				found = true
				break
			}
			day = day.AddDate(0, 0, 1)
		}
		if !found {
			return nil, bad("No available capacity within three years. Add dated capacity before planning.")
		}
	}
	return out, nil
}
func (a *App) sampleSchedule(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if !sampleProduction(u) {
		return nil, forbidden()
	}
	d, e := readBody(w, r)
	if e != nil {
		return nil, e
	}
	cid := id(d, "company_id")
	resource := str(d, "resource")
	start, e := time.Parse("2006-01-02", str(d, "start_date"))
	if e != nil {
		return nil, bad("Choose a start date.")
	}
	today := time.Now().In(time.FixedZone("IST", 19800)).Format("2006-01-02")
	if str(d, "start_date") < today {
		return nil, bad("Plan from today or a future date.")
	}
	list, ok := d["indents"].([]any)
	if !ok || len(list) == 0 || len(list) > 200 {
		return nil, bad("Select 1 to 200 indents in priority order.")
	}
	tx, e := a.DB.Begin(r.Context())
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(r.Context())
	if e = a.company(r.Context(), tx, u, cid, true); e != nil {
		return nil, e
	}
	if _, e = tx.Exec(r.Context(), `SELECT pg_advisory_xact_lock(816,$1::integer)`, cid); e != nil {
		return nil, e
	}
	caps, e := rows(r.Context(), tx, `SELECT id,version,effective_date::text,tailors,pieces_per_tailor,weekly_off,holidays::text[] AS holidays FROM sample_capacities WHERE company_id=$1 AND resource=$2 ORDER BY effective_date`, cid, resource)
	if e != nil {
		return nil, e
	}
	if len(caps) == 0 {
		return nil, bad("Super Admin must add capacity for this resource first.")
	}
	rules := []sampleCapacityRule{}
	for _, c := range caps {
		b, _ := json.Marshal(c)
		var v struct {
			Date     string   `json:"effective_date"`
			Tailors  int      `json:"tailors"`
			Per      int      `json:"pieces_per_tailor"`
			Off      []int    `json:"weekly_off"`
			Holidays []string `json:"holidays"`
		}
		json.Unmarshal(b, &v)
		rules = append(rules, sampleCapacityRule{v.Date, v.Tailors * v.Per, v.Off, v.Holidays})
	}
	// Count all bookings in this company/resource, including other buyer teams,
	// without disclosing their identities in the preview.
	bookings, e := rows(r.Context(), tx, `SELECT * FROM erp_sample_bookings($1,$2,$3)`, cid, resource, start)
	if e != nil {
		return nil, e
	}
	used := map[string]int{}
	for _, b := range bookings {
		used[str(b, "plan_date")] = int(id(b, "count"))
	}
	pieces := []M{}
	ready := []string{}
	seen := map[int64]bool{}
	for _, input := range list {
		item := materialProfile(input)
		iid := id(item, "id")
		if seen[iid] {
			return nil, bad("Select each indent only once.")
		}
		seen[iid] = true
		piece, e := row(r.Context(), tx, `SELECT i.*,d.profile,d.company_id FROM sample_indents i JOIN sample_requests d ON d.id=i.request_id WHERE `+sampleScope+` AND i.id=$8 AND d.status='Approved' FOR UPDATE OF i`, append(developmentArgs(u), iid)...)
		if e != nil {
			return nil, e
		}
		if id(piece, "company_id") != cid || !slices.Contains([]string{"Ready", "Pattern ready"}, str(piece, "status")) {
			return nil, bad("Choose unstarted indents from the selected company.")
		}
		if id(item, "version") != id(piece, "version") {
			return nil, failure{409, "An indent changed. Refresh the plan."}
		}
		var exists bool
		if e = tx.QueryRow(r.Context(), `SELECT EXISTS(SELECT 1 FROM sample_plan_items WHERE indent_id=$1)`, iid).Scan(&exists); e != nil {
			return nil, e
		}
		if exists {
			return nil, failure{409, "An indent is already in a frozen plan."}
		}
		p := materialProfile(piece["profile"])
		prod := materialProfile(piece["production"])
		date := start.Format("2006-01-02")
		for _, key := range []string{"fabric_expected_date", "trims_expected_date"} {
			v := str(prod, key)
			if v == "" {
				v = str(p, key)
			}
			if v != "" && v > date {
				date = v
			}
		}
		ready = append(ready, date)
		pieces = append(pieces, piece)
	}
	dates, e := sampleDates(start, ready, rules, used)
	if e != nil {
		return nil, e
	}
	preview := []M{}
	for i, p := range pieces {
		profile := materialProfile(p["profile"])
		preview = append(preview, M{"id": id(p, "id"), "request_id": id(p, "request_id"), "version": id(p, "version"), "plan_date": dates[i], "required_date": str(profile, "buyer_required_date"), "buyer_style": str(profile, "buyer_style"), "size": p["size"], "colour": p["colour"], "late": dates[i] > str(profile, "buyer_required_date")})
	}
	state, _ := json.Marshal(M{"company": cid, "resource": resource, "start": start, "preview": preview, "capacity": caps, "bookings": bookings})
	fingerprint := fmt.Sprintf("%x", sha256.Sum256(state))
	if str(d, "action") == "preview" {
		return M{"rows": preview, "token": fingerprint}, nil
	}
	if str(d, "action") != "freeze" {
		return nil, bad("Preview the plan before freezing.")
	}
	if str(d, "token") != fingerprint {
		return nil, failure{409, "Capacity or planning changed. Preview the latest dates before freezing."}
	}
	var planID int64
	if e = tx.QueryRow(r.Context(), `INSERT INTO sample_plans(company_id,resource,start_date,created_by) VALUES($1,$2,$3,$4) RETURNING id`, cid, resource, start, u.ID).Scan(&planID); e != nil {
		return nil, e
	}
	for i, piece := range pieces {
		if _, e = tx.Exec(r.Context(), `INSERT INTO sample_plan_items(indent_id,plan_id,position,plan_date) VALUES($1,$2,$3,$4)`, id(piece, "id"), planID, i+1, dates[i]); e != nil {
			return nil, e
		}
		p := materialProfile(piece["production"])
		p["frozen_plan_date"] = dates[i]
		p["planning_resource"] = resource
		p["plan_frozen_by"] = u.Name
		p["plan_frozen_at"] = time.Now().UTC().Format(time.RFC3339)
		p["plan_id"] = planID
		b, _ := json.Marshal(p)
		if _, e = tx.Exec(r.Context(), `UPDATE sample_indents SET production=$1,version=version+1 WHERE id=$2`, b, id(piece, "id")); e != nil {
			return nil, e
		}
		if e = a.sampleHistory(r.Context(), tx, u, id(piece, "request_id"), fmt.Sprintf("Indent %d: plan frozen", id(piece, "id")), p); e != nil {
			return nil, e
		}
	}
	if e = tx.Commit(r.Context()); e != nil {
		return nil, e
	}
	return M{"id": planID, "rows": preview}, nil
}
