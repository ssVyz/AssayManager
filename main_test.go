package main

import (
	"bytes"
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"AssayManager/internal/config"
)

// TestRunLogsSessionLifecycle verifies that run logs an explicit session-start
// event and, on graceful shutdown (context cancellation, as Ctrl+C / SIGTERM
// would trigger), a session-stop event.
func TestRunLogsSessionLifecycle(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))

	cfg := config.Config{
		Addr:           "127.0.0.1:0", // random free port
		DBPath:         ":memory:",
		MaxUploadBytes: 1 << 20,
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(150 * time.Millisecond) // let the listener bind
		cancel()
	}()

	if err := run(ctx, cfg, logger); err != nil {
		t.Fatalf("run returned error: %v", err)
	}

	out := buf.String()
	for _, want := range []string{"server session started", "server session stopped"} {
		if !strings.Contains(out, want) {
			t.Errorf("log output missing %q; got:\n%s", want, out)
		}
	}
}
