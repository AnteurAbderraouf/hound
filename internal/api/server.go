package api

import (
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

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
	mux.HandleFunc("/api/devices", s.handleDevices)
	mux.HandleFunc("/api/devices/", s.handleDevice) // single device by ip

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

// deviceResponse enriches the storage row with a computed display_name
// (best label from custom_name > hostname > mac > ip) so the UI doesn't
// have to reimplement the priority every render.
type deviceResponse struct {
	storage.Device
	DisplayName string `json:"display_name"`
}

func displayName(d storage.Device) string {
	switch {
	case d.CustomName != "":
		return d.CustomName
	case d.Hostname != "":
		return d.Hostname
	case d.MAC != "":
		return d.MAC
	default:
		return d.IP
	}
}

func (s *Server) handleDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	devs, err := s.Store.ListDevices()
	if err != nil {
		s.Log.Error("failed to list devices", "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	out := make([]deviceResponse, 0, len(devs))
	for _, d := range devs {
		out = append(out, deviceResponse{Device: d, DisplayName: displayName(d)})
	}
	writeJSON(w, http.StatusOK, out)
}

// handleDevice serves PATCH /api/devices/{ip} to rename a device. Other
// verbs on this path are 405.
func (s *Server) handleDevice(w http.ResponseWriter, r *http.Request) {
	ip := strings.TrimPrefix(r.URL.Path, "/api/devices/")
	if ip == "" || strings.Contains(ip, "/") {
		http.Error(w, "bad ip", http.StatusBadRequest)
		return
	}
	if r.Method != http.MethodPatch {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		CustomName string `json:"custom_name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, "invalid json body", http.StatusBadRequest)
		return
	}
	if err := s.Store.RenameDevice(ip, strings.TrimSpace(body.CustomName)); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "device not found", http.StatusNotFound)
			return
		}
		s.Log.Error("rename device failed", "ip", ip, "err", err)
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
