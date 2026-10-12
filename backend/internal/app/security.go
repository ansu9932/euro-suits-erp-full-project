package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

// Reject ambiguous duplicate keys before a map or struct decoder can silently
// replace a field. Bound individual strings, collections and nesting as well.
func jsonValue(dec *json.Decoder, depth int) (any, error) {
	if depth > 16 {
		return nil, bad("JSON is nested too deeply.")
	}
	token, err := dec.Token()
	if err != nil {
		return nil, bad("Invalid JSON request.")
	}
	switch value := token.(type) {
	case json.Delim:
		switch value {
		case '{':
			result := M{}
			for dec.More() {
				key, e := dec.Token()
				if e != nil {
					return nil, bad("Invalid JSON object.")
				}
				name, ok := key.(string)
				if !ok || len(name) > 100 {
					return nil, bad("Invalid field name.")
				}
				if _, exists := result[name]; exists {
					return nil, bad("Duplicate JSON field: " + name)
				}
				if len(result) >= 500 {
					return nil, bad("Too many fields.")
				}
				v, e := jsonValue(dec, depth+1)
				if e != nil {
					return nil, e
				}
				result[name] = v
			}
			if _, err = dec.Token(); err != nil {
				return nil, bad("Invalid JSON object.")
			}
			return result, nil
		case '[':
			result := []any{}
			for dec.More() {
				if len(result) >= 1000 {
					return nil, bad("Too many list entries.")
				}
				v, e := jsonValue(dec, depth+1)
				if e != nil {
					return nil, e
				}
				result = append(result, v)
			}
			if _, err = dec.Token(); err != nil {
				return nil, bad("Invalid JSON list.")
			}
			return result, nil
		}
		return nil, bad("Invalid JSON request.")
	case string:
		if len(value) > 20000 || !utf8.ValidString(value) || strings.ContainsRune(value, 0) {
			return nil, bad("Text is too long or contains invalid characters.")
		}
	}
	return token, nil
}
func allowedFields(d M, allowed ...string) error {
	for name := range d {
		found := false
		for _, key := range allowed {
			if name == key {
				found = true
				break
			}
		}
		if !found {
			return bad("Unsupported field: " + name)
		}
	}
	return nil
}
func clientIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		ip = r.RemoteAddr
	}
	// Only enabled by the private Docker deployment. Nginx overwrites this
	// header; never expose the API port when this setting is enabled.
	if os.Getenv("TRUST_PROXY") == "true" {
		if forwarded := net.ParseIP(r.Header.Get("X-Real-IP")); forwarded != nil {
			return forwarded.String()
		}
	}
	return ip
}
func (a *App) limit(ctx context.Context, key string, max int64, window time.Duration) error {
	n, err := a.Redis.Eval(ctx, `local n=redis.call('INCR',KEYS[1]);if n==1 then redis.call('EXPIRE',KEYS[1],ARGV[1]) end;return n`, []string{key}, int64(window.Seconds())).Int64()
	if err != nil {
		return failure{503, "Security service is temporarily unavailable. Please try again."}
	}
	if n > max {
		return failure{429, "Too many attempts. Please wait and try again."}
	}
	return nil
}
func (a *App) loginConfig(w http.ResponseWriter, r *http.Request) (any, error) {
	return M{"turnstile_site_key": os.Getenv("TURNSTILE_SITE_KEY")}, nil
}

type botResult struct {
	Success     bool   `json:"success"`
	Hostname    string `json:"hostname"`
	Action      string `json:"action"`
	ChallengeTS string `json:"challenge_ts"`
}

func validBotResult(result botResult, hostname string, now time.Time) bool {
	when, err := time.Parse(time.RFC3339, result.ChallengeTS)
	return err == nil && result.Success && result.Hostname == hostname && result.Action == "erp-login" && now.Sub(when) >= -time.Minute && now.Sub(when) <= 5*time.Minute
}
func (a *App) verifyBot(r *http.Request, token string) error {
	secret := os.Getenv("TURNSTILE_SECRET_KEY")
	if secret == "" {
		if a.Production {
			return failure{503, "Sign-in verification is not configured."}
		}
		return nil
	}
	if len(token) < 1 || len(token) > 2048 {
		return bad("Complete the sign-in verification and try again.")
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	form := url.Values{"secret": {secret}, "response": {token}, "remoteip": {clientIP(r)}}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://challenges.cloudflare.com/turnstile/v0/siteverify", strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	client := &http.Client{Timeout: 8 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	res, err := client.Do(req)
	if err != nil {
		return failure{503, "Sign-in verification is unavailable. Please retry."}
	}
	defer res.Body.Close()
	var result botResult
	origin, _ := url.Parse(a.Origin)
	if res.StatusCode != 200 || json.NewDecoder(io.LimitReader(res.Body, 16384)).Decode(&result) != nil || !validBotResult(result, origin.Hostname(), time.Now()) {
		return bad("Sign-in verification expired or failed. Please retry.")
	}
	return nil
}
func validateSecurityConfig(origin string, production bool) error {
	u, e := url.Parse(origin)
	if e != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" || (u.Scheme != "http" && u.Scheme != "https") {
		return errors.New("APP_ORIGIN must be a valid origin without a path")
	}
	if production && u.Scheme != "https" {
		return errors.New("production requires HTTPS")
	}
	if _, e = companyCipher(); e != nil {
		return errors.New("a valid KYC_ENCRYPTION_KEY is required")
	}
	site, secret := os.Getenv("TURNSTILE_SITE_KEY"), os.Getenv("TURNSTILE_SECRET_KEY")
	if (site == "") != (secret == "") {
		return errors.New("configure both Turnstile keys")
	}
	if production && (site == "" || strings.HasPrefix(site, "1x000") || strings.HasPrefix(site, "2x000") || strings.HasPrefix(site, "3x000") || strings.Contains(secret, "AAAAAAAAAA")) {
		return errors.New("production requires real Turnstile keys; test keys are forbidden")
	}
	return nil
}
func requestMediaType(value string) string {
	kind, _, err := mime.ParseMediaType(value)
	if err != nil {
		return ""
	}
	return kind
}
