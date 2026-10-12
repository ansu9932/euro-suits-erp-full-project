package brandedmail

import (
	"strings"
	"testing"
)

func TestWelcomeContentAndEscaping(t *testing.T) {
	d := WelcomeData{ID: 1, Name: "Asha <script>alert(1)</script>", Username: "asha", Password: "a<&b-secret", Designation: "Finance Head", Department: "Finance", Company: "Euro Suits", Scope: "Assigned companies", Dashboard: "https://erp.eurosuitsmfg.in/finance", Responsibilities: []string{"Create company records"}, Active: true}
	m := Welcome(d)
	b, e := HTML(m)
	if e != nil {
		t.Fatal(e)
	}
	html := string(b)
	for _, want := range []string{"cid:euro-suits-logo", "Open your dashboard", "https://erp.eurosuitsmfg.in/finance", "Create company records", "Finance Head", "a&lt;&amp;b-secret"} {
		if !strings.Contains(html, want) {
			t.Fatalf("Missing %q", want)
		}
	}
	if strings.Contains(html, "<script>") {
		t.Fatal("Unescaped employee data")
	}
	if !strings.Contains(m.Text, d.Password) || !strings.Contains(m.Text, d.Username) {
		t.Fatal("Plain-text details missing")
	}
	d.ID = 2
	if Welcome(d).Encouragement == m.Encouragement {
		t.Fatal("Messages do not vary")
	}
	d.Active = false
	if strings.Contains(Welcome(d).Status, "IS READY") {
		t.Fatal("Inactive account marked ready")
	}
	for _, link := range []string{"javascript:alert(1)", "https://user:pass@example.com", "https://example.com\r\nX: y"} {
		d.Dashboard = link
		if Welcome(d).ActionURL != "" {
			t.Fatal("Unsafe dashboard accepted")
		}
	}
}
