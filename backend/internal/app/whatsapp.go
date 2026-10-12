package app

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var waPhone = regexp.MustCompile(`^\+[1-9][0-9]{7,14}$`)
var waName = regexp.MustCompile(`^[a-z][a-z0-9_]{2,100}$`)
var waLanguage = regexp.MustCompile(`^[a-z]{2}(_[A-Z]{2})?$`)
var waPlaceholder = regexp.MustCompile(`\{\{[^}]*\}\}`)

func waConfigured() bool {
	return os.Getenv("WHATSAPP_ACCESS_TOKEN") != "" && os.Getenv("WHATSAPP_PHONE_ID") != "" && os.Getenv("WHATSAPP_WABA_ID") != "" && os.Getenv("WHATSAPP_APP_SECRET") != "" && os.Getenv("WHATSAPP_VERIFY_TOKEN") != ""
}
func waNormalizePhone(s string) (string, error) {
	s = strings.NewReplacer(" ", "", "-", "", "(", "", ")", "").Replace(s)
	if !waPhone.MatchString(s) {
		return "", bad("Use a WhatsApp number with country code, for example +918389841671.")
	}
	return s, nil
}
func waBodyValid(s string) bool {
	if len(s) < 20 || len(s) > 1024 || strings.HasPrefix(strings.TrimSpace(s), "{{") || strings.HasSuffix(strings.TrimSpace(s), "}}") {
		return false
	}
	matches := waPlaceholder.FindAllString(s, -1)
	if len(matches) != 4 {
		return false
	}
	for i, m := range matches {
		if m != fmt.Sprintf("{{%d}}", i+1) {
			return false
		}
	}
	return true
}
func waPreview(body string, params []string) string {
	for i, p := range params {
		body = strings.ReplaceAll(body, fmt.Sprintf("{{%d}}", i+1), p)
	}
	return body
}
func waParams(name, summary, detail, link string) []string {
	clean := func(s string) string {
		s = strings.Join(strings.Fields(s), " ")
		r := []rune(s)
		if len(r) > 240 {
			return string(r[:237]) + "..."
		}
		if s == "" {
			return "Not specified"
		}
		return s
	}
	return []string{clean(name), clean(summary), clean(detail), link}
}

// Fixed official host; callers cannot redirect the bearer token to an arbitrary URL.
func waAPI(ctx context.Context, method, path string, payload any) (M, int, error) {
	version := os.Getenv("WHATSAPP_API_VERSION")
	if !regexp.MustCompile(`^v[0-9]+\.0$`).MatchString(version) {
		version = "v25.0"
	}
	b, e := json.Marshal(payload)
	if e != nil {
		return nil, 0, e
	}
	var body io.Reader
	if payload != nil {
		body = bytes.NewReader(b)
	}
	req, e := http.NewRequestWithContext(ctx, method, "https://graph.facebook.com/"+version+"/"+path, body)
	if e != nil {
		return nil, 0, e
	}
	req.Header.Set("Authorization", "Bearer "+os.Getenv("WHATSAPP_ACCESS_TOKEN"))
	req.Header.Set("Content-Type", "application/json")
	client := &http.Client{Timeout: 15 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, e := client.Do(req)
	if e != nil {
		return nil, 0, fmt.Errorf("Meta connection failed; delivery may be uncertain")
	}
	defer resp.Body.Close()
	var result M
	e = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&result)
	if resp.StatusCode >= 300 {
		code := ""
		if v, ok := result["error"].(map[string]any); ok {
			code = fmt.Sprint(v["code"])
		}
		return nil, resp.StatusCode, fmt.Errorf("Meta rejected request (HTTP %d, code %s). Check sender, permissions, payment and template status.", resp.StatusCode, code)
	}
	if e != nil {
		return nil, resp.StatusCode, fmt.Errorf("Meta response could not be read")
	}
	return result, resp.StatusCode, nil
}
func (a *App) whatsappData(w http.ResponseWriter, r *http.Request) (any, error) {
	if e := developerRequired(r); e != nil {
		return nil, e
	}
	ctx := r.Context()
	out := M{"configured": waConfigured(), "phone_id": os.Getenv("WHATSAPP_PHONE_ID"), "business_account_id": os.Getenv("WHATSAPP_WABA_ID"), "webhook_url": a.Origin + "/api/whatsapp/webhook"}
	settings, e := row(ctx, a.DB, `SELECT * FROM whatsapp_settings WHERE id=1`)
	if e != nil {
		return nil, e
	}
	out["settings"] = settings
	for k, q := range map[string]string{
		"templates":  `SELECT * FROM whatsapp_templates ORDER BY kind`,
		"recipients": `SELECT u.id,u.name,u.username,u.phone,u.active,COALESCE(r.enabled,false) AS enabled,COALESCE(r.consent_note,'') AS consent_note,r.consent_at FROM users u LEFT JOIN whatsapp_recipients r ON r.user_id=u.id WHERE u.deleted_at IS NULL ORDER BY u.name`,
		"messages":   `SELECT m.id,m.kind,m.status,m.phone,m.error,m.created_at,m.updated_at,m.meta_id,m.attempts,u.username FROM whatsapp_messages m JOIN users u ON u.id=m.user_id ORDER BY m.id DESC LIMIT 200`,
	} {
		v, e := rows(ctx, a.DB, q)
		if e != nil {
			return nil, e
		}
		out[k] = v
	}
	return out, nil
}
func (a *App) whatsappAction(w http.ResponseWriter, r *http.Request) (any, error) {
	if e := developerRequired(r); e != nil {
		return nil, e
	}
	d, e := readBody(w, r)
	if e != nil {
		return nil, e
	}
	if e = allowedFields(d, "action", "password", "enabled", "reminder_hour", "daily_cap", "version", "user_id", "consent_note", "kind", "name", "language", "body", "phone", "consent_confirmed"); e != nil {
		return nil, e
	}
	if e = a.developerReauthenticate(r, d); e != nil {
		return nil, e
	}
	ctx := r.Context()
	action := str(d, "action")
	switch action {
	case "settings":
		enabled, ok := d["enabled"].(bool)
		hour, e := integer(d["reminder_hour"])
		cap, e2 := integer(d["daily_cap"])
		if !ok || e != nil || e2 != nil || hour > 23 || cap < 1 || cap > 10000 {
			return nil, bad("Choose a reminder hour from 0–23 and daily cap from 1–10000.")
		}
		if enabled && !waConfigured() {
			return nil, bad("Configure the Meta sender, access token, app secret and webhook verify token first.")
		}
		if enabled {
			var n int
			if e = a.DB.QueryRow(ctx, `SELECT count(*) FROM whatsapp_templates WHERE status='APPROVED' AND synced_at>now()-interval '24 hours'`).Scan(&n); e != nil {
				return nil, e
			}
			if n < 5 {
				return nil, bad("Submit and sync all five approved templates before enabling automatic messages.")
			}
		}
		tag, e := a.DB.Exec(ctx, `UPDATE whatsapp_settings SET enabled=$1,reminder_hour=$2,daily_cap=$3,version=version+1 WHERE id=1 AND version=$4`, enabled, hour, cap, id(d, "version"))
		if e != nil {
			return nil, e
		}
		if tag.RowsAffected() == 0 {
			return nil, failure{409, "Settings changed. Refresh and try again."}
		}
	case "recipient":
		enabled, ok := d["enabled"].(bool)
		note := str(d, "consent_note")
		if !ok || len(note) > 500 || (enabled && note == "") {
			return nil, bad("Record the employee's consent before enabling WhatsApp.")
		}
		u, e := row(ctx, a.DB, `SELECT phone FROM users WHERE id=$1 AND active AND deleted_at IS NULL`, id(d, "user_id"))
		if e != nil {
			return nil, e
		}
		if enabled {
			if _, e = waNormalizePhone(str(u, "phone")); e != nil {
				return nil, e
			}
		}
		_, e = a.DB.Exec(ctx, `INSERT INTO whatsapp_recipients(user_id,enabled,consent_note,consent_at) VALUES($1,$2,$3,CASE WHEN $2 THEN now() END) ON CONFLICT(user_id) DO UPDATE SET enabled=$2,consent_note=$3,consent_at=CASE WHEN $2 THEN now() ELSE NULL END,updated_at=now()`, id(d, "user_id"), enabled, note)
		if e != nil {
			return nil, e
		}
	case "template":
		name, lang, body := str(d, "name"), str(d, "language"), str(d, "body")
		if !waName.MatchString(name) || !waLanguage.MatchString(lang) || !waBodyValid(body) {
			return nil, bad("Use a valid template name/language and exactly {{1}}, {{2}}, {{3}}, {{4}} in that order (20–1024 characters).")
		}
		tag, e := a.DB.Exec(ctx, `UPDATE whatsapp_templates SET name=$1,language=$2,body=$3,status='DRAFT',meta_id='',synced_at=NULL,version=version+1 WHERE kind=$4 AND version=$5`, name, lang, body, str(d, "kind"), id(d, "version"))
		if e != nil {
			return nil, e
		}
		if tag.RowsAffected() == 0 {
			return nil, failure{409, "Template changed. Refresh and try again."}
		}
	case "submit", "sync":
		if !waConfigured() {
			return nil, bad("Meta server credentials are not configured.")
		}
		t, e := row(ctx, a.DB, `SELECT * FROM whatsapp_templates WHERE kind=$1`, str(d, "kind"))
		if e != nil {
			return nil, e
		}
		if action == "submit" {
			if str(t, "status") != "DRAFT" {
				return nil, bad("Only a draft can be submitted.")
			}
			sample := waParams("Ansu", "PO EXAMPLE / Sampling", "2 pieces pending; due 10 October 2026", a.Origin+"/sampling")
			res, _, err := waAPI(ctx, "POST", os.Getenv("WHATSAPP_WABA_ID")+"/message_templates", M{"name": str(t, "name"), "language": str(t, "language"), "category": "UTILITY", "components": []M{{"type": "BODY", "text": str(t, "body"), "example": M{"body_text": [][]string{sample}}}}})
			if err != nil {
				return nil, bad(err.Error())
			}
			_, e = a.DB.Exec(ctx, `UPDATE whatsapp_templates SET status='PENDING',meta_id=$1,synced_at=now(),version=version+1 WHERE kind=$2 AND version=$3`, str(res, "id"), str(t, "kind"), id(t, "version"))
			if e != nil {
				return nil, e
			}
		} else {
			res, _, err := waAPI(ctx, "GET", os.Getenv("WHATSAPP_WABA_ID")+"/message_templates?name="+str(t, "name"), nil)
			if err != nil {
				return nil, bad(err.Error())
			}
			status, metaID := "NOT_FOUND", ""
			if list, ok := res["data"].([]any); ok {
				for _, v := range list {
					m, ok := v.(map[string]any)
					if !ok || str(m, "name") != str(t, "name") || str(m, "language") != str(t, "language") {
						continue
					}
					metaID = str(m, "id")
					status = str(m, "status")
					matched := false
					if cs, ok := m["components"].([]any); ok {
						for _, v := range cs {
							c, _ := v.(map[string]any)
							if str(c, "type") == "BODY" && str(c, "text") == str(t, "body") {
								matched = true
							}
						}
					}
					if !matched {
						status = "CONTENT_MISMATCH"
					}
					if str(m, "category") != "UTILITY" {
						status = "CATEGORY_REVIEW"
					}
				}
			}
			_, e = a.DB.Exec(ctx, `UPDATE whatsapp_templates SET status=$1,meta_id=$2,synced_at=now(),version=version+1 WHERE kind=$3 AND version=$4`, status, metaID, str(t, "kind"), id(t, "version"))
			if e != nil {
				return nil, e
			}
		}
	case "test":
		if !waConfigured() {
			return nil, bad("Meta server credentials are not configured.")
		}
		if d["consent_confirmed"] != true {
			return nil, bad("Confirm the recipient requested this test.")
		}
		phone, e := waNormalizePhone(str(d, "phone"))
		if e != nil {
			return nil, e
		}
		var status string
		e = a.DB.QueryRow(ctx, `SELECT status FROM whatsapp_templates WHERE kind='test'`).Scan(&status)
		if e != nil {
			return nil, e
		}
		if status != "APPROVED" {
			return nil, bad("The connection-test template must be approved by Meta first.")
		}
		if e = a.limit(ctx, fmt.Sprintf("whatsapp:test:%d", current(r).ID), 3, time.Hour); e != nil {
			return nil, e
		}
		e = waQueue(ctx, a.DB, "test:"+token(), current(r).ID, phone, "test", waParams(current(r).Name, "Connection check", "Requested test; no company work records included", a.Origin+"/developer"))
		if e != nil {
			return nil, e
		}
	default:
		return nil, bad("Unknown WhatsApp action.")
	}
	if e = accessAudit(ctx, a.DB, current(r).ID, "WhatsApp "+action, "User", current(r).ID, M{"kind": str(d, "kind"), "subject_user": id(d, "user_id")}); e != nil {
		return nil, e
	}
	return M{"ok": true}, nil
}
func waQueue(ctx context.Context, q queryer, key string, uid int64, phone, kind string, params []string) error {
	b, _ := json.Marshal(params)
	_, e := q.Exec(ctx, `INSERT INTO whatsapp_messages(event_key,user_id,phone,kind,parameters) VALUES($1,$2,$3,$4,$5) ON CONFLICT(event_key) DO NOTHING`, key, uid, phone, kind, b)
	return e
}
func waSignatureValid(raw []byte, signature, secret string) bool {
	if secret == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(raw)
	got, e := hex.DecodeString(strings.TrimPrefix(signature, "sha256="))
	return e == nil && strings.HasPrefix(signature, "sha256=") && hmac.Equal(got, mac.Sum(nil))
}
func (a *App) whatsappWebhook(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		want := os.Getenv("WHATSAPP_VERIFY_TOKEN")
		if want != "" && r.URL.Query().Get("hub.mode") == "subscribe" && subtle.ConstantTimeCompare([]byte(want), []byte(r.URL.Query().Get("hub.verify_token"))) == 1 {
			w.Header().Set("Content-Type", "text/plain")
			io.WriteString(w, r.URL.Query().Get("hub.challenge"))
			return
		}
		http.Error(w, "Verification failed", 403)
		return
	}
	raw, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 1<<20))
	if e != nil {
		http.Error(w, "Invalid payload", 400)
		return
	}
	if !waSignatureValid(raw, r.Header.Get("X-Hub-Signature-256"), os.Getenv("WHATSAPP_APP_SECRET")) {
		http.Error(w, "Invalid signature", 403)
		return
	}
	var payload struct {
		Entry []struct {
			ID      string `json:"id"`
			Changes []struct {
				Value struct {
					Statuses []struct {
						ID, Status string
						Errors     []struct {
							Code int `json:"code"`
						} `json:"errors"`
					} `json:"statuses"`
					Messages []struct {
						From string
						Text struct{ Body string } `json:"text"`
					} `json:"messages"`
				} `json:"value"`
			} `json:"changes"`
		} `json:"entry"`
	}
	if json.Unmarshal(raw, &payload) != nil {
		http.Error(w, "Invalid payload", 400)
		return
	}
	ctx := context.WithValue(r.Context(), serviceKey{}, "whatsapp")
	for _, en := range payload.Entry {
		if en.ID != os.Getenv("WHATSAPP_WABA_ID") {
			continue
		}
		for _, c := range en.Changes {
			for _, s := range c.Value.Statuses {
				status := map[string]string{"sent": "Sent", "delivered": "Delivered", "read": "Read", "failed": "Failed"}[s.Status]
				if status == "" {
					continue
				}
				errText := ""
				if len(s.Errors) > 0 {
					errText = "Meta delivery error " + strconv.Itoa(s.Errors[0].Code)
				}
				_, e = a.DB.Exec(ctx, `UPDATE whatsapp_messages SET status=$1,error=$2,updated_at=now() WHERE meta_id=$3 AND status<>'Read' AND NOT (status='Delivered' AND $1<>'Read') AND NOT (status='Sent' AND $1='Accepted')`, status, errText, s.ID)
				if e != nil {
					http.Error(w, "Retry later", 503)
					return
				}
			}
			for _, m := range c.Value.Messages {
				if strings.EqualFold(strings.TrimSpace(m.Text.Body), "STOP") {
					_, e = a.DB.Exec(ctx, `UPDATE whatsapp_recipients r SET enabled=false,updated_at=now() FROM users u WHERE r.user_id=u.id AND regexp_replace(u.phone,'[^0-9]','','g')=$1`, m.From)
					if e != nil {
						http.Error(w, "Retry later", 503)
						return
					}
				}
			}
		}
	}
	writeJSON(w, 200, M{"ok": true})
}
