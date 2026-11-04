package app_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/uloydev/modulaengine/internal/app"
	"github.com/uloydev/modulaengine/internal/config"
)

func TestApp_Lifecycle_Standalone(t *testing.T) {
	t.Parallel()

	cfg := &config.Config{
		Server: config.ServerConfig{
			Port:     0, // random available OS port
			Headless: true,
			Env:      "development",
		},
		Security: config.SecurityConfig{
			JWTSecret: []byte("test-32bytes-secret-for-jwt-signing"),
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	application, err := app.New(ctx, cfg, "v1.0.0-test", "testcommit", "2026-11-04")
	if err != nil {
		t.Fatalf("app.New() error = %v", err)
	}

	if err := application.Start(); err != nil {
		t.Fatalf("app.Start() error = %v", err)
	}

	addr := application.Addr()
	if addr == "" {
		t.Fatalf("expected non-empty listener address")
	}

	// Verify HTTP /healthz responds over TCP
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://" + addr + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz error = %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want %d", resp.StatusCode, http.StatusOK)
	}

	// Stop application
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer stopCancel()

	if err := application.Stop(stopCtx); err != nil {
		t.Fatalf("app.Stop() error = %v", err)
	}

	// Verify idempotency of Stop
	if err := application.Stop(stopCtx); err != nil {
		t.Errorf("expected idempotent stop, got error = %v", err)
	}
}
