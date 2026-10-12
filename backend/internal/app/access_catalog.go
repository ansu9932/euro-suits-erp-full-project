package app

// Codes identify capabilities consistently in the administration screen. Grants
// remain server-enforced by their existing keys; display codes never grant access.
var accessCodes = map[string]map[string]string{
	"features": {
		"view": "CS-01", "create": "CS-02", "edit": "CS-03", "submit": "CS-04",
		"amend": "CS-05", "approve": "CS-06", "documents": "CS-07", "export": "CS-08",
		"reminders": "CS-09", "ports": "CS-10", "fields": "CS-11", "types": "CS-12", "users": "CS-13",
	},
	"permissions": {
		"view_orders": "OP-01", "orders": "OP-02", "approve": "OP-03", "plan": "OP-04",
		"develop": "OP-05", "sample": "OP-06", "cut": "OP-07", "sew": "OP-08",
		"finish": "OP-09", "pack": "OP-10", "quality": "OP-11", "warehouse": "OP-12",
		"finance": "OP-13", "suppliers": "OP-14", "inventory": "OP-15", "purchases": "OP-16",
		"companies": "OP-17", "audit": "OP-18", "documents": "OP-19", "users": "OP-20",
	},
}
