package app

import (
	"context"
	"crypto/tls"
	"eurosuits/erp/internal/brandedmail"
	"fmt"
	"net"
	"net/smtp"
	"os"
	"slices"
	"strconv"
	"strings"
	"time"
)

func companyRecipients(p CompanyPrivate) []string {
	out := []string{}
	for _, address := range append([]string{p.ReminderEmail}, p.ReminderEmails...) {
		address = strings.TrimSpace(address)
		if address != "" && !slices.Contains(out, address) {
			out = append(out, address)
		}
	}
	return out
}
func emailConfigured() bool {
	return os.Getenv("SMTP_HOST") != "" && validEmail(os.Getenv("SMTP_FROM")) && smtpCredentialsConfigured()
}

// Clamp to the last day of the target month (April 30 becomes February 28).
func companyReminderDate(expiry time.Time) time.Time {
	month := time.Date(expiry.Year(), expiry.Month()-2, 1, 0, 0, 0, 0, expiry.Location())
	last := month.AddDate(0, 1, -1).Day()
	return month.AddDate(0, 0, min(expiry.Day(), last)-1)
}
func (a *App) syncCompanyReminders(ctx context.Context, q queryer, cid int64, p CompanyPrivate, approved bool) error {
	if _, e := q.Exec(ctx, `UPDATE company_reminders SET active=false WHERE company_id=$1`, cid); e != nil {
		return e
	}
	if !approved {
		return nil
	}
	add := func(key, label, expiry string) error {
		end, e := date(expiry)
		if e != nil {
			return e
		}
		_, e = q.Exec(ctx, `INSERT INTO company_reminders(company_id,obligation_key,label,expires_on,remind_on) VALUES($1,$2,$3,$4,$5) ON CONFLICT(company_id,obligation_key,expires_on) DO UPDATE SET active=true,remind_on=excluded.remind_on`, cid, key, label, end, companyReminderDate(end))
		return e
	}
	company, e := a.setupRow(ctx, q, cid, false)
	if e != nil {
		return e
	}
	definitions, e := companyApplicableFields(ctx, q, company)
	if e != nil {
		return e
	}
	labels := map[int64]string{}
	for _, definition := range definitions {
		labels[id(definition, "id")] = str(definition, "label")
	}
	for _, f := range p.Fields {
		label, applies := labels[f.ID]
		if applies && !f.NotApplicable && f.Expiry && f.ExpiresOn != "" {
			if e := add("field:"+strconv.FormatInt(f.ID, 10), label, f.ExpiresOn); e != nil {
				return e
			}
		}
	}
	for _, t := range p.TradeRegistrations {
		if t.Expiry && t.ExpiresOn != "" {
			if e := add("trade:"+t.ID, t.Portal+" registration / licence", t.ExpiresOn); e != nil {
				return e
			}
		}
	}
	for _, p := range p.Ports {
		if p.Registered && p.Expiry && p.ExpiresOn != "" {
			var label string
			if e := q.QueryRow(ctx, `SELECT name FROM company_ports WHERE id=$1`, p.ID).Scan(&label); e != nil {
				return e
			}
			if e := add("port:"+strconv.FormatInt(p.ID, 10), "Port registration: "+label, p.ExpiresOn); e != nil {
				return e
			}
		}
	}
	return nil
}
func sendBrandedMail(to string, message brandedmail.Message) error {
	host, port, from := os.Getenv("SMTP_HOST"), os.Getenv("SMTP_PORT"), os.Getenv("SMTP_FROM")
	if port == "" {
		port = "587"
	}
	if !validEmail(to) || !validEmail(from) {
		return fmt.Errorf("invalid email configuration")
	}
	messageBytes, e := brandedmail.MIME(from, to, message)
	if e != nil {
		return e
	}
	conn, e := net.DialTimeout("tcp", net.JoinHostPort(host, port), 5*time.Second)
	if e != nil {
		return e
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(10 * time.Second))
	c, e := smtp.NewClient(conn, host)
	if e != nil {
		return e
	}
	defer c.Close()
	if e = c.StartTLS(&tls.Config{ServerName: host, MinVersion: tls.VersionTLS12}); e != nil {
		return e
	}
	if e = smtpAuthenticate(c, host); e != nil {
		return e
	}
	if e = c.Mail(from); e != nil {
		return e
	}
	if e = c.Rcpt(to); e != nil {
		return e
	}
	writer, e := c.Data()
	if e != nil {
		return e
	}
	_, e = writer.Write(messageBytes)
	if e != nil {
		return e
	}
	if e = writer.Close(); e != nil {
		return e
	}
	return nil
}
func (a *App) companyEmailReminders(ctx context.Context) {
	// Each row is locked while sending so multiple app instances cannot send it concurrently.
	for i := 0; i < 5; i++ {
		tx, e := a.DB.Begin(ctx)
		if e != nil {
			return
		}
		rec, e := row(ctx, tx, `SELECT cr.*,cr.expires_on::text AS expiry_date,CURRENT_DATE::text AS reminder_today FROM company_reminders cr JOIN company_setups c ON c.id=cr.company_id WHERE c.deleted_at IS NULL AND cr.active AND cr.remind_on<=CURRENT_DATE AND cr.next_attempt<=now() ORDER BY cr.id LIMIT 1 FOR UPDATE OF cr SKIP LOCKED`)
		if e != nil {
			tx.Rollback(ctx)
			return
		}
		status, lastError := "Sent", ""
		company, deliveryErr := a.setupRow(ctx, tx, id(rec, "company_id"), false)
		var p CompanyPrivate
		if deliveryErr == nil {
			p, deliveryErr = openCompany(company)
		}
		var recipients []string
		enabled := true
		if deliveryErr == nil {
			recipients, enabled, deliveryErr = companyReminderRecipients(ctx, tx, id(rec, "company_id"), p, str(rec, "obligation_key"))
		}
		if deliveryErr == nil && !enabled {
			status, lastError = "Disabled", "Email reminders are disabled for this document"
		} else if deliveryErr == nil && !emailConfigured() {
			status, lastError = "Not configured", "SMTP sender is not configured"
		} else if deliveryErr == nil {
			expiry, expiryErr := date(str(rec, "expiry_date"))
			today, todayErr := date(str(rec, "reminder_today"))
			if expiryErr != nil {
				deliveryErr = expiryErr
			} else if todayErr != nil {
				deliveryErr = todayErr
			}
			if deliveryErr == nil && len(recipients) == 0 {
				deliveryErr = fmt.Errorf("no reminder recipients")
			}
			if deliveryErr == nil {
				message := brandedmail.Renewal(str(company, "legal_name"), str(rec, "label"), expiry, today, a.Origin)
				for _, recipient := range recipients {
					if err := sendBrandedMail(recipient, message); err != nil {
						deliveryErr = err
						break
					}
				}
			}
		}
		if deliveryErr != nil {
			a.recordIssue(ctx, "Email", "Error", "Company reminder email failed; check sender and recipients", "reminder worker", 0, 0, "")
			status, lastError = "Failed", "Email delivery failed; check SMTP settings and server logs"
		}
		_, e = tx.Exec(ctx, `UPDATE company_reminders SET email_status=$1,last_error=$2,attempts=attempts+CASE WHEN $1='Disabled' THEN 0 ELSE 1 END,next_attempt=CASE WHEN $1 IN ('Sent','Disabled') THEN now()+interval '1 day' ELSE now()+interval '1 hour' END,sent_at=CASE WHEN $1='Sent' THEN now() ELSE sent_at END WHERE id=$3`, status, lastError, id(rec, "id"))
		if e != nil {
			tx.Rollback(ctx)
			return
		}
		if e = tx.Commit(ctx); e != nil {
			return
		}
	}
}
