package app

import "testing"

func TestUserContactInput(t *testing.T) {
	cases := []struct {
		name          string
		input, before M
		email, phone  string
		invalid       bool
	}{
		{"normalizes contact", M{"email": " Staff@Example.com ", "phone": "+91 (98765) 43210"}, M{}, "staff@example.com", "+919876543210", false},
		{"omitted preserves existing", M{}, M{"email": "saved@example.com", "phone": "+441234567890"}, "saved@example.com", "+441234567890", false},
		{"optional contact", M{"email": "", "phone": ""}, M{}, "", "", false},
		{"invalid email", M{"email": "bad"}, M{}, "", "", true},
		{"defaults India country code", M{"phone": "9876543210"}, M{}, "", "+919876543210", false},
		{"empty default prefix", M{"phone": "+91"}, M{}, "", "", false},
		{"rejects phone text", M{"phone": "+91abc12345"}, M{}, "", "", true},
		{"rejects malformed field", M{"phone": 12345}, M{}, "", "", true},
		{"retains MFA email requirement", M{"email": ""}, M{"email": "saved@example.com", "mfa_enabled": true}, "", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			email, phone, err := userContactInput(c.input, c.before)
			if (err != nil) != c.invalid {
				t.Fatalf("unexpected validation result: %v", err)
			}
			if !c.invalid && (email != c.email || phone != c.phone) {
				t.Fatalf("got %q %q", email, phone)
			}
		})
	}
}
