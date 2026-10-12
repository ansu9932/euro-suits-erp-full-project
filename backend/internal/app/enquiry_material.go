package app

import "context"

func (a *App) enquiryMaterialAvailable(ctx context.Context, q queryer, m M) error {
	if id(m, "material_id") == 0 {
		return nil
	}
	f, e := row(ctx, q, `SELECT supplier_setup_id FROM material_samples WHERE id=$1 AND company_id=$2 AND active`, id(m, "material_id"), id(m, "company_id"))
	if e != nil {
		return bad("The selected fabric is no longer active.")
	}
	return materialSupplierAvailable(ctx, q, id(f, "supplier_setup_id"))
}
