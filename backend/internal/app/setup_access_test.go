package app

import "testing"

func TestLocalTestPasswordPolicy(t *testing.T) {
	for _, env := range []string{"local", "production", ""} {
		t.Setenv("APP_ENV", env)
		if got := validPassword("1234"); got != (env == "local") {
			t.Errorf("test password policy for %q: %v", env, got)
		}
		if validPassword("abcd") {
			t.Error("other short passwords should not be accepted")
		}
		if !validPassword("Strong-Password-2026") {
			t.Error("strong password rejected")
		}
	}
}
func TestCompanyFeaturesCannotExceedRole(t *testing.T) {
	for _, role := range []string{"Finance Head", "MD"} {
		u := User{Role: role, SetupCustomAccess: true, SetupFeatures: []string{"view", "users", "fields"}}
		if !u.SetupCan("view") || u.SetupCan("users") || u.SetupCan("fields") {
			t.Errorf("role boundary failed: %s", role)
		}
	}
	u := User{Role: "MD", SetupCustomAccess: true, SetupFeatures: []string{}}
	if u.SetupCan("approve") || u.Can("users") {
		t.Error("restricted MD gained access")
	}
}
