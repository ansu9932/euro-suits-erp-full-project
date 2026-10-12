package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

type postalTransport func(*http.Request) (*http.Response, error)

func (f postalTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

const postalFixture = `[{"Status":"Success","PostOffice":[
 {"Name":"Office one","Block":"Bengaluru North","District":"Bengaluru","State":"Karnataka","Country":"India","Pincode":"560001"},
 {"Name":"Office duplicate","Block":"bengaluru north","District":"Bengaluru","State":"Karnataka","Country":"India","Pincode":"560001"},
 {"Name":"Office two","Block":"NA","District":"Bengaluru","State":"Karnataka","Country":"India","Pincode":"560001"},
 {"Name":"Other PIN","Block":"Do not use","District":"Other","State":"Other","Country":"India","Pincode":"560002"},
 {"Name":"Other country","Block":"Do not use","District":"Other","State":"Other","Country":"Elsewhere","Pincode":"560001"}
]}]`

func TestPostalLocationsKeepAmbiguousSuggestions(t *testing.T) {
	got, err := parsePostalLocations([]byte(postalFixture), "560001")
	want := []postalLocation{{"Bengaluru", "Karnataka", "Bengaluru"}, {"Bengaluru North", "Karnataka", "Bengaluru"}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, %v; want %#v", got, err, want)
	}
	for _, value := range []string{"NA", "N/A", "Not Available", "null", " - ", "Not applicable", "N.A."} {
		if got := postalPlace(value); got != "" {
			t.Errorf("placeholder accepted: %q", value)
		}
	}
	for _, raw := range []string{`[{"Status":"Error","Message":"No records found","PostOffice":null}]`, `[{"Status":"Success","PostOffice":[]}]`} {
		got, err := parsePostalLocations([]byte(raw), "560001")
		if err != nil || got == nil || len(got) != 0 {
			t.Fatalf("no match must return non-null empty list: %v, %v", got, err)
		}
	}
	for _, raw := range []string{`null`, `[]`, `{}`, `[{"Status":"Success"},{"Status":"Success"}]`, `[{"Status":"Error","Message":"internal failure"}]`, `[{"Status":"Success","PostOffice":"bad"}]`, postalFixture + `{}`} {
		if _, err := parsePostalLocations([]byte(raw), "560001"); err == nil {
			t.Errorf("accepted malformed response %s", raw)
		}
	}
}

func TestPostalLookupBoundedProviderAndCache(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: postalTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.Header.Get("User-Agent") != "EuroSuitsERP/1.0 (+https://erp.eurosuitsmfg.in)" {
			t.Fatal("postal provider requires the explicit ERP client identity")
		}
		if r.URL.String() != "https://api.postalpincode.in/pincode/560001" || r.Method != "GET" || r.Body != nil || r.Header.Get("Cookie") != "" || r.Header.Get("Authorization") != "" {
			t.Fatal("lookup sent unexpected destination or private information")
		}
		deadline, ok := r.Context().Deadline()
		if !ok || time.Until(deadline) > 5*time.Second {
			t.Fatal("lookup has no bounded deadline")
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(strings.NewReader(postalFixture))}, nil
	})}
	service := newPostalLookupService(client)
	for i := 0; i < 2; i++ {
		got, err := service.lookup(context.Background(), "560001")
		if err != nil || got.Country != "IN" || got.PostalCode != "560001" || len(got.Locations) != 2 {
			t.Fatalf("unexpected result: %#v, %v", got, err)
		}
	}
	if calls != 1 {
		t.Fatalf("cache missed: %d provider calls", calls)
	}
	for _, pin := range []string{"", "000000", "56000", "5600011", "56000x", "560001/../secret", "560001?x=y"} {
		if _, err := service.lookup(context.Background(), pin); err == nil {
			t.Errorf("accepted PIN %q", pin)
		}
	}
	if calls != 1 {
		t.Fatal("invalid PIN reached provider")
	}
	service.cache["560001"] = postalCacheEntry{expires: time.Now().Add(-time.Second)}
	for i := 0; i < 255; i++ {
		service.cache[fmt.Sprint(i)] = postalCacheEntry{expires: time.Now().Add(time.Hour)}
	}
	if _, err := service.lookup(context.Background(), "560001"); err != nil || len(service.cache) > 256 {
		t.Fatal("expired cache was not refreshed within bounded capacity")
	}
	for i := 0; i < cap(service.slots); i++ {
		service.slots <- struct{}{}
	}
	if _, err := service.lookup(context.Background(), "560002"); err == nil || calls != 2 {
		t.Fatal("concurrency overload reached provider")
	}
}

func TestPostalLookupProviderFailures(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		err    error
	}{
		{"network", 0, "", errors.New("private network detail")},
		{"redirect", 302, "", nil}, {"bad status", 500, "private error", nil},
		{"oversized", 200, strings.Repeat(" ", (1<<20)+1), nil},
		{"bad json", 200, "<html>error</html>", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			service := newPostalLookupService(&http.Client{Transport: postalTransport(func(r *http.Request) (*http.Response, error) {
				if tc.err != nil {
					return nil, tc.err
				}
				return &http.Response{StatusCode: tc.status, Body: io.NopCloser(strings.NewReader(tc.body))}, nil
			})})
			_, err := service.lookup(context.Background(), "560001")
			if err == nil || err.Error() != postalUnavailable().Error() || len(service.cache) != 0 {
				t.Fatalf("failure leaked details or was cached: %v", err)
			}
		})
	}
	if companyPostalService.client.CheckRedirect(nil, nil) != http.ErrUseLastResponse || companyPostalService.client.Timeout != 5*time.Second {
		t.Fatal("production client must prevent redirects and time out")
	}
}

func TestPostalLookupPermissionsAndInput(t *testing.T) {
	for _, tc := range []struct {
		role     string
		features []string
		allowed  bool
	}{
		{"Finance Head", []string{"view", "create"}, true}, {"Finance Head", []string{"view", "edit"}, true},
		{"Finance Head", []string{"view"}, false}, {"Finance Head", []string{"create"}, false},
		{"Finance Head", nil, false}, {"MD", []string{"view", "edit"}, false}, {"Super Admin", nil, false},
	} {
		u := &User{Role: tc.role, SetupCustomAccess: true, SetupFeatures: tc.features}
		if postalLookupAllowed(u) != tc.allowed {
			t.Errorf("wrong access for %s %v", tc.role, tc.features)
		}
		if !tc.allowed {
			r := httptest.NewRequest("GET", "/?country=IN&postal_code=560001", nil).WithContext(context.WithValue(context.Background(), authKey{}, u))
			if _, err := (&App{}).companyPostalLookup(httptest.NewRecorder(), r); err == nil {
				t.Fatal("unauthorized lookup accepted")
			}
		}
	}
	u := &User{Role: "Finance Head"}
	for _, query := range []string{"country=GB&postal_code=560001", "country=IN&postal_code=5600", "postal_code=560001", "country=IN&postal_code=000000"} {
		r := httptest.NewRequest("GET", "/?"+query, nil).WithContext(context.WithValue(context.Background(), authKey{}, u))
		if _, err := (&App{}).companyPostalLookup(httptest.NewRecorder(), r); err == nil {
			t.Errorf("invalid query accepted: %s", query)
		}
	}
}

func TestCompanyCreateAddressValidationAndEncryption(t *testing.T) {
	t.Setenv("KYC_ENCRYPTION_KEY", base64.StdEncoding.EncodeToString(make([]byte, 32)))
	p, err := companyCreateAddress(M{"address": " Private street ", "state": " Manual state ", "postal_code": " UNKNOWN ", "city": "Separate city"})
	if err != nil || p.Address != "Private street" || p.State != "Manual state" || p.PostalCode != "UNKNOWN" {
		t.Fatalf("manual details rejected: %#v %v", p, err)
	}
	sealed, err := sealCompany(p)
	if err != nil || strings.Contains(string(sealed), "Private street") {
		t.Fatal("address not encrypted")
	}
	opened, err := openCompany(M{"private_data": sealed})
	if err != nil || opened.Address != p.Address || opened.State != p.State || opened.PostalCode != p.PostalCode {
		t.Fatal("encrypted address did not round trip")
	}
	for _, tc := range []struct {
		key string
		max int
	}{{"address", 2000}, {"state", 100}, {"postal_code", 30}} {
		if _, err := companyCreateAddress(M{tc.key: strings.Repeat("x", tc.max)}); err != nil {
			t.Fatal("valid boundary rejected")
		}
		for _, value := range []any{nil, true, 12, []string{}, M{}, strings.Repeat("x", tc.max+1)} {
			if _, err := companyCreateAddress(M{tc.key: value}); err == nil {
				t.Errorf("invalid %s accepted: %T", tc.key, value)
			}
		}
	}
	if _, err := companyCreateAddress(M{}); err != nil {
		t.Fatal("legacy create rejected")
	}
	// Existing save JSON decoding rejects malformed nested address types.
	for _, raw := range []string{`{"state":42}`, `{"postal_code":false}`, `{"address":[]}`, `{"city":"not a private profile field"}`} {
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&CompanyPrivate{}); err == nil {
			t.Errorf("malformed profile accepted: %s", raw)
		}
	}
}

func TestPostal721433ReturnsAllEditableLocalities(t *testing.T) {
	raw, err := os.ReadFile("testdata/company_postal_721433.json")
	if err != nil {
		t.Fatal(err)
	}
	locations, err := parsePostalLocations(raw, "721433")
	if err != nil || len(locations) != 8 {
		t.Fatalf("expected eight localities, got %v, %v", locations, err)
	}
	for _, location := range locations {
		if location.State != "West Bengal" || location.District != "East Midnapore" || location.City == "" {
			t.Fatalf("incorrect PIN locality: %#v", location)
		}
	}
}
