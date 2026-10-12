package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode"
)

type postalLocation struct {
	City     string `json:"city"`
	State    string `json:"state"`
	District string `json:"district"`
}

type postalResult struct {
	Country    string           `json:"country"`
	PostalCode string           `json:"postal_code"`
	Locations  []postalLocation `json:"locations"`
}

type postalCacheEntry struct {
	result  postalResult
	expires time.Time
}

type postalLookupService struct {
	client *http.Client
	slots  chan struct{}
	mu     sync.Mutex
	cache  map[string]postalCacheEntry
}

func newPostalLookupService(client *http.Client) *postalLookupService {
	return &postalLookupService{client: client, slots: make(chan struct{}, 4), cache: make(map[string]postalCacheEntry)}
}

// There is deliberately no configurable URL: only the PIN leaves the ERP.
var companyPostalService = newPostalLookupService(&http.Client{
	Timeout:       5 * time.Second,
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
})

var indianPIN = regexp.MustCompile(`^[1-9][0-9]{5}$`)

func postalLookupAllowed(u *User) bool {
	return u.Role == "Finance Head" && u.SetupCan("view") && (u.SetupCan("create") || u.SetupCan("edit"))
}

func (a *App) companyPostalLookup(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if !postalLookupAllowed(u) {
		return nil, forbidden()
	}
	country := strings.ToUpper(strings.TrimSpace(r.URL.Query().Get("country")))
	pin := strings.TrimSpace(r.URL.Query().Get("postal_code"))
	if country != "IN" || !indianPIN.MatchString(pin) {
		return nil, bad("Enter a valid six-digit Indian PIN code. Other countries can be entered manually.")
	}
	if err := a.limit(r.Context(), fmt.Sprintf("company-postal:%d", u.ID), 20, time.Minute); err != nil {
		return nil, err
	}
	return companyPostalService.lookup(r.Context(), pin)
}

func postalUnavailable() error {
	return failure{503, "PIN lookup is unavailable. Enter the city and state manually."}
}

func (s *postalLookupService) lookup(ctx context.Context, pin string) (postalResult, error) {
	empty := postalResult{Country: "IN", PostalCode: pin, Locations: []postalLocation{}}
	if !indianPIN.MatchString(pin) {
		return empty, bad("Enter a valid six-digit Indian PIN code.")
	}
	s.mu.Lock()
	entry, exists := s.cache[pin]
	s.mu.Unlock()
	if exists && time.Now().Before(entry.expires) {
		return entry.result, nil
	}
	select {
	case s.slots <- struct{}{}:
		defer func() { <-s.slots }()
	default:
		return empty, postalUnavailable()
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.postalpincode.in/pincode/"+pin, nil)
	if err != nil {
		return empty, postalUnavailable()
	}
	req.Header.Set("Accept", "application/json")
	// Identify this integration explicitly. The provider resets connections from
	// Go's generic User-Agent, even when TLS and the PIN are otherwise valid.
	req.Header.Set("User-Agent", "EuroSuitsERP/1.0 (+https://erp.eurosuitsmfg.in)")
	response, err := s.client.Do(req)
	if err != nil {
		return empty, postalUnavailable()
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return empty, postalUnavailable()
	}
	raw, err := io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(raw) > 1<<20 {
		return empty, postalUnavailable()
	}
	locations, err := parsePostalLocations(raw, pin)
	if err != nil {
		return empty, postalUnavailable()
	}
	empty.Locations = locations
	ttl := time.Hour
	if len(locations) == 0 {
		ttl = 15 * time.Minute
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	// Bound process memory independently of the number of PINs requested.
	if len(s.cache) >= 256 {
		oldest := ""
		for key, value := range s.cache {
			if oldest == "" || value.expires.Before(s.cache[oldest].expires) {
				oldest = key
			}
		}
		delete(s.cache, oldest)
	}
	s.cache[pin] = postalCacheEntry{result: empty, expires: time.Now().Add(ttl)}
	return empty, nil
}

func postalPlace(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 100 || strings.ContainsFunc(value, unicode.IsControl) {
		return ""
	}
	switch strings.ToLower(value) {
	case "", "na", "n/a", "n.a.", "not available", "not applicable", "null", "nil", "none", "-":
		return ""
	}
	return value
}

func parsePostalLocations(raw []byte, pin string) ([]postalLocation, error) {
	var records []struct {
		Status     string
		Message    string
		PostOffice []struct{ Name, Block, District, State, Country, Pincode string }
	}
	if err := json.Unmarshal(raw, &records); err != nil || len(records) != 1 {
		return nil, postalUnavailable()
	}
	entry := records[0]
	if entry.Status != "Success" && !(entry.Status == "Error" && strings.EqualFold(strings.TrimSpace(entry.Message), "No records found")) {
		return nil, postalUnavailable()
	}
	locations := []postalLocation{}
	seen := map[string]bool{}
	for _, office := range entry.PostOffice {
		if office.Pincode != pin || !strings.EqualFold(strings.TrimSpace(office.Country), "India") {
			continue
		}
		state, district := postalPlace(office.State), postalPlace(office.District)
		city := postalPlace(office.Block)
		if city == "" {
			city = district
		}
		if state == "" || city == "" {
			continue
		}
		key := strings.ToLower(city + "\x00" + state + "\x00" + district)
		if !seen[key] {
			seen[key] = true
			locations = append(locations, postalLocation{City: city, State: state, District: district})
		}
	}
	slices.SortFunc(locations, func(a, b postalLocation) int {
		return strings.Compare(a.State+"\x00"+a.City+"\x00"+a.District, b.State+"\x00"+b.City+"\x00"+b.District)
	})
	return locations, nil
}

// Create accepts only these existing private address fields, never a generic
// profile that could bypass document and bank validation.
func companyCreateAddress(d M) (CompanyPrivate, error) {
	p := CompanyPrivate{}
	for _, field := range []struct {
		key string
		max int
		dst *string
	}{{"address", 2000, &p.Address}, {"state", 100, &p.State}, {"postal_code", 30, &p.PostalCode}} {
		if value, exists := d[field.key]; exists {
			text, ok := value.(string)
			if !ok || len(text) > field.max {
				return p, bad("Enter a valid " + strings.ReplaceAll(field.key, "_", " ") + ".")
			}
			*field.dst = strings.TrimSpace(text)
		}
	}
	return p, nil
}
