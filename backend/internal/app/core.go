package app

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	_ "embed"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"golang.org/x/crypto/argon2"
	"io"
	"log/slog"
	"net/http"
	"os"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

//go:embed schema.sql
var schema string

//go:embed migration_v2.sql
var migrationV2 string

//go:embed migration_v3.sql
var migrationV3 string

//go:embed migration_v4.sql
var migrationV4 string

//go:embed migration_v5.sql
var migrationV5 string

//go:embed migration_v6.sql
var migrationV6 string

//go:embed migration_v7.sql
var migrationV7 string

//go:embed migration_v8.sql
var migrationV8 string

//go:embed migration_v9.sql
var migrationV9 string

//go:embed migration_v11.sql
var migrationV11 string

//go:embed migration_v12.sql
var migrationV12 string

//go:embed migration_v14.sql
var migrationV14 string

//go:embed migration_v13.sql
var migrationV13 string

//go:embed migration_v10.sql
var migrationV10 string

//go:embed migration_v15.sql
var migrationV15 string

//go:embed migration_v16.sql
var migrationV16 string

//go:embed migration_v17.sql
var migrationV17 string

//go:embed migration_v18.sql
var migrationV18 string

//go:embed migration_v19.sql
var migrationV19 string

//go:embed migration_v20.sql
var migrationV20 string

//go:embed migration_v21.sql
var migrationV21 string

//go:embed migration_v22.sql
var migrationV22 string

//go:embed migration_v23.sql
var migrationV23 string

//go:embed migration_v24.sql
var migrationV24 string

//go:embed migration_v25.sql
var migrationV25 string

//go:embed migration_v26.sql
var migrationV26 string

type M = map[string]any
type App struct {
	Capacity           CapacityConfig
	Performance        *performanceTracker
	IssueQueue         chan issueEvent
	IssuesDropped      atomic.Int64
	IssueWriteFailures atomic.Int64
	StartedAt          time.Time
	RequestSlots       chan struct{}
	DB                 *Database
	Redis              *redis.Client
	Origin, FileRoot   string
	Production         bool
	LoginSlots         chan struct{}
	UploadSlots        chan struct{}
	DummyHash          string
	Requests, Failures atomic.Int64
}
type User struct {
	ReportsToID            int64    `json:"reports_to_id"`
	Territory              string   `json:"territory"`
	RestrictBuyers         bool     `json:"restrict_buyers"`
	BuyerIDs               []int64  `json:"buyer_ids"`
	ProductionAreas        []int64  `json:"production_areas"`
	IsDeveloper            bool     `json:"is_developer"`
	Email                  string   `json:"email"`
	IPRestricted           bool     `json:"-"`
	AllowedIPs             []string `json:"-"`
	MFAEnabled             bool     `json:"mfa_enabled"`
	DesignationID          int64    `json:"designation_id"`
	Designation            string   `json:"designation"`
	Department             string   `json:"department"`
	InheritDesignation     bool     `json:"inherit_designation"`
	DesignationPermissions []string `json:"-"`
	DesignationFeatures    []string `json:"-"`
	RenewalBlocks          []M      `json:"renewal_blocks"`
	SetupFeatures          []string `json:"setup_features"`
	SetupCustomAccess      bool     `json:"setup_custom_access"`
	SetupGrants            []string `json:"setup_grants"`
	Portal                 string   `json:"portal"`
	ID                     int64    `json:"id"`
	Username               string   `json:"username"`
	Name                   string   `json:"name"`
	Role                   string   `json:"role"`
	Scope                  string   `json:"scope"`
	Companies              []int64  `json:"companies"`
	Teams                  []int64  `json:"teams"`
	Lines                  []int64  `json:"lines"`
	Orders                 []int64  `json:"orders"`
	Units                  []string `json:"finishing_units"`
	Version                int      `json:"-"`
	Active                 bool     `json:"active"`
	Grants                 []string `json:"grants"`
	Permissions            []string `json:"permissions"`
	CustomPermissions      bool     `json:"custom_permissions"`
}
type Session struct {
	UserID      int64
	Version     int
	CSRF        string
	MFAVerified bool
	IssuedAt    int64
}
type authKey struct{}
type sessionKey struct{}
type failure struct {
	Code    int
	Message string
}

func (e failure) Error() string { return e.Message }
func bad(s string) error        { return failure{400, s} }
func forbidden() error          { return failure{403, "Your account cannot perform this action."} }
func missing() error            { return failure{404, "Record not found or outside your access."} }

var Roles = []string{"Super Admin", "Finance Head", "MD", "CEO", "Finance", "Marketing", "Planning", "Development", "Sourcing", "Stores", "Sampling", "Cutting", "Production", "Finishing", "Quality", "Warehouse"}
var Products = []string{"Suits", "Trousers", "Shirts", "General"}
var Stages = []string{"Planning", "Development", "Sampling", "Cutting", "Sewing", "Finishing", "Packing", "Warehouse", "Invoiced", "Dispatched"}
var Categories = []string{"Fabrics", "Trims", "Office consumables", "Machines & parts", "Production consumables", "Job workers", "Service providers", "Housekeeping", "Refreshments", "Travel", "Export logistics", "Cargo transport", "Import logistics"}
var roleGrants = map[string][]string{
	"Super Admin": {"view_orders", "companies", "audit", "users"}, "Finance Head": {},
	"MD":      {"orders", "approve", "plan", "develop", "sample", "cut", "sew", "finish", "pack", "quality", "warehouse", "finance", "suppliers", "inventory", "purchases", "companies", "users", "audit", "documents"},
	"CEO":     {"orders", "approve", "plan", "develop", "sample", "cut", "sew", "finish", "pack", "quality", "warehouse", "finance", "suppliers", "inventory", "purchases", "audit", "documents"},
	"Finance": {"approve", "finance", "companies", "suppliers", "purchases", "inventory", "audit", "documents"}, "Marketing": {"orders", "documents"}, "Planning": {"plan", "documents"}, "Development": {"develop", "documents"}, "Sourcing": {"suppliers", "purchases", "documents"}, "Stores": {"inventory", "documents"}, "Sampling": {"sample", "documents"}, "Cutting": {"cut", "documents"}, "Production": {"sew", "documents"}, "Finishing": {"finish", "pack", "documents"}, "Quality": {"quality", "documents"}, "Warehouse": {"warehouse", "documents"},
}

func init() {
	for role, caps := range roleGrants {
		if role != "Finance Head" && len(caps) > 0 && !slices.Contains(caps, "view_orders") {
			roleGrants[role] = append([]string{"view_orders"}, caps...)
		}
	}
}

func (u *User) Can(g string) bool {
	if g == "export_audit_check" && slices.Contains([]string{"Finance", "Finance Head"}, u.Role) {
		return exportExplicitAudit(u)
	}
	if u.Role == "Super Admin" && slices.Contains([]string{"material_view", "material_commercial", "fabric_edit", "trim_edit", "material_masters"}, g) {
		return true
	}
	if g == "users" {
		return u.Role == "Super Admin"
	}
	if u.DesignationID != 0 && !slices.Contains(u.DesignationPermissions, g) {
		return false
	}
	if u.InheritDesignation {
		return slices.Contains(u.DesignationPermissions, g) && slices.Contains(roleGrants[u.Role], g)
	}
	if u.CustomPermissions {
		return slices.Contains(u.Permissions, g) && slices.Contains(roleGrants[u.Role], g)
	}
	return slices.Contains(roleGrants[u.Role], g)
}
func (u *User) Require(g string) error {
	if !u.Can(g) {
		return forbidden()
	}
	return nil
}
func current(r *http.Request) *User { return r.Context().Value(authKey{}).(*User) }
func token() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func digest(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
func passwordHash(password string) string {
	salt := make([]byte, 16)
	if _, e := rand.Read(salt); e != nil {
		panic(e)
	}
	h := argon2.IDKey([]byte(password), salt, 3, 64*1024, 2, 32)
	return "argon2id$" + base64.RawStdEncoding.EncodeToString(salt) + "$" + base64.RawStdEncoding.EncodeToString(h)
}
func checkPassword(password, hash string) bool {
	p := strings.Split(hash, "$")
	if len(p) != 3 || p[0] != "argon2id" {
		return false
	}
	s, e := base64.RawStdEncoding.DecodeString(p[1])
	if e != nil || len(s) != 16 {
		return false
	}
	want, e := base64.RawStdEncoding.DecodeString(p[2])
	if e != nil || len(want) != 32 {
		return false
	}
	got := argon2.IDKey([]byte(password), s, 3, 64*1024, 2, 32)
	return subtle.ConstantTimeCompare(want, got) == 1
}
func validPassword(s string) bool {
	return (os.Getenv("APP_ENV") == "local" && s == "1234") || (len(s) >= 12 && len(s) <= 128)
}
func str(d M, k string) string { v, _ := d[k].(string); return strings.TrimSpace(v) }
func text(d M, k string, max int) (string, error) {
	s := str(d, k)
	if s == "" || len(s) > max {
		return "", bad("Enter a valid " + strings.ReplaceAll(k, "_", " ") + ".")
	}
	return s, nil
}
func integer(v any) (int64, error) {
	var s string
	switch x := v.(type) {
	case json.Number:
		s = x.String()
	case string:
		s = x
	case int64:
		return x, nil
	case int32:
		return int64(x), nil
	case int:
		return int64(x), nil
	default:
		return 0, bad("Enter a whole number.")
	}
	n, e := strconv.ParseInt(s, 10, 64)
	if e != nil || n < 0 || n > 1000000000 {
		return 0, bad("Enter a valid whole number.")
	}
	return n, nil
}
func id(d M, k string) int64 { n, _ := integer(d[k]); return n }
func ids(v any) ([]int64, error) {
	a, ok := v.([]any)
	if !ok {
		return nil, bad("Invalid assignments.")
	}
	r := []int64{}
	for _, v := range a {
		n, e := integer(v)
		if e != nil || n <= 0 || slices.Contains(r, n) {
			return nil, bad("Invalid or duplicate assignment.")
		}
		r = append(r, n)
	}
	return r, nil
}
func minor(v any) (int64, error) {
	s := fmt.Sprint(v)
	if strings.HasPrefix(s, "-") || strings.HasPrefix(s, "+") {
		return 0, bad("Enter a non-negative amount without a sign.")
	}
	p := strings.Split(s, ".")
	if len(p) > 2 {
		return 0, bad("Use at most two decimal places.")
	}
	whole, e := strconv.ParseInt(p[0], 10, 64)
	if e != nil || whole < 0 || whole > 10000000 {
		return 0, bad("Enter a valid non-negative amount.")
	}
	f := ""
	if len(p) == 2 {
		f = p[1]
	}
	if len(f) > 2 {
		return 0, bad("Use at most two decimal places.")
	}
	for len(f) < 2 {
		f += "0"
	}
	frac, e := strconv.ParseInt(f, 10, 64)
	if e != nil || frac < 0 || frac > 99 {
		return 0, bad("Invalid amount.")
	}
	return whole*100 + frac, nil
}
func date(s string) (time.Time, error) {
	t, e := time.Parse("2006-01-02", s)
	if e != nil || t.Year() < 2000 || t.Year() > 2100 {
		return t, bad("Enter a date between 2000 and 2100.")
	}
	return t, nil
}
func readBody(w http.ResponseWriter, r *http.Request) (M, error) {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.UseNumber()
	value, e := jsonValue(dec, 0)
	d, ok := value.(M)
	if e != nil {
		return nil, e
	}
	if !ok {
		return nil, bad("Send a JSON object.")
	}
	if e := dec.Decode(new(any)); e != io.EOF {
		return nil, bad("Only one JSON object is allowed.")
	}
	for _, key := range []string{"password_hash", "session_version", "private_data", "deleted_at", "deleted_by", "approved_by", "created_by", "assigned_to", "responsible_user_id", "storage_key", "csrf"} {
		if _, exists := d[key]; exists {
			return nil, bad("This field is managed by the server: " + key)
		}
	}
	return d, nil
}
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

type queryer interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
	QueryRow(context.Context, string, ...any) pgx.Row
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}

func rows(ctx context.Context, q queryer, sql string, args ...any) ([]M, error) {
	r, e := q.Query(ctx, sql, args...)
	if e != nil {
		return nil, e
	}
	defer r.Close()
	out := []M{}
	for r.Next() {
		values, e := r.Values()
		if e != nil {
			return nil, e
		}
		m := M{}
		for i, f := range r.FieldDescriptions() {
			m[f.Name] = values[i]
		}
		out = append(out, m)
	}
	return out, r.Err()
}
func row(ctx context.Context, q queryer, sql string, args ...any) (M, error) {
	r, e := rows(ctx, q, sql, args...)
	if e != nil {
		return nil, e
	}
	if len(r) == 0 {
		return nil, missing()
	}
	return r[0], nil
}

type endpoint func(http.ResponseWriter, *http.Request) (any, error)

func (a *App) handle(fn endpoint) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		v, e := fn(w, r)
		if e != nil {
			a.Failures.Add(1)
			code := 500
			msg := "The request could not be completed. Please try again."
			var f failure
			var pg *pgconn.PgError
			if errors.As(e, &f) {
				code = f.Code
				msg = f.Message
			} else if errors.Is(e, pgx.ErrNoRows) {
				code = 404
				msg = "Record not found or outside your access."
			} else if errors.As(e, &pg) {
				switch pg.Code {
				case "23505":
					code = 409
					msg = "This reference already exists."
				case "23503", "23514", "22P02", "22001":
					code = 400
					msg = "Check the record values and company assignments."
				}
			}
			if code >= 500 {
				slog.Error("request failed", "error", e, "path", r.URL.Path, "request_id", middleware.GetReqID(r.Context()))
			}
			writeJSON(w, code, M{"error": msg})
			return
		}
		if v != nil {
			writeJSON(w, 200, v)
		}
	}
}
func (a *App) security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		a.Requests.Add(1)
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; sandbox")
		w.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		if a.Production {
			w.Header().Set("Strict-Transport-Security", "max-age=31536000")
		}
		if r.Header.Get("Sec-Fetch-Site") == "cross-site" {
			writeJSON(w, 403, M{"error": "Cross-site requests are not allowed."})
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" && r.URL.Path != "/api/whatsapp/webhook" {
			if r.Header.Get("Origin") != a.Origin {
				writeJSON(w, 403, M{"error": "Request origin is not allowed."})
				return
			}
			if requestMediaType(r.Header.Get("Content-Type")) != "application/json" && requestMediaType(r.Header.Get("Content-Type")) != "multipart/form-data" {
				writeJSON(w, 415, M{"error": "Unsupported content type."})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}
func (a *App) loadUser(ctx context.Context, uid int64) (*User, error) {
	ctx = context.WithValue(ctx, serviceKey{}, "identity")
	tx, err := a.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	u := &User{}
	e := tx.QueryRow(ctx, `SELECT id,username,name,role,scope,companies,teams,lines,orders,finishing_units,session_version,active,permissions,custom_permissions,setup_features,setup_custom_access,COALESCE(designation_id,0),inherit_designation,email,is_developer,mfa_enabled,COALESCE(reports_to_id,0),territory,restrict_buyers,buyer_ids,production_areas,ip_restricted,allowed_ips FROM users WHERE id=$1 AND deleted_at IS NULL`, uid).Scan(&u.ID, &u.Username, &u.Name, &u.Role, &u.Scope, &u.Companies, &u.Teams, &u.Lines, &u.Orders, &u.Units, &u.Version, &u.Active, &u.Permissions, &u.CustomPermissions, &u.SetupFeatures, &u.SetupCustomAccess, &u.DesignationID, &u.InheritDesignation, &u.Email, &u.IsDeveloper, &u.MFAEnabled, &u.ReportsToID, &u.Territory, &u.RestrictBuyers, &u.BuyerIDs, &u.ProductionAreas, &u.IPRestricted, &u.AllowedIPs)
	if e != nil {
		return nil, e
	}
	if u.DesignationID != 0 {
		if e = tx.QueryRow(ctx, `SELECT name,department,permissions,features FROM access_designations WHERE id=$1`, u.DesignationID).Scan(&u.Designation, &u.Department, &u.DesignationPermissions, &u.DesignationFeatures); e != nil {
			return nil, e
		}
	}
	if u.Role == "Super Admin" {
		u.Scope = "company"
		companies, err := rows(ctx, tx, `SELECT id FROM companies WHERE setup_id IS NULL OR setup_id IN (SELECT id FROM company_setups WHERE deleted_at IS NULL)`)
		if err != nil {
			return nil, err
		}
		u.Companies = []int64{}
		for _, c := range companies {
			u.Companies = append(u.Companies, id(c, "id"))
		}
	}
	validCompanies, err := rows(ctx, tx, `SELECT c.id FROM companies c LEFT JOIN company_setups s ON s.id=c.setup_id WHERE c.id=ANY($1) AND (c.setup_id IS NULL OR s.deleted_at IS NULL)`, u.Companies)
	if err != nil {
		return nil, err
	}
	u.Companies = []int64{}
	for _, c := range validCompanies {
		u.Companies = append(u.Companies, id(c, "id"))
	}
	u.RenewalBlocks = []M{}
	if u.Role != "Super Admin" {
		u.RenewalBlocks, e = rows(ctx, tx, `SELECT DISTINCT c.id,c.legal_name,cr.label,cr.expires_on::text FROM company_setups c JOIN company_reminders cr ON cr.company_id=c.id WHERE c.deleted_at IS NULL AND c.responsible_user_id=$1 AND cr.active AND cr.expires_on<=CURRENT_DATE-30 ORDER BY c.id,cr.label`, u.ID)
		if e != nil {
			return nil, e
		}
	}
	u.SetupGrants = []string{}
	u.Portal = setupPortal(u.Role)
	if u.IsDeveloper {
		u.Portal = "developer"
	}
	for _, feature := range setupCaps[u.Role] {
		if u.SetupCan(feature) {
			u.SetupGrants = append(u.SetupGrants, feature)
		}
	}
	u.Grants = []string{}
	for _, grant := range rolePermissionCapabilities(u.Role) {
		if u.Can(grant) {
			u.Grants = append(u.Grants, grant)
		}
	}
	if e != nil {
		return nil, e
	}
	return u, tx.Commit(ctx)
}
func (a *App) getSession(r *http.Request) (*User, Session, error) {
	s := Session{}
	c, e := r.Cookie("erp_session")
	if e != nil {
		return nil, s, failure{401, "Please sign in."}
	}
	raw, e := a.Redis.Get(r.Context(), "session:"+digest(c.Value)).Result()
	if errors.Is(e, redis.Nil) {
		return nil, s, failure{401, "Your session expired. Please sign in."}
	}
	if e != nil {
		return nil, s, e
	}
	if e = json.Unmarshal([]byte(raw), &s); e != nil {
		return nil, s, e
	}
	// This bucket belongs to the verified server-side session, not the office IP.
	if e := a.limit(r.Context(), fmt.Sprintf("session-requests:%d", s.UserID), a.Capacity.UserRequestsPerMinute, time.Minute); e != nil {
		return nil, s, e
	}
	u, e := a.loadUser(r.Context(), s.UserID)
	if e != nil {
		if errors.Is(e, pgx.ErrNoRows) {
			return nil, s, failure{401, "Your session is no longer active."}
		}
		return nil, s, e
	}
	if !u.Active || u.Version != s.Version || ((u.MFAEnabled || u.IsDeveloper) && !s.MFAVerified) {
		return nil, s, failure{401, "Your session is no longer active."}
	}
	if e := checkUserIP(u, r); e != nil {
		return nil, s, e
	}
	return u, s, nil
}
func (a *App) auth(next http.Handler) http.Handler {
	return a.handle(func(w http.ResponseWriter, r *http.Request) (any, error) {
		u, s, e := a.getSession(r)
		if e != nil {
			return nil, e
		}
		if r.Method != "GET" && subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(s.CSRF)) != 1 {
			return nil, failure{403, "Security token expired. Refresh and try again."}
		}
		if len(u.RenewalBlocks) > 0 && !renewalRouteAllowed(r, u) {
			return nil, failure{423, "Your dashboard is locked by an overdue renewal. Open Company Setup to correct the document and obtain approval."}
		}
		if r.Method != "GET" {
			if e = a.limit(r.Context(), fmt.Sprintf("write:%d", u.ID), 240, 5*time.Minute); e != nil {
				w.Header().Set("Retry-After", "300")
				return nil, e
			}
		}
		if o, ok := r.Context().Value(observationKey{}).(*observation); ok {
			o.userID = u.ID
		}
		ctx := context.WithValue(r.Context(), authKey{}, u)
		ctx = context.WithValue(ctx, sessionKey{}, s)
		next.ServeHTTP(w, r.WithContext(ctx))
		return nil, nil
	})
}
func (a *App) login(w http.ResponseWriter, r *http.Request) (any, error) {
	d, e := readBody(w, r)
	if e != nil {
		return nil, e
	}
	name := strings.ToLower(str(d, "username"))
	password, _ := d["password"].(string)
	if len(name) > 150 || len(password) > 128 || (a.Production && len(password) < 12) {
		return nil, bad("Invalid credentials.")
	}
	ip := clientIP(r)
	// Nginx overwrites the client header on the private API network.
	for key, limit := range map[string]int64{"login:user:" + digest(name): 8, "login:peer:" + digest(ip): a.Capacity.LoginPeerLimit} {
		n, e := a.Redis.Eval(r.Context(), `local n=redis.call('INCR',KEYS[1]); if n==1 then redis.call('EXPIRE',KEYS[1],900) end; return n`, []string{key}).Int64()
		if e != nil {
			return nil, e
		}
		if n > limit {
			w.Header().Set("Retry-After", "900")
			return nil, failure{429, "Too many login attempts. Try again in 15 minutes."}
		}
	}
	if e = allowedFields(d, "username", "password", "portal", "bot_token"); e != nil {
		return nil, e
	}
	if e = a.verifyBot(r, str(d, "bot_token")); e != nil {
		return nil, e
	}
	waitForLogin := time.NewTimer(2500 * time.Millisecond)
	defer waitForLogin.Stop()
	select {
	case a.LoginSlots <- struct{}{}:
		defer func() { <-a.LoginSlots }()
	case <-r.Context().Done():
		return nil, r.Context().Err()
	case <-waitForLogin.C:
		w.Header().Set("Retry-After", "3")
		return nil, failure{429, "Sign-in is busy. Please try again shortly."}
	}
	var uid int64
	hash := a.DummyHash
	var stored string
	e = a.DB.QueryRow(r.Context(), `SELECT id,password_hash FROM users WHERE username=$1 AND active=true AND deleted_at IS NULL`, name).Scan(&uid, &stored)
	if e != nil && !errors.Is(e, pgx.ErrNoRows) {
		return nil, e
	}
	if stored != "" {
		hash = stored
	}
	ok := checkPassword(password, hash)
	if !ok || uid == 0 {
		return nil, failure{401, "Incorrect username or password."}
	}
	u, e := a.loadUser(r.Context(), uid)
	if e != nil {
		return nil, e
	}
	if e := checkUserIP(u, r); e != nil {
		return nil, e
	}
	if portal := str(d, "portal"); portal != "" && portal != u.Portal {
		return nil, failure{403, "This account belongs to a different role login page."}
	}
	if u.MFAEnabled || u.IsDeveloper {
		return a.startOTP(w, r, u)
	}
	return a.issueSession(w, r, u, false)
}
func (a *App) issueSession(w http.ResponseWriter, r *http.Request, u *User, verified bool) (any, error) {
	if e := checkUserIP(u, r); e != nil {
		return nil, e
	}
	if c, e := r.Cookie("erp_session"); e == nil {
		if e = a.Redis.Del(r.Context(), "session:"+digest(c.Value)).Err(); e != nil {
			return nil, e
		}
	}
	session := Session{UserID: u.ID, Version: u.Version, CSRF: token(), MFAVerified: verified, IssuedAt: time.Now().Unix()}
	raw, _ := json.Marshal(session)
	sessionToken := token()
	if e := a.Redis.Set(r.Context(), "session:"+digest(sessionToken), raw, 8*time.Hour).Err(); e != nil {
		return nil, e
	}
	a.Redis.Del(r.Context(), "login:user:"+digest(u.Username))
	http.SetCookie(w, &http.Cookie{Name: "erp_session", Value: sessionToken, Path: "/", HttpOnly: true, Secure: a.Production, SameSite: http.SameSiteLaxMode, MaxAge: 28800})
	return M{"user": u, "csrf": session.CSRF}, nil
}
func (a *App) session(w http.ResponseWriter, r *http.Request) (any, error) {
	u, s, e := a.getSession(r)
	if e != nil {
		var f failure
		if errors.As(e, &f) && f.Code == 401 {
			return M{"user": nil, "local": !a.Production}, nil
		}
		return nil, e
	}
	return M{"user": u, "csrf": s.CSRF, "local": !a.Production}, nil
}
func (a *App) logout(w http.ResponseWriter, r *http.Request) (any, error) {
	if c, e := r.Cookie("erp_session"); e == nil {
		if e = a.Redis.Del(r.Context(), "session:"+digest(c.Value)).Err(); e != nil {
			return nil, e
		}
	}
	http.SetCookie(w, &http.Cookie{Name: "erp_session", Value: "", Path: "/", MaxAge: -1, HttpOnly: true, Secure: a.Production, SameSite: http.SameSiteLaxMode})
	return M{"ok": true}, nil
}
func (a *App) password(w http.ResponseWriter, r *http.Request) (any, error) {
	return nil, failure{403, "Account changes are managed by the developer. Contact your administrator."}
}
func (a *App) Router() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID, a.observe, middleware.Recoverer, middleware.Timeout(40*time.Second), a.security, a.admission)
	r.Get("/api/whatsapp/webhook", a.whatsappWebhook)
	r.Post("/api/whatsapp/webhook", a.whatsappWebhook)
	r.Get("/api/health", a.handle(a.health))
	r.Get("/metrics", a.metrics)
	r.Get("/api/session", a.handle(a.session))
	r.Get("/api/login-config", a.handle(a.loginConfig))
	r.With(a.preventDuplicateRequest).Post("/api/login", a.handle(a.login))
	r.With(a.preventDuplicateRequest).Post("/api/login/otp", a.handle(a.verifyOTP))
	r.Group(func(r chi.Router) {
		r.Use(a.auth, a.preventDuplicateRequest)
		r.Get("/api/developer/whatsapp", a.handle(a.whatsappData))
		r.Post("/api/developer/whatsapp", a.handle(a.whatsappAction))
		r.Get("/api/developer", a.handle(a.developerData))
		r.Get("/api/developer/performance", a.handle(a.performanceData))
		r.Post("/api/developer/users/{id}", a.handle(a.developerUser))
		r.Post("/api/developer/users/{id}/delete", a.handle(a.developerDeleteUser))
		r.Post("/api/developer/issues/{id}", a.handle(a.developerIssue))
		r.Post("/api/developer/notifications", a.handle(a.developerNotifications))
		r.Post("/api/client-issues", a.handle(a.clientIssue))
		r.Post("/api/logout", a.handle(a.logout))
		r.Post("/api/password", a.handle(a.password))
		r.Get("/api/data", a.handle(a.data))

		r.Get("/api/exports/options", a.handle(a.exportOptions))
		r.Post("/api/exports/parties", a.handle(a.exportPartyOptionCreate))
		r.Get("/api/exports/export", a.handle(a.exportRegisterExcel))
		r.Get("/api/exports/statement", a.handle(a.exportStatementExcel))
		r.Get("/api/exports", a.handle(a.exportList))
		r.Post("/api/exports", a.handle(a.exportSave))
		r.Get("/api/exports/{id}", a.handle(a.exportDetail))
		r.Post("/api/exports/{id}", a.handle(a.exportSave))
		r.Get("/api/exports/{id}/history/{history_id}", a.handle(a.exportHistoryDetail))
		r.Post("/api/exports/{id}/check", a.handle(a.exportCheck))
		r.Post("/api/exports/{id}/discount", a.handle(a.exportDiscount))
		r.Post("/api/exports/{id}/workflow", a.handle(a.exportWorkflow))
		r.Post("/api/exports/{id}/comments", a.handle(a.exportComment))
		r.Post("/api/exports/{id}/reconcile", a.handle(a.exportReconcile))
		r.Post("/api/exports/{id}/reconciliation/ebrc", a.handle(a.exportReconciliationEBRC))
		r.Post("/api/exports/{id}/reconciliation/files", a.handle(a.exportReconciliationFileUpload))
		r.Post("/api/exports/{id}/reconciliation/files/{file_id}/remove", a.handle(a.exportReconciliationFileRemove))
		r.Post("/api/exports/{id}/files", a.handle(a.exportFileUpload))
		r.Post("/api/exports/{id}/files/{file_id}/remove", a.handle(a.exportFileRemove))
		r.Post("/api/exports/{id}/bank-email/prepare", a.handle(a.exportBankEmailPrepare))
		r.Post("/api/exports/{id}/bank-email/{email_id}/send", a.handle(a.exportBankEmailSend))
		r.Get("/api/exports/{id}/bank-email", a.handle(a.exportBankEmails))
		r.Get("/api/exports/{id}/export", a.handle(a.exportDetailExcel))
		r.Get("/api/export-files/{id}", a.handle(a.exportFileGet))
		r.Post("/api/enquiries/{id}/orders", a.handle(a.enquiryOrder))
		r.Post("/api/enquiries/{id}/preview", a.handle(a.enquiryPreview))
		r.Post("/api/enquiries/{id}/files", a.handle(a.enquiryFileUpload))
		r.Get("/api/enquiry-files/{id}", a.handle(a.enquiryFileGet))
		r.Get("/api/enquiries", a.handle(a.enquiryList))
		r.Post("/api/enquiries", a.handle(a.enquirySave))
		r.Get("/api/enquiries/{id}/export", a.handle(a.enquiryExport))
		r.Get("/api/enquiries/{id}", a.handle(a.enquiryDetail))
		r.Post("/api/enquiries/{id}", a.handle(a.enquirySave))
		r.Get("/api/enquiry-history/{id}", a.handle(a.enquiryHistoryDetail))
		r.Post("/api/enquiry-options", a.handle(a.enquiryOption))
		r.Post("/api/sample-requests/{id}/files", a.handle(a.sampleFileUpload))
		r.Get("/api/sample-files/{id}", a.handle(a.sampleFileGet))
		r.Get("/api/sample-requests", a.handle(a.sampleList))
		r.Get("/api/sample-capacities", a.handle(a.sampleCapacityList))
		r.Post("/api/sample-capacities", a.handle(a.sampleCapacitySave))
		r.Post("/api/sample-schedule", a.handle(a.sampleSchedule))
		r.Get("/api/sample-planning", a.handle(a.samplePlanning))
		r.Get("/api/sample-requests/{id}", a.handle(a.sampleDetail))
		r.Post("/api/sample-requests", a.handle(a.sampleSave))
		r.Post("/api/sample-requests/{id}", a.handle(a.sampleSave))
		r.Post("/api/sample-indents/{id}/spec-import", a.handle(a.sampleSpecImport))
		r.Post("/api/sample-indents/{id}", a.handle(a.samplePiece))
		r.Post("/api/sample-options", a.handle(a.sampleOption))
		r.Post("/api/sample-stock", a.handle(a.sampleStock))
		r.Get("/api/development-masters", a.handle(a.developmentList))
		r.Post("/api/development-masters", a.handle(a.developmentSave))
		r.Post("/api/development-masters/{id}", a.handle(a.developmentSave))
		r.Get("/api/materials", a.handle(a.materialList))
		r.Post("/api/materials", a.handle(a.materialSave))
		r.Get("/api/materials/export", a.handle(a.materialExport))
		r.Get("/api/materials/{id}", a.handle(a.materialDetail))
		r.Post("/api/materials/{id}", a.handle(a.materialSave))
		r.Post("/api/material-options", a.handle(a.materialOptionSave))
		r.Post("/api/material-options/{id}", a.handle(a.materialOptionSave))
		r.Post("/api/material-suppliers/{id}/rating", a.handle(a.materialSupplierRate))
		r.Post("/api/materials/{id}/photos", a.handle(a.materialPhotoUpload))
		r.Get("/api/material-photos/{id}", a.handle(a.materialPhotoGet))
		r.Get("/api/company-setup", a.handle(a.companySetupList))
		r.Get("/api/company-postal-lookup", a.handle(a.companyPostalLookup))
		r.Post("/api/company-setup", a.handle(a.companySetupCreate))
		r.Get("/api/company-setup/{id}", a.handle(a.companySetupDetail))
		r.Post("/api/company-setup/{id}", a.handle(a.companySetupAction))
		r.Post("/api/company-setup/{id}/delete", a.handle(a.companySetupDelete))
		r.Post("/api/company-setup/{id}/files", a.handle(a.companySetupUpload))
		r.Get("/api/company-files/{id}", a.handle(a.companySetupDownload))
		r.Get("/api/company-setup/{id}/export", a.handle(a.companySetupExport))
		r.Get("/api/setup-users", a.handle(a.setupUsers))
		r.Post("/api/setup-users", a.handle(a.setupUserSave))
		r.Post("/api/setup-users/{id}", a.handle(a.setupUserSave))
		r.Post("/api/access-designations", a.handle(a.designationSave))
		r.Post("/api/access-designations/{id}", a.handle(a.designationSave))
		r.Post("/api/setup-users/{id}/replace", a.handle(a.replaceUser))
		r.Post("/api/access-resources", a.handle(a.accessResourceSave))
		r.Post("/api/company-setup/{id}/responsibility", a.handle(a.companyResponsibility))
		r.Post("/api/company-fields", a.handle(a.companyFieldCreate))
		r.Post("/api/company-setup/{id}/fields", a.handle(a.companyAdditionalFieldCreate))
		r.Post("/api/company-types", a.handle(a.companyTypeCreate))
		r.Post("/api/company-ports", a.handle(a.companyPortCreate))
		r.Post("/api/orders", a.handle(a.createOrder))
		r.Get("/api/orders/export", a.handle(a.exportOrders))
		r.Get("/api/orders/{id}", a.handle(a.orderDetail))
		r.Post("/api/orders/{id}", a.handle(a.orderAction))
		r.Post("/api/records/{kind}", a.handle(a.createRecord))
		r.Post("/api/records/{kind}/{id}", a.handle(a.recordAction))
		r.Get("/api/inventory/{id}", a.handle(a.inventoryHistory))
		r.Post("/api/inventory/{id}", a.handle(a.stockMovement))
		r.Post("/api/users", a.handle(a.setupUserSave))
		r.Post("/api/users/{id}", a.handle(a.setupUserSave))
		r.Post("/api/orders/{id}/documents", a.handle(a.uploadDocument))
		r.Get("/api/documents/{id}", a.handle(a.downloadDocument))
	})
	return r
}
func (a *App) Migrate(ctx context.Context) error {
	if len(os.Args) == 1 {
		var version int
		if err := a.DB.QueryRow(ctx, `SELECT COALESCE(max(version),0) FROM schema_versions`).Scan(&version); err != nil {
			return err
		}
		if version != 26 {
			return errors.New("run the migration service before starting the API")
		}
		return nil
	}
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(824731)`); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `CREATE TABLE IF NOT EXISTS schema_versions(version int PRIMARY KEY,applied_at timestamptz NOT NULL DEFAULT now())`); e != nil {
		return e
	}
	var exists bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_versions WHERE version=1)`).Scan(&exists); e != nil {
		return e
	}
	if !exists {
		if _, e = tx.Exec(ctx, schema); e != nil {
			return e
		}
	}
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_versions WHERE version=2)`).Scan(&exists); e != nil {
		return e
	}
	if !exists {
		if _, e = tx.Exec(ctx, migrationV2); e != nil {
			return e
		}
	}
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_versions WHERE version=3)`).Scan(&exists); e != nil {
		return e
	}
	if !exists {
		if _, e = tx.Exec(ctx, migrationV3); e != nil {
			return e
		}
	}
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_versions WHERE version=4)`).Scan(&exists); e != nil {
		return e
	}
	if !exists {
		if _, e = tx.Exec(ctx, migrationV4); e != nil {
			return e
		}
	}
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_versions WHERE version=5)`).Scan(&exists); e != nil {
		return e
	}
	if !exists {
		if _, e = tx.Exec(ctx, migrationV5); e != nil {
			return e
		}
	}
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_versions WHERE version=6)`).Scan(&exists); e != nil {
		return e
	}
	if !exists {
		if _, e = tx.Exec(ctx, migrationV6); e != nil {
			return e
		}
	}
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_versions WHERE version=7)`).Scan(&exists); e != nil {
		return e
	}
	if !exists {
		if _, e = tx.Exec(ctx, migrationV7); e != nil {
			return e
		}
	}
	if _, e = tx.Exec(ctx, `SELECT version FROM schema_versions WHERE version=8`); e != nil {
		return e
	}
	var v8 bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_versions WHERE version=8)`).Scan(&v8); e != nil {
		return e
	}
	if !v8 {
		if _, e = tx.Exec(ctx, migrationV8); e != nil {
			return e
		}
	}
	var v9 bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_versions WHERE version=9)`).Scan(&v9); e != nil {
		return e
	}
	if !v9 {
		if _, e = tx.Exec(ctx, migrationV9); e != nil {
			return e
		}
	}
	var v10 bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_versions WHERE version=10)`).Scan(&v10); e != nil {
		return e
	}
	if !v10 {
		if _, e = tx.Exec(ctx, migrationV10); e != nil {
			return e
		}
	}
	var v11 bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_versions WHERE version=11)`).Scan(&v11); e != nil {
		return e
	}
	if !v11 {
		if _, e = tx.Exec(ctx, migrationV11); e != nil {
			return e
		}
	}
	var v12 bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_versions WHERE version=12)`).Scan(&v12); e != nil {
		return e
	}
	if !v12 {
		if _, e = tx.Exec(ctx, migrationV12); e != nil {
			return e
		}
	}

	var v13 bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_versions WHERE version=13)`).Scan(&v13); e != nil {
		return e
	}
	if !v13 {
		if _, e = tx.Exec(ctx, migrationV13); e != nil {
			return e
		}
	}

	var v14 bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_versions WHERE version=14)`).Scan(&v14); e != nil {
		return e
	}
	if !v14 {
		if _, e = tx.Exec(ctx, migrationV14); e != nil {
			return e
		}
	}

	var v15 bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_versions WHERE version=15)`).Scan(&v15); e != nil {
		return e
	}
	if !v15 {
		if _, e = tx.Exec(ctx, migrationV15); e != nil {
			return e
		}
	}
	var v16 bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_versions WHERE version=16)`).Scan(&v16); e != nil {
		return e
	}
	if !v16 {
		if _, e = tx.Exec(ctx, migrationV16); e != nil {
			return e
		}
	}

	var v17 bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_versions WHERE version=17)`).Scan(&v17); e != nil {
		return e
	}
	if !v17 {
		if _, e = tx.Exec(ctx, migrationV17); e != nil {
			return e
		}
	}

	var v18 bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_versions WHERE version=18)`).Scan(&v18); e != nil {
		return e
	}
	if !v18 {
		if _, e = tx.Exec(ctx, migrationV18); e != nil {
			return e
		}
	}

	var v19 bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_versions WHERE version=19)`).Scan(&v19); e != nil {
		return e
	}
	if !v19 {
		if _, e = tx.Exec(ctx, migrationV19); e != nil {
			return e
		}
	}

	var v20 bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_versions WHERE version=20)`).Scan(&v20); e != nil {
		return e
	}
	if !v20 {
		if _, e = tx.Exec(ctx, migrationV20); e != nil {
			return e
		}
	}

	var v21 bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_versions WHERE version=21)`).Scan(&v21); e != nil {
		return e
	}
	if !v21 {
		if _, e = tx.Exec(ctx, migrationV21); e != nil {
			return e
		}
	}
	var v22 bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_versions WHERE version=22)`).Scan(&v22); e != nil {
		return e
	}
	if !v22 {
		if _, e = tx.Exec(ctx, migrationV22); e != nil {
			return e
		}
	}
	var v23 bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_versions WHERE version=23)`).Scan(&v23); e != nil {
		return e
	}
	if !v23 {
		if _, e = tx.Exec(ctx, migrationV23); e != nil {
			return e
		}
	}
	var v24 bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_versions WHERE version=24)`).Scan(&v24); e != nil {
		return e
	}
	if !v24 {
		if _, e = tx.Exec(ctx, migrationV24); e != nil {
			return e
		}
	}
	var v25 bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_versions WHERE version=25)`).Scan(&v25); e != nil {
		return e
	}
	if !v25 {
		if _, e = tx.Exec(ctx, migrationV25); e != nil {
			return e
		}
	}

	var v26 bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM schema_versions WHERE version=26)`).Scan(&v26); e != nil {
		return e
	}
	if !v26 {
		if _, e = tx.Exec(ctx, migrationV26); e != nil {
			return e
		}
	}

	return tx.Commit(ctx)
}
func New(ctx context.Context) (*App, error) {
	origin := os.Getenv("APP_ORIGIN")
	prod := os.Getenv("APP_ENV") == "production"
	if e := validateSecurityConfig(origin, prod && len(os.Args) == 1); e != nil {
		return nil, e
	}
	cfg, e := pgxpool.ParseConfig(os.Getenv("DATABASE_URL"))
	if e != nil {
		return nil, errors.New("invalid database configuration")
	}
	capacity, e := readCapacityConfig()
	if e != nil {
		return nil, e
	}
	cfg.MaxConns = int32(capacity.DBConnections)
	cfg.MinConns = 2
	cfg.MaxConnLifetime = 30 * time.Minute
	cfg.MaxConnLifetimeJitter = 5 * time.Minute
	cfg.MaxConnIdleTime = 5 * time.Minute
	cfg.ConnConfig.ConnectTimeout = 5 * time.Second
	// Administrative migrations keep their own limits. Runtime queries are bounded.
	if len(os.Args) == 1 {
		cfg.ConnConfig.RuntimeParams["statement_timeout"] = "25000"
		cfg.ConnConfig.RuntimeParams["lock_timeout"] = "5000"
		cfg.ConnConfig.RuntimeParams["idle_in_transaction_session_timeout"] = "30000"
	}
	db, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		return nil, e
	}
	if e = db.Ping(ctx); e != nil {
		return nil, e
	}
	opt, e := redis.ParseURL(os.Getenv("REDIS_URL"))
	if e != nil {
		return nil, e
	}
	rc := redis.NewClient(opt)
	if e = rc.Ping(ctx).Err(); e != nil {
		return nil, e
	}
	root := os.Getenv("FILE_ROOT")
	if root == "" {
		root = "files"
	}
	if e = os.MkdirAll(root, 0700); e != nil {
		return nil, e
	}
	return &App{Capacity: capacity, Performance: &performanceTracker{}, StartedAt: time.Now(), IssueQueue: make(chan issueEvent, 1024), RequestSlots: make(chan struct{}, 256), DB: &Database{Pool: db}, Redis: rc, Origin: origin, Production: prod, FileRoot: root, LoginSlots: make(chan struct{}, capacity.LoginConcurrency), UploadSlots: make(chan struct{}, 4), DummyHash: passwordHash(token())}, nil
}
