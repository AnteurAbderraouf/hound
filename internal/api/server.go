package api

import (
	"embed"
	"encoding/json"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/AnteurAbderraouf/hound/internal/categorizer"
	"github.com/AnteurAbderraouf/hound/internal/storage"
)

//go:embed static
var staticFS embed.FS

type Server struct {
	Addr        string
	Store       *storage.Store
	Categorizer *categorizer.Categorizer
	Log         *slog.Logger
}

func (s *Server) Start() error {
	mux := http.NewServeMux()

	mux.HandleFunc("/api/health", s.handleHealth)
	mux.HandleFunc("/api/queries", s.handleQueries)
	mux.HandleFunc("/api/categories", s.handleCategories)

	sub, err := fs.Sub(staticFS, "static")
	if err != nil {
		return err
	}
	mux.Handle("/", http.FileServer(http.FS(sub)))

	s.Log.Info("http server listening", "addr", s.Addr)
	return http.ListenAndServe(s.Addr, mux)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleQueries(w http.ResponseWriter, r *http.Request) {
	limit := 100
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}

	queries, err := s.Store.RecentQueries(limit)
	if err != nil {
		s.Log.Error("failed to fetch queries", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusOK, queries)
}

// handleCategories exposes the name -> color map so the UI does not need to
// hardcode the palette.
func (s *Server) handleCategories(w http.ResponseWriter, r *http.Request) {
	if s.Categorizer == nil {
		writeJSON(w, http.StatusOK, map[string]string{})
		return
	}
	writeJSON(w, http.StatusOK, s.Categorizer.Categories())
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
