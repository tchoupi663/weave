package gatewaysvc

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	"github.com/tchoupi663/weave/services/app/internal/httpx"
)

// Config contains the upstream service URLs.
type Config struct {
	AuthURL    string
	CatalogURL string
	OrdersURL  string
	PaymentURL string
}

// Gateway routes and authorizes Target App requests.
type Gateway struct {
	cfg    Config
	client *http.Client
}

// New returns a Gateway.
func New(cfg Config, client *http.Client) *Gateway {
	if client == nil {
		client = &http.Client{}
	}
	return &Gateway{cfg: cfg, client: client}
}

// Handler returns the Gateway HTTP handler.
func (g *Gateway) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/auth/", g.proxyTo(g.cfg.AuthURL))
	mux.HandleFunc("/api/products", g.proxyTo(g.cfg.CatalogURL))
	mux.HandleFunc("/api/products/", g.proxyTo(g.cfg.CatalogURL))
	mux.HandleFunc("/api/orders", g.protected(g.cfg.OrdersURL))
	mux.HandleFunc("/api/orders/", g.protected(g.cfg.OrdersURL))
	mux.HandleFunc("/api/payments", g.protected(g.cfg.PaymentURL))
	mux.HandleFunc("/admin/reset", g.handleReset)
	mux.HandleFunc("/health", httpx.Health)
	return mux
}

func (g *Gateway) protected(target string) http.HandlerFunc {
	proxy := g.proxyTo(target)
	return func(w http.ResponseWriter, r *http.Request) {
		user, status, err := g.validate(r.Context(), r.Header.Get("Authorization"))
		if err != nil || status != http.StatusOK {
			if status == http.StatusUnauthorized {
				httpx.Error(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			slog.Error("validate token", "err", err)
			httpx.Error(w, http.StatusBadGateway, "auth unavailable")
			return
		}
		r.Header.Set("X-User", user)
		proxy(w, r)
	}
}

func (g *Gateway) validate(ctx context.Context, authHeader string) (string, int, error) {
	if authHeader == "" {
		return "", http.StatusUnauthorized, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, g.cfg.AuthURL+"/api/auth/validate", nil)
	if err != nil {
		return "", http.StatusBadGateway, err
	}
	req.Header.Set("Authorization", authHeader)

	resp, err := g.client.Do(req)
	if err != nil {
		return "", http.StatusBadGateway, err
	}
	defer resp.Body.Close() //nolint:errcheck

	if resp.StatusCode == http.StatusUnauthorized {
		return "", http.StatusUnauthorized, nil
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return "", http.StatusBadGateway, fmt.Errorf("auth status %d: %s", resp.StatusCode, string(body))
	}
	var out struct {
		Username string `json:"username"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return "", http.StatusBadGateway, err
	}
	if out.Username == "" {
		return "", http.StatusBadGateway, fmt.Errorf("auth returned an empty username")
	}
	return out.Username, http.StatusOK, nil
}

func (g *Gateway) proxyTo(target string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		user := r.Header.Get("X-User")

		req, err := http.NewRequestWithContext(r.Context(), r.Method, target+r.URL.RequestURI(), r.Body)
		if err != nil {
			httpx.Error(w, http.StatusBadGateway, "could not build upstream request")
			return
		}
		req.ContentLength = r.ContentLength
		copyHeaders(req.Header, r.Header)
		req.Header.Del("X-User")
		if user != "" {
			req.Header.Set("X-User", user)
		}

		resp, err := g.client.Do(req)
		if err != nil {
			slog.Error("proxy upstream", "target", target, "err", err)
			httpx.Error(w, http.StatusBadGateway, "upstream unavailable")
			return
		}
		defer resp.Body.Close() //nolint:errcheck

		copyHeaders(w.Header(), resp.Header)
		w.WriteHeader(resp.StatusCode)
		if _, err := io.Copy(w, resp.Body); err != nil {
			slog.Error("copy upstream response", "err", err)
		}
	}
}

func (g *Gateway) handleReset(w http.ResponseWriter, r *http.Request) {
	for _, target := range []string{g.cfg.AuthURL, g.cfg.CatalogURL, g.cfg.OrdersURL} {
		req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, target+"/internal/reset", nil)
		if err != nil {
			httpx.Error(w, http.StatusBadGateway, "could not build reset request")
			return
		}
		resp, err := g.client.Do(req)
		if err != nil {
			slog.Error("reset upstream", "target", target, "err", err)
			httpx.Error(w, http.StatusBadGateway, "reset failed")
			return
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close() //nolint:errcheck
		if resp.StatusCode != http.StatusOK {
			httpx.Error(w, http.StatusBadGateway, "reset failed")
			return
		}
	}
	httpx.JSON(w, http.StatusOK, map[string]string{"status": "reset"})
}

var hopByHop = map[string]struct{}{
	"Connection":          {},
	"Keep-Alive":          {},
	"Proxy-Authenticate":  {},
	"Proxy-Authorization": {},
	"Te":                  {},
	"Trailer":             {},
	"Transfer-Encoding":   {},
	"Upgrade":             {},
}

func copyHeaders(dst, src http.Header) {
	for key, values := range src {
		if _, skip := hopByHop[http.CanonicalHeaderKey(key)]; skip {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}
