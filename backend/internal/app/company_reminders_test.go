package app

import "testing"

func TestCompanyReminderCalendarBoundaries(t *testing.T) {
	for expiry, want := range map[string]string{
		"2026-04-30": "2026-02-28", "2028-04-30": "2028-02-29",
		"2026-01-31": "2025-11-30", "2026-08-31": "2026-06-30",
		"2026-10-16": "2026-08-16",
	} {
		d, err := date(expiry)
		if err != nil {
			t.Fatal(err)
		}
		if got := companyReminderDate(d).Format("2006-01-02"); got != want {
			t.Errorf("expiry %s: got %s, want %s", expiry, got, want)
		}
	}
}
