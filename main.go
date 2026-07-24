package main

import (
	"context"
	"fmt"
	"io"
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
const Version = "0.3.0"

func main() {
	// Load .env first so it can supply any AM_* setting (real env vars win).
	envLoaded, envErr := config.LoadEnvFile(".env")
	cfg := config.Load()

	// Log to both the console and an append-only file, so session history
	// accumulates across restarts.
	logFile, err := os.OpenFile(cfg.LogPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot open log file %q: %v\n", cfg.LogPath, err)
		os.Exit(1)
	}
	defer logFile.Close()
	logger := slog.New(slog.NewTextHandler(io.MultiWriter(os.Stdout, logFile), nil))

	if envErr != nil {
		logger.Warn("could not fully read .env", "err", envErr)
	}
	if envLoaded > 0 {
		logger.Info(".env loaded", "vars", envLoaded)
	}
	if cfg.NCBIEmail != "" {
		// The email is a contact address (not secret); the API key is never logged.
		logger.Info("NCBI contact configured", "email", cfg.NCBIEmail)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, cfg, logger); err != nil {
		logger.Error("fatal", "err", err)
		os.Exit(1)
	}
}

// run starts the server and blocks until ctx is cancelled (Ctrl+C / SIGTERM),
// then shuts down gracefully. Session start/stop are logged so the log file
// records each run's lifecycle.
func run(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	st, err := store.Open(cfg.DBPath)
	if err != nil {
		return fmt.Errorf("open database %q: %w", cfg.DBPath, err)
	}
	defer st.Close()

	sessions := auth.NewManager(24 * time.Hour)
	analyzer := analysis.NewCLI(cfg.InclusivityBin, cfg.AnalysisTimeout, cfg.NCBIEmail, cfg.NCBITool, logger)

	srv, err := web.New(cfg, logger, st, sessions, analyzer)
	if err != nil {
		return fmt.Errorf("init server: %w", err)
	}

	httpSrv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}

	logger.Info("server session started",
		"version", Version, "addr", cfg.Addr, "db", cfg.DBPath, "log", cfg.LogPath, "pid", os.Getpid())

	// Recurring-job scheduler runs until the context is cancelled (shutdown).
	srv.StartScheduler(ctx)

	serveErr := make(chan error, 1)
	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serveErr <- err
		}
	}()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received, stopping")
	case err := <-serveErr:
		return fmt.Errorf("http server: %w", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown", "err", err)
	}
	logger.Info("server session stopped")
	return nil
}
