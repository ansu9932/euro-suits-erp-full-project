package app

import "testing"

func TestInitialPasswordConfirmation(t *testing.T) {
	const password = "A valid password-2026 "
	for _, tc := range []struct {
		name      string
		data      M
		wantError bool
	}{
		{"legacy client omits confirmation", M{}, false},
		{"exact match", M{"password_confirmation": password}, false},
		{"different password", M{"password_confirmation": "Another valid password"}, true},
		{"case is significant", M{"password_confirmation": "a valid password-2026 "}, true},
		{"trailing space is significant", M{"password_confirmation": "A valid password-2026"}, true},
		{"empty confirmation", M{"password_confirmation": ""}, true},
		{"null confirmation", M{"password_confirmation": nil}, true},
		{"numeric confirmation", M{"password_confirmation": 1234}, true},
		{"array confirmation", M{"password_confirmation": []any{password}}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := validateInitialPasswordConfirmation(tc.data, password)
			if (err != nil) != tc.wantError {
				t.Errorf("validation error = %v, wantError = %v", err, tc.wantError)
			}
		})
	}
}
