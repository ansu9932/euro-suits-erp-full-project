package brandedmail

import (
	"bytes"
	"encoding/base64"
	"io"
	"mime"
	"mime/multipart"
	"net/mail"
	"strings"
	"testing"
	"time"
)

func TestMIMERoundTrip(t *testing.T) {
	raw, err := MIME("sender@example.com", "recipient@example.com", SignIn("004291"))
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range bytes.Split(raw, []byte("\r\n")) {
		if len(line) > 998 {
			t.Fatalf("SMTP line too long: %d", len(line))
		}
	}
	m, err := mail.ReadMessage(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	from, err := m.Header.AddressList("From")
	if err != nil || len(from) != 1 || from[0].Name != "Euro Suits" {
		t.Fatal("Missing company sender name")
	}
	kind, params, err := mime.ParseMediaType(m.Header.Get("Content-Type"))
	if err != nil || kind != "multipart/alternative" {
		t.Fatal("Missing accessible mail alternatives")
	}
	alternative := multipart.NewReader(m.Body, params["boundary"])
	plain, err := alternative.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	text, _ := io.ReadAll(plain)
	if !strings.HasPrefix(plain.Header.Get("Content-Type"), "text/plain") || !bytes.Contains(text, []byte("code is: 004291")) {
		t.Fatal("Text fallback lost the leading-zero code")
	}
	related, err := alternative.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	kind, params, err = mime.ParseMediaType(related.Header.Get("Content-Type"))
	if err != nil || kind != "multipart/related" {
		t.Fatal("Missing related logo container")
	}
	parts := multipart.NewReader(related, params["boundary"])
	htmlPart, err := parts.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	html, _ := io.ReadAll(htmlPart)
	if !bytes.Contains(html, []byte("cid:euro-suits-logo")) || !bytes.Contains(html, []byte("004291")) {
		t.Fatal("HTML code or logo missing")
	}
	image, err := parts.NextPart()
	if err != nil {
		t.Fatal(err)
	}
	if image.Header.Get("Content-ID") != "<euro-suits-logo>" {
		t.Fatal("Logo CID does not match HTML")
	}
	decoded, err := io.ReadAll(base64.NewDecoder(base64.StdEncoding, image))
	expected, _ := assets.ReadFile("assets/logo.png")
	if err != nil || !bytes.Equal(decoded, expected) {
		t.Fatal("Logo is not the unchanged company asset")
	}
	if _, err = parts.NextPart(); err != io.EOF {
		t.Fatal("Related body is not closed")
	}
	if _, err = alternative.NextPart(); err != io.EOF {
		t.Fatal("Alternative body is not closed")
	}
}

func TestUntrustedEmailContent(t *testing.T) {
	today := time.Date(2026, 9, 21, 0, 0, 0, 0, time.UTC)
	for _, origin := range []string{"javascript:alert(1)", "data:text/html,unsafe", "https://user:password@example.com", "https://example.com\r\nBcc:other@example.com"} {
		m := Renewal(`<img src=x onerror="alert(1)">`, "Tissu & vêtements", today, today, origin)
		html, err := HTML(m)
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(html, []byte("<img src=x")) || bytes.Contains(html, []byte("#ZgotmplZ")) || m.ActionURL != "" {
			t.Fatal("Unsafe content reached HTML or CTA")
		}
		if !bytes.Contains(html, []byte("&lt;img")) || !bytes.Contains(html, []byte("vêtements")) {
			t.Fatal("Escaped or international text lost")
		}
	}
	for _, address := range []string{"victim@example.com\r\nBcc: other@example.com", "Name <sender@example.com>"} {
		if _, err := MIME(address, "recipient@example.com", SignIn("012345")); err == nil {
			t.Fatal("Unsafe sender accepted")
		}
		if _, err := MIME("sender@example.com", address, SignIn("012345")); err == nil {
			t.Fatal("Unsafe recipient accepted")
		}
	}
	m := SignIn("012345")
	m.Subject = "Subject\r\nBcc: other@example.com"
	if _, err := MIME("sender@example.com", "recipient@example.com", m); err == nil {
		t.Fatal("Header injection accepted")
	}
}

func TestRenewalTiming(t *testing.T) {
	today := time.Date(2026, 9, 21, 23, 59, 0, 0, time.FixedZone("factory", 19800))
	for _, tc := range []struct {
		days    int
		status  string
		blocked bool
	}{
		{30, "DUE IN 30 DAYS", false}, {1, "DUE TOMORROW", false}, {0, "DUE TODAY", false},
		{-1, "1 DAY OVERDUE", false}, {-29, "29 DAYS OVERDUE", false}, {-30, "30 DAYS OVERDUE", true},
	} {
		t.Run(tc.status, func(t *testing.T) {
			m := Renewal("Example Garments", "Factory licence", today.AddDate(0, 0, tc.days), today, "https://erp.example.com")
			if m.Status != tc.status || m.Overdue != (tc.days < 0) || strings.Contains(m.Note, "dashboard is blocked") != tc.blocked {
				t.Fatal("Wrong renewal urgency or access policy")
			}
			if !strings.Contains(m.Text, tc.status) {
				t.Fatal("Plain text lost urgency")
			}
		})
	}
}
