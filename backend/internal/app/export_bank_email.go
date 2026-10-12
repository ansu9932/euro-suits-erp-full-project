package app

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/http"
	"net/mail"
	"net/smtp"
	"net/textproto"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
)

var exportBankKinds = []string{"invoice", "shipping_bill", "bill_of_lading", "packing_list", "bank_advice", "payment_details"}

func exportBankMailAllowed(u *User) bool { return exportEditor(u) && exportCan(u, "download") }

func exportBankMailInput(d M) (M, error) {
	if e := allowedFields(d, "version", "recipients", "subject", "body", "file_ids"); e != nil {
		return nil, e
	}
	subject, e := text(d, "subject", 160)
	if e != nil || strings.ContainsAny(subject, "\r\n\x00") {
		return nil, bad("Enter an email subject on one line, up to 160 characters.")
	}
	body, e := text(d, "body", 10000)
	if e != nil || strings.ContainsRune(body, 0) {
		return nil, bad("Enter an email message up to 10,000 characters.")
	}
	raw, ok := d["recipients"].([]any)
	if !ok || len(raw) < 1 || len(raw) > 5 {
		return nil, bad("Enter one to five bank email addresses.")
	}
	recipients := []string{}
	for _, v := range raw {
		s, ok := v.(string)
		s = strings.TrimSpace(s)
		a, err := mail.ParseAddress(s)
		if !ok || err != nil || a.Address != s || !validEmail(s) || strings.ContainsAny(s, "\r\n") {
			return nil, bad("Enter valid bank email addresses without display names.")
		}
		if !slices.Contains(recipients, s) {
			recipients = append(recipients, s)
		}
	}
	ids, ok := d["file_ids"].([]any)
	if !ok || len(ids) < 1 || len(ids) > 12 {
		return nil, bad("Select one to twelve saved documents to send to the bank.")
	}
	files := []int64{}
	for _, v := range ids {
		n, e := integer(v)
		if e != nil || n < 1 || slices.Contains(files, n) {
			return nil, bad("Select each saved document only once.")
		}
		files = append(files, n)
	}
	slices.Sort(files)
	return M{"recipients": recipients, "subject": subject, "body": body, "file_ids": files}, nil
}

func (a *App) exportBankMailParent(ctx context.Context, q queryer, u *User, rid int64, lock bool) (M, error) {
	if !exportBankMailAllowed(u) {
		return nil, forbidden()
	}
	m, e := a.exportRecord(ctx, q, u, rid, lock)
	if e != nil {
		return nil, e
	}
	if exportRecordKind(m) != "export" {
		return nil, bad("Bank email is available for export invoices only.")
	}
	if e = a.exportWritableCompany(ctx, q, u, m); e != nil {
		return nil, e
	}
	return m, nil
}

const exportBankMailSelect = `SELECT e.id,e.invoice_id,e.invoice_version,e.recipients,e.subject,e.body,e.files,e.status,e.created_at,e.sent_at,e.error_message,e.created_by,(SELECT name FROM users WHERE id=e.created_by) AS created_by_name FROM export_bank_emails e `

func (a *App) exportBankEmails(w http.ResponseWriter, r *http.Request) (any, error) {
	rid := materialID(r)
	if _, e := a.exportBankMailParent(r.Context(), a.DB, current(r), rid, false); e != nil {
		return nil, e
	}
	items, e := rows(r.Context(), a.DB, exportBankMailSelect+`WHERE invoice_id=$1 ORDER BY id DESC LIMIT 100`, rid)
	return M{"items": items, "configured": emailConfigured(), "can_send": exportBankMailAllowed(current(r))}, e
}

func (a *App) exportBankEmailPrepare(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if !exportBankMailAllowed(u) {
		return nil, forbidden()
	}
	d, e := readBody(w, r)
	if e != nil {
		return nil, e
	}
	input, e := exportBankMailInput(d)
	if e != nil {
		return nil, e
	}
	ctx := r.Context()
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	rid := materialID(r)
	parent, e := a.exportBankMailParent(ctx, tx, u, rid, true)
	if e != nil {
		return nil, e
	}
	if id(parent, "version") != id(d, "version") {
		return nil, exportConflict()
	}
	files, e := rows(ctx, tx, `SELECT f.id,f.kind,f.filename,f.mime,f.size,f.sha256,f.receipt_key::text AS receipt_key,f.field_key FROM export_files f WHERE invoice_id=$1 AND f.id=ANY($2::bigint[]) AND NOT EXISTS(SELECT 1 FROM export_file_removals x WHERE x.file_id=f.id) ORDER BY f.id`, rid, input["file_ids"])
	if e != nil {
		return nil, e
	}
	if len(files) != len(input["file_ids"].([]int64)) {
		return nil, bad("A selected document is unavailable. Refresh the invoice and select its current documents.")
	}
	var total int64
	for _, f := range files {
		if !slices.Contains(exportBankKinds, str(f, "kind")) {
			return nil, bad("Select invoice, shipping bill, BL/AWB, packing list, payment details or bank advice documents.")
		}
		total += id(f, "size")
	}
	if total > 15<<20 {
		return nil, bad("Select documents totalling no more than 15 MB per bank email.")
	}
	exportLabelReceiptFiles(parent, files)
	input["files"] = files
	input["invoice_id"] = rid
	input["invoice_version"] = id(parent, "version")
	input["created_by"] = u.ID
	canonical, _ := json.Marshal(input)
	key := fmt.Sprintf("%x", sha256.Sum256(canonical))
	recipients, _ := json.Marshal(input["recipients"])
	fileJSON, _ := json.Marshal(files)
	var eid int64
	e = tx.QueryRow(ctx, `INSERT INTO export_bank_emails(invoice_id,invoice_version,created_by,recipients,subject,body,files,request_hash) VALUES($1,$2,$3,$4,$5,$6,$7,$8) ON CONFLICT(request_hash) DO UPDATE SET request_hash=excluded.request_hash RETURNING id`, rid, id(parent, "version"), u.ID, recipients, input["subject"], input["body"], fileJSON, key).Scan(&eid)
	if e != nil {
		return nil, e
	}
	result, e := row(ctx, tx, exportBankMailSelect+`WHERE e.id=$1`, eid)
	if e != nil {
		return nil, e
	}
	return result, tx.Commit(ctx)
}

type exportMailAttachment struct {
	Name, Type string
	Data       []byte
}

func exportBankMIME(from string, recipients []string, subject, body string, files []exportMailAttachment, eid int64) ([]byte, error) {
	sender, e := mail.ParseAddress(from)
	if e != nil || sender.Address != from || strings.ContainsAny(subject+from, "\r\n") {
		return nil, fmt.Errorf("invalid mail header")
	}
	var b bytes.Buffer
	mixed := multipart.NewWriter(&b)
	fmt.Fprintf(&b, "From: %s\r\nTo: %s\r\nSubject: %s\r\nDate: %s\r\nMessage-ID: <export-bank-%d@%s>\r\nMIME-Version: 1.0\r\nContent-Type: multipart/mixed; boundary=%q\r\n\r\n", from, strings.Join(recipients, ", "), mime.QEncoding.Encode("utf-8", subject), time.Now().UTC().Format(time.RFC1123Z), eid, strings.Split(sender.Address, "@")[1], mixed.Boundary())
	h := textproto.MIMEHeader{}
	h.Set("Content-Type", "text/plain; charset=utf-8")
	h.Set("Content-Transfer-Encoding", "quoted-printable")
	p, e := mixed.CreatePart(h)
	if e != nil {
		return nil, e
	}
	qp := quotedprintable.NewWriter(p)
	if _, e = qp.Write([]byte(body)); e != nil {
		return nil, e
	}
	if e = qp.Close(); e != nil {
		return nil, e
	}
	for _, f := range files {
		h = textproto.MIMEHeader{}
		h.Set("Content-Type", "application/octet-stream")
		h.Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": f.Name}))
		h.Set("Content-Transfer-Encoding", "base64")
		p, e = mixed.CreatePart(h)
		if e != nil {
			return nil, e
		}
		encoded := base64.StdEncoding.EncodeToString(f.Data)
		for len(encoded) > 0 {
			n := min(76, len(encoded))
			if _, e = fmt.Fprint(p, encoded[:n]+"\r\n"); e != nil {
				return nil, e
			}
			encoded = encoded[n:]
		}
	}
	if e = mixed.Close(); e != nil {
		return nil, e
	}
	return b.Bytes(), nil
}

// Once DATA starts an error is ambiguous. Never automatically retry a message
// the bank may already have accepted, including after an API process crash.
func sendExportBankMail(recipients []string, message []byte) (uncertain bool, err error) {
	host, port, from := os.Getenv("SMTP_HOST"), os.Getenv("SMTP_PORT"), os.Getenv("SMTP_FROM")
	if port == "" {
		port = "587"
	}
	conn, e := net.DialTimeout("tcp", net.JoinHostPort(host, port), 5*time.Second)
	if e != nil {
		return false, e
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(20 * time.Second))
	c, e := smtp.NewClient(conn, host)
	if e != nil {
		return false, e
	}
	defer c.Close()
	if e = c.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); e != nil {
		return false, e
	}
	if e = smtpAuthenticate(c, host); e != nil {
		return false, e
	}
	if e = c.Mail(from); e != nil {
		return false, e
	}
	for _, to := range recipients {
		if e = c.Rcpt(to); e != nil {
			return false, e
		}
	}
	writer, e := c.Data()
	if e != nil {
		return false, e
	}
	if _, e = writer.Write(message); e != nil {
		return true, e
	}
	if e = writer.Close(); e != nil {
		return true, e
	}
	return false, nil
}

func (a *App) exportBankEmailSend(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if !exportBankMailAllowed(u) {
		return nil, forbidden()
	}
	select {
	case a.UploadSlots <- struct{}{}:
		defer func() { <-a.UploadSlots }()
	default:
		return nil, failure{429, "Document services are busy. Retry shortly."}
	}
	d, e := readBody(w, r)
	if e != nil {
		return nil, e
	}
	if e = allowedFields(d, "version", "confirmed"); e != nil {
		return nil, e
	}
	if d["confirmed"] != true {
		return nil, bad("Review the recipient, message and attachments, then confirm sending.")
	}
	if !emailConfigured() {
		return nil, bad("Bank email is not configured. Ask the administrator to check the email sender.")
	}
	ctx := r.Context()
	if e = a.limit(ctx, fmt.Sprintf("bank-mail:%d", u.ID), 5, time.Minute); e != nil {
		return nil, e
	}
	eid, e := strconv.ParseInt(chi.URLParam(r, "email_id"), 10, 64)
	if e != nil || eid < 1 {
		return nil, missing()
	}
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	rid := materialID(r)
	parent, e := a.exportBankMailParent(ctx, tx, u, rid, true)
	if e != nil {
		return nil, e
	}
	saved, e := row(ctx, tx, exportBankMailSelect+`WHERE e.id=$1 AND invoice_id=$2 AND created_by=$3 FOR UPDATE OF e`, eid, rid, u.ID)
	if e != nil {
		return nil, e
	}
	if !slices.Contains([]string{"Prepared", "Failed"}, str(saved, "status")) {
		return nil, bad("This email was already sent or its result is unconfirmed. Check the bank before sending another message.")
	}
	if id(parent, "version") != id(d, "version") || id(saved, "invoice_version") != id(parent, "version") {
		return nil, exportConflict()
	}
	attachments := []exportMailAttachment{}
	cipher, e := companyCipher()
	if e != nil {
		return nil, e
	}
	for _, f := range enquiryRows(saved["files"]) {
		stored, e := row(ctx, tx, `SELECT f.*, f.receipt_key::text AS receipt_key FROM export_files f WHERE id=$1 AND invoice_id=$2 AND NOT EXISTS(SELECT 1 FROM export_file_removals x WHERE x.file_id=f.id)`, id(f, "id"), rid)
		if e != nil {
			return nil, bad("A reviewed attachment is no longer available. Prepare a new email.")
		}
		if str(stored, "sha256") != str(f, "sha256") || str(stored, "receipt_key") != str(f, "receipt_key") || str(stored, "field_key") != str(f, "field_key") {
			return nil, bad("A reviewed attachment changed. Prepare a new email.")
		}
		raw, ok := stored["ciphertext"].([]byte)
		if !ok || len(raw) < cipher.NonceSize() {
			return nil, bad("An attachment could not be opened.")
		}
		plain, e := cipher.Open(nil, raw[:cipher.NonceSize()], raw[cipher.NonceSize():], []byte(fmt.Sprintf("export:%d", rid)))
		if e != nil {
			return nil, e
		}
		attachments = append(attachments, exportMailAttachment{Name: str(stored, "filename"), Type: str(stored, "mime"), Data: plain})
	}
	recipients := []string{}
	for _, v := range saved["recipients"].([]any) {
		recipients = append(recipients, v.(string))
	}
	message, e := exportBankMIME(os.Getenv("SMTP_FROM"), recipients, str(saved, "subject"), str(saved, "body"), attachments, eid)
	if e != nil {
		return nil, e
	}
	if _, e = tx.Exec(ctx, `UPDATE export_bank_emails SET status='Sending',error_message='',sent_at=NULL WHERE id=$1`, eid); e != nil {
		return nil, e
	}
	if e = a.exportHistory(ctx, tx, u, rid, fmt.Sprintf("Bank email #%d sending confirmed", eid)); e != nil {
		return nil, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, e
	}
	uncertain, sendError := sendExportBankMail(recipients, message)
	status, description := "Accepted", ""
	if sendError != nil {
		status, description = "Failed", "The email could not be sent. Check the recipient and email configuration before trying again."
		if uncertain {
			status, description = "Uncertain", "The bank may have received this email. Check before sending again; no automatic retry will occur."
		}
	}
	// Preserve authenticated RLS identity while allowing the result to be saved
	// after a browser disconnect. A process crash leaves non-replayable Sending.
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	tx2, e := a.DB.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx2.Rollback(ctx)
	if _, e = tx2.Exec(ctx, `UPDATE export_bank_emails SET status=$1,error_message=$2,sent_at=CASE WHEN $1='Accepted' THEN now() ELSE NULL END WHERE id=$3 AND status='Sending'`, status, description, eid); e != nil {
		return nil, e
	}
	if e = a.exportHistory(ctx, tx2, u, rid, fmt.Sprintf("Bank email #%d %s", eid, strings.ToLower(status))); e != nil {
		return nil, e
	}
	result, e := row(ctx, tx2, exportBankMailSelect+`WHERE e.id=$1`, eid)
	if e != nil {
		return nil, e
	}
	return result, tx2.Commit(ctx)
}
