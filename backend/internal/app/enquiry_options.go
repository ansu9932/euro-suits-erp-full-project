package app

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"strings"
)

func enquiryCheckOptions(ctx context.Context, q queryer, cid int64, p M, mapping map[string]string) error {
	for key, kind := range mapping {
		value := str(p, key)
		if value == "" {
			continue
		}
		found := false
		for _, v := range enquiryDefaults[kind] {
			if strings.EqualFold(v, value) {
				p[key] = v
				found = true
				break
			}
		}
		if found {
			continue
		}
		var canonical string
		e := q.QueryRow(ctx, `SELECT label FROM enquiry_options WHERE company_id=$1 AND kind=$2 AND lower(label)=lower($3)`, cid, kind, value).Scan(&canonical)
		if e == nil {
			p[key] = canonical
			continue
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		// Previously approved styles remain usable without re-entering their masters.
		if kind == "brand" || kind == "season" || kind == "product" {
			var exists bool
			if e = q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM development_masters WHERE company_id=$1 AND status='Approved' AND profile->>$2=$3)`, cid, kind, value).Scan(&exists); e != nil {
				return e
			}
			if exists {
				continue
			}
		}
		return bad("Add the " + strings.ReplaceAll(key, "_", " ") + " to its dropdown before selecting it.")
	}
	return nil
}
