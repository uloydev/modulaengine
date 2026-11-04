package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/uloydev/modulaengine/internal/app"
	"github.com/uloydev/modulaengine/internal/config"
	"github.com/uloydev/modulaengine/internal/logger"
)

var (
	Version   = "v0.1.0-dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

func main() {
	envFile := flag.String("env-file", ".env", "Path to environment file")
	port := flag.Int("port", 0, "HTTP server listening port (overrides config)")
	headless := flag.Bool("headless", false, "Run in headless mode without embedded UI")
	logLevel := flag.String("log-level", "", "Log level: debug, info, warn, error")
	logFile := flag.String("log-file", "", "Log file path for lumberjack rotation")
	logConsole := flag.Bool("log-console", false, "Use human-readable console log format")
	showVersion := flag.Bool("version", false, "Print version information and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("ModulaEngine %s (commit: %s, built: %s)\n", Version, Commit, BuildTime)
		os.Exit(0)
	}

	flags := config.FlagOverrides{}
	flag.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "port":
			flags.Port = port
		case "headless":
			flags.Headless = headless
		case "log-level":
			flags.LogLevel = logLevel
		case "log-file":
			flags.LogFile = logFile
		case "log-console":
			flags.LogConsole = logConsole
		}
	})

	cfg, err := config.Load(*envFile, flags)
	if err != nil {
		fmt.Fprintf(os.Stderr, "configuration error: %v\n", err)
		os.Exit(1)
	}

	appLogger := logger.Setup(logger.Config{
		Level:      cfg.Log.Level,
		LogFile:    cfg.Log.LogFile,
		Console:    cfg.Log.Console,
		MaxSize:    50,
		MaxBackups: 5,
		MaxAge:     30,
		Compress:   true,
	})
	log.Logger = appLogger

	appLogger.Info().
		Str("version", Version).
		Str("commit", Commit).
		Int("port", cfg.Server.Port).
		Bool("headless", cfg.Server.Headless).
		Str("env", cfg.Server.Env).
		Str("log_level", cfg.Log.Level).
		Msg("initializing modulaengine runtime")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	application, err := app.New(ctx, cfg, Version, Commit, BuildTime)
	if err != nil {
		appLogger.Error().Err(err).Msg("failed to initialize application")
		os.Exit(1)
	}

	if err := application.Start(); err != nil {
		appLogger.Error().Err(err).Msg("failed to start application")
		os.Exit(1)
	}

	<-ctx.Done()
	appLogger.Info().Msg("shutdown signal received; commencing graceful teardown")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := application.Stop(shutdownCtx); err != nil {
		appLogger.Error().Err(err).Msg("shutdown completed with errors")
	} else {
		appLogger.Info().Msg("modulaengine stopped cleanly")
	}
}
