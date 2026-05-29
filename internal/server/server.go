// Package server wires the HTTP layer: a chi router that exposes the /api
// surface and, in production builds, serves the embedded React SPA.
package server

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/elgnas/dbviz/internal/audit"
	"github.com/elgnas/dbviz/internal/connection"
)

// Config controls server construction.
type Config struct {
	// Dev selects dev behavior: /api only, no embedded SPA (user hits Vite:5173).
	Dev bool
	// BindAddr is the listen address, e.g. "127.0.0.1:7777".
	BindAddr string
	// Version is the build version string, surfaced at GET /api/version.
	Version string
	// Logger is the structured logger; if nil, slog.Default() is used.
	Logger *slog.Logger
	// AuditEnabled toggles the query audit log (§18). Default off in the zero
	// value; the serve command defaults it to on.
	AuditEnabled bool
}

// Server owns the http.Server and its lifecycle.
type Server struct {
	cfg          Config
	http         *http.Server
	log          *slog.Logger
	mgr          *connection.Manager
	audit        audit.Logger
	reaperCancel context.CancelFunc
}

// New constructs a Server with the router mounted.
func New(cfg Config) *Server {
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}
	mgr := connection.NewManager(connection.Config{}, log)

	var auditLog audit.Logger = audit.Nop{}
	if cfg.AuditEnabled {
		if al, err := audit.New(""); err != nil {
			log.Warn("audit log disabled: could not open audit directory", "error", err)
		} else {
			auditLog = al
		}
	}

	return &Server{
		cfg:   cfg,
		log:   log,
		mgr:   mgr,
		audit: auditLog,
		http: &http.Server{
			Addr:              cfg.BindAddr,
			Handler:           NewRouter(cfg, log, mgr, auditLog),
			ReadHeaderTimeout: 10 * time.Second,
		},
	}
}

// Start begins serving and blocks until the server stops.
func (s *Server) Start() error {
	reaperCtx, cancel := context.WithCancel(context.Background())
	s.reaperCancel = cancel
	go s.mgr.StartReaper(reaperCtx)

	s.log.Info("server starting", "addr", s.cfg.BindAddr, "dev", s.cfg.Dev)
	err := s.http.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

// Shutdown gracefully stops the server and closes all live connections.
func (s *Server) Shutdown(ctx context.Context) error {
	if s.reaperCancel != nil {
		s.reaperCancel()
	}
	_ = s.mgr.CloseAll()
	if s.audit != nil {
		_ = s.audit.Close()
	}
	return s.http.Shutdown(ctx)
}

// NewRouter builds the chi router for the given config.
func NewRouter(cfg Config, log *slog.Logger, mgr *connection.Manager, auditLog audit.Logger) *chi.Mux {
	if log == nil {
		log = slog.Default()
	}
	if auditLog == nil {
		auditLog = audit.Nop{}
	}
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(requestLogger(log))
	r.Use(middleware.Recoverer)

	a := &api{mgr: mgr, log: log, audit: auditLog}
	r.Route("/api", func(r chi.Router) {
		r.Get("/health", handleHealth)
		r.Get("/version", handleVersion(cfg.Version))
		r.Get("/docker/containers", a.listDockerContainers)

		r.Route("/connections", func(r chi.Router) {
			r.Post("/", a.createConnection)
			r.Get("/", a.listConnections)
			r.Post("/test", a.testConnection)

			r.Route("/{id}", func(r chi.Router) {
				r.Get("/", a.getConnection)
				r.Delete("/", a.deleteConnection)
				r.Post("/ping", a.pingConnection)

				r.With(timeout(introspectTimeout)).Get("/schema", a.getSchema)
				r.With(timeout(introspectTimeout)).Get("/schema/refresh", a.getSchema)
				r.Get("/schemas", a.listSchemas)
				r.With(queryTimeoutMW).Get("/tables/{nodeId}/sample", a.sampleData)
				r.Get("/tables/{nodeId}/stats", a.tableStats)

				r.With(timeout(introspectTimeout)).Get("/simulate/cascade/{nodeId}", a.simulateCascade)
				r.With(timeout(introspectTimeout)).Post("/simulate/crud", a.simulateCRUD)
				r.With(timeout(explainTimeout)).Post("/explain", a.explain)
			})
		})
	})

	if cfg.Dev {
		r.Get("/", func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "dev mode: visit http://127.0.0.1:5173", http.StatusNotFound)
		})
	} else {
		mountSPA(r)
	}

	return r
}

// mountSPA serves the embedded React build, falling back to index.html so that
// client-side routes resolve (SPA history mode).
func mountSPA(r chi.Router) {
	webFS := WebFS()
	fileServer := http.FileServer(http.FS(webFS))
	r.Get("/*", func(w http.ResponseWriter, req *http.Request) {
		if _, err := fs.Stat(webFS, trimLeadingSlash(req.URL.Path)); err != nil {
			// Unknown path → serve index.html for SPA routing.
			req2 := req.Clone(req.Context())
			req2.URL.Path = "/"
			fileServer.ServeHTTP(w, req2)
			return
		}
		fileServer.ServeHTTP(w, req)
	})
}

func trimLeadingSlash(p string) string {
	if len(p) > 0 && p[0] == '/' {
		p = p[1:]
	}
	if p == "" {
		return "."
	}
	return p
}

func handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func handleVersion(version string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"version": version})
	}
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
