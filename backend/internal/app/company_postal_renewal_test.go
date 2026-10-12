package app

import (
	"context"
	"net/http/httptest"
	"testing"
)

func TestRenewalCorrectionAllowsOnlyPostalLookupGET(t *testing.T) {
	u := &User{Role: "Finance Head", RenewalBlocks: []M{{"id": int64(7)}}}
	for _, tc := range []struct {
		method, path string
		allowed      bool
	}{
		{"GET", "/api/company-postal-lookup?country=IN&postal_code=721433", true},
		{"POST", "/api/company-postal-lookup", false},
		{"PUT", "/api/company-postal-lookup", false},
		{"DELETE", "/api/company-postal-lookup", false},
		{"HEAD", "/api/company-postal-lookup", false},
		{"GET", "/api/company-postal-lookup/", false},
		{"GET", "/api/company-postal-lookup/anything", false},
		{"GET", "/api/company-postal-lookup-extra", false},
		{"GET", "/api/orders", false},
		{"POST", "/api/company-setup", false},
		{"GET", "/api/company-setup", true},
	} {
		r := httptest.NewRequest(tc.method, tc.path, nil)
		if got := renewalRouteAllowed(r, u); got != tc.allowed {
			t.Errorf("%s %s: allowed=%v, want %v", tc.method, tc.path, got, tc.allowed)
		}
	}
	// The renewal exception only passes middleware. It must not grant access to
	// the lookup handler or allow provider requests for restricted accounts.
	for _, actor := range []*User{
		{Role: "Finance Head", SetupCustomAccess: true, SetupFeatures: []string{"view"}},
		{Role: "Finance Head", SetupCustomAccess: true, SetupFeatures: []string{"edit"}},
		{Role: "MD"},
		{Role: "Super Admin"},
	} {
		actor.RenewalBlocks = u.RenewalBlocks
		r := httptest.NewRequest("GET", "/api/company-postal-lookup?country=IN&postal_code=721433", nil)
		r = r.WithContext(context.WithValue(r.Context(), authKey{}, actor))
		if !renewalRouteAllowed(r, actor) {
			t.Fatal("exact correction lookup rejected by middleware")
		}
		_, err := (&App{}).companyPostalLookup(httptest.NewRecorder(), r)
		if err == nil || err != forbidden() {
			t.Errorf("renewal exception bypassed lookup grant checks for %s: %v", actor.Role, err)
		}
	}
}
