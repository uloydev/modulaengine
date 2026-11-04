package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/uloydev/modulaengine/internal/api"
	"github.com/uloydev/modulaengine/internal/command"
	"github.com/uloydev/modulaengine/internal/database"
	"github.com/uloydev/modulaengine/internal/logger"
	"github.com/uloydev/modulaengine/internal/query"
	"github.com/uloydev/modulaengine/internal/queue"
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
	jwtSecretStr := flag.String("jwt-secret", "", "JWT signing secret (defaults to JWT_SECRET env)")
	logLevel := flag.String("log-level", "info", "Log level (debug, info, warn, error)")
	logFile := flag.String("log-file", "", "Log file path for lumberjack rotation (optional)")
	logConsole := flag.Bool("log-console", false, "Use human-readable console log format")
	headless := flag.Bool("headless", false, "Run in headless mode without embedded UI")
	showVersion := flag.Bool("version", false, "Print version information and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("ModulaEngine %s (commit: %s, built: %s)\n", Version, Commit, BuildTime)
		os.Exit(0)
	}

	logCfg := logger.DefaultConfig()
	logCfg.Level = *logLevel
	logCfg.LogFile = *logFile
	logCfg.Console = *logConsole
	if os.Getenv("LOG_FILE") != "" && logCfg.LogFile == "" {
		logCfg.LogFile = os.Getenv("LOG_FILE")
	}
	if os.Getenv("LOG_LEVEL") != "" {
		logCfg.Level = os.Getenv("LOG_LEVEL")
	}

	appLogger := logger.Setup(logCfg)
	log.Logger = appLogger

	appLogger.Info().
		Str("version", Version).
		Str("commit", Commit).
		Int("port", *port).
		Bool("headless", *headless).
		Str("log_level", logCfg.Level).
		Str("log_file", logCfg.LogFile).
		Msg("initializing modulaengine runtime")

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if *dbURL == "" {
		*dbURL = os.Getenv("DATABASE_URL")
	}
	if *controlDBURL == "" {
		*controlDBURL = os.Getenv("DATABASE_CONTROL_URL")
	}
	if *jwtSecretStr == "" {
		*jwtSecretStr = os.Getenv("JWT_SECRET")
		if *jwtSecretStr == "" {
			*jwtSecretStr = "modulaengine-default-development-secret-change-in-production-32b"
		}
	}

	serverCfg := api.ServerConfig{
		JWTSecret: []byte(*jwtSecretStr),
		Version:   Version,
		Commit:    Commit,
		BuildTime: BuildTime,
	}

	registry := query.NewRegistry()
	queryCompiler := query.NewCompiler(registry)
	cmdCompiler := command.NewCompiler()
	executor := command.NewExecutor()
	hub := api.NewSSEHub()

	var (
		poolMgr    *database.PoolManager
		workerPool *queue.WorkerPool
		reaper     *queue.Reaper
		dataDB     api.TxDB
	)

	if *dbURL != "" && *controlDBURL != "" {
		dbCfg := database.DefaultConfig(*dbURL, *controlDBURL)
		var err error
		poolMgr, err = database.NewPoolManager(ctx, dbCfg)
		if err != nil {
			log.Error().Err(err).Msg("failed to connect to database pools")
			os.Exit(1)
		}
		defer poolMgr.Close()

		dataDB = poolMgr.DataPool
		workerCfg := queue.DefaultWorkerConfig(fmt.Sprintf("worker-%d", os.Getpid()))
		workerPool = queue.NewWorkerPool(poolMgr.DataPool, workerCfg)
		workerPool.Start(ctx)

		reaperCfg := queue.DefaultReaperConfig()
		reaper = queue.NewReaper(poolMgr.ControlPool, reaperCfg)
		reaper.Start(ctx)

		log.Info().Msg("connected to postgresql pools and started background workers")
	} else {
		log.Warn().Msg("database connection URLs not configured; running in standalone API mode")
	}

	dataHandler := api.NewDataHandler(dataDB, registry, queryCompiler, cmdCompiler, executor, hub)
	router := api.NewRouter(serverCfg, dataDB, dataHandler, hub)

	server := &http.Server{
		Addr:              fmt.Sprintf(":%d", *port),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Info().Str("addr", server.Addr).Msg("modulaengine listening for requests")
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error().Err(err).Msg("http server failed")
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	log.Info().Msg("shutdown signal received; commencing graceful teardown")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error().Err(err).Msg("http server shutdown error")
	}

	if workerPool != nil {
		workerPool.Stop()
	}
	if reaper != nil {
		reaper.Stop()
	}

	log.Info().Msg("modulaengine stopped cleanly")
}
