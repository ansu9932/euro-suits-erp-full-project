package app

import "testing"

func TestExistingUserCredentialInput(t *testing.T) {
	t.Setenv("APP_ENV", "production")
	const next = "Secure changed password-2026"
	for _, tc := range []struct {
		name               string
		data               M
		username, password string
		wantError          bool
	}{
		{"omitted credentials", M{}, "employee.one", "", false},
		{"blank password unchanged", M{"new_password": "", "new_password_confirmation": ""}, "employee.one", "", false},
		{"legacy password field ignored", M{"password": next}, "employee.one", "", false},
		{"username normalized", M{"username": " Employee.Two "}, "employee.two", "", false},
		{"username empty", M{"username": ""}, "", "", true},
		{"username null", M{"username": nil}, "", "", true},
		{"username numeric", M{"username": 42}, "", "", true},
		{"username invalid", M{"username": "employee name"}, "", "", true},
		{"username invalid prefix", M{"username": ".employee"}, "", "", true},
		{"reset matching", M{"new_password": next, "new_password_confirmation": next}, "employee.one", next, false},
		{"reset unconfirmed", M{"new_password": next}, "", "", true},
		{"reset mismatched", M{"new_password": next, "new_password_confirmation": "different"}, "", "", true},
		{"reset short", M{"new_password": "1234", "new_password_confirmation": "1234"}, "", "", true},
		{"reset null", M{"new_password": nil}, "", "", true},
		{"reset numeric", M{"new_password": 42}, "", "", true},
		{"confirmation only", M{"new_password_confirmation": next}, "", "", true},
		{"confirmation numeric", M{"new_password": next, "new_password_confirmation": 42}, "", "", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			username, password, err := existingUserCredentialInput(tc.data, "employee.one")
			if (err != nil) != tc.wantError {
				t.Fatalf("validation error=%v, expected error=%v", err, tc.wantError)
			}
			if !tc.wantError && (username != tc.username || password != tc.password) {
				t.Fatal("unexpected credential update decision")
			}
		})
	}
	t.Setenv("APP_ENV", "local")
	_, password, err := existingUserCredentialInput(M{"new_password": "", "new_password_confirmation": ""}, "employee.one")
	if err != nil || password != "" {
		t.Fatal("local edit must never default the password")
	}
}
