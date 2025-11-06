package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/veverkap/calibre-web-automated-book-downloader/internal/auth"
	"github.com/veverkap/calibre-web-automated-book-downloader/internal/config"
	"github.com/veverkap/calibre-web-automated-book-downloader/internal/downloader"
	"github.com/veverkap/calibre-web-automated-book-downloader/internal/models"
	"go.uber.org/zap"
)

// Handler holds the API handler dependencies
type Handler struct {
	config     *config.Config
	logger     *zap.Logger
	auth       *auth.Authenticator
	bookQueue  *models.BookQueue
	workerPool *downloader.WorkerPool
}

// NewHandler creates a new API handler
func NewHandler(cfg *config.Config, logger *zap.Logger) *Handler {
	authenticator := auth.NewAuthenticator(cfg.CWADBPath)
	bookQueue := models.NewBookQueue(time.Duration(cfg.StatusTimeout) * time.Second)
	workerPool := downloader.NewWorkerPool(cfg, logger, bookQueue)
	
	// Start worker pool
	workerPool.Start()
	
	return &Handler{
		config:     cfg,
		logger:     logger,
		auth:       authenticator,
		bookQueue:  bookQueue,
		workerPool: workerPool,
	}
}

// Shutdown gracefully shuts down the handler and its dependencies
func (h *Handler) Shutdown() {
	if h.workerPool != nil {
		h.workerPool.Stop()
	}
}

// RegisterRoutes registers all API routes
func (h *Handler) RegisterRoutes(r chi.Router) {
	// Serve static files
	fileServer := http.FileServer(http.Dir("web/static"))
	r.Handle("/static/*", http.StripPrefix("/static/", fileServer))
	r.Handle("/request/static/*", http.StripPrefix("/request/static/", fileServer))

	// Favicon routes
	r.Get("/favico*", h.serveFavicon)
	r.Get("/request/favico*", h.serveFavicon)
	r.Get("/request/static/favico*", h.serveFavicon)

	// Index route with authentication
	r.Get("/", h.basicAuth(h.handleIndex))
	r.Get("/request", h.basicAuth(h.handleIndex))

	// API routes with authentication
	r.Route("/api", func(r chi.Router) {
		r.Use(h.basicAuthMiddleware)
		
		r.Get("/search", h.handleSearch)
		r.Get("/info", h.handleInfo)
		r.Get("/download", h.handleDownload)
		r.Get("/status", h.handleStatus)
		r.Get("/localdownload", h.handleLocalDownload)
		r.Delete("/download/{book_id}/cancel", h.handleCancelDownload)
		r.Put("/queue/{book_id}/priority", h.handleSetPriority)
		r.Post("/queue/reorder", h.handleReorderQueue)
		r.Get("/queue/order", h.handleQueueOrder)
		r.Get("/downloads/active", h.handleActiveDownloads)
		r.Delete("/queue/clear", h.handleClearCompleted)
	})

	// Register routes with /request prefix
	r.Route("/request/api", func(r chi.Router) {
		r.Use(h.basicAuthMiddleware)
		
		r.Get("/search", h.handleSearch)
		r.Get("/info", h.handleInfo)
		r.Get("/download", h.handleDownload)
		r.Get("/status", h.handleStatus)
		r.Get("/localdownload", h.handleLocalDownload)
		r.Delete("/download/{book_id}/cancel", h.handleCancelDownload)
		r.Put("/queue/{book_id}/priority", h.handleSetPriority)
		r.Post("/queue/reorder", h.handleReorderQueue)
		r.Get("/queue/order", h.handleQueueOrder)
		r.Get("/downloads/active", h.handleActiveDownloads)
		r.Delete("/queue/clear", h.handleClearCompleted)
	})

	// Error handlers
	r.NotFound(h.handleNotFound)
	r.MethodNotAllowed(h.handleMethodNotAllowed)
}

// basicAuthMiddleware is a middleware for Basic Auth
func (h *Handler) basicAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// If no database is configured, skip authentication
		if h.config.CWADBPath == "" {
			next.ServeHTTP(w, r)
			return
		}

		// Check if database path is set but invalid
		if h.config.CWADBPath != "" {
			// In production, you'd check if the file exists
			// For now, we'll skip this check
		}

		// Get Basic Auth credentials
		username, password, ok := r.BasicAuth()
		if !ok {
			h.requestAuth(w)
			return
		}

		// Authenticate
		authenticated, err := h.auth.Authenticate(username, password)
		if err != nil {
			h.logger.Error("Authentication error", zap.Error(err))
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		if !authenticated {
			h.logger.Error("Authentication failed", zap.String("username", username))
			h.requestAuth(w)
			return
		}

		h.logger.Info("Authentication successful", zap.String("username", username))
		next.ServeHTTP(w, r)
	})
}

// basicAuth wraps a handler with Basic Auth
func (h *Handler) basicAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Get Basic Auth credentials
		username, password, ok := r.BasicAuth()
		if !ok {
			// If no database is configured, allow access
			if h.config.CWADBPath == "" {
				next(w, r)
				return
			}
			h.requestAuth(w)
			return
		}

		// Authenticate
		authenticated, err := h.auth.Authenticate(username, password)
		if err != nil {
			h.logger.Error("Authentication error", zap.Error(err))
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		if !authenticated {
			h.logger.Error("Authentication failed", zap.String("username", username))
			h.requestAuth(w)
			return
		}

		h.logger.Info("Authentication successful", zap.String("username", username))
		next(w, r)
	}
}

// requestAuth requests authentication from the client
func (h *Handler) requestAuth(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", `Basic realm="Calibre-Web Book Downloader"`)
	http.Error(w, "Unauthorized", http.StatusUnauthorized)
}

// serveFavicon serves the favicon
func (h *Handler) serveFavicon(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "web/static/favicon.ico")
}

// handleIndex serves the main page
func (h *Handler) handleIndex(w http.ResponseWriter, r *http.Request) {
	// For now, return a simple response
	// In production, this would render the HTML template
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(`<!DOCTYPE html>
<html>
<head>
	<title>Calibre-Web Book Downloader</title>
</head>
<body>
	<h1>Calibre-Web Book Downloader</h1>
	<p>API is running. Use the API endpoints to interact with the service.</p>
	<p>Build Version: ` + h.config.BuildVersion + `</p>
	<p>Release Version: ` + h.config.ReleaseVersion + `</p>
</body>
</html>`))
}

// handleNotFound handles 404 errors
func (h *Handler) handleNotFound(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(w, http.StatusNotFound, map[string]string{
		"error": "Not Found",
	})
}

// handleMethodNotAllowed handles 405 errors
func (h *Handler) handleMethodNotAllowed(w http.ResponseWriter, r *http.Request) {
	h.writeJSON(w, http.StatusMethodNotAllowed, map[string]string{
		"error": "Method Not Allowed",
	})
}

// writeJSON writes a JSON response
func (h *Handler) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(data); err != nil {
		h.logger.Error("Failed to encode JSON", zap.Error(err))
	}
}

// writeError writes an error response
func (h *Handler) writeError(w http.ResponseWriter, status int, message string) {
	h.writeJSON(w, status, map[string]string{"error": message})
}
