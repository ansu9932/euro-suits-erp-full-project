package app

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Both charts use the same per-garment records and the same scope as request details.
func (a *App) samplePlanning(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if !sampleRead(u) {
		return nil, forbidden()
	}
	q := r.URL.Query()
	search := strings.TrimSpace(q.Get("search"))
	if len(search) > 120 {
		return nil, bad("Search must be 120 characters or fewer.")
	}
	page, _ := strconv.Atoi(q.Get("page"))
	if page < 1 {
		page = 1
	}
	if page > 100000 {
		return nil, bad("Invalid page.")
	}
	cid, _ := strconv.ParseInt(q.Get("company"), 10, 64)
	for _, key := range []string{"from", "to"} {
		if q.Get(key) != "" {
			if _, e := time.Parse("2006-01-02", q.Get(key)); e != nil {
				return nil, bad("Choose valid date filters.")
			}
		}
	}
	args := append(developmentArgs(u), cid, q.Get("view") == "live", q.Get("stage"), search, (page-1)*50, q.Get("from"), q.Get("to"), q.Get("resource"), q.Get("unplanned") == "true", q.Get("group"), q.Get("brand"), q.Get("fit"))
	list, e := rows(r.Context(), a.DB, `SELECT i.id,i.request_id,i.size,i.colour,i.status,i.production-'audit_reports' AS production,i.version,i.created_at,pi.plan_date::text AS frozen_date,pi.plan_id,pl.resource,
	 d.company_id,d.buyer_id,d.profile-'email_message' AS profile,b.name AS buyer,u.name AS merchant,
	 m.code AS fabric,am.code AS alternate_fabric,count(*) OVER() AS total
	 FROM sample_indents i LEFT JOIN sample_plan_items pi ON pi.indent_id=i.id LEFT JOIN sample_plans pl ON pl.id=pi.plan_id JOIN sample_requests d ON d.id=i.request_id JOIN buyers b ON b.id=d.buyer_id
	 JOIN users u ON u.id=d.created_by LEFT JOIN material_samples m ON m.id::text=d.profile->>'material_id'
	 LEFT JOIN material_samples am ON am.id::text=d.profile->>'alternate_material_id'
	 WHERE `+sampleScope+` AND d.status='Approved' AND ($8::bigint=0 OR d.company_id=$8)
	 AND (NOT $9 OR i.status NOT IN ('Handed over','Rejected')) AND ($10='' OR i.status=$10)
	 AND ($11='' OR strpos(lower(concat_ws(' ',i.id::text,b.name,u.name,d.profile->>'buyer_style',d.profile->>'est',m.code,i.production->>'sample_maker')),lower($11))>0)
	 AND ($13='' OR d.profile->>'buyer_required_date'>=$13) AND ($14='' OR d.profile->>'buyer_required_date'<=$14)
 AND ($15='' OR pl.resource=$15) AND (NOT $16 OR (pi.indent_id IS NULL AND i.status IN ('Ready','Pattern ready')))
 AND ($17='' OR CASE WHEN lower(d.profile->>'product') IN ('suits','suit','blazer','blazers','jacket','jackets') THEN 'Suits' ELSE 'General' END=$17)
 AND ($18='' OR strpos(lower(d.profile->>'brand'),lower($18))>0) AND ($19='' OR strpos(lower(d.profile->>'fit'),lower($19))>0)
 ORDER BY d.profile->>'buyer_required_date',i.id LIMIT 50 OFFSET $12`, args...)
	if e != nil {
		return nil, e
	}
	total := int64(0)
	if len(list) > 0 {
		total = id(list[0], "total")
	}
	for _, item := range list {
		delete(item, "total")
	}
	return M{"rows": list, "total": total, "page": page, "page_size": 50, "today": time.Now().In(time.FixedZone("IST", 19800)).Format("2006-01-02")}, nil
}
