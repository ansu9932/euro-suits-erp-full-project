package app

import (
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"sync"
	"time"
)

type CapacityConfig struct {
	DBConnections         int   `json:"database_connections"`
	LoginConcurrency      int   `json:"parallel_password_checks"`
	LoginPeerLimit        int64 `json:"office_logins_per_15_minutes"`
	OTPPeerLimit          int64 `json:"office_otp_checks_per_15_minutes"`
	UserRequestsPerMinute int64 `json:"requests_per_user_per_minute"`
}

func readCapacityConfig() (CapacityConfig, error) {
	c := CapacityConfig{}
	settings := []struct {
		key                string
		fallback, min, max int
		target             *int64
	}{
		{"LOGIN_PEER_LIMIT", 2000, 240, 10000, &c.LoginPeerLimit},
		{"OTP_PEER_LIMIT", 3000, 60, 15000, &c.OTPPeerLimit},
		{"USER_REQUESTS_PER_MINUTE", 300, 60, 1200, &c.UserRequestsPerMinute},
	}
	for _, s := range settings {
		v, e := boundedEnv(s.key, s.fallback, s.min, s.max)
		if e != nil {
			return c, e
		}
		*s.target = int64(v)
	}
	var e error
	c.DBConnections, e = boundedEnv("DB_MAX_CONNECTIONS", 24, 4, 64)
	if e != nil {
		return c, e
	}
	c.LoginConcurrency, e = boundedEnv("LOGIN_CONCURRENCY", 4, 1, 8)
	return c, e
}
func boundedEnv(key string, fallback, min, max int) (int, error) {
	raw := os.Getenv(key)
	if raw == "" {
		return fallback, nil
	}
	n, e := strconv.Atoi(raw)
	if e != nil || n < min || n > max {
		return 0, fmt.Errorf("%s must be between %d and %d", key, min, max)
	}
	return n, nil
}

// Bounded admission avoids thousands of requests waiting for the database pool.
func (a *App) admission(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case a.RequestSlots <- struct{}{}:
			defer func() { <-a.RequestSlots }()
		default:
			w.Header().Set("Retry-After", "2")
			a.handle(func(http.ResponseWriter, *http.Request) (any, error) {
				return nil, failure{503, "The service is busy. Please try again shortly."}
			})(w, r)
			return
		}
		next.ServeHTTP(w, r)
	})
}

var latencyBounds = [...]int64{50, 100, 250, 500, 1000, 2000, 5000, 10000}

type performanceBucket struct {
	Second                                          int64
	Requests, ServerErrors, Rejected, Limited, Slow int64
	DurationMicros                                  int64
	Histogram                                       [9]int64
}
type performanceTracker struct {
	mu      sync.Mutex
	buckets [60]performanceBucket
	active  int64
}

func (p *performanceTracker) begin() { p.mu.Lock(); p.active++; p.mu.Unlock() }
func (p *performanceTracker) finish(now time.Time, elapsed time.Duration, status int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.active--
	slot := now.Unix() % 60
	b := &p.buckets[slot]
	if b.Second != now.Unix() {
		*b = performanceBucket{Second: now.Unix()}
	}
	b.Requests++
	b.DurationMicros += elapsed.Microseconds()
	if status >= 500 {
		b.ServerErrors++
	}
	if status >= 400 && status < 500 {
		b.Rejected++
	}
	if status == 429 {
		b.Limited++
	}
	if elapsed >= time.Second {
		b.Slow++
	}
	i := 0
	for i < len(latencyBounds) && elapsed > time.Duration(latencyBounds[i])*time.Millisecond {
		i++
	}
	b.Histogram[i]++
}
func (p *performanceTracker) snapshot(now time.Time) M {
	p.mu.Lock()
	defer p.mu.Unlock()
	total := performanceBucket{}
	for _, b := range p.buckets {
		if b.Second <= now.Unix()-60 || b.Second > now.Unix() {
			continue
		}
		total.Requests += b.Requests
		total.DurationMicros += b.DurationMicros
		total.ServerErrors += b.ServerErrors
		total.Rejected += b.Rejected
		total.Limited += b.Limited
		total.Slow += b.Slow
		for i, n := range b.Histogram {
			total.Histogram[i] += n
		}
	}
	mean := float64(0)
	p95 := "No requests"
	if total.Requests > 0 {
		mean = float64(total.DurationMicros) / float64(total.Requests) / 1000
		var seen int64
		for i, n := range total.Histogram {
			seen += n
			if seen*100 >= total.Requests*95 {
				if i < len(latencyBounds) {
					p95 = fmt.Sprintf("≤ %d ms", latencyBounds[i])
				} else {
					p95 = "> 10000 ms"
				}
				break
			}
		}
	}
	return M{"window_seconds": 60, "requests": total.Requests, "requests_per_second": float64(total.Requests) / 60, "active": p.active, "mean_ms": mean, "p95_bucket": p95, "server_errors": total.ServerErrors, "rejected": total.Rejected, "rate_limited": total.Limited, "slow_requests": total.Slow}
}
func (a *App) performanceData(w http.ResponseWriter, r *http.Request) (any, error) {
	if e := developerRequired(r); e != nil {
		return nil, e
	}
	s := a.DB.Stat()
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	worker, _ := a.Redis.Get(r.Context(), "worker:last_success").Int64()
	workerAge := int64(-1)
	if worker > 0 {
		workerAge = time.Now().Unix() - worker
	}
	return M{
		"observed_at": time.Now().UTC(), "uptime_seconds": int64(time.Since(a.StartedAt).Seconds()),
		"traffic":     a.Performance.snapshot(time.Now()),
		"database":    M{"max": s.MaxConns(), "total": s.TotalConns(), "in_use": s.AcquiredConns(), "idle": s.IdleConns(), "acquires": s.AcquireCount(), "waited_acquires": s.EmptyAcquireCount(), "cancelled_acquires": s.CanceledAcquireCount(), "wait_ms_total": s.EmptyAcquireWaitTime().Milliseconds()},
		"process":     M{"heap_mb": float64(mem.HeapAlloc) / 1024 / 1024, "reserved_mb": float64(mem.Sys) / 1024 / 1024, "goroutines": runtime.NumGoroutine()},
		"diagnostics": M{"queued": len(a.IssueQueue), "capacity": cap(a.IssueQueue), "dropped_total": a.IssuesDropped.Load(), "write_failures_total": a.IssueWriteFailures.Load()},
		"worker":      M{"last_success_unix": worker, "age_seconds": workerAge, "healthy": workerAge >= 0 && workerAge < 180},
		"limits":      a.Capacity,
	}, nil
}
