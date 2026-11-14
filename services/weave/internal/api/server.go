package api

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/tchoupi663/weave/services/weave/internal/history"
	"github.com/tchoupi663/weave/services/weave/internal/topology"
)

type Server struct {
	store   *topology.Store
	webDir  string
	log     *slog.Logger
	history *history.History
}

func New(store *topology.Store, webDir string, log *slog.Logger) http.Handler {
	return NewWithHistory(store, webDir, nil, log)
}

func NewWithHistory(store *topology.Store, webDir string, hist *history.History, log *slog.Logger) http.Handler {
	s := &Server{store: store, webDir: webDir, log: log, history: hist}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/topology", s.topology)
	mux.HandleFunc("GET /api/stream", s.stream)
	mux.HandleFunc("POST /api/reset", s.reset)
	mux.HandleFunc("POST /v1/traces", s.traces)
	mux.HandleFunc("POST /api/incidents", s.createIncident)
	mux.HandleFunc("GET /api/incidents", s.listIncidents)
	mux.HandleFunc("GET /api/incidents/{id}", s.getIncident)
	mux.HandleFunc("PATCH /api/incidents/{id}", s.patchIncident)
	mux.HandleFunc("POST /api/incidents/{id}/recompute", s.recomputeIncident)
	mux.HandleFunc("GET /api/incidents/{id}/timeline", s.incidentTimeline)
	mux.HandleFunc("GET /api/changes", s.changes)
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /health", s.health)
	mux.HandleFunc("/", s.static)
	return securityHeaders(mux)
}

func (s *Server) topology(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.store.Snapshot())
}

func (s *Server) reset(w http.ResponseWriter, _ *http.Request) {
	s.store.Reset()
	writeJSON(w, http.StatusOK, map[string]string{"status": "reset"})
}

func (s *Server) health(w http.ResponseWriter, _ *http.Request) {
	snapshot := s.store.Snapshot()
	status := "ok"
	if !snapshot.Hubble.Connected {
		status = "degraded"
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":           status,
		"hubble_connected": snapshot.Hubble.Connected,
		"observed_flows":   snapshot.Hubble.Observed,
	})
}

func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	fmt.Fprint(w, "retry: 1500\n\n")
	flusher.Flush()

	send := func() bool {
		payload, err := json.Marshal(s.store.Snapshot())
		if err != nil {
			s.log.Error("marshal topology snapshot", "error", err)
			return false
		}
		if _, err := fmt.Fprintf(w, "event: topology\ndata: %s\n\n", payload); err != nil {
			return false
		}
		flusher.Flush()
		return true
	}

	if !send() {
		return
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if !send() {
				return
			}
		}
	}
}

func (s *Server) static(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	index := filepath.Join(s.webDir, "index.html")
	if s.webDir == "" || !fileExists(index) {
		http.Error(w, "Weave UI is not built. Run `npm run build` in services/weave/web.", http.StatusServiceUnavailable)
		return
	}

	clean := path.Clean("/" + r.URL.Path)
	rel := strings.TrimPrefix(clean, "/")
	if rel == "." || rel == "" {
		serveIndex(w, r, index)
		return
	}

	full := filepath.Join(s.webDir, filepath.FromSlash(rel))
	root, err := filepath.Abs(s.webDir)
	if err != nil {
		http.Error(w, "invalid web directory", http.StatusInternalServerError)
		return
	}
	target, err := filepath.Abs(full)
	if err != nil || !strings.HasPrefix(target, root+string(filepath.Separator)) {
		serveIndex(w, r, index)
		return
	}
	info, err := os.Stat(target)
	if err != nil || info.IsDir() {
		serveIndex(w, r, index)
		return
	}

	if strings.HasPrefix(clean, "/assets/") {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	} else {
		w.Header().Set("Cache-Control", "no-cache")
	}
	http.ServeFile(w, r, target)
}

func serveIndex(w http.ResponseWriter, r *http.Request, index string) {
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, index)
}

func fileExists(name string) bool {
	info, err := os.Stat(name)
	return err == nil && !info.IsDir()
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self' data:; connect-src 'self'; base-uri 'none'; form-action 'self'")
		next.ServeHTTP(w, r)
	})
}
