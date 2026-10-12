package app

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

type observationKey struct{}
type observation struct{ userID int64 }

var safeRequestID = regexp.MustCompile(`^[a-zA-Z0-9_./-]{1,150}$`)

func (a *App) observe(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		if a.Performance != nil {
			a.Performance.begin()
		}
		entry := &observation{}
		r = r.WithContext(context.WithValue(r.Context(), observationKey{}, entry))
		wrapped := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		defer func() {
			status := wrapped.Status()
			if status == 0 {
				status = 200
			}
			if a.Performance != nil {
				a.Performance.finish(time.Now(), time.Since(started), status)
			}
			if status < 400 {
				return
			}
			route := chi.RouteContext(r.Context()).RoutePattern()
			if route == "" {
				route = "unmatched endpoint"
			}
			category, title, severity := "Validation", "Request rejected; check submitted values", "Info"
			switch {
			case status >= 500:
				category, title, severity = "Server", "Request failed; inspect server logs using the request ID", "Error"
			case status == 401 || status == 403 || status == 429:
				category, title, severity = "Security", "Rejected sign-in, access check or rate limit", "Warning"
			case status == 423:
				category, title, severity = "Workflow", "Dashboard blocked by overdue renewal", "Warning"
			case status == 409:
				category, title = "Workflow", "Concurrent edit or duplicate reference"
			case strings.Contains(route, "/files") || strings.Contains(route, "/documents"):
				category, title, severity = "Upload", "File upload or download failed", "Warning"
			}
			requestID := middleware.GetReqID(r.Context())
			if !safeRequestID.MatchString(requestID) {
				requestID = ""
			}
			a.recordIssue(r.Context(), category, severity, title, r.Method+" "+route, status, entry.userID, requestID)
		}()
		next.ServeHTTP(wrapped, r)
	})
}

type issueEvent struct {
	Fingerprint string `json:"fingerprint"`
	Category    string `json:"category"`
	Severity    string `json:"severity"`
	Title       string `json:"title"`
	Route       string `json:"route"`
	Status      int    `json:"http_status"`
	UserID      int64  `json:"user_id"`
	RequestID   string `json:"request_id"`
	Count       int64  `json:"occurrences"`
}

func (a *App) recordIssue(_ context.Context, category, severity, title, route string, status int, uid int64, requestID string) {
	// Only fixed descriptions and route templates. Never enqueue request bodies or secrets.
	event := issueEvent{digest(fmt.Sprintf("%s|%s|%d|%d", category, route, status, uid)), category, severity, title, route, status, uid, requestID, 1}
	select {
	case a.IssueQueue <- event:
	default:
		a.IssuesDropped.Add(1)
	}
}
func aggregateIssues(events []issueEvent) []issueEvent {
	byKey := make(map[string]int, len(events))
	result := make([]issueEvent, 0, len(events))
	for _, e := range events {
		if i, ok := byKey[e.Fingerprint]; ok {
			result[i].Count += e.Count
			result[i].RequestID = e.RequestID
		} else {
			byKey[e.Fingerprint] = len(result)
			result = append(result, e)
		}
	}
	return result
}
func (a *App) flushIssues(events []issueEvent) {
	if len(events) == 0 {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithValue(context.Background(), serviceKey{}, "diagnostics"), 2*time.Second)
	defer cancel()
	raw, _ := json.Marshal(aggregateIssues(events))
	_, e := a.DB.Exec(ctx, `INSERT INTO system_issues(fingerprint,category,severity,title,route,http_status,user_id,request_id,occurrences)
 SELECT fingerprint,category,severity,title,route,http_status,NULLIF(user_id,0),request_id,occurrences
 FROM jsonb_to_recordset($1::jsonb) AS x(fingerprint text,category text,severity text,title text,route text,http_status integer,user_id bigint,request_id text,occurrences integer)
 ON CONFLICT(fingerprint) DO UPDATE SET occurrences=system_issues.occurrences+excluded.occurrences,last_seen=now(),request_id=excluded.request_id,status='Open',version=system_issues.version+1`, string(raw))
	if e != nil {
		a.IssueWriteFailures.Add(1)
		a.IssuesDropped.Add(int64(len(events)))
	}
}
func (a *App) DiagnosticsWorker(ctx context.Context) {
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			a.drainIssues()
			return
		case <-tick.C:
			a.drainIssues()
		}
	}
}
func (a *App) drainIssues() {
	// One bounded batch per tick prevents an error storm from monopolising the DB.
	events := make([]issueEvent, 0, 1024)
	for len(events) < 1024 {
		select {
		case event := <-a.IssueQueue:
			events = append(events, event)
		default:
			a.flushIssues(events)
			return
		}
	}
	a.flushIssues(events)
}
func (a *App) clientIssue(w http.ResponseWriter, r *http.Request) (any, error) {
	if e := a.limit(r.Context(), fmt.Sprintf("client-issue:%d", current(r).ID), 20, time.Hour); e != nil {
		return nil, e
	}
	d, e := readBody(w, r)
	if e != nil {
		return nil, e
	}
	if e = allowedFields(d, "kind", "page"); e != nil {
		return nil, e
	}
	kind, page := str(d, "kind"), str(d, "page")
	if !slices.Contains([]string{"runtime", "promise", "network", "upload"}, kind) {
		return nil, bad("Unknown browser issue type.")
	}
	if !regexp.MustCompile(`^/[a-zA-Z/-]{0,80}$`).MatchString(page) {
		page = "/workspace"
	}
	a.recordIssue(r.Context(), "Browser", "Warning", "Browser-reported "+kind+" issue (needs investigation)", page, 0, current(r).ID, "")
	return M{"ok": true}, nil
}
