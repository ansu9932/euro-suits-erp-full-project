package app

import (
	"context"
	"fmt"
	"net/http"
	"regexp"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

var requestKeyPattern = regexp.MustCompile(`^[a-zA-Z0-9_-][a-zA-Z0-9_-]{15,79}$`)

// The browser supplies a fresh key for each intended mutation. A repeated key
// cannot execute again, across tabs or API processes. Store only an opaque
// reservation, never passwords, OTPs, cookies, uploads or response bodies.
func (a *App) preventDuplicateRequest(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestKey := r.Header.Get("Idempotency-Key")
		// Older clients keep their existing optimistic-lock/OTP protections.
		if requestKey == "" || r.Method == http.MethodGet || r.Method == http.MethodHead || r.URL.Path == "/api/client-issues" {
			next.ServeHTTP(w, r)
			return
		}
		if !requestKeyPattern.MatchString(requestKey) {
			writeJSON(w, 400, M{"error": "Invalid action identifier. Refresh and try again."})
			return
		}
		scope := "login:" + clientIP(r)
		if user, ok := r.Context().Value(authKey{}).(*User); ok {
			scope = fmt.Sprintf("user:%d", user.ID)
		}
		key := "action:" + digest(scope+":"+r.Method+":"+r.URL.Path+":"+requestKey)
		reserved, err := a.Redis.SetNX(r.Context(), key, "reserved", 24*time.Hour).Result()
		if err != nil {
			writeJSON(w, 503, M{"error": "Action protection is temporarily unavailable. Please try again shortly."})
			return
		}
		if !reserved {
			writeJSON(w, 409, M{"error": "This action was already submitted. Wait for it to finish, then refresh to check the result.", "code": "action_already_submitted"})
			return
		}
		response := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(response, r)
		// Validation/auth failures can be corrected and retried. Keep successful
		// and uncertain (5xx/disconnected/crashed) attempts reserved to avoid
		// executing a side effect again after losing its response.
		if response.Status() >= 400 && response.Status() < 500 {
			ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 2*time.Second)
			defer cancel()
			a.Redis.Del(ctx, key)
		}
	})
}
