package logger

import (
	"context"
	"io"
	"os"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
	"gopkg.in/natefinch/lumberjack.v2"
)

type ctxKeyLogger struct{}
type ctxKeyRequestID struct{}

type Config struct {
	Level      string `json:"level"`
	LogFile    string `json:"log_file"`
	MaxSize    int    `json:"max_size"`
	MaxBackups int    `json:"max_backups"`
	MaxAge     int    `json:"max_age"`
	Compress   bool   `json:"compress"`
	Console    bool   `json:"console"`
}

func DefaultConfig() Config {
	return Config{
		Level:      "info",
		LogFile:    "",
		MaxSize:    50, // 50MB
		MaxBackups: 5,
		MaxAge:     30, // 30 days
		Compress:   true,
		Console:    false,
	}
}

func init() {
	zerolog.TimeFieldFormat = time.RFC3339Nano
}

func Setup(cfg Config) zerolog.Logger {
	var lvl zerolog.Level
	switch strings.ToLower(cfg.Level) {
	case "debug":
		lvl = zerolog.DebugLevel
	case "warn":
		lvl = zerolog.WarnLevel
	case "error":
		lvl = zerolog.ErrorLevel
	default:
		lvl = zerolog.InfoLevel
	}

	var writers []io.Writer

	if cfg.Console {
		writers = append(writers, zerolog.ConsoleWriter{
			Out:        os.Stdout,
			TimeFormat: "15:04:05.000",
		})
	} else {
		writers = append(writers, os.Stdout)
	}

	if cfg.LogFile != "" {
		rotator := &lumberjack.Logger{
			Filename:   cfg.LogFile,
			MaxSize:    cfg.MaxSize,
			MaxBackups: cfg.MaxBackups,
			MaxAge:     cfg.MaxAge,
			Compress:   cfg.Compress,
		}
		writers = append(writers, rotator)
	}

	multi := io.MultiWriter(writers...)
	l := zerolog.New(multi).Level(lvl).With().Timestamp().Logger()
	return l
}

func WithLogger(ctx context.Context, l *zerolog.Logger) context.Context {
	return context.WithValue(ctx, ctxKeyLogger{}, l)
}

func FromContext(ctx context.Context) *zerolog.Logger {
	if val := ctx.Value(ctxKeyLogger{}); val != nil {
		if l, ok := val.(*zerolog.Logger); ok {
			return l
		}
	}
	return &log.Logger
}

func WithRequestID(ctx context.Context, requestID string) context.Context {
	ctx = context.WithValue(ctx, ctxKeyRequestID{}, requestID)
	l := FromContext(ctx)
	subLogger := l.With().Str("request_id", requestID).Logger()
	return WithLogger(ctx, &subLogger)
}

func GetRequestID(ctx context.Context) string {
	if val := ctx.Value(ctxKeyRequestID{}); val != nil {
		if reqID, ok := val.(string); ok {
			return reqID
		}
	}
	return ""
}
