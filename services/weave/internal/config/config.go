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
	defaultWebDir          = "web/dist"
	minWindow              = 30 * time.Second
	maxWindow              = time.Hour
	windowBucketCount      = 60
	minimumBucketDuration  = time.Second
)

// Config is the process-level configuration. Everything has a working default
// for the in-cluster deployment so the binary can also be run locally with only
// HUBBLE_RELAY_ADDR changed.
type Config struct {
	ListenAddr      string
	HubbleRelayAddr string
	WatchNamespace  string
	Window          time.Duration
	WebDir          string
	DataDir         string
	ArgoCDURL       string
	ArgoCDToken     string
	ArgoCDApp       string
	ArgoCDNamespace string
}

func Load() (Config, error) {
	cfg := Config{
		ListenAddr:      envOr("LISTEN_ADDR", defaultListenAddr),
		HubbleRelayAddr: envOr("HUBBLE_RELAY_ADDR", defaultHubbleRelayAddr),
		WatchNamespace:  envOr("WATCH_NAMESPACE", defaultWatchNamespace),
		WebDir:          envOr("WEB_DIR", defaultWebDir),
		DataDir:         envOr("DATA_DIR", ""),
		ArgoCDURL:       envOr("ARGOCD_URL", ""),
		ArgoCDToken:     envOr("ARGOCD_TOKEN", ""),
		ArgoCDApp:       envOr("ARGOCD_APP", "target-app-local"),
		ArgoCDNamespace: envOr("ARGOCD_NAMESPACE", ""),
	}

	window, err := parseWindow(envOr("WINDOW", defaultWindow.String()))
	if err != nil {
		return Config{}, err
	}
	cfg.Window = window

	if strings.TrimSpace(cfg.ListenAddr) == "" {
		return Config{}, fmt.Errorf("LISTEN_ADDR must not be empty")
	}
	if strings.TrimSpace(cfg.HubbleRelayAddr) == "" {
		return Config{}, fmt.Errorf("HUBBLE_RELAY_ADDR must not be empty")
	}
	if strings.TrimSpace(cfg.WatchNamespace) == "" {
		return Config{}, fmt.Errorf("WATCH_NAMESPACE must not be empty")
	}
	if cfg.ArgoCDNamespace == "" {
		cfg.ArgoCDNamespace = cfg.WatchNamespace
	}
	if cfg.Window < minWindow || cfg.Window > maxWindow {
		return Config{}, fmt.Errorf("WINDOW must be between %s and %s", minWindow, maxWindow)
	}
	if cfg.Window/windowBucketCount < minimumBucketDuration {
		return Config{}, fmt.Errorf("WINDOW must be at least %s", windowBucketCount*minimumBucketDuration)
	}

	return cfg, nil
}

func envOr(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return fallback
}

func parseWindow(raw string) (time.Duration, error) {
	d, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("parse WINDOW: %w", err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("WINDOW must be positive")
	}
	return d, nil
}
