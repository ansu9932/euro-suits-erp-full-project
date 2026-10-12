package app

import (
	"crypto/rand"
	"encoding/json"
	"eurosuits/erp/internal/brandedmail"
	"fmt"
	"math/big"
	"net/http"
	"regexp"
	"strings"
	"time"
)

type otpChallenge struct {
	UserID   int64  `json:"user_id"`
	Version  int    `json:"version"`
	CodeHash string `json:"code_hash"`
	Attempts int    `json:"attempts"`
}

func (a *App) startOTP(w http.ResponseWriter, r *http.Request, u *User) (any, error) {
	if !validEmail(u.Email) || !emailConfigured() {
		return nil, failure{503, "Email verification is required. The developer must configure the company email sender before you can sign in."}
	}
	if e := a.limit(r.Context(), fmt.Sprintf("otp:send:%d", u.ID), 5, time.Hour); e != nil {
		return nil, e
	}
	ready, e := a.Redis.SetNX(r.Context(), fmt.Sprintf("otp:cooldown:%d", u.ID), "1", time.Minute).Result()
	if e != nil {
		return nil, e
	}
	if !ready {
		return nil, failure{429, "Please wait one minute before requesting another code."}
	}
	number, e := rand.Int(rand.Reader, big.NewInt(1000000))
	if e != nil {
		return nil, e
	}
	code := fmt.Sprintf("%06d", number.Int64())
	challenge := token()
	key := "otp:challenge:" + digest(challenge)
	raw, _ := json.Marshal(otpChallenge{UserID: u.ID, Version: u.Version, CodeHash: digest(challenge + code)})
	// One pending code per account. Replacement and consumption are atomic.
	_, e = a.Redis.Eval(r.Context(), `local old=redis.call('GET',KEYS[1]); if old then redis.call('DEL',old) end; redis.call('SET',KEYS[1],KEYS[2],'EX',300); redis.call('SET',KEYS[2],ARGV[1],'EX',300); return 1`, []string{fmt.Sprintf("otp:pending:%d", u.ID), key}, string(raw)).Result()
	if e != nil {
		return nil, e
	}
	if e = sendBrandedMail(u.Email, brandedmail.SignIn(code)); e != nil {
		a.Redis.Del(r.Context(), key)
		a.recordIssue(r.Context(), "Email", "Error", "Sign-in email could not be sent", "/api/login", 503, u.ID, "")
		return nil, failure{503, "The verification email could not be sent. Check the company email configuration."}
	}
	http.SetCookie(w, &http.Cookie{Name: "erp_otp", Value: challenge, Path: "/api/login", HttpOnly: true, Secure: a.Production, SameSite: http.SameSiteStrictMode, MaxAge: 300})
	parts := strings.SplitN(u.Email, "@", 2)
	masked := string(parts[0][0]) + "***@" + parts[1]
	return M{"otp_required": true, "email_hint": masked, "expires_in": 300}, nil
}

func (a *App) verifyOTP(w http.ResponseWriter, r *http.Request) (any, error) {
	if e := a.limit(r.Context(), "otp:peer:"+digest(clientIP(r)), a.Capacity.OTPPeerLimit, 15*time.Minute); e != nil {
		return nil, e
	}
	d, e := readBody(w, r)
	if e != nil {
		return nil, e
	}
	if e = allowedFields(d, "code"); e != nil {
		return nil, e
	}
	c, e := r.Cookie("erp_otp")
	if e != nil || len(c.Value) > 100 {
		return nil, failure{401, "Verification expired. Sign in with your password again."}
	}
	code := str(d, "code")
	if !regexp.MustCompile(`^[0-9]{6}$`).MatchString(code) {
		return nil, bad("Enter the 6-digit email code.")
	}
	raw, e := a.Redis.Eval(r.Context(), `local raw=redis.call('GET',KEYS[1]); if not raw then return '' end; local v=cjson.decode(raw); v.attempts=v.attempts+1; if v.code_hash==ARGV[1] then redis.call('DEL',KEYS[1]); return raw end; if v.attempts>=5 then redis.call('DEL',KEYS[1]) else redis.call('SET',KEYS[1],cjson.encode(v),'KEEPTTL') end; return ''`, []string{"otp:challenge:" + digest(c.Value)}, digest(c.Value+code)).Text()
	if e != nil {
		return nil, e
	}
	if raw == "" {
		return nil, failure{401, "Invalid or expired code. After five incorrect attempts, sign in again."}
	}
	var challenge otpChallenge
	if e = json.Unmarshal([]byte(raw), &challenge); e != nil {
		return nil, e
	}
	u, e := a.loadUser(r.Context(), challenge.UserID)
	if e != nil || !u.Active || u.Version != challenge.Version {
		return nil, failure{401, "Your account changed. Sign in again."}
	}
	http.SetCookie(w, &http.Cookie{Name: "erp_otp", Path: "/api/login", MaxAge: -1, HttpOnly: true, Secure: a.Production, SameSite: http.SameSiteStrictMode})
	return a.issueSession(w, r, u, true)
}
