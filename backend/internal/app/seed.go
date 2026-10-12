package app

import (
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strings"
)

//go:embed seed_data.json
var seedData []byte

func (a *App) Seed(ctx context.Context) error {
	if a.Production {
		return errors.New("demo seeding is disabled in production")
	}
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(824732)`); e != nil {
		return e
	}
	var count int
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM users`).Scan(&count); e != nil {
		return e
	}
	if count > 0 {
		return nil
	}
	password := os.Getenv("OWNER_PASSWORD")
	if !validPassword(password) {
		return errors.New("OWNER_PASSWORD must be at least 12 characters for first local seed")
	}
	hash := passwordHash(password)
	var fixture map[string][]M
	dec := json.NewDecoder(strings.NewReader(string(seedData)))
	dec.UseNumber()
	if e = dec.Decode(&fixture); e != nil {
		return e
	}
	for _, table := range []string{"companies", "teams", "buyers", "lines", "users", "suppliers", "inventory", "orders", "entries", "quality", "milestones", "work_items", "movements", "purchases"} {
		if table == "users" {
			_, e = tx.Exec(ctx, `INSERT INTO users(id,username,name,password_hash,role,scope,companies,teams,lines,orders,finishing_units) VALUES(1,'admin','Operations Admin',$1,'MD','company','{1,2}','{}','{}','{}','{}'),(2,'merchant.a','Team A Merchant',$1,'Marketing','team','{1}','{1}','{}','{}','{}'),(3,'pm.lines1-6','Production Manager',$1,'Production','line','{1}','{}','{1,2,3,4,5,6}','{}','{}'),(4,'finishing.suits','Suits Finishing',$1,'Finishing','po','{1}','{}','{}','{1,4,7,10}','{Suits}'),(5,'stores','Stores Executive',$1,'Stores','company','{1}','{}','{}','{}','{}')`, hash)
			if e != nil {
				return e
			}
		} else {
			for _, r := range fixture[table] {
				cols := []string{}
				for k := range r {
					cols = append(cols, k)
				}
				sort.Strings(cols)
				vals := []any{}
				params := []string{}
				for i, k := range cols {
					v := r[k]
					if n, ok := v.(json.Number); ok {
						v = n.String()
					}
					vals = append(vals, v)
					params = append(params, fmt.Sprintf("$%d", i+1))
				}
				if _, e = tx.Exec(ctx, `INSERT INTO `+table+` (`+strings.Join(cols, ",")+`) VALUES (`+strings.Join(params, ",")+`)`, vals...); e != nil {
					return fmt.Errorf("seed %s: %w", table, e)
				}
			}
		}
		if _, e = tx.Exec(ctx, `SELECT setval(pg_get_serial_sequence('`+table+`','id'),COALESCE((SELECT max(id) FROM `+table+`),1))`); e != nil {
			return e
		}
	}
	if _, e = tx.Exec(ctx, `INSERT INTO audit(company_id,actor_id,action,entity,entity_id,detail) SELECT company_id,1,'Demo order loaded','Order',id,po||' · '||stage FROM orders`); e != nil {
		return e
	}
	return tx.Commit(ctx)
}

// Bootstrap exactly one Super Admin on a fresh database. Real companies still
// pass through Finance preparation and MD/Super Admin approval.
func (a *App) CreateOwner(ctx context.Context) error {
	username := strings.ToLower(strings.TrimSpace(os.Getenv("OWNER_USERNAME")))
	name := strings.TrimSpace(os.Getenv("OWNER_NAME"))
	password := os.Getenv("OWNER_PASSWORD")
	if !regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{1,79}$`).MatchString(username) || name == "" || len(name) > 120 || len(password) < 12 || !validPassword(password) {
		return errors.New("set OWNER_USERNAME, OWNER_NAME and a unique 12–128 character OWNER_PASSWORD")
	}
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(824732)`); e != nil {
		return e
	}
	var exists bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users)`).Scan(&exists); e != nil {
		return e
	}
	if exists {
		return errors.New("accounts already exist; bootstrap refuses to change the workspace")
	}
	var uid int64
	if e = tx.QueryRow(ctx, `INSERT INTO users(username,name,password_hash,role,scope,designation_id,inherit_designation) SELECT $1,$2,$3,'Super Admin','company',id,true FROM access_designations WHERE role='Super Admin' ORDER BY id LIMIT 1 RETURNING users.id`, username, name, passwordHash(password)).Scan(&uid); e != nil {
		return e
	}
	if e = accessAudit(ctx, tx, uid, "Initial administrator created", "User", uid, M{"name": name, "role": "Super Admin"}); e != nil {
		return e
	}
	return tx.Commit(ctx)
}
func (a *App) EnsureDemoUsers(ctx context.Context) error {
	if a.Production {
		return errors.New("demo accounts disabled in production")
	}
	password := os.Getenv("OWNER_PASSWORD")
	if !validPassword(password) {
		return errors.New("local OWNER_PASSWORD is required")
	}
	hash := passwordHash(password)
	tests := []struct {
		username, name, role, scope string
		teams, lines                []int64
		units                       []string
	}{
		{"ceo", "CEO Demo", "CEO", "company", nil, nil, nil}, {"finance", "Finance Demo", "Finance", "company", nil, nil, nil},
		{"merchant.b", "Team B Merchant", "Marketing", "team", []int64{2}, nil, nil}, {"planning", "Planning Manager", "Planning", "company", nil, nil, nil},
		{"cad", "Development & CAD", "Development", "company", nil, nil, nil}, {"sampling", "Sampling Manager", "Sampling", "company", nil, nil, nil},
		{"sourcing", "Sourcing Manager", "Sourcing", "company", nil, nil, nil}, {"cutting", "Cutting Manager", "Cutting", "company", nil, nil, nil},
		{"quality", "Quality Inspector", "Quality", "company", nil, nil, nil}, {"finishing", "Finishing Manager", "Finishing", "company", nil, nil, Products},
		{"warehouse", "Warehouse & Logistics", "Warehouse", "company", nil, nil, nil}, {"pm.lines7-20", "Production Lines 7–20", "Production", "line", nil, []int64{7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}, nil},
	}
	for _, t := range tests {
		teams := t.teams
		if teams == nil {
			teams = []int64{}
		}
		lines := t.lines
		if lines == nil {
			lines = []int64{}
		}
		units := t.units
		if units == nil {
			units = []string{}
		}
		if _, e := a.DB.Exec(ctx, `INSERT INTO users(username,name,password_hash,role,scope,companies,teams,lines,finishing_units) VALUES($1,$2,$3,$4,$5,'{1}',$6,$7,$8) ON CONFLICT(username) DO NOTHING`, t.username, t.name, hash, t.role, t.scope, teams, lines, units); e != nil {
			return e
		}
	}
	return nil
}

// InitializeCompanyWorkspace bootstraps an empty local database without deleting data.
func (a *App) InitializeCompanyWorkspace(ctx context.Context) error {
	if a.Production {
		return errors.New("local account initialization is disabled in production")
	}
	password := os.Getenv("OWNER_PASSWORD")
	if !validPassword(password) {
		return errors.New("OWNER_PASSWORD is required")
	}
	if _, e := companyCipher(); e != nil {
		return e
	}
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(824732)`); e != nil {
		return e
	}
	var exists bool
	if e = tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM users) OR EXISTS(SELECT 1 FROM companies) OR EXISTS(SELECT 1 FROM company_setups)`).Scan(&exists); e != nil {
		return e
	}
	if exists {
		return errors.New("workspace already initialized; existing accounts and records retained")
	}
	_, e = tx.Exec(ctx, `INSERT INTO users(username,name,password_hash,role,scope,companies) VALUES ('admin','Super Administrator',$1,'Super Admin','company','{}'),('finance','Finance Head',$1,'Finance Head','company','{}'),('md','Managing Director',$1,'MD','company','{}')`, passwordHash(password))
	if e != nil {
		return e
	}
	return tx.Commit(ctx)
}

// ResetCompanyWorkspace is an explicit local-only reset, never a startup migration.
func (a *App) ResetCompanyWorkspace(ctx context.Context) error {
	if a.Production || os.Getenv("RESET_COMPANY_WORKSPACE") != "REMOVE_LOCAL_TEST_DATA" {
		return errors.New("local reset acknowledgement is required")
	}
	password := os.Getenv("OWNER_PASSWORD")
	if !validPassword(password) {
		return errors.New("OWNER_PASSWORD is required")
	}
	tx, e := a.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	// A fresh Company Setup module must not overwrite actual company records.
	var count int
	if e = tx.QueryRow(ctx, `SELECT count(*) FROM company_setups`).Scan(&count); e != nil {
		return e
	}
	if count > 0 {
		return errors.New("company records exist; refusing local demo reset")
	}
	_, e = tx.Exec(ctx, `TRUNCATE reminders,audit,documents,purchases,materials,movements,quality,entries,work_items,milestones,orders,inventory,suppliers,buyers,lines,teams,companies,users,company_setup_history,company_setup_files,company_reminders,company_setups RESTART IDENTITY CASCADE`)
	if e != nil {
		return e
	}
	hash := passwordHash(password)
	_, e = tx.Exec(ctx, `INSERT INTO users(username,name,password_hash,role,scope,companies) VALUES ('admin','Super Administrator',$1,'Super Admin','company','{}'),('finance','Finance Head',$1,'Finance Head','company','{}'),('md','Managing Director',$1,'MD','company','{}')`, hash)
	if e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, `UPDATE users SET session_version=extract(epoch FROM now())::int,designation_id=(SELECT id FROM access_designations d WHERE d.role=users.role ORDER BY id LIMIT 1)`); e != nil {
		return e
	}
	return tx.Commit(ctx)
}

// SetLocalTestPasswords is an explicit operator command, never run at normal startup.
func (a *App) SetLocalTestPasswords(ctx context.Context) error {
	if a.Production || os.Getenv("APP_ENV") != "local" {
		return errors.New("test passwords are only allowed in the local environment")
	}
	_, err := a.DB.Exec(ctx, `UPDATE users SET password_hash=$1,session_version=session_version+1`, passwordHash("1234"))
	return err
}
