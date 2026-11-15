package edge

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"github.com/tchoupi663/weave/services/weave/internal/httpx"
)

// Config is the weave-api configuration.
type Config struct {
	ListenAddr    string
	WebDir        string
	LiveURL       string
	HistoryURL    string
	InventoryURL  string
	HealthTimeout time.Duration
}

// Edge is the single browser origin: it serves the built SPA and proxies the
// frozen /api contract to the owning services.
type Edge struct {
	cfg    Config
	log    *slog.Logger
	client *http.Client
}

// New builds the edge service.
func New(cfg Config, log *slog.Logger) (*Edge, error) {
	timeout := cfg.HealthTimeout
	if timeout <= 0 {
		timeout = time.Second
	}
	cfg.HealthTimeout = timeout
	if _, err := url.Parse(cfg.LiveURL); err != nil {
		return nil, fmt.Errorf("LIVE_URL: %w", err)
	}
	if _, err := url.Parse(cfg.HistoryURL); err != nil {
		return nil, fmt.Errorf("HISTORY_URL: %w", err)
	}
	if _, err := url.Parse(cfg.InventoryURL); err != nil {
		return nil, fmt.Errorf("INVENTORY_URL: %w", err)
	}
	return &Edge{
		cfg:    cfg,
		log:    log,
		client: &http.Client{Timeout: timeout},
	}, nil
}

// Handler returns the browser-facing handler.
func (e *Edge) Handler() http.Handler {
	mux := http.NewServeMux()
	live := e.proxy(e.cfg.LiveURL, "weave-live")
	history := e.proxy(e.cfg.HistoryURL, "weave-history")
	inventory := e.proxy(e.cfg.InventoryURL, "weave-inventory")

	mux.Handle("GET /api/topology", live)
	mux.Handle("GET /api/stream", live)
	mux.Handle("POST /api/reset", live)

	mux.Handle("POST /api/incidents", history)
	mux.Handle("GET /api/incidents", history)
	mux.Handle("GET /api/incidents/{id}", history)
	mux.Handle("PATCH /api/incidents/{id}", history)
	mux.Handle("POST /api/incidents/{id}/recompute", history)
	mux.Handle("GET /api/incidents/{id}/timeline", history)
	mux.Handle("GET /api/incidents/{id}/alerts", history)
	mux.Handle("GET /api/metrics/edges", history)
	mux.Handle("GET /api/metrics/routes", history)

	mux.Handle("GET /api/changes", inventory)

	mux.HandleFunc("GET /api/health", e.aggregateHealth)
	mux.HandleFunc("GET /health", e.localHealth)
	mux.Handle("/", httpx.Static(e.cfg.WebDir))
	return httpx.SecurityHeaders(mux)
}

func (e *Edge) proxy(rawURL, service string) http.Handler {
	target, err := url.Parse(rawURL)
	if err != nil {
		// Configuration was validated in New; keep a hard failure visible.
		return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			httpx.WriteError(w, http.StatusBadGateway, service+" is misconfigured")
		})
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.FlushInterval = -1
	proxy.ErrorHandler = func(w http.ResponseWriter, _ *http.Request, err error) {
		e.log.Warn("upstream request failed", "service", service, "error", err)
		httpx.WriteError(w, http.StatusBadGateway, service+" is unavailable")
	}
	return proxy
}

func (e *Edge) localHealth(w http.ResponseWriter, _ *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (e *Edge) aggregateHealth(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), e.cfg.HealthTimeout)
	defer cancel()

	dependencies := map[string]string{}
	depStatus := func(name, rawURL string) map[string]any {
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL+"/health", nil)
		if err != nil {
			dependencies[name] = "unavailable"
			return nil
		}
		response, err := e.client.Do(request)
		if err != nil {
			dependencies[name] = "unavailable"
			return nil
		}
		defer response.Body.Close()
		if response.StatusCode != http.StatusOK {
			dependencies[name] = "unavailable"
			return nil
		}
		var payload map[string]any
		if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
			dependencies[name] = "unavailable"
			return nil
		}
		status, _ := payload["status"].(string)
		if status == "" {
			status = "ok"
		}
		if status != "ok" {
			dependencies[name] = "degraded"
		} else {
			dependencies[name] = "ok"
		}
		return payload
	}

	live := depStatus("live", e.cfg.LiveURL)
	history := depStatus("history", e.cfg.HistoryURL)
	depStatus("inventory", e.cfg.InventoryURL)

	overall := "ok"
	for _, status := range dependencies {
		if status != "ok" {
			overall = "degraded"
		}
	}
	body := map[string]any{
		"status":       overall,
		"dependencies": dependencies,
	}
	if live != nil {
		for _, field := range []string{
			"hubble_connected", "observed_flows", "recording_gaps",
			"dropped_observations", "gap_active", "history_connected",
		} {
			if value, ok := live[field]; ok {
				body[field] = value
			}
		}
	}
	if history != nil {
		for _, field := range []string{
			"detection_enabled", "rules_loaded", "rule_pack_version",
			"alerts_created", "incidents_created", "detection_suppressed_gaps",
			"detection_evaluations", "last_detection_evaluation",
		} {
			if value, ok := history[field]; ok {
				body[field] = value
			}
		}
	}
	if _, ok := body["hubble_connected"]; !ok {
		body["hubble_connected"] = false
	}
	if _, ok := body["observed_flows"]; !ok {
		body["observed_flows"] = 0
	}
	httpx.WriteJSON(w, http.StatusOK, body)
}
