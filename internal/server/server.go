package server

import (
	"io/fs"
	"net/http"

	"statusline/internal/config"
	"statusline/internal/storage"
)

type Server struct {
	cfg    *config.ServerConfig
	store  *storage.Store
	router *http.ServeMux
	webFS  fs.FS
}

func NewServer(cfg *config.ServerConfig, store *storage.Store, webFS fs.FS) *Server {
	s := &Server{
		cfg:    cfg,
		store:  store,
		router: http.NewServeMux(),
		webFS:  webFS,
	}
	s.routes()
	return s
}

func (s *Server) Router() http.Handler {
	return loggingMiddleware(s.router)
}

func (s *Server) routes() {
	// Ingestion endpoint (POST /api/v1/report and alias /api/telemetry)
	s.router.HandleFunc("POST /api/v1/report", s.handleReport)
	s.router.HandleFunc("POST /api/telemetry", s.handleReport)
	s.router.HandleFunc("POST /api/v1/refresh", s.handleRefresh)
	s.router.HandleFunc("POST /api/refresh", s.handleRefresh)

	// REST APIs
	s.router.HandleFunc("GET /api/v1/sessions", s.handleGetSessions)
	s.router.HandleFunc("GET /api/sessions", s.handleGetSessions)

	s.router.HandleFunc("GET /api/v1/nodes", s.handleGetNodes)
	s.router.HandleFunc("GET /api/nodes", s.handleGetNodes)

	s.router.HandleFunc("GET /api/v1/overview", s.handleGetOverview)
	s.router.HandleFunc("GET /api/overview", s.handleGetOverview)

	s.router.HandleFunc("GET /api/v1/history", s.handleGetHistory)
	s.router.HandleFunc("GET /api/history", s.handleGetHistory)

	s.router.HandleFunc("GET /api/v1/quotas", s.handleGetQuotas)
	s.router.HandleFunc("GET /api/quotas", s.handleGetQuotas)

	s.router.HandleFunc("GET /api/v1/tokens/daily", s.handleGetDailyTokens)
	s.router.HandleFunc("GET /api/tokens/daily", s.handleGetDailyTokens)

	s.router.HandleFunc("GET /api/v1/debug/raw", s.handleGetDebugRaw)
	s.router.HandleFunc("GET /api/debug/raw", s.handleGetDebugRaw)

	// Display Aliases Config APIs
	s.router.HandleFunc("GET /api/v1/config/aliases", s.handleGetAliases)
	s.router.HandleFunc("GET /api/config/aliases", s.handleGetAliases)
	s.router.HandleFunc("POST /api/v1/config/aliases", s.handleSaveAliases)
	s.router.HandleFunc("POST /api/config/aliases", s.handleSaveAliases)
	s.router.HandleFunc("DELETE /api/v1/config/aliases", s.handleDeleteAlias)
	s.router.HandleFunc("DELETE /api/config/aliases", s.handleDeleteAlias)

	// Embedded Web Dashboard
	if s.webFS != nil {
		fileServer := http.FileServer(http.FS(s.webFS))
		s.router.Handle("/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Disable browser caching for local real-time dashboard assets
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Expires", "0")
			fileServer.ServeHTTP(w, r)
		}))
	} else {
		s.router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Write([]byte("Statusline Server (API online)"))
		})
	}
}
