package app

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/png"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

func reminderBool(v bool) *bool { return &v }

type reminderConfigRow struct {
	addresses []string
	err       error
}

func (r reminderConfigRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	*dest[0].(*[]string) = r.addresses
	return nil
}

type reminderConfigQuery struct {
	rows  []reminderConfigRow
	calls int
}

func (q *reminderConfigQuery) QueryRow(_ context.Context, _ string, _ ...any) pgx.Row {
	r := q.rows[q.calls]
	q.calls++
	return r
}
func (*reminderConfigQuery) Query(context.Context, string, ...any) (pgx.Rows, error) {
	panic("unexpected query")
}
func (*reminderConfigQuery) Exec(context.Context, string, ...any) (pgconn.CommandTag, error) {
	panic("unexpected mutation")
}

func TestCompanyDocumentReminderRecipientIsolation(t *testing.T) {
	p := CompanyPrivate{ReminderEmail: "company@example.test", ReminderEmails: []string{"other@example.test"}, Fields: []CompanyField{
		{ID: 1, Expiry: true, ReminderEnabled: reminderBool(true), ReminderEmails: []string{" GST@Example.test ", "gst@example.test", "review@example.test"}},
		{ID: 2, Expiry: true, ReminderEnabled: reminderBool(false), ReminderEmails: []string{"ignored@example.test"}},
		{ID: 3, Expiry: true},
		{ID: 4, Expiry: true, NotApplicable: true, ReminderEnabled: reminderBool(true)},
	}, Ports: []CompanyPort{{ID: 8, Registered: true, Expiry: true, ReminderEnabled: reminderBool(true), ReminderEmails: []string{"port@example.test"}}}, TradeRegistrations: []CompanyTrade{{ID: "dgft", Expiry: true, ReminderEnabled: reminderBool(true), ReminderEmails: []string{"trade@example.test"}}}}
	for _, tc := range []struct {
		key     string
		want    []string
		enabled bool
	}{
		{"field:1", []string{"gst@example.test", "review@example.test"}, true},
		{"field:2", []string{}, false}, {"field:4", []string{}, false}, {"field:999", []string{}, false},
		{"port:8", []string{"port@example.test"}, true}, {"trade:dgft", []string{"trade@example.test"}, true},
	} {
		// A nil queryer proves explicit settings cannot read global recipients.
		got, enabled, err := companyReminderRecipients(context.Background(), nil, 1, p, tc.key)
		if err != nil || enabled != tc.enabled || !reflect.DeepEqual(got, tc.want) {
			t.Fatalf("%s got %v enabled %v err %v", tc.key, got, enabled, err)
		}
	}
	q := &reminderConfigQuery{rows: []reminderConfigRow{{addresses: []string{"override@example.test"}}, {addresses: []string{"DEFAULT@example.test", "override@example.test"}}}}
	got, enabled, err := companyReminderRecipients(context.Background(), q, 1, p, "field:3")
	if err != nil || !enabled || q.calls != 2 || !reflect.DeepEqual(got, []string{"override@example.test", "default@example.test"}) {
		t.Fatalf("legacy override/default delivery changed: %v, %v", got, err)
	}
	q = &reminderConfigQuery{rows: []reminderConfigRow{{err: pgx.ErrNoRows}, {addresses: []string{"default@example.test"}}}}
	got, _, err = companyReminderRecipients(context.Background(), q, 1, p, "field:3")
	if err != nil || !reflect.DeepEqual(got, []string{"company@example.test", "other@example.test", "default@example.test"}) {
		t.Fatalf("legacy profile/default delivery changed: %v, %v", got, err)
	}
	q = &reminderConfigQuery{rows: []reminderConfigRow{{err: errors.New("query failed")}}}
	got, _, err = companyReminderRecipients(context.Background(), q, 1, p, "field:3")
	if err == nil || got != nil {
		t.Fatal("recipient configuration failure did not fail closed")
	}
}

func TestCompanyDocumentReminderValidation(t *testing.T) {
	many := make([]string, 21)
	for i := range many {
		many[i] = "valid@example.test"
	}
	for _, tc := range []struct {
		name       string
		flag       *bool
		emails     []string
		applicable bool
		valid      bool
	}{
		{"legacy", nil, nil, true, true}, {"disabled", reminderBool(false), nil, false, true},
		{"enabled", reminderBool(true), []string{" One@Example.test ", "one@example.test"}, true, true},
		{"empty", reminderBool(true), nil, true, false}, {"bad address", reminderBool(true), []string{"not-email"}, true, false},
		{"header injection", reminderBool(true), []string{"x@example.test\r\nBcc: z@example.test"}, true, false},
		{"many", reminderBool(true), many, true, false}, {"inapplicable", reminderBool(true), []string{"one@example.test"}, false, false},
		{"no explicit choice", nil, []string{"one@example.test"}, true, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := len(validateDocumentReminder(tc.flag, tc.emails, tc.applicable, "GST")) == 0; got != tc.valid {
				t.Fatalf("valid=%v want=%v", got, tc.valid)
			}
		})
	}
}

func TestCompanyDocumentPreviewAndPermissions(t *testing.T) {
	var imageBytes bytes.Buffer
	if err := png.Encode(&imageBytes, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name                     string
		data                     []byte
		preview                  bool
		disposition, contentType string
	}{
		{"proof.pdf", []byte("%PDF-1.4\nprivate"), true, "inline", "application/pdf"},
		{"proof.pdf", []byte("%PDF-1.4\nprivate"), false, "attachment", "application/octet-stream"},
		{"proof.pdf", []byte("<html>unsafe</html>"), true, "attachment", "application/octet-stream"},
		{"proof.png", imageBytes.Bytes(), true, "inline", "image/png"},
		{"proof.jpg", imageBytes.Bytes(), true, "attachment", "application/octet-stream"},
		{"proof.docx", []byte("not an office file"), true, "attachment", "application/octet-stream"},
		{"proof.svg", []byte("<svg/>"), true, "attachment", "application/octet-stream"},
	} {
		d, c := companyDocumentPresentation(tc.name, tc.data, tc.preview)
		if d != tc.disposition || c != tc.contentType {
			t.Fatalf("%s %s %s", tc.name, d, c)
		}
	}
	for _, u := range []*User{{Role: "Merchant"}, {Role: "Finance Head", SetupCustomAccess: true, SetupFeatures: []string{"view"}}} {
		r := httptest.NewRequest("GET", "/api/company-files/1?preview=1", nil)
		r = r.WithContext(context.WithValue(r.Context(), authKey{}, u))
		if _, err := (&App{}).companySetupDownload(httptest.NewRecorder(), r); err != forbidden() {
			t.Fatalf("preview bypassed document access: %v", err)
		}
	}
}

func TestRemovedCompanyDocumentLinksAudit(t *testing.T) {
	before := CompanyPrivate{Fields: []CompanyField{{ID: 1, Documents: []int64{8, 9}}}, Banks: []CompanyBank{{ID: "b", Documents: []int64{8}}}}
	after := CompanyPrivate{Fields: []CompanyField{{ID: 1, Documents: []int64{9}}}, Banks: []CompanyBank{{ID: "b", Documents: []int64{8}}}}
	if removedCompanyDocumentLinks(before, after) != 1 {
		t.Fatal("removing one field link must not count bank link or retain removed link")
	}
	if strings.Contains(strings.Join(validateDocumentReminder(reminderBool(true), nil, true, "GST"), " "), "@") {
		t.Fatal("validation must not echo addresses")
	}
}
