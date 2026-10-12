package app

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"
)

func exportPartyOptionName(value any) (string, error) {
	name, ok := value.(string)
	if !ok {
		return "", bad("Enter a customer / company name.")
	}
	name = strings.TrimSpace(name)
	for _, char := range name {
		if unicode.IsControl(char) {
			return "", bad("Enter the customer / company name on one line.")
		}
	}
	name = strings.Join(strings.Fields(name), " ")
	if name == "" || len(name) > 160 {
		return "", bad("Enter a customer / company name up to 160 characters.")
	}
	return name, nil
}

func exportPartyOptions(ctx context.Context, q queryer, u *User) ([]M, error) {
	if !exportStandalone(u) {
		return []M{}, nil
	}
	// Approved manual names remain selectable without creating or mutating any
	// master. A saved option takes precedence over a matching historical label.
	return rows(ctx, q, `WITH historical AS (
 SELECT DISTINCT ON (lower(regexp_replace(btrim(party_name),'[[:space:]]+',' ','g')))
  NULL::bigint AS id,regexp_replace(btrim(party_name),'[[:space:]]+',' ','g') AS name,
  lower(regexp_replace(btrim(party_name),'[[:space:]]+',' ','g')) AS name_key
 FROM export_invoices d WHERE company_id IS NULL AND party_type='manual' AND workflow_status='Approved' AND record_kind='export'
 ORDER BY lower(regexp_replace(btrim(party_name),'[[:space:]]+',' ','g')),d.id DESC
 ) SELECT id,name FROM export_party_options
 UNION ALL SELECT h.id,h.name FROM historical h WHERE NOT EXISTS(SELECT 1 FROM export_party_options p WHERE p.name_key=h.name_key)
 ORDER BY name,id`)
}

func (a *App) exportPartyOptionCreate(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if !exportEditor(u) {
		return nil, forbidden()
	}
	d, err := readBody(w, r)
	if err != nil {
		return nil, err
	}
	if err = allowedFields(d, "name"); err != nil {
		return nil, err
	}
	name, err := exportPartyOptionName(d["name"])
	if err != nil {
		return nil, err
	}
	ctx := r.Context()
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var optionID int64
	var canonicalName string
	err = tx.QueryRow(ctx, `INSERT INTO export_party_options(name,created_by) VALUES($1,$2) ON CONFLICT(name_key) DO NOTHING RETURNING id,name`, name, u.ID).Scan(&optionID, &canonicalName)
	created := err == nil
	if errors.Is(err, pgx.ErrNoRows) {
		// A fresh statement sees a concurrent insert after ON CONFLICT waited.
		err = tx.QueryRow(ctx, `SELECT id,name FROM export_party_options WHERE name_key=lower(regexp_replace(btrim($1::text),'[[:space:]]+',' ','g'))`, name).Scan(&optionID, &canonicalName)
	}
	if err != nil {
		return nil, err
	}
	if created {
		if _, err = tx.Exec(ctx, `INSERT INTO audit(company_id,actor_id,action,entity,entity_id,detail) VALUES(NULL,$1,'Export customer added','Export customer option',$2,$3)`, u.ID, optionID, canonicalName); err != nil {
			return nil, err
		}
	}
	return M{"id": optionID, "name": canonicalName, "created": created}, tx.Commit(ctx)
}
