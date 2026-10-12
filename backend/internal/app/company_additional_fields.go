package app

import (
	"net/http"
	"slices"
	"strings"
)

// Finance may extend the Additional section of its own editable company only.
// Global field administration continues to use the Super Admin endpoint.
func (a *App) companyAdditionalFieldCreate(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if u.Role != "Finance Head" || !u.SetupCan("view") || !u.SetupCan("edit") {
		return nil, forbidden()
	}
	d, err := readBody(w, r)
	if err != nil {
		return nil, err
	}
	if err = allowedFields(d, "version", "label", "input_type", "options", "required", "document_required", "expiry_enabled"); err != nil {
		return nil, err
	}
	label, err := text(d, "label", 100)
	if err != nil {
		return nil, err
	}
	kind := str(d, "input_type")
	if !slices.Contains([]string{"text", "textarea", "number", "date", "url", "email", "tel", "select", "checkbox"}, kind) {
		return nil, bad("Choose a valid field type.")
	}
	for _, name := range []string{"required", "document_required", "expiry_enabled"} {
		if value, exists := d[name]; exists {
			if _, ok := value.(bool); !ok {
				return nil, bad("Choose valid field settings.")
			}
		}
	}
	options := []string{}
	if value, exists := d["options"]; exists {
		values, ok := value.([]any)
		if !ok {
			return nil, bad("Provide dropdown choices as a list.")
		}
		for _, value := range values {
			option, ok := value.(string)
			option = strings.TrimSpace(option)
			if !ok || option == "" || len(option) > 100 || slices.Contains(options, option) {
				return nil, bad("Dropdown choices must be unique, non-empty and at most 100 characters.")
			}
			options = append(options, option)
		}
	}
	if kind == "select" && (len(options) < 2 || len(options) > 50) {
		return nil, bad("Provide 2–50 dropdown choices.")
	}
	if kind != "select" && len(options) > 0 {
		return nil, bad("Only dropdown fields have choices.")
	}
	ctx := r.Context()
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	company, err := a.setupRow(ctx, tx, setupID(r), true)
	if err != nil {
		return nil, err
	}
	if !setupEditable(u, company) {
		return nil, forbidden()
	}
	if id(d, "version") != id(company, "version") {
		return nil, failure{409, "This company changed. Reload before adding a field."}
	}
	definitions, err := companyApplicableFields(ctx, tx, company)
	if err != nil {
		return nil, err
	}
	if len(definitions) >= 150 {
		return nil, bad("This company already has the maximum number of fields.")
	}
	for _, definition := range definitions {
		if str(definition, "section") == "Additional" && strings.EqualFold(str(definition, "label"), label) {
			return nil, bad("An Additional field with this name already exists for this company.")
		}
	}
	field, err := row(ctx, tx, `INSERT INTO company_field_definitions(label,section,country,categories,input_type,required,document_required,expiry_enabled,options,business_modes,company_setup_id) VALUES($1,'Additional',$2,$3,$4,$5,$6,$7,$8,'{}',$9) RETURNING *`, label, str(company, "country"), []string{str(company, "category")}, kind, d["required"] == true, d["document_required"] == true, d["expiry_enabled"] == true, options, id(company, "id"))
	if err != nil {
		return nil, err
	}
	var version int64
	if err = tx.QueryRow(ctx, `UPDATE company_setups SET version=version+1,updated_at=now() WHERE id=$1 RETURNING version`, id(company, "id")).Scan(&version); err != nil {
		return nil, err
	}
	if err = setupHistory(ctx, tx, u, company, "Additional field added", "Company-only Additional field added: "+label); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return M{"id": id(field, "id"), "version": version, "field": field}, nil
}
