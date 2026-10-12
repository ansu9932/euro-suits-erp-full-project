package main

import (
	"context"
	"eurosuits/erp/internal/app"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	a, e := app.New(ctx)
	if e != nil {
		slog.Error("startup failed", "error", e)
		os.Exit(1)
	}
	defer a.DB.Close()
	defer a.Redis.Close()
	if e = a.Migrate(ctx); e != nil {
		slog.Error("migration failed", "error", e)
		os.Exit(1)
	}
	if len(os.Args) > 1 && os.Args[1] == "migrate" {
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "reset-company-workspace" {
		if e = a.ResetCompanyWorkspace(ctx); e != nil {
			slog.Error("reset failed", "error", e)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "create-developer" {
		if e = a.CreateDeveloper(ctx); e != nil {
			slog.Error("developer setup failed", "error", e)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "create-owner" {
		if e = a.CreateOwner(ctx); e != nil {
			slog.Error("owner setup failed", "error", e)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "initialize-company-workspace" {
		if e = a.InitializeCompanyWorkspace(ctx); e != nil {
			slog.Error("company workspace initialization failed", "error", e)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "set-local-test-passwords" {
		if e = a.SetLocalTestPasswords(ctx); e != nil {
			slog.Error("local password update failed", "error", e)
			os.Exit(1)
		}
		return
	}
	if os.Getenv("DEMO_SEED") == "true" {
		if a.Production {
			slog.Error("demo seed forbidden in production")
			os.Exit(1)
		}
		if e = a.Seed(ctx); e != nil {
			slog.Error("demo seed failed", "error", e)
			os.Exit(1)
		}
		if e = a.EnsureDemoUsers(ctx); e != nil {
			slog.Error("demo account setup failed", "error", e)
			os.Exit(1)
		}
	}
	if e = a.EnableRuntimeSecurity(ctx); e != nil {
		slog.Error("unsafe database configuration", "error", e)
		os.Exit(1)
	}
	diagnosticsDone := make(chan struct{})
	go func() { defer close(diagnosticsDone); a.DiagnosticsWorker(ctx) }()
	defer func() { stop(); <-diagnosticsDone }()
	go a.Worker(ctx)
	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8081"
	}
	srv := &http.Server{Addr: addr, Handler: a.Router(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 45 * time.Second, WriteTimeout: 45 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	go func() {
		<-ctx.Done()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		srv.Shutdown(shutdown)
	}()
	slog.Info("ERP API started", "address", addr)
	if e = srv.ListenAndServe(); e != nil && e != http.ErrServerClosed {
		slog.Error("server failed", "error", e)
		os.Exit(1)
	}
}
