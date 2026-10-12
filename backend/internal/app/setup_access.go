package app

import (
	"net/http"
	"slices"
)

var setupCaps = map[string][]string{
	"Finance Head": {"view", "create", "edit", "submit", "amend", "documents", "export", "reminders", "ports"},
	"MD":           {"view", "approve", "export", "reminders"},
	"Super Admin":  {"view", "approve", "documents", "export", "reminders", "ports", "fields", "types", "users"},
}
var setupLabels = map[string]string{
	"view": "View company register", "create": "Add companies", "edit": "Edit own drafts", "submit": "Submit for approval",
	"amend": "Start amendments", "approve": "Approve or request changes", "documents": "Upload / download permitted documents",
	"export": "Export Excel", "reminders": "View expiry reminders", "ports": "Register ports", "fields": "Add custom fields",
	"types": "Add company types", "users": "Manage users and access",
}

func (u *User) SetupCan(feature string) bool {
	if u.Role == "Super Admin" {
		return slices.Contains(setupCaps[u.Role], feature)
	}
	if u.DesignationID != 0 && !slices.Contains(u.DesignationFeatures, feature) {
		return false
	}
	return slices.Contains(setupCaps[u.Role], feature) && (u.InheritDesignation || !u.SetupCustomAccess || slices.Contains(u.SetupFeatures, feature))
}
func setupPortal(role string) string {
	return map[string]string{"Finance Head": "finance", "MD": "md", "Super Admin": "admin", "CEO": "management", "Finance": "accounts", "Marketing": "merchandising", "Planning": "planning", "Development": "cad", "Sourcing": "sourcing", "Stores": "stores", "Sampling": "sampling", "Cutting": "cutting", "Production": "sewing", "Finishing": "finishing", "Quality": "quality", "Warehouse": "warehouse"}[role]
}
func (a *App) setupUsers(w http.ResponseWriter, r *http.Request) (any, error) {
	if current(r).Role != "Super Admin" {
		return nil, forbidden()
	}
	return a.accessMetadata(r.Context())
}
func setupFeatureInput(d M, role string) ([]string, error) {
	caps := setupCaps[role]
	if !slices.Contains(Roles, role) {
		return nil, bad("Choose a Company Setup role.")
	}
	values, ok := d["features"].([]any)
	if !ok {
		return nil, bad("Select the features for this user.")
	}
	features := []string{}
	for _, value := range values {
		s, ok := value.(string)
		if !ok || !slices.Contains(caps, s) || slices.Contains(features, s) {
			return nil, bad("A feature is invalid for the selected role.")
		}
		features = append(features, s)
	}
	if role == "Super Admin" {
		return append([]string{}, caps...), nil
	}
	if len(features) > 0 && !slices.Contains(features, "view") {
		return nil, bad("Enable View company register before adding other features.")
	}
	if (slices.Contains(features, "create") || slices.Contains(features, "submit") || slices.Contains(features, "amend")) && !slices.Contains(features, "edit") {
		return nil, bad("Create, submit and amendment access require Edit own drafts.")
	}
	return features, nil
}
