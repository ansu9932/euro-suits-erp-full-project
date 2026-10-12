package app

import (
	"net/http"
	"net/netip"
	"strings"
)

func ipPolicyInput(d, before M) (bool, []string, error) {
	enabled, _ := before["ip_restricted"].(bool)
	ips := toStrings(before["allowed_ips"])
	if value, exists := d["ip_restricted"]; exists {
		var ok bool
		enabled, ok = value.(bool)
		if !ok {
			return false, nil, bad("Choose whether IP restrictions are enabled.")
		}
	}
	if value, exists := d["allowed_ips"]; exists {
		list, ok := value.([]any)
		if !ok || len(list) > 50 {
			return false, nil, bad("Use at most 50 IP addresses or CIDR ranges.")
		}
		ips = []string{}
		seen := map[string]bool{}
		for _, item := range list {
			raw, ok := item.(string)
			if !ok {
				return false, nil, bad("Enter valid IP addresses or CIDR ranges.")
			}
			raw = strings.TrimSpace(raw)
			var normalized string
			if addr, e := netip.ParseAddr(raw); e == nil && addr.Zone() == "" {
				normalized = addr.Unmap().String()
			} else if prefix, e := netip.ParsePrefix(raw); e == nil && !prefix.Addr().Is4In6() && prefix.Bits() > 0 {
				normalized = prefix.Masked().String()
			} else {
				return false, nil, bad("Enter valid IP addresses or CIDR ranges; unrestricted /0 ranges are not allowed.")
			}
			if !seen[normalized] {
				ips = append(ips, normalized)
				seen[normalized] = true
			}
		}
	}
	if before["is_developer"] == true && (enabled || len(ips) > 0) {
		return false, nil, bad("Developer access must remain available from any IP.")
	}
	if enabled && len(ips) == 0 {
		return false, nil, bad("Add at least one allowed IP before enabling restrictions.")
	}
	if ips == nil {
		ips = []string{}
	}
	return enabled, ips, nil
}

func userIPAllowed(u *User, raw string) bool {
	if u.IsDeveloper || !u.IPRestricted {
		return true
	}
	addr, e := netip.ParseAddr(raw)
	if e != nil || addr.Zone() != "" {
		return false
	}
	addr = addr.Unmap()
	for _, entry := range u.AllowedIPs {
		if ip, e := netip.ParseAddr(entry); e == nil && ip.Unmap() == addr {
			return true
		}
		if prefix, e := netip.ParsePrefix(entry); e == nil && prefix.Contains(addr) {
			return true
		}
	}
	return false
}
func checkUserIP(u *User, r *http.Request) error {
	if !userIPAllowed(u, clientIP(r)) {
		return failure{403, "This account is restricted to approved IP addresses. Contact your developer administrator."}
	}
	return nil
}
