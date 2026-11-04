package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

var (
	ErrWeakSecret = errors.New("jwt secret is too weak for production: must be at least 32 bytes and not use default development secret")
)

const (
	DefaultPort         = 8080
	DefaultEnv          = "development"
	DefaultLogLevel     = "info"
	DefaultDevJWTSecret = "modulaengine-default-development-secret-change-in-production-32b"
)

type Config struct {
	Server   ServerConfig
	Database DatabaseConfig
	Security SecurityConfig
	Log      LogConfig
}

type ServerConfig struct {
	Port     int
	Headless bool
	Env      string // "development", "production"
}

type DatabaseConfig struct {
	DataURL    string
	ControlURL string
	ReadURL    string
}

type SecurityConfig struct {
	JWTSecret []byte
}

type LogConfig struct {
	Level   string
	LogFile string
	Console bool
}

type FlagOverrides struct {
	Port       *int
	Headless   *bool
	LogLevel   *string
	LogFile    *string
	LogConsole *bool
}

// Load loads configuration adhering to the 12-factor hierarchy:
// Defaults -> .env file -> OS environment variables -> CLI flag overrides.
func Load(envFilePath string, flags FlagOverrides) (*Config, error) {
	if err := LoadEnvFile(envFilePath); err != nil {
		return nil, fmt.Errorf("load env file: %w", err)
	}

	cfg := &Config{
		Server: ServerConfig{
			Port:     DefaultPort,
			Headless: false,
			Env:      DefaultEnv,
		},
		Database: DatabaseConfig{
			DataURL:    "",
			ControlURL: "",
			ReadURL:    "",
		},
		Security: SecurityConfig{
			JWTSecret: []byte(DefaultDevJWTSecret),
		},
		Log: LogConfig{
			Level:   DefaultLogLevel,
			LogFile: "",
			Console: false,
		},
	}

	if val := os.Getenv("PORT"); val != "" {
		port, err := strconv.Atoi(val)
		if err != nil {
			return nil, fmt.Errorf("parse PORT %q: %w", val, err)
		}
		cfg.Server.Port = port
	}

	if val := os.Getenv("HEADLESS"); val != "" {
		b, err := strconv.ParseBool(val)
		if err != nil {
			return nil, fmt.Errorf("parse HEADLESS %q: %w", val, err)
		}
		cfg.Server.Headless = b
	}

	if val := os.Getenv("ENV"); val != "" {
		cfg.Server.Env = val
	}

	if val := os.Getenv("DATABASE_URL"); val != "" {
		cfg.Database.DataURL = val
	}

	if val := os.Getenv("DATABASE_CONTROL_URL"); val != "" {
		cfg.Database.ControlURL = val
	}

	if val := os.Getenv("DATABASE_READ_URL"); val != "" {
		cfg.Database.ReadURL = val
	}

	if val := os.Getenv("JWT_SECRET"); val != "" {
		cfg.Security.JWTSecret = []byte(val)
	}

	if val := os.Getenv("LOG_LEVEL"); val != "" {
		cfg.Log.Level = val
	}

	if val := os.Getenv("LOG_FILE"); val != "" {
		cfg.Log.LogFile = val
	}

	if val := os.Getenv("LOG_CONSOLE"); val != "" {
		b, err := strconv.ParseBool(val)
		if err != nil {
			return nil, fmt.Errorf("parse LOG_CONSOLE %q: %w", val, err)
		}
		cfg.Log.Console = b
	}

	if flags.Port != nil {
		cfg.Server.Port = *flags.Port
	}
	if flags.Headless != nil {
		cfg.Server.Headless = *flags.Headless
	}
	if flags.LogLevel != nil {
		cfg.Log.Level = *flags.LogLevel
	}
	if flags.LogFile != nil {
		cfg.Log.LogFile = *flags.LogFile
	}
	if flags.LogConsole != nil {
		cfg.Log.Console = *flags.LogConsole
	}

	if strings.ToLower(cfg.Server.Env) == "production" {
		secret := string(cfg.Security.JWTSecret)
		if len(cfg.Security.JWTSecret) < 32 ||
			secret == DefaultDevJWTSecret ||
			secret == "super-secret-jwt-signing-key-change-in-production" ||
			strings.Contains(secret, "change-in-production") {
			return nil, ErrWeakSecret
		}
	}

	return cfg, nil
}
