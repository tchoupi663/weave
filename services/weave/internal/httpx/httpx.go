// Package httpx holds the small HTTP helpers shared by the Weave services that
// serve JSON (live, history, inventory, edge).
package httpx

import (
	"encoding/json"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// WriteJSON writes a JSON response with the no-store cache policy.
func WriteJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

// WriteError writes a JSON error object.
func WriteError(w http.ResponseWriter, status int, message string) {
	WriteJSON(w, status, map[string]string{"error": message})
}

// DecodeJSON decodes a bounded JSON request body and rejects unknown fields.
func DecodeJSON(w http.ResponseWriter, r *http.Request, target any, maxBytes int64) error {
	body := http.MaxBytesReader(w, r.Body, maxBytes)
	defer body.Close()
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(target)
}

// SecurityHeaders applies the API's fixed header policy.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self' data:; connect-src 'self'; base-uri 'none'; form-action 'self'")
		next.ServeHTTP(w, r)
	})
}

// Static serves the built SPA from webDir with index fallback and immutable
// asset caching. Unknown API and internal paths return JSON 404 instead of the
// SPA shell.
func Static(webDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			WriteError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		clean := path.Clean("/" + r.URL.Path)
		if clean == "/api" || strings.HasPrefix(clean, "/api/") || clean == "/internal" || strings.HasPrefix(clean, "/internal/") {
			WriteError(w, http.StatusNotFound, "not found")
			return
		}

		index := filepath.Join(webDir, "index.html")
		if webDir == "" || !fileExists(index) {
			http.Error(w, "Weave UI is not built. Run `npm run build` in services/weave/web.", http.StatusServiceUnavailable)
			return
		}

		rel := strings.TrimPrefix(clean, "/")
		if rel == "." || rel == "" {
			serveIndex(w, r, index)
			return
		}

		full := filepath.Join(webDir, filepath.FromSlash(rel))
		root, err := filepath.Abs(webDir)
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
}

func serveIndex(w http.ResponseWriter, r *http.Request, index string) {
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeFile(w, r, index)
}

func fileExists(name string) bool {
	info, err := os.Stat(name)
	return err == nil && !info.IsDir()
}
