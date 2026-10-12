package app

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"github.com/go-chi/chi/v5"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
)

type CompanyField struct {
	ID              int64    `json:"id"`
	Value           string   `json:"value"`
	NotApplicable   bool     `json:"not_applicable"`
	Reason          string   `json:"reason"`
	Expiry          bool     `json:"expiry"`
	ValidFrom       string   `json:"valid_from"`
	ExpiresOn       string   `json:"expires_on"`
	Documents       []int64  `json:"documents"`
	ReminderEnabled *bool    `json:"reminder_enabled"`
	ReminderEmails  []string `json:"reminder_emails"`
}
type CompanyContact struct {
	Website    string `json:"website"`
	Skype      string `json:"skype"`
	Speciality string `json:"speciality"`
	Role       string `json:"role"`
	Name       string `json:"name"`
	Email      string `json:"email"`
	Phone      string `json:"phone"`
}
type CompanyBank struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Holder      string  `json:"holder"`
	Account     string  `json:"account"`
	AccountType string  `json:"account_type"`
	Routing     string  `json:"routing"`
	Currency    string  `json:"currency"`
	Documents   []int64 `json:"documents"`
}
type CompanyPort struct {
	ID              int64    `json:"id"`
	Default         bool     `json:"default"`
	Registered      bool     `json:"registered"`
	Registration    string   `json:"registration"`
	BankID          string   `json:"bank_id"`
	Expiry          bool     `json:"expiry"`
	ValidFrom       string   `json:"valid_from"`
	ExpiresOn       string   `json:"expires_on"`
	Documents       []int64  `json:"documents"`
	ReminderEnabled *bool    `json:"reminder_enabled"`
	ReminderEmails  []string `json:"reminder_emails"`
}
type CompanyTrade struct {
	ID              string   `json:"id"`
	Portal          string   `json:"portal"`
	Registration    string   `json:"registration"`
	Licence         string   `json:"licence"`
	BankID          string   `json:"bank_id"`
	PortIDs         []int64  `json:"port_ids"`
	Expiry          bool     `json:"expiry"`
	ValidFrom       string   `json:"valid_from"`
	ExpiresOn       string   `json:"expires_on"`
	Documents       []int64  `json:"documents"`
	ReminderEnabled *bool    `json:"reminder_enabled"`
	ReminderEmails  []string `json:"reminder_emails"`
}
type CompanyPrivate struct {
	SupplierCode       string         `json:"supplier_code"`
	Fax                string         `json:"fax"`
	DialCode           string         `json:"dial_code"`
	ShortCode          string         `json:"short_code"`
	TradeRegistrations []CompanyTrade `json:"trade_registrations"`

	Address          string           `json:"address"`
	State            string           `json:"state"`
	PostalCode       string           `json:"postal_code"`
	Email            string           `json:"email"`
	Phone            string           `json:"phone"`
	Website          string           `json:"website"`
	Owner            string           `json:"owner"`
	SampleAddress    string           `json:"sample_address"`
	Brands           string           `json:"brands"`
	Departments      []string         `json:"departments"`
	Ownership        string           `json:"ownership"`
	Relationship     string           `json:"relationship"`
	NominatedBy      string           `json:"nominated_by"`
	SupplierServices string           `json:"supplier_services"`
	ReminderEmails   []string         `json:"reminder_emails"`
	SupplierGroup    string           `json:"supplier_group"`
	ReminderEmail    string           `json:"reminder_email"`
	Notes            string           `json:"notes"`
	Contacts         []CompanyContact `json:"contacts"`
	Banks            []CompanyBank    `json:"banks"`
	Ports            []CompanyPort    `json:"ports"`
	Fields           []CompanyField   `json:"fields"`
}

var companyTypes = map[string][]string{"Internal": {"Self", "Branch", "JV support company"}, "Buyer": {"Retailer", "Brand owner", "Online", "Uniform supplier", "Wholesaler"}, "Supplier": {"Raw materials", "Finished goods", "Fabrics", "Trims", "Office consumables", "Machines and parts", "Production consumables", "Job workers", "Service providers", "Housekeeping", "Refreshments", "Travel", "Export logistics", "Cargo transport", "Import logistics", "CHA import/export"}}

// Legacy scalar values remain available for older clients and field-definition filters.
var companyBusinessModes = []string{"Not specified", "Domestic", "Import", "Export", "Import & export"}
var companyBusinessFlowChoices = []string{"Domestic", "Import", "Export"}

func businessMode(d M) (string, error) {
	mode := "Not specified"
	if value, exists := d["business_mode"]; exists {
		var ok bool
		mode, ok = value.(string)
		if !ok {
			return "", bad("Choose a valid business flow.")
		}
		mode = strings.TrimSpace(mode)
		if mode == "" {
			mode = "Not specified"
		}
	}
	if !slices.Contains(companyBusinessModes, mode) {
		return "", bad("Choose a valid business flow.")
	}
	return mode, nil
}

func legacyBusinessFlows(mode string) []string {
	switch mode {
	case "Import & export":
		return []string{"Import", "Export"}
	case "Domestic", "Import", "Export":
		return []string{mode}
	default:
		return []string{}
	}
}

// Validate shape as well as values. Null and scalar JSON must never silently clear selections.
func parseBusinessFlows(value any) ([]string, error) {
	var values []any
	switch v := value.(type) {
	case []any:
		values = v
	case []string:
		for _, item := range v {
			values = append(values, item)
		}
	default:
		return nil, bad("Business flows must be a list of Domestic, Import or Export.")
	}
	selected := map[string]bool{}
	for _, value := range values {
		flow, ok := value.(string)
		if !ok || !slices.Contains(companyBusinessFlowChoices, flow) {
			return nil, bad("Choose valid business flows: Domestic, Import or Export.")
		}
		selected[flow] = true
	}
	flows := []string{}
	for _, flow := range companyBusinessFlowChoices {
		if selected[flow] {
			flows = append(flows, flow)
		}
	}
	return flows, nil
}

func companyBusinessFlows(c M) []string {
	if value, exists := c["business_flows"]; exists {
		if flows, err := parseBusinessFlows(value); err == nil {
			return flows
		}
	}
	return legacyBusinessFlows(str(c, "business_mode"))
}

// The array is authoritative. A legacy client echoing the current scalar must not
// discard other selections; omission also preserves them during ordinary edits.
func businessFlowsInput(d, current M) ([]string, error) {
	mode, err := businessMode(d)
	if err != nil {
		return nil, err
	}
	if value, exists := d["business_flows"]; exists {
		return parseBusinessFlows(value)
	}
	if current != nil {
		if _, exists := d["business_mode"]; !exists || mode == str(current, "business_mode") {
			return companyBusinessFlows(current), nil
		}
	}
	return legacyBusinessFlows(mode), nil
}

// Prefer the trade direction for the compatibility scalar. Full selections are
// returned separately and used for all business rules and human-readable exports.
func businessModeForFlows(flows []string) string {
	if slices.Contains(flows, "Import") && slices.Contains(flows, "Export") {
		return "Import & export"
	}
	for _, flow := range []string{"Import", "Export", "Domestic"} {
		if slices.Contains(flows, flow) {
			return flow
		}
	}
	return "Not specified"
}

func companyBusinessFlowFilters(c M) []string {
	flows := companyBusinessFlows(c)
	if len(flows) == 0 {
		return []string{"Not specified"}
	}
	if slices.Contains(flows, "Import") && slices.Contains(flows, "Export") {
		flows = append(flows, "Import & export")
	}
	return flows
}

func companyBusinessFlowLabel(c M) string {
	flows := companyBusinessFlows(c)
	if len(flows) == 0 {
		return "Not specified"
	}
	return strings.Join(flows, ", ")
}

func companyApplicableFields(ctx context.Context, q queryer, c M) ([]M, error) {
	return rows(ctx, q, `SELECT * FROM company_field_definitions WHERE active AND (company_setup_id=$4 OR (company_setup_id IS NULL AND (country='*' OR country=$1) AND $2=ANY(categories) AND (cardinality(business_modes)=0 OR business_modes && $3::text[]))) ORDER BY id`, str(c, "country"), str(c, "category"), companyBusinessFlowFilters(c), id(c, "id"))
}
func setupRole(u *User) bool {
	return u.SetupCan("view")
}
func setupPrivate(u *User, r M) bool {
	return u.Role == "Super Admin" || u.Role == "Finance Head" && id(r, "assigned_to") == u.ID && slices.Contains([]string{"Draft", "Changes requested"}, str(r, "status"))
}
func setupEditable(u *User, r M) bool {
	return u.SetupCan("edit") && u.Role == "Finance Head" && id(r, "assigned_to") == u.ID && slices.Contains([]string{"Draft", "Changes requested"}, str(r, "status"))
}
func setupID(r *http.Request) int64 {
	n, _ := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	return n
}
func companyCipher() (cipher.AEAD, error) {
	key, e := base64.StdEncoding.DecodeString(os.Getenv("KYC_ENCRYPTION_KEY"))
	if e != nil || len(key) != 32 {
		return nil, errors.New("KYC_ENCRYPTION_KEY must contain a base64 encoded 32-byte key")
	}
	block, e := aes.NewCipher(key)
	if e != nil {
		return nil, e
	}
	return cipher.NewGCM(block)
}
func sealCompany(p CompanyPrivate) ([]byte, error) {
	a, e := companyCipher()
	if e != nil {
		return nil, e
	}
	b, e := json.Marshal(p)
	if e != nil {
		return nil, e
	}
	nonce := make([]byte, a.NonceSize())
	if _, e = rand.Read(nonce); e != nil {
		return nil, e
	}
	return a.Seal(nonce, nonce, b, []byte("company-kyc-v1")), nil
}
func openCompany(r M) (CompanyPrivate, error) {
	p := CompanyPrivate{}
	a, e := companyCipher()
	if e != nil {
		return p, e
	}
	b, ok := r["private_data"].([]byte)
	if !ok || len(b) < a.NonceSize() {
		return p, errors.New("invalid encrypted company profile")
	}
	b, e = a.Open(nil, b[:a.NonceSize()], b[a.NonceSize():], []byte("company-kyc-v1"))
	if e != nil {
		return p, e
	}
	e = json.Unmarshal(b, &p)
	return p, e
}
func (a *App) setupRow(ctx context.Context, q queryer, uid int64, lock bool) (M, error) {
	sql := `SELECT *,created_at::text AS created_at,updated_at::text AS updated_at FROM company_setups WHERE id=$1 AND deleted_at IS NULL`
	if lock {
		sql += ` FOR UPDATE`
	}
	return row(ctx, q, sql, uid)
}
func setupHistory(ctx context.Context, q queryer, u *User, c M, action, note string) error {
	_, e := q.Exec(ctx, `INSERT INTO company_setup_history(company_id,actor_id,action,note,revision) VALUES($1,$2,$3,$4,$5)`, id(c, "id"), u.ID, action, note, id(c, "revision"))
	return e
}
func (a *App) companySetupList(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if !setupRole(u) {
		return nil, forbidden()
	}
	ctx := r.Context()
	list, e := rows(ctx, a.DB, `SELECT id,legal_name,category,company_type,country,city,business_mode,business_flows,status,revision,version,created_by,assigned_to,responsible_user_id,created_at,updated_at FROM company_setups WHERE deleted_at IS NULL AND ($1<>'Finance Head' OR assigned_to=$2) ORDER BY updated_at DESC`, u.Role, u.ID)
	if e != nil {
		return nil, e
	}
	fields, e := rows(ctx, a.DB, `SELECT * FROM company_field_definitions WHERE active ORDER BY id`)
	if e != nil {
		return nil, e
	}
	ports, e := rows(ctx, a.DB, `SELECT * FROM company_ports WHERE active ORDER BY name`)
	if e != nil {
		return nil, e
	}
	reminders, e := rows(ctx, a.DB, `SELECT cr.id,cr.company_id,c.legal_name,cr.label,cr.expires_on::text,cr.remind_on::text,cr.email_status FROM company_reminders cr JOIN company_setups c ON c.id=cr.company_id WHERE c.deleted_at IS NULL AND cr.active AND cr.remind_on<=CURRENT_DATE AND ($1<>'Finance Head' OR c.assigned_to=$2) ORDER BY cr.expires_on`, u.Role, u.ID)
	if e != nil {
		return nil, e
	}
	if !u.SetupCan("reminders") {
		reminders = []M{}
	}
	types, e := a.setupTypes(ctx)
	if e != nil {
		return nil, e
	}
	return M{"user": u, "companies": list, "fields": fields, "ports": ports, "types": types, "reminders": reminders, "email_configured": emailConfigured(), "local": !a.Production}, nil
}
func (a *App) companyHeader(ctx context.Context, d M) (string, string, string, string, string, error) {
	name, e := text(d, "legal_name", 150)
	if e != nil {
		return "", "", "", "", "", e
	}
	category, typ, country, city := str(d, "category"), str(d, "company_type"), strings.ToUpper(str(d, "country")), str(d, "city")
	types, err := a.setupTypes(ctx)
	if err != nil {
		return "", "", "", "", "", err
	}
	if !slices.Contains(types[category], typ) || !regexp.MustCompile(`^[A-Z]{2}$`).MatchString(country) || len(city) > 100 {
		return "", "", "", "", "", bad("Choose a company category, type and two-letter country code.")
	}
	return name, category, typ, country, city, nil
}
func (a *App) companySetupCreate(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if u.Role != "Finance Head" || !u.SetupCan("create") {
		return nil, forbidden()
	}
	d, e := readBody(w, r)
	if e != nil {
		return nil, e
	}
	if e = allowedFields(d, "legal_name", "category", "company_type", "country", "city", "business_mode", "business_flows", "address", "state", "postal_code"); e != nil {
		return nil, e
	}
	name, cat, typ, country, city, e := a.companyHeader(r.Context(), d)
	if e != nil {
		return nil, e
	}
	flows, e := businessFlowsInput(d, nil)
	if e != nil {
		return nil, e
	}
	profile, e := companyCreateAddress(d)
	if e != nil {
		return nil, e
	}
	enc, e := sealCompany(profile)
	if e != nil {
		return nil, e
	}
	ctx := r.Context()
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	var cid int64
	e = tx.QueryRow(ctx, `INSERT INTO company_setups(legal_name,category,company_type,country,city,private_data,created_by,assigned_to,responsible_user_id,business_mode,business_flows) VALUES($1,$2,$3,$4,$5,$6,$7,$7,$7,$8,$9) RETURNING id`, name, cat, typ, country, city, enc, u.ID, businessModeForFlows(flows), flows).Scan(&cid)
	if e != nil {
		return nil, e
	}
	if e = setupHistory(ctx, tx, u, M{"id": cid, "revision": int64(1)}, "Created", "Company draft created"); e != nil {
		return nil, e
	}
	return M{"id": cid}, tx.Commit(ctx)
}
func (a *App) companySetupDetail(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if !setupRole(u) {
		return nil, forbidden()
	}
	ctx := r.Context()
	c, e := a.setupRow(ctx, a.DB, setupID(r), false)
	if e != nil {
		return nil, e
	}
	if u.Role == "Finance Head" && id(c, "assigned_to") != u.ID {
		return nil, missing()
	}
	p, e := openCompany(c)
	if e != nil {
		return nil, e
	}
	delete(c, "private_data")
	c["can_edit"] = setupEditable(u, c)
	c["can_view_private"] = setupPrivate(u, c)
	docs, e := rows(ctx, a.DB, `SELECT id,name,size,sha256,created_at,storage_key FROM company_setup_files WHERE company_id=$1 ORDER BY id`, id(c, "id"))
	if e != nil {
		return nil, e
	}
	validation, e := a.validateCompany(ctx, a.DB, c, p, true)
	if e != nil {
		return nil, e
	}
	// Summaries disclose completeness and expiry, never identifier values or attachments.
	checklist := []M{}
	definitions, e := companyApplicableFields(ctx, a.DB, c)
	if e != nil {
		return nil, e
	}
	for _, def := range definitions {
		state := "Not entered"
		count := 0
		expiry := ""
		for _, f := range p.Fields {
			if f.ID == id(def, "id") {
				if f.NotApplicable {
					state = "Not applicable"
				} else if f.Value != "" {
					state = "Provided"
				}
				count = len(f.Documents)
				if f.Expiry {
					expiry = f.ExpiresOn
				}
			}
		}
		checklist = append(checklist, M{"label": str(def, "label"), "state": state, "documents": count, "expires_on": expiry})
	}
	c["summary"] = M{"checklist": checklist, "contacts": len(p.Contacts), "bank_accounts": len(p.Banks), "ports": len(p.Ports), "documents": len(docs), "fields": len(p.Fields), "ready": len(validation) == 0, "issues": validation}
	if setupPrivate(u, c) {
		c["profile"] = p
		for _, doc := range docs {
			a.addCompanyStorageMetadata(doc)
		}
		c["files"] = docs
		// Notification tables are service-scoped. Expose only the effective list
		// for this already-authorized private company, never the settings records.
		reminderContext := context.WithValue(ctx, serviceKey{}, "reminders")
		legacyRecipients, err := legacyCompanyReminderRecipients(reminderContext, a.DB, id(c, "id"), p)
		if err != nil {
			return nil, err
		}
		c["legacy_reminder_emails"] = legacyRecipients
	}
	hist, e := rows(ctx, a.DB, `SELECT h.id,h.action,h.note,h.revision,h.created_at,u.role AS actor_role FROM company_setup_history h JOIN users u ON u.id=h.actor_id WHERE company_id=$1 ORDER BY h.id DESC`, id(c, "id"))
	if e != nil {
		return nil, e
	}
	c["history"] = hist
	return c, nil
}
func validEmail(s string) bool {
	v, e := mail.ParseAddress(s)
	return e == nil && v.Address == s && !strings.ContainsAny(s, "\r\n")
}
func validURL(s string) bool {
	u, e := url.Parse(s)
	return e == nil && (u.Scheme == "https" || u.Scheme == "http") && u.Hostname() != "" && u.User == nil
}
func validity(from, to string, enabled bool, required bool) string {
	if !enabled {
		return ""
	}
	if to == "" {
		if required {
			return "Expiry date is required."
		}
		return ""
	}
	end, e := date(to)
	if e != nil {
		return "Use a valid expiry date."
	}
	if from != "" {
		start, e := date(from)
		if e != nil || start.After(end) {
			return "Validity start must not be later than expiry."
		}
	}
	return ""
}
func (a *App) validateCompany(ctx context.Context, q queryer, c M, p CompanyPrivate, complete bool) ([]string, error) {
	issues := []string{}
	add := func(ok bool, msg string) {
		if !ok {
			issues = append(issues, msg)
		}
	}
	if complete {
		add(p.Address != "", "Registered address is missing.")
		add(str(c, "city") != "", "City is missing.")
		if str(c, "category") == "Internal" {
			add(p.Owner != "", "Owner / legal representative is missing.")
		}
		add(p.Email != "", "Company email is missing.")
		add(p.Phone != "", "Company contact number is missing.")
	}
	add(len(p.ReminderEmails) <= 20, "Use at most 20 reminder recipients.")
	for _, s := range append([]string{p.Email, p.ReminderEmail}, p.ReminderEmails...) {
		if s != "" {
			add(validEmail(s), "A company or reminder email address is invalid.")
		}
	}
	if p.Website != "" {
		add(validURL(p.Website), "Website must be an http or https URL.")
	}
	add(len(p.TradeRegistrations) <= 30, "Too many trade registrations.")
	add(len(p.Fields) <= 150 && len(p.Banks) <= 30 && len(p.Contacts) <= 30 && len(p.Ports) <= 30, "Too many detail rows.")
	docs, e := rows(ctx, q, `SELECT id FROM company_setup_files WHERE company_id=$1`, id(c, "id"))
	if e != nil {
		return nil, e
	}
	docIDs := []int64{}
	for _, d := range docs {
		docIDs = append(docIDs, id(d, "id"))
	}
	validateDocs := func(ids []int64, label string) {
		for _, fid := range ids {
			add(slices.Contains(docIDs, fid), label+": attachment does not belong to this company.")
		}
	}
	defs, e := companyApplicableFields(ctx, q, c)
	if e != nil {
		return nil, e
	}
	byID := map[int64]M{}
	for _, d := range defs {
		byID[id(d, "id")] = d
	}
	seen := map[int64]bool{}
	for _, f := range p.Fields {
		def, ok := byID[f.ID]
		add(ok, "A KYC field does not apply to this country / category.")
		if !ok {
			continue
		}
		label := str(def, "label")
		add(!seen[f.ID], "Duplicate field: "+label)
		seen[f.ID] = true
		add(len(f.Value) <= 2000 && len(f.Reason) <= 500, "Field value is too long: "+label)
		issues = append(issues, validateDocumentReminder(f.ReminderEnabled, f.ReminderEmails, f.Expiry && !f.NotApplicable, label)...)
		if f.NotApplicable {
			add(f.Value == "" && len(f.Documents) == 0 && !f.Expiry, "Not-applicable fields cannot contain values, documents or validity dates.")
			if complete {
				add(strings.TrimSpace(f.Reason) != "", "Explain why "+label+" is not applicable.")
			}
			continue
		}
		if complete && (def["required"] == true || str(def, "section") == "KYC") {
			add(strings.TrimSpace(f.Value) != "", label+" is required.")
		}
		if f.Value != "" {
			switch str(def, "input_type") {
			case "email":
				add(validEmail(f.Value), label+": enter a valid email.")
			case "checkbox":
				add(f.Value == "true" || f.Value == "false", label+": choose Yes or No.")
			case "select":
				options := []string{}
				raw, _ := json.Marshal(def["options"])
				_ = json.Unmarshal(raw, &options)
				add(slices.Contains(options, f.Value), label+": choose one of the configured options.")
			}
			if str(def, "input_type") == "url" {
				add(validURL(f.Value), label+": enter a valid URL.")
			}
			if str(def, "input_type") == "date" {
				_, e := date(f.Value)
				add(e == nil, label+": enter a valid date.")
			}
			if str(def, "input_type") == "number" {
				add(regexp.MustCompile(`^[0-9]+(?:\.[0-9]+)?$`).MatchString(f.Value), label+": enter a positive number.")
			}
			if complete && def["document_required"] == true {
				add(len(f.Documents) > 0, label+": supporting document is missing.")
			}
		}
		if msg := validity(f.ValidFrom, f.ExpiresOn, f.Expiry, complete && (f.Value != "" || len(f.Documents) > 0 || def["required"] == true)); msg != "" {
			issues = append(issues, label+": "+msg)
		}
		validateDocs(f.Documents, label)
	}
	if complete {
		for _, d := range defs {
			if d["required"] == true || str(d, "section") == "KYC" {
				add(seen[id(d, "id")], str(d, "label")+" is required.")
			}
		}
	}
	add(len(p.SupplierCode) <= 50 && len(p.Fax) <= 80 && len(p.DialCode) <= 20, "Supplier code, fax or dial code is too long.")
	for _, co := range p.Contacts {
		add(len(co.Skype) <= 120 && len(co.Speciality) <= 500, "Contact Skype or speciality is too long.")
		if co.Website != "" {
			add(validURL(co.Website), "Contact website must be an http or https URL.")
		}
		add(len(co.Role) <= 100 && len(co.Name) <= 120 && len(co.Phone) <= 80, "Contact details are too long.")
		if co.Email != "" {
			add(validEmail(co.Email), "A contact email is invalid.")
		}
		if complete {
			add(co.Name != "" && co.Role != "", "Each contact needs a name and role.")
		}
	}
	banks := map[string]CompanyBank{}
	for _, b := range p.Banks {
		_, duplicate := banks[b.ID]
		add(b.ID != "" && !duplicate, "Each bank account needs a unique reference.")
		banks[b.ID] = b
		add(slices.Contains([]string{"", "Current", "Savings", "Cash credit", "Overdraft", "Other"}, b.AccountType), "Choose a valid bank account type.")
		if complete {
			add(b.Name != "" && b.Holder != "" && b.Account != "", "Bank name, account holder and account number are required.")
		}
		validateDocs(b.Documents, "Bank account")
	}
	ports, e := rows(ctx, q, `SELECT id,mode FROM company_ports WHERE active`)
	if e != nil {
		return nil, e
	}
	portModes := map[int64]string{}
	for _, po := range ports {
		portModes[id(po, "id")] = str(po, "mode")
	}
	tradeIDs := map[string]bool{}
	for _, t := range p.TradeRegistrations {
		add(t.ID != "" && !tradeIDs[t.ID], "Each trade registration needs a unique reference.")
		tradeIDs[t.ID] = true
		add(slices.Contains([]string{"DGFT", "ICEGATE", "Other"}, t.Portal), "Choose a trade registration portal.")
		add(len(t.Registration) <= 200 && len(t.Licence) <= 200, "Trade registration details are too long.")
		if complete {
			add(t.Registration != "" || t.Licence != "", "Enter a trade registration or licence reference.")
			add(len(t.Documents) > 0, "Trade registration supporting document is missing.")
		}
		if t.BankID != "" {
			_, exists := banks[t.BankID]
			add(exists, "Trade registration bank account is invalid.")
		}
		seenPorts := map[int64]bool{}
		for _, pid := range t.PortIDs {
			_, exists := portModes[pid]
			add(exists && !seenPorts[pid], "Choose valid unique trade registration ports.")
			seenPorts[pid] = true
		}
		issues = append(issues, validateDocumentReminder(t.ReminderEnabled, t.ReminderEmails, t.Expiry, "Trade registration")...)
		validateDocs(t.Documents, "Trade registration")
		if msg := validity(t.ValidFrom, t.ExpiresOn, t.Expiry, complete); msg != "" {
			issues = append(issues, "Trade registration: "+msg)
		}
	}
	defaults := map[string]int{}
	portSeen := map[int64]bool{}
	for _, po := range p.Ports {
		mode, ok := portModes[po.ID]
		add(ok && !portSeen[po.ID], "Choose a valid, unique port.")
		portSeen[po.ID] = true
		if mode == "ICD" {
			mode = "Sea"
		}
		if po.Default {
			defaults[mode]++
		}
		issues = append(issues, validateDocumentReminder(po.ReminderEnabled, po.ReminderEmails, po.Registered && po.Expiry, "Port registration")...)
		validateDocs(po.Documents, "Port registration")
		if msg := validity(po.ValidFrom, po.ExpiresOn, po.Expiry, complete); msg != "" {
			issues = append(issues, "Port registration: "+msg)
		}
		if po.Registered && complete {
			bank, exists := banks[po.BankID]
			add(po.Registration != "", "Port registration number is missing.")
			add(len(po.Documents) > 0, "Port registration proof is missing.")
			add(exists && len(bank.Documents) > 0, "Registered port must link to a bank account with supporting proof.")
			if po.Expiry && po.ExpiresOn != "" {
				add(po.ExpiresOn >= time.Now().Format("2006-01-02"), "A port registration has expired.")
			}
		}
	}
	add(defaults["Sea"] <= 1 && defaults["Air"] <= 1, "Choose only one default sea/ICD port and one default airport.")
	if complete && str(c, "category") == "Internal" {
		add(defaults["Sea"] == 1 && defaults["Air"] == 1, "Select a default sea/ICD port and airport.")
	}
	if str(c, "category") == "Buyer" && complete {
		add(p.SampleAddress != "", "Sample delivery address is missing.")
	}
	if p.SupplierGroup != "" {
		add(slices.Contains([]string{"Raw materials", "Finished goods", "Consumables", "Machinery", "Services", "Logistics"}, p.SupplierGroup), "Choose a valid supplier group.")
	}
	if str(c, "category") == "Supplier" {
		if complete {
			add(slices.Contains([]string{"Nominated", "Direct", "Non-nominated"}, p.Relationship), "Choose the supplier relationship.")
		}
		if p.Relationship == "Nominated" && complete {
			add(p.NominatedBy != "", "Nominating buyer reference is missing.")
		}
	}
	return issues, nil
}
func (a *App) companySetupAction(w http.ResponseWriter, r *http.Request) (any, error) {
	u := current(r)
	if !setupRole(u) {
		return nil, forbidden()
	}
	d, e := readBody(w, r)
	if e != nil {
		return nil, e
	}
	if e = allowedFields(d, "action", "version", "note", "legal_name", "category", "company_type", "country", "city", "business_mode", "business_flows", "profile"); e != nil {
		return nil, e
	}
	ctx := r.Context()
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	c, e := a.setupRow(ctx, tx, setupID(r), true)
	if e != nil {
		return nil, e
	}
	if u.Role == "Finance Head" && id(c, "assigned_to") != u.ID {
		return nil, missing()
	}
	if id(d, "version") != id(c, "version") {
		return nil, failure{409, "This company changed. Reload before saving."}
	}
	action := str(d, "action")
	requiredFeature := map[string]string{"save": "edit", "submit": "submit", "approve": "approve", "request_changes": "approve", "amend": "amend"}[action]
	if !u.SetupCan(requiredFeature) {
		return nil, forbidden()
	}
	note := ""
	p, e := openCompany(c)
	if e != nil {
		return nil, e
	}
	switch action {
	case "save":
		if !setupEditable(u, c) {
			return nil, forbidden()
		}
		name, cat, typ, country, city, e := a.companyHeader(r.Context(), d)
		if e != nil {
			return nil, e
		}
		raw, e := json.Marshal(d["profile"])
		if e != nil {
			return nil, e
		}
		if len(raw) > 200000 {
			return nil, bad("Company details are too large.")
		}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		previousProfile := p
		p = CompanyPrivate{}
		if e = decoder.Decode(&p); e != nil {
			return nil, bad("Invalid company details.")
		}
		flows, e := businessFlowsInput(d, c)
		if e != nil {
			return nil, e
		}
		mode := businessModeForFlows(flows)
		c["category"], c["country"], c["city"], c["business_mode"], c["business_flows"] = cat, country, city, mode, flows
		issues, e := a.validateCompany(ctx, tx, c, p, false)
		if e != nil {
			return nil, e
		}
		if len(issues) > 0 {
			return nil, bad(strings.Join(issues, " "))
		}
		normalizeDocumentReminderEmails(&p)
		enc, e := sealCompany(p)
		if e != nil {
			return nil, e
		}
		_, e = tx.Exec(ctx, `UPDATE company_setups SET legal_name=$1,category=$2,company_type=$3,country=$4,city=$5,private_data=$6,business_mode=$8,business_flows=$9 WHERE id=$7`, name, cat, typ, country, city, enc, id(c, "id"), mode, flows)
		if e != nil {
			return nil, e
		}
		note = "Draft details saved"
		if removed := removedCompanyDocumentLinks(previousProfile, p); removed > 0 {
			note += fmt.Sprintf("; %d document link(s) removed; protected originals retained", removed)
		}
	case "submit":
		if !setupEditable(u, c) {
			return nil, forbidden()
		}
		issues, e := a.validateCompany(ctx, tx, c, p, true)
		if e != nil {
			return nil, e
		}
		if len(issues) > 0 {
			return nil, bad(strings.Join(issues, " "))
		}
		_, e = tx.Exec(ctx, `UPDATE company_setups SET status='Pending',submitted_at=now(),approval_note='',approved_by=NULL,approved_at=NULL WHERE id=$1`, id(c, "id"))
		if e != nil {
			return nil, e
		}
		note = "Submitted for MD / Super Admin approval"
	case "approve", "request_changes":
		if !slices.Contains([]string{"MD", "Super Admin"}, u.Role) {
			return nil, forbidden()
		}
		if str(c, "status") != "Pending" {
			return nil, bad("Only submitted companies can be reviewed.")
		}
		if action == "approve" {
			issues, e := a.validateCompany(ctx, tx, c, p, true)
			if e != nil {
				return nil, e
			}
			if len(issues) > 0 {
				return nil, bad("Company is no longer ready: " + strings.Join(issues, " "))
			}
			_, e = tx.Exec(ctx, `UPDATE company_setups SET status='Approved',approved_by=$1,approved_at=now(),approval_note='Approved' WHERE id=$2`, u.ID, id(c, "id"))
			note = "Company approved"
		} else {
			note, e = text(d, "note", 500)
			if e != nil {
				return nil, e
			}
			_, e = tx.Exec(ctx, `UPDATE company_setups SET status='Changes requested',approval_note=$1 WHERE id=$2`, note, id(c, "id"))
		}
		if e != nil {
			return nil, e
		}
	case "amend":
		if u.Role != "Finance Head" || id(c, "assigned_to") != u.ID || str(c, "status") != "Approved" {
			return nil, forbidden()
		}
		_, e = tx.Exec(ctx, `UPDATE company_setups SET status='Draft',revision=revision+1,approved_by=NULL,approved_at=NULL,approval_note='' WHERE id=$1`, id(c, "id"))
		if e != nil {
			return nil, e
		}
		c["revision"] = id(c, "revision") + 1
		note = "Amendment started; company requires fresh approval"
	default:
		return nil, bad("Unknown company action.")
	}
	if action == "approve" {
		if e = a.syncCompanyReminders(ctx, tx, id(c, "id"), p, action == "approve"); e != nil {
			return nil, e
		}
	}
	if action == "approve" && str(c, "category") == "Internal" {
		_, e = tx.Exec(ctx, `INSERT INTO companies(name,kind,status,setup_id) VALUES($1,'Self','Approved',$2) ON CONFLICT(setup_id) DO UPDATE SET name=excluded.name,status='Approved'`, str(c, "legal_name"), id(c, "id"))
		if e != nil {
			return nil, e
		}
	}
	if action == "amend" {
		if _, e = tx.Exec(ctx, `UPDATE companies SET status='Pending' WHERE setup_id=$1`, id(c, "id")); e != nil {
			return nil, e
		}
	}
	if _, e = tx.Exec(ctx, `UPDATE company_setups SET version=version+1,updated_at=now() WHERE id=$1`, id(c, "id")); e != nil {
		return nil, e
	}
	if e = setupHistory(ctx, tx, u, c, action, note); e != nil {
		return nil, e
	}
	return M{"ok": true}, tx.Commit(ctx)
}
func (a *App) companyFieldCreate(w http.ResponseWriter, r *http.Request) (any, error) {
	if !current(r).SetupCan("fields") {
		return nil, forbidden()
	}
	d, e := readBody(w, r)
	if e != nil {
		return nil, e
	}
	label, e := text(d, "label", 100)
	if e != nil {
		return nil, e
	}
	country := strings.ToUpper(str(d, "country"))
	if country != "*" && !regexp.MustCompile(`^[A-Z]{2}$`).MatchString(country) {
		return nil, bad("Country must be * or a two-letter code.")
	}
	categories := []string{}
	if vals, ok := d["categories"].([]any); ok {
		for _, v := range vals {
			s, ok := v.(string)
			if !ok || companyTypes[s] == nil || slices.Contains(categories, s) {
				return nil, bad("Choose valid company categories.")
			}
			categories = append(categories, s)
		}
	}
	if len(categories) == 0 {
		return nil, bad("Choose at least one company category.")
	}
	section, kind := str(d, "section"), str(d, "input_type")
	if !slices.Contains([]string{"Overview", "Contacts", "KYC", "Bank accounts", "Ports", "Property", "Insurance", "Additional", "Documents"}, section) || !slices.Contains([]string{"text", "textarea", "number", "date", "url", "email", "tel", "select", "checkbox"}, kind) {
		return nil, bad("Choose a valid section and field type.")
	}
	options := []string{}
	if vals, ok := d["options"].([]any); ok {
		for _, v := range vals {
			s, ok := v.(string)
			s = strings.TrimSpace(s)
			if !ok || s == "" || len(s) > 100 || slices.Contains(options, s) {
				return nil, bad("Dropdown choices must be unique, non-empty and at most 100 characters.")
			}
			options = append(options, s)
		}
	}
	if kind == "select" && (len(options) < 2 || len(options) > 50) {
		return nil, bad("Provide 2–50 dropdown choices.")
	}
	if kind != "select" && len(options) > 0 {
		return nil, bad("Only dropdown fields have choices.")
	}
	modes := []string{}
	if d["business_modes"] != nil {
		modes, e = stringChoices(d["business_modes"], companyBusinessModes)
		if e != nil {
			return nil, e
		}
	}
	var fid int64
	e = a.DB.QueryRow(r.Context(), `INSERT INTO company_field_definitions(label,section,country,categories,input_type,required,document_required,expiry_enabled,options,business_modes) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) RETURNING id`, label, str(d, "section"), country, categories, str(d, "input_type"), d["required"] == true, d["document_required"] == true, d["expiry_enabled"] == true, options, modes).Scan(&fid)
	return M{"id": fid}, e
}
func (a *App) companyPortCreate(w http.ResponseWriter, r *http.Request) (any, error) {
	if !current(r).SetupCan("ports") {
		return nil, forbidden()
	}
	d, e := readBody(w, r)
	if e != nil {
		return nil, e
	}
	name, e := text(d, "name", 100)
	if e != nil {
		return nil, e
	}
	var pid int64
	e = a.DB.QueryRow(r.Context(), `INSERT INTO company_ports(name,code,country,mode,postal_code,address) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, name, str(d, "code"), strings.ToUpper(str(d, "country")), str(d, "mode"), str(d, "postal_code"), str(d, "address")).Scan(&pid)
	return M{"id": pid}, e
}

// Inline strings keep spreadsheet formulas inert. This export follows the same KYC boundary as the API.
func (a *App) companySetupExport(w http.ResponseWriter, r *http.Request) (any, error) {
	if !current(r).SetupCan("export") {
		return nil, forbidden()
	}
	v, e := a.companySetupDetail(w, r)
	if e != nil {
		return nil, e
	}
	c := v.(M)
	pairs := [][2]string{{"Company", str(c, "legal_name")}, {"Category", str(c, "category")}, {"Type", str(c, "company_type")}, {"Country", str(c, "country")}, {"City", str(c, "city")}, {"Business flow", companyBusinessFlowLabel(c)}, {"Status", str(c, "status")}}
	var flatten func(string, any)
	flatten = func(prefix string, v any) {
		switch x := v.(type) {
		case map[string]any:
			keys := make([]string, 0, len(x))
			for k := range x {
				keys = append(keys, k)
			}
			slices.Sort(keys)
			for _, k := range keys {
				flatten(prefix+" / "+k, x[k])
			}
		case []any:
			for i, v := range x {
				flatten(prefix+" / "+strconv.Itoa(i+1), v)
			}
		default:
			pairs = append(pairs, [2]string{prefix, fmt.Sprint(x)})
		}
	}
	for _, key := range []string{"profile", "summary", "files"} {
		if v, ok := c[key]; ok {
			b, _ := json.Marshal(v)
			var x any
			json.Unmarshal(b, &x)
			if key == "profile" {
				if profile, ok := x.(map[string]any); ok {
					defs, _ := rows(r.Context(), a.DB, `SELECT id,label FROM company_field_definitions`)
					labels := map[int64]string{}
					for _, d := range defs {
						labels[id(d, "id")] = str(d, "label")
					}
					if fs, ok := profile["fields"].([]any); ok {
						for _, f := range fs {
							v := f.(map[string]any)
							if n, ok := v["id"].(float64); ok {
								v["label"] = labels[int64(n)]
							}
						}
					}
					ports, _ := rows(r.Context(), a.DB, `SELECT id,name FROM company_ports`)
					for _, port := range ports {
						if ps, ok := profile["ports"].([]any); ok {
							for _, p := range ps {
								v := p.(map[string]any)
								if n, ok := v["id"].(float64); ok && int64(n) == id(port, "id") {
									v["port_name"] = str(port, "name")
								}
							}
						}
					}
				}
			}
			flatten(key, x)
		}
	}
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	parts := map[string]string{"[Content_Types].xml": `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/></Types>`, "_rels/.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`, "xl/workbook.xml": `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Company setup" sheetId="1" r:id="rId1"/></sheets></workbook>`, "xl/_rels/workbook.xml.rels": `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`}
	var sheet strings.Builder
	sheet.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><cols><col min="1" max="1" width="45" customWidth="1"/><col min="2" max="2" width="80" customWidth="1"/></cols><sheetData>`)
	for i, p := range pairs {
		fmt.Fprintf(&sheet, `<row r="%d">`, i+1)
		for j, value := range p {
			var b bytes.Buffer
			xml.EscapeText(&b, []byte(value))
			fmt.Fprintf(&sheet, `<c r="%c%d" t="inlineStr"><is><t xml:space="preserve">%s</t></is></c>`, 'A'+j, i+1, b.String())
		}
		sheet.WriteString(`</row>`)
	}
	sheet.WriteString(`</sheetData></worksheet>`)
	parts["xl/worksheets/sheet1.xml"] = sheet.String()
	for name, body := range parts {
		f, e := z.Create(name)
		if e != nil {
			return nil, e
		}
		if _, e = f.Write([]byte(body)); e != nil {
			return nil, e
		}
	}
	if e = z.Close(); e != nil {
		return nil, e
	}
	w.Header().Set("Content-Type", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet")
	w.Header().Set("Content-Disposition", `attachment; filename="company-setup.xlsx"`)
	_, e = w.Write(buf.Bytes())
	return nil, e
}
