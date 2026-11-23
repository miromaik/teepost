package server

import (
	"io/fs"
	"net/http"
	"os"
	"path"
	"strings"

	"teepost/internal/db"
	"teepost/internal/media"
)

// Config holds runtime configuration loaded from environment variables.
type Config struct {
	Port          string
	DBPath        string
	MediaDir      string
	AdminPassword string
}

// LoadConfig reads environment variables and sets defaults (default port: 8787).
func LoadConfig() Config {
	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "8787"
	}
	dbPath := strings.TrimSpace(os.Getenv("DB_PATH"))
	if dbPath == "" {
		dbPath = "./data/crossposter.db"
	}
	mediaDir := strings.TrimSpace(os.Getenv("MEDIA_DIR"))
	if mediaDir == "" {
		mediaDir = "./data/media"
	}
	return Config{
		Port:          port,
		DBPath:        dbPath,
		MediaDir:      mediaDir,
		AdminPassword: os.Getenv("ADMIN_PASSWORD"),
	}
}

// Server wires HTTP routes, auth middleware, API handlers, and the embedded SPA.
type Server struct {
	cfg        Config
	store      *db.Store
	media      *media.Manager
	auth       *authManager
	frontendFS fs.FS
	handler    http.Handler
}

// New creates an initialized HTTP server ready to listen.
func New(cfg Config, store *db.Store, mediaMgr *media.Manager, distFS fs.FS) *Server {
	s := &Server{
		cfg:        cfg,
		store:      store,
		media:      mediaMgr,
		auth:       newAuthManager(store, cfg.AdminPassword),
		frontendFS: distFS,
	}

	mux := http.NewServeMux()

	// Probes & System endpoints
	mux.HandleFunc("GET /healthz", s.handleHealth)
	mux.HandleFunc("GET /ready", s.handleReady)
	mux.HandleFunc("GET /api/health", s.handleHealth)
	mux.HandleFunc("GET /api/system/info", s.handleSystemInfo)

	// Auth endpoints
	mux.HandleFunc("GET /api/auth/status", s.handleAuthStatus)
	mux.HandleFunc("POST /api/auth/login", s.handleAuthLogin)
	mux.HandleFunc("POST /api/auth/logout", s.handleAuthLogout)
	mux.HandleFunc("POST /api/auth/password", s.handleSetAuthPassword)

	// Accounts endpoints
	mux.HandleFunc("GET /api/accounts", s.handleListAccounts)
	mux.HandleFunc("POST /api/accounts", s.handleCreateAccount)
	mux.HandleFunc("POST /api/accounts/test", s.handleTestUnsavedCredentials)
	mux.HandleFunc("PUT /api/accounts/{id}", s.handleUpdateAccount)
	mux.HandleFunc("DELETE /api/accounts/{id}", s.handleDeleteAccount)
	mux.HandleFunc("POST /api/accounts/{id}/test", s.handleTestSavedAccount)

	// Media endpoints
	mux.HandleFunc("POST /api/media", s.handleUploadMedia)
	mux.HandleFunc("GET /api/media/{filename}", s.handleServeMedia)

	// Publishing, Scheduling, Queue, Drafts & History endpoints
	mux.HandleFunc("POST /api/publish", s.handlePublish)
	mux.HandleFunc("POST /api/posts/save", s.handleSavePost)
	mux.HandleFunc("POST /api/posts/{id}/publish-now", s.handlePublishSavedPostNow)
	mux.HandleFunc("POST /api/posts/{postId}/retry/{accountId}", s.handleRetryPostAccount)
	mux.HandleFunc("GET /api/posts", s.handleListPosts)
	mux.HandleFunc("DELETE /api/posts/{id}", s.handleDeletePost)

	// Posting Schedule & Queue Controls
	mux.HandleFunc("GET /api/schedule-slots", s.handleGetScheduleSlots)
	mux.HandleFunc("PUT /api/schedule-slots", s.handleUpdateScheduleSlots)
	mux.HandleFunc("POST /api/queue/pause", s.handleSetQueuePaused)
	mux.HandleFunc("POST /api/queue/reorder", s.handleReorderQueue)
	mux.HandleFunc("POST /api/queue/shuffle", s.handleShuffleQueue)

	// Hashtag Groups & Display Preferences
	mux.HandleFunc("GET /api/hashtag-groups", s.handleListHashtagGroups)
	mux.HandleFunc("PUT /api/hashtag-groups", s.handleSaveHashtagGroups)
	mux.HandleFunc("GET /api/preferences", s.handleGetPreferences)
	mux.HandleFunc("PUT /api/preferences", s.handleSavePreferences)

	// Embedded React SPA fallback
	mux.Handle("/", s.spaHandler())

	s.handler = s.auth.middleware(mux)
	return s
}

// Handler returns the root http.Handler.
func (s *Server) Handler() http.Handler {
	return s.handler
}

func (s *Server) spaHandler() http.Handler {
	fileServer := http.FileServer(http.FS(s.frontendFS))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeJSONError(w, http.StatusNotFound, "API endpoint not found")
			return
		}

		cleanPath := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if cleanPath == "" || cleanPath == "." {
			cleanPath = "index.html"
		}

		if info, err := fs.Stat(s.frontendFS, cleanPath); err == nil && !info.IsDir() {
			fileServer.ServeHTTP(w, r)
			return
		}

		indexBytes, err := fs.ReadFile(s.frontendFS, "index.html")
		if err != nil {
			http.Error(w, "frontend assets missing", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(indexBytes)
	})
}
