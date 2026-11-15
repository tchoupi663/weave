// Package config holds the local investigator and MCP server settings.
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	DefaultWeaveURL      = "http://127.0.0.1:8090"
	DefaultOllamaURL     = "http://127.0.0.1:11434"
	DefaultModel         = "qwen3.5:4b"
	DefaultModelDigest   = "7f3251fa6878a78a606bcb3c074283306e5ad2ea411a7774a9dd98a4bc7f2d13"
	DefaultNamespace     = "weave"
	DefaultPromptVersion = "investigator-v1"
	DefaultMaxToolCalls  = 20
	DefaultMaxTokens     = 160000
	DefaultMaxWall       = 10 * time.Minute
	DefaultNumCtx        = 32768
	DefaultNumPredict    = 2000
	DefaultTemperature   = 0.0
	DefaultSourceRoot    = "services/app"
)

type Config struct {
	WeaveURL      string
	OllamaURL     string
	ModelName     string
	ModelDigest   string
	KubeConfig    string
	Namespace     string
	SourceRoots   []string
	ScopeFile     string
	RunDir        string
	PromptVersion string
	Temperature   float64
	Think         bool
	NumCtx        int
	NumPredict    int
	MaxToolCalls  int
	MaxTokens     int
	MaxWall       time.Duration
}

// FromEnv loads the local agent configuration. Empty values use local defaults
// so a smoke test needs no flags.
func FromEnv() Config {
	cfg := Config{
		WeaveURL:      envOr("WEAVE_URL", DefaultWeaveURL),
		OllamaURL:     envOr("OLLAMA_URL", DefaultOllamaURL),
		ModelName:     envOr("WEAVE_MODEL", DefaultModel),
		ModelDigest:   envOr("WEAVE_MODEL_DIGEST", DefaultModelDigest),
		KubeConfig:    strings.TrimSpace(os.Getenv("KUBECONFIG")),
		Namespace:     envOr("WEAVE_NAMESPACE", DefaultNamespace),
		SourceRoots:   splitList(envOr("WEAVE_SOURCE_ROOTS", DefaultSourceRoot)),
		ScopeFile:     strings.TrimSpace(os.Getenv("WEAVE_SCOPE_FILE")),
		RunDir:        strings.TrimSpace(os.Getenv("WEAVE_RUN_DIR")),
		PromptVersion: envOr("WEAVE_PROMPT_VERSION", DefaultPromptVersion),
		Temperature:   envFloat("WEAVE_TEMPERATURE", DefaultTemperature),
		Think:         envBool("WEAVE_MODEL_THINK", true),
		NumCtx:        envInt("WEAVE_NUM_CTX", DefaultNumCtx),
		NumPredict:    envInt("WEAVE_NUM_PREDICT", DefaultNumPredict),
		MaxToolCalls:  envInt("WEAVE_MAX_TOOL_CALLS", DefaultMaxToolCalls),
		MaxTokens:     envInt("WEAVE_MAX_TOKENS", DefaultMaxTokens),
		MaxWall:       envDuration("WEAVE_MAX_WALL", DefaultMaxWall),
	}
	return cfg
}

func envOr(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func splitList(raw string) []string {
	var result []string
	for _, item := range strings.Split(raw, ",") {
		item = strings.TrimSpace(item)
		if item != "" {
			result = append(result, item)
		}
	}
	return result
}

func envInt(key string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return value
}

func envBool(key string, fallback bool) bool {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseBool(raw)
	if err != nil {
		return fallback
	}
	return value
}

func envFloat(key string, fallback float64) float64 {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return fallback
	}
	return value
}

func envDuration(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return value
}
