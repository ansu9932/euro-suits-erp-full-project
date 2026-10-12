package app

import (
	"context"
	"net/http"
	"strings"
)

func (a *App) setupTypes(ctx context.Context) (map[string][]string, error) {
	result := map[string][]string{}
	for category, labels := range companyTypes {
		result[category] = append([]string{}, labels...)
	}
	records, err := rows(ctx, a.DB, `SELECT category,label FROM company_custom_types ORDER BY label`)
	if err != nil {
		return nil, err
	}
	for _, record := range records {
		category := str(record, "category")
		result[category] = append(result[category], str(record, "label"))
	}
	return result, nil
}

func (a *App) companyTypeCreate(w http.ResponseWriter, r *http.Request) (any, error) {
	if current(r).Role != "Super Admin" {
		return nil, forbidden()
	}
	d, err := readBody(w, r)
	if err != nil {
		return nil, err
	}
	label, err := text(d, "label", 100)
	if err != nil {
		return nil, err
	}
	category := str(d, "category")
	if companyTypes[category] == nil {
		return nil, bad("Choose a valid company category.")
	}
	for _, existing := range companyTypes[category] {
		if strings.EqualFold(existing, label) {
			return nil, bad("This company type already exists.")
		}
	}
	var id int64
	err = a.DB.QueryRow(r.Context(), `INSERT INTO company_custom_types(category,label,created_by) VALUES($1,$2,$3) RETURNING id`, category, label, current(r).ID).Scan(&id)
	return M{"id": id}, err
}
