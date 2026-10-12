package app

import (
	"net/http/httptest"
	"testing"
)

func TestUserIPPolicy(t *testing.T) {
	u := &User{Role: "Super Admin", IPRestricted: true, AllowedIPs: []string{"203.0.113.10", "2001:db8:1234::/48", "198.51.100.0/24"}}
	for _, tc := range []struct {
		ip   string
		want bool
	}{{"203.0.113.10", true}, {"::ffff:203.0.113.10", true}, {"203.0.113.11", false}, {"198.51.100.99", true}, {"2001:db8:1234::9", true}, {"2001:db8:4321::9", false}, {"broken", false}} {
		if userIPAllowed(u, tc.ip) != tc.want {
			t.Errorf("IP %s", tc.ip)
		}
	}
	u.IsDeveloper = true
	if !userIPAllowed(u, "203.0.113.99") {
		t.Fatal("Developer locked out")
	}
	u.IsDeveloper = false
	u.AllowedIPs = nil
	if userIPAllowed(u, "203.0.113.10") {
		t.Fatal("Empty list failed open")
	}
	u.IPRestricted = false
	if !userIPAllowed(u, "203.0.113.10") {
		t.Fatal("Disabled restriction enforced")
	}
}
func TestIPPolicyInput(t *testing.T) {
	for _, d := range []M{{"ip_restricted": true}, {"ip_restricted": "yes"}, {"allowed_ips": []any{"bad"}}, {"allowed_ips": []any{"0.0.0.0/0"}}, {"allowed_ips": []any{"fe80::1%en0"}}, {"allowed_ips": "1.2.3.4"}} {
		if _, _, e := ipPolicyInput(d, M{}); e == nil {
			t.Fatalf("Accepted invalid input %v", d)
		}
	}
	enabled, ips, e := ipPolicyInput(M{"ip_restricted": true, "allowed_ips": []any{"203.0.113.1", " 203.0.113.1 ", "198.51.100.15/24"}}, M{})
	if e != nil || !enabled || len(ips) != 2 || ips[1] != "198.51.100.0/24" {
		t.Fatalf("Normalization failed: %v %v", ips, e)
	}
	if _, _, e := ipPolicyInput(M{"ip_restricted": true, "allowed_ips": []any{"203.0.113.1"}}, M{"is_developer": true}); e == nil {
		t.Fatal("Developer restriction accepted")
	}
}
func TestClientIPTrustBoundary(t *testing.T) {
	r := httptest.NewRequest("GET", "https://erp.example.com/api/session", nil)
	r.RemoteAddr = "203.0.113.9:1234"
	r.Header.Set("X-Real-IP", "198.51.100.10")
	r.Header.Set("X-Forwarded-For", "192.0.2.5")
	t.Setenv("TRUST_PROXY", "false")
	if clientIP(r) != "203.0.113.9" {
		t.Fatal("Untrusted header used")
	}
	t.Setenv("TRUST_PROXY", "true")
	if clientIP(r) != "198.51.100.10" {
		t.Fatal("Nginx IP not used")
	}
	r.Header.Set("X-Real-IP", "invalid")
	if clientIP(r) != "203.0.113.9" {
		t.Fatal("Invalid header used")
	}
}
