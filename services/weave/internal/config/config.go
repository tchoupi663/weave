package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

const (
	defaultListenAddr      = ":8080"
	defaultHubbleRelayAddr = "hubble-relay.kube-system.svc.cluster.local:80"
	defaultWatchNamespace  = "weave"
	defaultWindow          = 5 * time.Minute
	defaultHistoryURL      = "http://weave-history:8080"
	defaultLiveURL         = "http://weave-live:8080"
	defaultInventoryURL    = "http://weave-inventory:8080"
	defaultDataDir         = "/data"
	defaultWebDir          = "web/dist"
	defaultFlushGrace      = 15 * time.Second
	defaultReconcileEvery  = 30 * time.Second
	defaultLiveTimeout     = time.Second
	defaultFeedInterval    = time.Second
	defaultFeedBatch       = 500
	defaultHealthTimeout   = time.Second
	minWindow              = 30 * time.Second
	maxWindow              = time.Hour
	windowBucketCount      = 60
	minimumBucketDuration  = time.Second
)

// Live is the configuration of weave-live.
type Live struct {
	ListenAddr      string
	HubbleRelayAddr string
	WatchNamespace  string
	Window          time.Duration
	HistoryURL      string
	FeedInterval    time.Duration
	FeedBatch       int
}

// LoadLive reads the weave-live configuration.
func LoadLive() (Live, error) {
	window, err := parseWindow(envOr("WINDOW", defaultWindow.String()))
	if err != nil {
		return Live{}, err
	}
	cfg := Live{
		ListenAddr:      envOr("LISTEN_ADDR", defaultListenAddr),
		HubbleRelayAddr: envOr("HUBBLE_RELAY_ADDR", defaultHubbleRelayAddr),
		WatchNamespace:  envOr("WATCH_NAMESPACE", defaultWatchNamespace),
		Window:          window,
		HistoryURL:      strings.TrimRight(strings.TrimSpace(os.Getenv("HISTORY_URL")), "/"),
		FeedInterval:    defaultFeedInterval,
		FeedBatch:       defaultFeedBatch,
	}
	if cfg.ListenAddr == "" {
		return Live{}, fmt.Errorf("LISTEN_ADDR must not be empty")
	}
	if cfg.HubbleRelayAddr == "" {
		return Live{}, fmt.Errorf("HUBBLE_RELAY_ADDR must not be empty")
	}
	return cfg, nil
}

// History is the configuration of weave-history.
type History struct {
	ListenAddr     string
	WatchNamespace string
	DataDir        string
	FlushGrace     time.Duration
}

// LoadHistory reads the weave-history configuration.
func LoadHistory() (History, error) {
	grace, err := time.ParseDuration(envOr("FLUSH_GRACE", defaultFlushGrace.String()))
	if err != nil {
		return History{}, fmt.Errorf("FLUSH_GRACE: %w", err)
	}
	cfg := History{
		ListenAddr:     envOr("LISTEN_ADDR", defaultListenAddr),
		WatchNamespace: envOr("WATCH_NAMESPACE", defaultWatchNamespace),
		DataDir:        envOr("DATA_DIR", defaultDataDir),
		FlushGrace:     grace,
	}
	if cfg.ListenAddr == "" {
		return History{}, fmt.Errorf("LISTEN_ADDR must not be empty")
	}
	if strings.TrimSpace(cfg.DataDir) == "" {
		return History{}, fmt.Errorf("DATA_DIR must not be empty")
	}
	if cfg.FlushGrace < 0 {
		return History{}, fmt.Errorf("FLUSH_GRACE must not be negative")
	}
	return cfg, nil
}

// Inventory is the configuration of weave-inventory.
type Inventory struct {
	ListenAddr        string
	WatchNamespace    string
	DataDir           string
	LiveURL           string
	LiveTimeout       time.Duration
	ReconcileInterval time.Duration
	ArgoCDURL         string
	ArgoCDToken       string
	ArgoCDApp         string
	ArgoCDNamespace   string
}

// LoadInventory reads the weave-inventory configuration.
func LoadInventory() (Inventory, error) {
	cfg := Inventory{
		ListenAddr:        envOr("LISTEN_ADDR", defaultListenAddr),
		WatchNamespace:    envOr("WATCH_NAMESPACE", defaultWatchNamespace),
		DataDir:           envOr("DATA_DIR", defaultDataDir),
		LiveURL:           strings.TrimRight(strings.TrimSpace(envOr("LIVE_URL", defaultLiveURL)), "/"),
		LiveTimeout:       defaultLiveTimeout,
		ReconcileInterval: defaultReconcileEvery,
		ArgoCDURL:         strings.TrimRight(strings.TrimSpace(os.Getenv("ARGOCD_URL")), "/"),
		ArgoCDToken:       os.Getenv("ARGOCD_TOKEN"),
		ArgoCDApp:         envOr("ARGOCD_APP", "target-app-local"),
		ArgoCDNamespace:   envOr("ARGOCD_NAMESPACE", defaultWatchNamespace),
	}
	if cfg.ListenAddr == "" {
		return Inventory{}, fmt.Errorf("LISTEN_ADDR must not be empty")
	}
	if strings.TrimSpace(cfg.DataDir) == "" {
		return Inventory{}, fmt.Errorf("DATA_DIR must not be empty")
	}
	if cfg.ArgoCDURL != "" && cfg.ArgoCDToken == "" {
		return Inventory{}, fmt.Errorf("ARGOCD_TOKEN is required when ARGOCD_URL is set")
	}
	return cfg, nil
}

// Edge is the configuration of weave-api.
type Edge struct {
	ListenAddr    string
	WebDir        string
	LiveURL       string
	HistoryURL    string
	InventoryURL  string
	HealthTimeout time.Duration
}

// LoadEdge reads the weave-api configuration.
func LoadEdge() (Edge, error) {
	cfg := Edge{
		ListenAddr:    envOr("LISTEN_ADDR", defaultListenAddr),
		WebDir:        envOr("WEB_DIR", defaultWebDir),
		LiveURL:       strings.TrimRight(strings.TrimSpace(envOr("LIVE_URL", defaultLiveURL)), "/"),
		HistoryURL:    strings.TrimRight(strings.TrimSpace(envOr("HISTORY_URL", defaultHistoryURL)), "/"),
		InventoryURL:  strings.TrimRight(strings.TrimSpace(envOr("INVENTORY_URL", defaultInventoryURL)), "/"),
		HealthTimeout: defaultHealthTimeout,
	}
	if cfg.ListenAddr == "" {
		return Edge{}, fmt.Errorf("LISTEN_ADDR must not be empty")
	}
	return cfg, nil
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func parseWindow(raw string) (time.Duration, error) {
	window, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil {
		return 0, fmt.Errorf("WINDOW: %w", err)
	}
	if window < minWindow || window > maxWindow {
		return 0, fmt.Errorf("WINDOW must be between %s and %s", minWindow, maxWindow)
	}
	if window/windowBucketCount < minimumBucketDuration {
		return 0, fmt.Errorf("WINDOW must give at least %s buckets", minimumBucketDuration)
	}
	return window, nil
}
