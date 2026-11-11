package server

import (
	"io/fs"
	"net/http"

	"statusline/internal/config"
	"statusline/internal/storage"
)

type Server struct {
	cfg    *config.Config
	store  *storage.Store
	router *http.ServeMux
	webFS  fs.FS
}

func NewServer(cfg *config.Config, store *storage.Store, webFS fs.FS) *Server {
	s := &Server{
		cfg:    cfg,
		store:  store,
		router: http.NewServeMux(),
		webFS:  webFS,
	}
	s.routes()
	return s
}

func (s *Server) Router() *http.ServeMux {
	return s.router
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

	// Embedded Web Dashboard
	if s.webFS != nil {
		s.router.Handle("/", http.FileServer(http.FS(s.webFS)))
	} else {
		s.router.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			w.Write([]byte("Statusline Server (API online)"))
		})
	}
}
