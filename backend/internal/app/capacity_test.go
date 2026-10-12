package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestCapacityConfigBounds(t *testing.T) {
	for _, k := range []string{"DB_MAX_CONNECTIONS", "LOGIN_CONCURRENCY", "LOGIN_PEER_LIMIT", "OTP_PEER_LIMIT", "USER_REQUESTS_PER_MINUTE"} {
		t.Setenv(k, "")
	}
	c, e := readCapacityConfig()
	if e != nil || c.DBConnections != 24 || c.LoginPeerLimit < 500 || c.OTPPeerLimit < 500 {
		t.Fatalf("%+v %v", c, e)
	}
	for _, bad := range []string{"0", "65", "-1", "abc"} {
		t.Setenv("DB_MAX_CONNECTIONS", bad)
		if _, e := readCapacityConfig(); e == nil {
			t.Fatal("accepted unsafe DB limit", bad)
		}
	}
}
func TestPerformanceWindowConcurrent(t *testing.T) {
	p := &performanceTracker{}
	now := time.Unix(120, 0)
	var wg sync.WaitGroup
	for i := 0; i < 500; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); p.begin(); p.finish(now, 200*time.Millisecond, 200) }()
	}
	wg.Wait()
	p.begin()
	p.finish(now, time.Second, 429)
	s := p.snapshot(now)
	if s["requests"] != int64(501) || s["rate_limited"] != int64(1) || s["active"] != int64(0) || s["p95_bucket"] != "≤ 250 ms" {
		t.Fatal(s)
	}
	if p.snapshot(now.Add(60 * time.Second))["requests"] != int64(0) {
		t.Fatal("expired window retained")
	}
}
func TestIssueQueueBoundedAndCounts(t *testing.T) {
	a := &App{IssueQueue: make(chan issueEvent, 2)}
	for i := 0; i < 500; i++ {
		a.recordIssue(context.Background(), "Security", "Warning", "Rejected", "POST /api/login", 429, 0, "")
	}
	if len(a.IssueQueue) != 2 || a.IssuesDropped.Load() != 498 {
		t.Fatal("queue did not bound memory")
	}
	events := aggregateIssues([]issueEvent{<-a.IssueQueue, <-a.IssueQueue})
	if len(events) != 1 || events[0].Count != 2 {
		t.Fatal(events)
	}
}
func TestPerformanceDeveloperOnly(t *testing.T) {
	// No database is needed: unauthorised users must be rejected before collecting metrics.
	a := &App{}
	r := httptest.NewRequest("GET", "/api/developer/performance", nil)
	r = r.WithContext(context.WithValue(r.Context(), authKey{}, &User{Role: "Super Admin"}))
	_, e := a.performanceData(httptest.NewRecorder(), r)
	var f failure
	if !errors.As(e, &f) || f.Code != 403 {
		t.Fatalf("non-developer admitted: %v", e)
	}
}
func TestAdmissionOverloadReleasesSlots(t *testing.T) {
	a := &App{RequestSlots: make(chan struct{}, 1)}
	called := false
	h := a.admission(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true; w.WriteHeader(204) }))
	a.RequestSlots <- struct{}{}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if called || w.Code != 503 || w.Header().Get("Retry-After") == "" {
		t.Fatal("overload not rejected")
	}
	<-a.RequestSlots
	w = httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if !called || w.Code != 204 || len(a.RequestSlots) != 0 {
		t.Fatal("slot leaked")
	}
}
