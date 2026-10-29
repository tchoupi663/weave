package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "run.yaml")
	data := []byte(`run:
  target: https://localhost:8443
  expected_topology: expected-topology.yaml
  duration: 5s
  concurrency: 2
  mix: {bounce: 30, browse: 50, order: 20}
`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Run.ID == "" {
		t.Fatal("expected generated run id")
	}
	if cfg.Run.Seed != 42 {
		t.Fatalf("seed = %d", cfg.Run.Seed)
	}
	if cfg.Run.Ramp.D() != 30*time.Second {
		t.Fatalf("ramp = %s", cfg.Run.Ramp.D())
	}
	if cfg.Run.ThinkTime.Min.D() != time.Second || cfg.Run.ThinkTime.Max.D() != 5*time.Second {
		t.Fatalf("think time = %s..%s", cfg.Run.ThinkTime.Min.D(), cfg.Run.ThinkTime.Max.D())
	}
	if cfg.Run.Accounts.Pool != 2000 {
		t.Fatalf("pool = %d", cfg.Run.Accounts.Pool)
	}
	if !cfg.Run.Reset || !cfg.Run.TLS.InsecureSkipVerify {
		t.Fatal("expected reset and insecure TLS defaults")
	}
}

func TestValidateMixSum(t *testing.T) {
	cfg := Default()
	cfg.Run.Target = "https://localhost:8443"
	cfg.Run.Mix = map[string]int{"bounce": 30, "browse": 50, "order": 19}
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected mix sum error")
	}
	cfg.Run.Mix["order"] = 20
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
}

func TestValidateTarget(t *testing.T) {
	cfg := Default()
	cfg.Run.Target = "not a url"
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected target error")
	}
}
