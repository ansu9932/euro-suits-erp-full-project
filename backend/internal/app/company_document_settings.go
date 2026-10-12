package app

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

// A nil flag preserves pre-existing delivery rules. An explicit choice is never
// supplemented by company-level or application-level notification recipients.
type companyReminderSelection struct {
	explicit   bool
	enabled    bool
	recipients []string
}

func documentReminderSelection(p CompanyPrivate, key string) companyReminderSelection {
	choose := func(flag *bool, addresses []string, applicable bool) companyReminderSelection {
		if !applicable {
			return companyReminderSelection{explicit: true}
		}
		if flag == nil {
			return companyReminderSelection{enabled: true}
		}
		return companyReminderSelection{explicit: true, enabled: *flag, recipients: normalizeReminderEmails(addresses)}
	}
	for _, f := range p.Fields {
		if key == "field:"+strconv.FormatInt(f.ID, 10) {
			return choose(f.ReminderEnabled, f.ReminderEmails, !f.NotApplicable && f.Expiry)
		}
	}
	for _, port := range p.Ports {
		if key == "port:"+strconv.FormatInt(port.ID, 10) {
			return choose(port.ReminderEnabled, port.ReminderEmails, port.Registered && port.Expiry)
		}
	}
	for _, trade := range p.TradeRegistrations {
		if key == "trade:"+trade.ID {
			return choose(trade.ReminderEnabled, trade.ReminderEmails, trade.Expiry)
		}
	}
	// A removed document must not fall back to unrelated recipients. Its existing
	// expiry obligation is still retained until the amendment is approved.
	return companyReminderSelection{explicit: true}
}

func normalizeReminderEmails(addresses []string) []string {
	out := []string{}
	for _, address := range addresses {
		address = strings.ToLower(strings.TrimSpace(address))
		if address != "" && !slices.Contains(out, address) {
			out = append(out, address)
		}
	}
	return out
}

func validateDocumentReminder(enabled *bool, addresses []string, applicable bool, label string) []string {
	issues := []string{}
	add := func(message string) { issues = append(issues, label+": "+message) }
	if len(addresses) > 20 {
		add("use at most 20 reminder email addresses.")
	}
	for _, address := range addresses {
		if len(strings.TrimSpace(address)) > 254 || !validEmail(strings.TrimSpace(address)) {
			add("enter a valid reminder email address.")
			break
		}
	}
	if enabled == nil && len(addresses) > 0 {
		add("choose whether document email reminders are enabled.")
	}
	if enabled != nil && *enabled {
		if !applicable {
			add("email reminders require an applicable document with expiry enabled.")
		}
		if len(normalizeReminderEmails(addresses)) == 0 {
			add("add at least one reminder email address.")
		}
	}
	return issues
}

func normalizeDocumentReminderEmails(p *CompanyPrivate) {
	for i := range p.Fields {
		p.Fields[i].ReminderEmails = normalizeReminderEmails(p.Fields[i].ReminderEmails)
	}
	for i := range p.Ports {
		p.Ports[i].ReminderEmails = normalizeReminderEmails(p.Ports[i].ReminderEmails)
	}
	for i := range p.TradeRegistrations {
		p.TradeRegistrations[i].ReminderEmails = normalizeReminderEmails(p.TradeRegistrations[i].ReminderEmails)
	}
}

func legacyCompanyReminderRecipients(ctx context.Context, q queryer, cid int64, p CompanyPrivate) ([]string, error) {
	recipients := companyRecipients(p)
	var override, defaults []string
	err := q.QueryRow(ctx, `SELECT recipients FROM company_notification_settings WHERE company_id=$1`, cid).Scan(&override)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return nil, err
	}
	if len(override) > 0 {
		recipients = override
	}
	if err = q.QueryRow(ctx, `SELECT recipients FROM notification_settings WHERE id=1`).Scan(&defaults); err != nil {
		return nil, err
	}
	return normalizeReminderEmails(append(recipients, defaults...)), nil
}

func companyReminderRecipients(ctx context.Context, q queryer, cid int64, p CompanyPrivate, key string) ([]string, bool, error) {
	selection := documentReminderSelection(p, key)
	if !selection.enabled {
		return []string{}, false, nil
	}
	if selection.explicit {
		return selection.recipients, true, nil
	}
	recipients, err := legacyCompanyReminderRecipients(ctx, q, cid, p)
	return recipients, true, err
}

func removedCompanyDocumentLinks(before, after CompanyPrivate) int {
	links := func(p CompanyPrivate) map[string]bool {
		out := map[string]bool{}
		add := func(key string, ids []int64) {
			for _, id := range ids {
				out[fmt.Sprintf("%s:%d", key, id)] = true
			}
		}
		for _, f := range p.Fields {
			add(fmt.Sprintf("field:%d", f.ID), f.Documents)
		}
		for _, b := range p.Banks {
			add("bank:"+b.ID, b.Documents)
		}
		for _, p := range p.Ports {
			add(fmt.Sprintf("port:%d", p.ID), p.Documents)
		}
		for _, t := range p.TradeRegistrations {
			add("trade:"+t.ID, t.Documents)
		}
		return out
	}
	oldLinks, newLinks := links(before), links(after)
	removed := 0
	for key := range oldLinks {
		if !newLinks[key] {
			removed++
		}
	}
	return removed
}
