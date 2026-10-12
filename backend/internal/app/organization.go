package app

import (
	"context"
	"strings"
)

// Reporting lines describe responsibility. They never silently expand record or
// action permissions; the administrator assigns those explicitly on each user.
func userOrganization(ctx context.Context, q queryer, uid int64, d, before M, profile *User, active bool) (M, error) {
	for _, key := range []string{"reports_to_id", "territory", "restrict_buyers", "buyer_ids", "production_areas"} {
		if _, supplied := d[key]; !supplied {
			d[key] = before[key]
			if key == "buyer_ids" || key == "production_areas" {
				values := toIDs(before[key])
				list := make([]any, len(values))
				for i, value := range values {
					list[i] = value
				}
				d[key] = list
			}
		}
	}
	territory := strings.TrimSpace(str(d, "territory"))
	if len(territory) > 120 {
		return nil, bad("Region / territory must be 120 characters or less.")
	}
	manager := int64(0)
	if d["reports_to_id"] != nil {
		var err error
		manager, err = integer(d["reports_to_id"])
		if err != nil {
			return nil, bad("Choose a valid reporting manager.")
		}
	}
	if manager < 0 || (uid > 0 && manager == uid) {
		return nil, bad("A user cannot report to themselves.")
	}
	if manager > 0 {
		var valid bool
		if err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1 AND active AND deleted_at IS NULL AND NOT is_developer)`, manager).Scan(&valid); err != nil {
			return nil, err
		}
		if !valid {
			return nil, bad("Choose an active reporting manager.")
		}
		var cycle bool
		if err := q.QueryRow(ctx, `WITH RECURSIVE chain AS (
 SELECT id,reports_to_id FROM users WHERE id=$1
 UNION SELECT u.id,u.reports_to_id FROM users u JOIN chain c ON u.id=c.reports_to_id
 ) SELECT EXISTS(SELECT 1 FROM chain WHERE id=$2)`, manager, uid).Scan(&cycle); err != nil {
			return nil, err
		}
		if cycle {
			return nil, bad("This reporting line creates a cycle. Choose another manager.")
		}
	}
	if !active && uid > 0 {
		var reports int
		if err := q.QueryRow(ctx, `SELECT count(*) FROM users WHERE reports_to_id=$1 AND active AND deleted_at IS NULL`, uid).Scan(&reports); err != nil {
			return nil, err
		}
		if reports > 0 {
			return nil, bad("Reassign active team members or replace this employee before disabling their manager.")
		}
	}
	restricted := false
	if d["restrict_buyers"] != nil {
		var ok bool
		restricted, ok = d["restrict_buyers"].(bool)
		if !ok {
			return nil, bad("Choose whether to restrict buyers / brands.")
		}
	}
	buyers := []int64{}
	if restricted {
		if d["buyer_ids"] == nil {
			d["buyer_ids"] = []any{}
		}
		var err error
		buyers, err = ids(d["buyer_ids"])
		if err != nil {
			return nil, err
		}
		var count int
		if err = q.QueryRow(ctx, `SELECT count(*) FROM buyers WHERE id=ANY($1) AND company_id=ANY($2) AND ($3<>'team' OR team_id=ANY($4))`, buyers, profile.Companies, profile.Scope, profile.Teams).Scan(&count); err != nil {
			return nil, err
		}
		if count != len(buyers) {
			return nil, bad("Choose buyers / brands within the selected companies and buyer teams.")
		}
	}
	if profile.Role == "Super Admin" && restricted {
		return nil, bad("Super Admin administrative visibility is fixed.")
	}
	areas := []int64{}
	if profile.Scope == "cutting_table" || profile.Scope == "finishing_line" {
		if d["production_areas"] == nil {
			d["production_areas"] = []any{}
		}
		var err error
		areas, err = ids(d["production_areas"])
		if err != nil {
			return nil, err
		}
		var count int
		if err = q.QueryRow(ctx, `SELECT count(*) FROM production_areas WHERE id=ANY($1) AND company_id=ANY($2) AND kind=$3`, areas, profile.Companies, profile.Scope).Scan(&count); err != nil {
			return nil, err
		}
		if count != len(areas) || (len(profile.Companies) > 0 && count == 0) {
			return nil, bad("Select production areas within the assigned companies and visibility scope.")
		}
	}
	return M{"production_areas": areas, "reports_to_id": manager, "territory": territory, "restrict_buyers": restricted, "buyer_ids": buyers}, nil
}

func saveUserOrganization(ctx context.Context, q queryer, uid int64, org M) error {
	_, err := q.Exec(ctx, `UPDATE users SET reports_to_id=NULLIF($1::bigint,0),territory=$2,restrict_buyers=$3,buyer_ids=$4,production_areas=$5 WHERE id=$6`, org["reports_to_id"], org["territory"], org["restrict_buyers"], org["buyer_ids"], org["production_areas"], uid)
	return err
}
