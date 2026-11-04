package logger_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
	"github.com/uloydev/modulaengine/internal/logger"
)

func TestLogger_WithRequestID(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	reqID := "req-test-12345"

	var buf bytes.Buffer
	testLogger := zerolog.New(&buf).With().Timestamp().Logger()
	ctx = logger.WithLogger(ctx, &testLogger)
	ctx = logger.WithRequestID(ctx, reqID)

	extracted := logger.GetRequestID(ctx)
	if extracted != reqID {
		t.Errorf("GetRequestID() = %s, want %s", extracted, reqID)
	}

	l := logger.FromContext(ctx)
	l.Info().Msg("test message")

	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("failed to parse log JSON: %v, raw: %s", err, buf.String())
	}

	if logEntry["request_id"] != reqID {
		t.Errorf("log entry missing request_id: %+v", logEntry)
	}
	if logEntry["message"] != "test message" {
		t.Errorf("log entry message mismatch: %+v", logEntry)
	}
}

func TestLogger_LumberjackFileRotation(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "modulaengine_test.log")

	cfg := logger.DefaultConfig()
	cfg.LogFile = logPath
	cfg.Level = "debug"

	testLogger := logger.Setup(cfg)
	testLogger.Info().Str("module", "test").Msg("written to lumberjack file")

	// Verify file was created and contains log line
	content, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	if !bytes.Contains(content, []byte("written to lumberjack file")) {
		t.Fatalf("log file did not contain expected message: %s", string(content))
	}
}
