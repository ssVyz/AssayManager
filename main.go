package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"AssayManager/internal/analysis"
	"AssayManager/internal/auth"
	"AssayManager/internal/config"
	"AssayManager/internal/store"
	"AssayManager/internal/web"
)

// Version is the authoritative version of AssayManager (semantic versioning).
//
// Bump rules:
//   - PATCH: any agent that changes code bumps this.
//   - MINOR / MAJOR: humans only, on explicit request.
//
// Keep this in sync with the latest entry in CHANGELOG.md.
const Version = "0.1.2"

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	cfg := config.Load()

	st, err := store.Open(cfg.DBPath)
	if err != nil {
		logger.Error("open database", "path", cfg.DBPath, "err", err)
		os.Exit(1)
	}
	defer st.Close()

	sessions := auth.NewManager(24 * time.Hour)
	analyzer := analysis.Stub{}

	srv, err := web.New(cfg, logger, st, sessions, analyzer)
	if err != nil {
		logger.Error("init server", "err", err)
		os.Exit(1)
	}

	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		logger.Info("AssayManager starting", "version", Version, "addr", cfg.Addr, "db", cfg.DBPath)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http server", "err", err)
			os.Exit(1)
		}
	}()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	<-ctx.Done()

	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown", "err", err)
	}
}
