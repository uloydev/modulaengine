package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
)

type ServerConfig struct {
	JWTSecret []byte
	Version   string
	Commit    string
	BuildTime string
}

func NewRouter(cfg ServerConfig, db TxDB, dataHandler *DataHandler, hub *SSEHub, spaHandler http.Handler) http.Handler {
	r := chi.NewRouter()

	r.Use(RequestIDAndLoggerMiddleware())
	r.Use(chimw.Recoverer)

	// Public system endpoints
	r.Get("/livez", func(w http.ResponseWriter, r *http.Request) {
		RespondJSON(w, http.StatusOK, map[string]string{"status": "alive"})
	})

	readyHandler := func(w http.ResponseWriter, r *http.Request) {
		if db != nil {
			if err := db.Ping(r.Context()); err != nil {
				RespondError(w, r, http.StatusServiceUnavailable, "Database Unavailable", err.Error())
				return
			}
		}
		RespondJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	}

	r.Get("/readyz", readyHandler)
	r.Get("/healthz", readyHandler)

	r.Get("/version", func(w http.ResponseWriter, r *http.Request) {
		RespondJSON(w, http.StatusOK, map[string]string{
			"version":    cfg.Version,
			"commit":     cfg.Commit,
			"build_time": cfg.BuildTime,
		})
	})

	// Authenticated API surface
	r.Group(func(apiRouter chi.Router) {
		apiRouter.Use(JWTAuthMiddleware(cfg.JWTSecret))

		if hub != nil {
			apiRouter.Get("/api/realtime", hub.HandleSSE)
		}

		if dataHandler != nil {
			apiRouter.Route("/api/data/{collection}", func(colRouter chi.Router) {
				colRouter.Get("/", dataHandler.HandleList)
				colRouter.Post("/", dataHandler.HandleCreate)
				colRouter.Get("/{id}", dataHandler.HandleGet)
				colRouter.Patch("/{id}", dataHandler.HandleUpdate)
				colRouter.Delete("/{id}", dataHandler.HandleDelete)
			})
		}
	})

	// Embedded Svelte 5 Single Page Application
	if spaHandler != nil {
		r.Handle("/*", spaHandler)
	}

	return r
}
