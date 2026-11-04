package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

var (
	Version   = "v0.1.0-dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

func main() {
	port := flag.Int("port", 8080, "HTTP server listening port")
	dbURL := flag.String("db", "", "PostgreSQL database connection URL")
	controlDBURL := flag.String("db-control", "", "PostgreSQL control pool connection URL")
	headless := flag.Bool("headless", false, "Run in headless mode without embedded UI")
	showVersion := flag.Bool("version", false, "Print version information and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("ModulaEngine %s (commit: %s, built: %s)\n", Version, Commit, BuildTime)
		os.Exit(0)
	}

	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	slog.Info("initializing modulaengine runtime",
		"version", Version,
		"commit", Commit,
		"port", *port,
		"headless", *headless,
	)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if *dbURL == "" {
		*dbURL = os.Getenv("DATABASE_URL")
	}
	if *controlDBURL == "" {
		*controlDBURL = os.Getenv("DATABASE_CONTROL_URL")
	}

	slog.Info("engine ready to accept connections", "port", *port)

	<-ctx.Done()
	slog.Info("graceful shutdown signal received, exiting")
}
