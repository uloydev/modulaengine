package app

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"github.com/uloydev/modulaengine/internal/api"
	"github.com/uloydev/modulaengine/internal/command"
	"github.com/uloydev/modulaengine/internal/config"
	"github.com/uloydev/modulaengine/internal/database"
	"github.com/uloydev/modulaengine/internal/query"
	"github.com/uloydev/modulaengine/internal/queue"
	"github.com/uloydev/modulaengine/web"
)

type App struct {
	cfg        *config.Config
	poolMgr    *database.PoolManager
	registry   *query.Registry
	workerPool *queue.WorkerPool
	reaper     *queue.Reaper
	httpServer *http.Server

	listener     net.Listener
	workerCtx    context.Context
	workerCancel context.CancelFunc
	mu           sync.Mutex
	started      bool
	stopped      bool
}

func New(ctx context.Context, cfg *config.Config, version, commit, buildTime string) (*App, error) {
	if cfg == nil {
		return nil, errors.New("nil configuration provided")
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

	if cfg.Database.DataURL != "" && cfg.Database.ControlURL != "" {
		dbCfg := database.DefaultConfig(cfg.Database.DataURL, cfg.Database.ControlURL)
		dbCfg.ReadURL = cfg.Database.ReadURL

		var err error
		poolMgr, err = database.NewPoolManagerWithRetry(ctx, dbCfg, 5, 500*time.Millisecond)
		if err != nil {
			return nil, fmt.Errorf("connect to database pools: %w", err)
		}

		// Bootstrap system catalog schema via Control Pool
		migrator := database.NewMigrator(poolMgr.ControlPool)
		if err := migrator.Run(ctx); err != nil {
			poolMgr.Close()
			return nil, fmt.Errorf("execute catalog bootstrap migrations: %w", err)
		}

		dataDB = poolMgr.DataPool
		workerCfg := queue.DefaultWorkerConfig(fmt.Sprintf("worker-%d", os.Getpid()))
		workerPool = queue.NewWorkerPool(poolMgr.DataPool, workerCfg)

		reaperCfg := queue.DefaultReaperConfig()
		reaper = queue.NewReaper(poolMgr.ControlPool, reaperCfg)

		log.Info().Msg("connected to postgresql pools and initialized background workers")
	} else if cfg.Database.DataURL != "" || cfg.Database.ControlURL != "" {
		return nil, errors.New("both DataURL and ControlURL are required for database configuration")
	} else {
		log.Warn().Msg("database connection URLs not configured; running in standalone API mode")
	}

	dataHandler := api.NewDataHandler(dataDB, registry, queryCompiler, cmdCompiler, executor, hub)

	var spaHandler http.Handler
	if !cfg.Server.Headless {
		spaHandler = web.AssetHandler()
		log.Info().Msg("embedded Svelte 5 admin SPA enabled")
	} else {
		log.Info().Msg("running in headless mode; embedded UI disabled")
	}

	serverCfg := api.ServerConfig{
		JWTSecret: cfg.Security.JWTSecret,
		Version:   version,
		Commit:    commit,
		BuildTime: buildTime,
	}

	router := api.NewRouter(serverCfg, dataDB, dataHandler, hub, spaHandler)

	httpServer := &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Server.Port),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	if ctx == nil {
		ctx = context.Background()
	}
	workerCtx, workerCancel := context.WithCancel(ctx)

	return &App{
		cfg:          cfg,
		poolMgr:      poolMgr,
		registry:     registry,
		workerPool:   workerPool,
		reaper:       reaper,
		httpServer:   httpServer,
		workerCtx:    workerCtx,
		workerCancel: workerCancel,
	}, nil
}

func (a *App) Start() error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.started {
		return errors.New("app already started")
	}

	if a.workerPool != nil {
		a.workerPool.Start(a.workerCtx)
	}
	if a.reaper != nil {
		a.reaper.Start(a.workerCtx)
	}

	ln, err := net.Listen("tcp", a.httpServer.Addr)
	if err != nil {
		if a.workerCancel != nil {
			a.workerCancel()
		}
		if a.workerPool != nil {
			a.workerPool.Stop()
		}
		if a.reaper != nil {
			a.reaper.Stop()
		}
		return fmt.Errorf("listen on %s: %w", a.httpServer.Addr, err)
	}

	a.listener = ln
	a.started = true

	log.Info().Str("addr", ln.Addr().String()).Msg("modulaengine listening for requests")

	go func() {
		if err := a.httpServer.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error().Err(err).Msg("http server failed")
		}
	}()

	return nil
}

func (a *App) Stop(ctx context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.stopped {
		return nil
	}
	a.stopped = true

	var errs []error

	if a.httpServer != nil {
		if err := a.httpServer.Shutdown(ctx); err != nil {
			errs = append(errs, fmt.Errorf("shutdown http server: %w", err))
		}
	}

	if a.workerCancel != nil {
		a.workerCancel()
	}
	if a.workerPool != nil {
		a.workerPool.Stop()
	}
	if a.reaper != nil {
		a.reaper.Stop()
	}

	if a.poolMgr != nil {
		a.poolMgr.Close()
	}

	return errors.Join(errs...)
}

func (a *App) Config() *config.Config {
	return a.cfg
}

func (a *App) PoolManager() *database.PoolManager {
	return a.poolMgr
}

func (a *App) Registry() *query.Registry {
	return a.registry
}

func (a *App) WorkerPool() *queue.WorkerPool {
	return a.workerPool
}

func (a *App) Reaper() *queue.Reaper {
	return a.reaper
}

func (a *App) HTTPServer() *http.Server {
	return a.httpServer
}

func (a *App) Addr() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.listener != nil {
		return a.listener.Addr().String()
	}
	if a.httpServer != nil {
		return a.httpServer.Addr
	}
	return ""
}
