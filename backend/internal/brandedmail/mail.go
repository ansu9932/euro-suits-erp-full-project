// Package brandedmail renders transactional mail without remote assets or scripts.
package brandedmail

import (
	"bytes"
	"crypto/rand"
	"embed"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"html/template"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net/mail"
	"net/textproto"
	"net/url"
	"strings"
	"time"
)

//go:embed assets/logo.png template.html
var assets embed.FS

var layout = template.Must(template.ParseFS(assets, "template.html"))

type Detail struct{ Label, Value string }
type Message struct {
	ActionLabel, Encouragement                 string
	Responsibilities                           []string
	Welcome                                    bool
	Subject, Preheader, Category, Title, Intro string
	Code, Status, Note, ActionURL, Text        string
	Details                                    []Detail
	Overdue                                    bool
}

func SignIn(code string) Message {
	return Message{
		Subject: "Euro Suits: sign-in verification", Category: "ACCOUNT SECURITY",
		Preheader: "Your verification code is ready. It expires in 5 minutes.",
		Title:     "Your secure sign-in.", Intro: "Use this verification code to finish signing in to your Euro Suits workspace.",
		Code: code, Status: "ONE-TIME VERIFICATION",
		Details: []Detail{{"Valid for", "5 minutes"}, {"Use", "One sign-in only"}},
		Note:    "Keep this code private. If you did not request this sign-in, do not share the code and contact your developer.",
		Text:    "EURO SUITS | ACCOUNT SECURITY\n\nYour sign-in code is: " + code + "\n\nIt expires in 5 minutes and can be used once. Do not share it. If you did not request this sign-in, contact your developer.",
	}
}

// Compare calendar dates rather than clock times, including around DST changes.
func Renewal(company, item string, expires, today time.Time, origin string) Message {
	day := func(t time.Time) time.Time { return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC) }
	days := int(day(expires).Sub(day(today)).Hours() / 24)
	m := Message{
		Subject: "Euro Suits: company renewal reminder", Category: "DOCUMENT RENEWAL",
		Title: "Keep every detail current.", Intro: "A company document or registration is due for renewal. Please review the details and arrange the updated documents.",
		Status:  fmt.Sprintf("DUE IN %d DAYS", days),
		Details: []Detail{{"Company", company}, {"Document / registration", item}, {"Expiry date", expires.Format("02 Jan 2006")}},
		Note:    "Sign in to Company Setup to review and update this record. No KYC identifiers or document attachments are included in this email.",
	}
	if days == 1 {
		m.Status = "DUE TOMORROW"
	} else if days == 0 {
		m.Status, m.Title = "DUE TODAY", "A renewal needs your attention."
	} else if days < 0 {
		m.Overdue, m.Category, m.Title = true, "ACTION REQUIRED", "This renewal is overdue."
		m.Status = fmt.Sprintf("%d DAYS OVERDUE", -days)
		if days == -1 {
			m.Status = "1 DAY OVERDUE"
		}
		m.Intro = "A company document or registration has expired. Please arrange its renewal and update the record in Company Setup."
		m.Subject = "Euro Suits: overdue company renewal"
		if days <= -30 {
			m.Note = "This renewal is at least 30 days overdue. The responsible user's dashboard is blocked under the renewal policy. Contact an administrator to arrange the record update. No KYC identifiers or document attachments are included in this email."
		}
	}
	// Never put credentials, scripts, or untrusted schemes into an email link.
	u, err := url.Parse(origin)
	if err == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Hostname() != "" && u.User == nil && !strings.ContainsAny(origin, "\r\n") {
		m.ActionURL = u.String()
	}
	m.Preheader = m.Status + ": review the company renewal details in your workspace."
	m.Text = "EURO SUITS | " + m.Category + "\n\n" + m.Title + "\n" + m.Status + "\n\n" + m.Intro + "\n"
	for _, d := range m.Details {
		m.Text += "\n" + d.Label + ": " + d.Value
	}
	if m.ActionURL != "" {
		m.Text += "\n\nOpen Company Setup: " + m.ActionURL
	}
	m.Text += "\n\n" + m.Note
	return m
}

func HTML(m Message) ([]byte, error) {
	var b bytes.Buffer
	err := layout.Execute(&b, m)
	return b.Bytes(), err
}

// Preview embeds the same image in a standalone review artifact. Outgoing mail
// uses Content-ID instead, which does not rely on data-URI email support.
func Preview(m Message) ([]byte, error) {
	html, err := HTML(m)
	if err != nil {
		return nil, err
	}
	logo, err := assets.ReadFile("assets/logo.png")
	if err != nil {
		return nil, err
	}
	return bytes.ReplaceAll(html, []byte("cid:euro-suits-logo"), []byte("data:image/png;base64,"+base64.StdEncoding.EncodeToString(logo))), nil
}

// MIME includes text/plain first, followed by HTML and its inline company logo.
func MIME(from, to string, m Message) ([]byte, error) {
	for _, address := range []string{from, to} {
		a, err := mail.ParseAddress(address)
		if err != nil || a.Address != address || strings.ContainsAny(address, "\r\n") {
			return nil, fmt.Errorf("invalid email address")
		}
	}
	if strings.ContainsAny(m.Subject, "\r\n") {
		return nil, fmt.Errorf("invalid email subject")
	}
	html, err := HTML(m)
	if err != nil {
		return nil, err
	}
	logo, err := assets.ReadFile("assets/logo.png")
	if err != nil {
		return nil, err
	}
	id := make([]byte, 16)
	if _, err = rand.Read(id); err != nil {
		return nil, err
	}
	var body bytes.Buffer
	alternative := multipart.NewWriter(&body)
	writeText := func(w *multipart.Writer, contentType string, data []byte) error {
		part, err := w.CreatePart(textproto.MIMEHeader{"Content-Type": {contentType}, "Content-Transfer-Encoding": {"quoted-printable"}})
		if err != nil {
			return err
		}
		qp := quotedprintable.NewWriter(part)
		if _, err = qp.Write(data); err != nil {
			return err
		}
		return qp.Close()
	}
	if err = writeText(alternative, "text/plain; charset=UTF-8", []byte(m.Text)); err != nil {
		return nil, err
	}
	var relatedBody bytes.Buffer
	related := multipart.NewWriter(&relatedBody)
	if err = writeText(related, "text/html; charset=UTF-8", html); err != nil {
		return nil, err
	}
	part, err := related.CreatePart(textproto.MIMEHeader{"Content-Type": {"image/png"}, "Content-Transfer-Encoding": {"base64"}, "Content-ID": {"<euro-suits-logo>"}, "Content-Disposition": {`inline; filename="euro-suits-logo.png"`}})
	if err != nil {
		return nil, err
	}
	encoded := base64.StdEncoding.EncodeToString(logo)
	for len(encoded) > 0 {
		n := min(76, len(encoded))
		if _, err = fmt.Fprint(part, encoded[:n]+"\r\n"); err != nil {
			return nil, err
		}
		encoded = encoded[n:]
	}
	if err = related.Close(); err != nil {
		return nil, err
	}
	part, err = alternative.CreatePart(textproto.MIMEHeader{"Content-Type": {mime.FormatMediaType("multipart/related", map[string]string{"boundary": related.Boundary(), "type": "text/html"})}})
	if err != nil {
		return nil, err
	}
	if _, err = part.Write(relatedBody.Bytes()); err != nil {
		return nil, err
	}
	if err = alternative.Close(); err != nil {
		return nil, err
	}
	var out bytes.Buffer
	domain := strings.SplitN(from, "@", 2)[1]
	fmt.Fprintf(&out, "From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMessage-ID: <%s@%s>\r\nMIME-Version: 1.0\r\nAuto-Submitted: auto-generated\r\nContent-Type: %s\r\n\r\n", (&mail.Address{Name: "Euro Suits", Address: from}).String(), (&mail.Address{Address: to}).String(), mime.QEncoding.Encode("UTF-8", m.Subject), time.Now().Format(time.RFC1123Z), hex.EncodeToString(id), domain, mime.FormatMediaType("multipart/alternative", map[string]string{"boundary": alternative.Boundary()}))
	out.Write(body.Bytes())
	return out.Bytes(), nil
}
