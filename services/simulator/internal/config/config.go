package config

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

// Duration is a YAML duration such as 30s or 10m.
type Duration time.Duration

func (d Duration) D() time.Duration {
	return time.Duration(d)
}

// UnmarshalYAML accepts strings (10m) and integers (seconds).
func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var s string
	if err := value.Decode(&s); err == nil {
		parsed, err := time.ParseDuration(s)
		if err != nil {
			return fmt.Errorf("invalid duration %q: %w", s, err)
		}
		*d = Duration(parsed)
		return nil
	}
	var seconds int64
	if err := value.Decode(&seconds); err != nil {
		return fmt.Errorf("invalid duration: %w", err)
	}
	*d = Duration(time.Duration(seconds) * time.Second)
	return nil
}

// MarshalJSON renders the duration as a Go duration string.
func (d Duration) MarshalJSON() ([]byte, error) {
	return json.Marshal(time.Duration(d).String())
}

// Config is the run spec.
type Config struct {
	Run Run `yaml:"run" json:"run"`
}

// Run is one Simulator execution.
type Run struct {
	ID               string         `yaml:"id" json:"id"`
	Seed             int64          `yaml:"seed" json:"seed"`
	Target           string         `yaml:"target" json:"target"`
	ExpectedTopology string         `yaml:"expected_topology" json:"expected_topology"`
	Duration         Duration       `yaml:"duration" json:"duration"`
	Concurrency      int            `yaml:"concurrency" json:"concurrency"`
	Ramp             Duration       `yaml:"ramp" json:"ramp"`
	ThinkTime        ThinkTime      `yaml:"think_time" json:"think_time"`
	Mix              map[string]int `yaml:"mix" json:"mix"`
	Accounts         Accounts       `yaml:"accounts" json:"accounts"`
	Reset            bool           `yaml:"reset" json:"reset"`
	TLS              TLS            `yaml:"tls" json:"tls"`
}

// ThinkTime is the pause range between user-visible steps.
type ThinkTime struct {
	Min Duration `yaml:"min" json:"min"`
	Max Duration `yaml:"max" json:"max"`
}

// Accounts configures the Auth account pool.
type Accounts struct {
	Pool int `yaml:"pool" json:"pool"`
}

// TLS configures the external HTTPS client.
type TLS struct {
	InsecureSkipVerify bool `yaml:"insecure_skip_verify" json:"insecure_skip_verify"`
}

// Default returns the documented defaults.
func Default() Config {
	return Config{Run: Run{
		Seed:             42,
		ExpectedTopology: "expected-topology.yaml",
		Duration:         Duration(10 * time.Minute),
		Concurrency:      50,
		Ramp:             Duration(30 * time.Second),
		ThinkTime: ThinkTime{
			Min: Duration(time.Second),
			Max: Duration(5 * time.Second),
		},
		Mix:      map[string]int{"bounce": 30, "browse": 50, "order": 20},
		Accounts: Accounts{Pool: 2000},
		Reset:    true,
		TLS:      TLS{InsecureSkipVerify: true},
	}}
}

// Load reads, defaults, and validates a run spec.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	cfg := Default()
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	if cfg.Run.ID == "" {
		cfg.Run.ID = "run-" + randomHex(6)
	}
	return cfg, nil
}

// Validate checks the run spec invariants.
func (cfg Config) Validate() error {
	r := cfg.Run
	if r.Target == "" {
		return fmt.Errorf("run.target is required")
	}
	u, err := url.Parse(r.Target)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("run.target must be an http(s) URL")
	}
	if r.ExpectedTopology == "" {
		return fmt.Errorf("run.expected_topology is required")
	}
	if r.Duration <= 0 {
		return fmt.Errorf("run.duration must be positive")
	}
	if r.Concurrency <= 0 {
		return fmt.Errorf("run.concurrency must be positive")
	}
	if r.Ramp < 0 {
		return fmt.Errorf("run.ramp must not be negative")
	}
	if r.ThinkTime.Min < 0 || r.ThinkTime.Max < r.ThinkTime.Min {
		return fmt.Errorf("run.think_time must satisfy 0 <= min <= max")
	}
	if r.Accounts.Pool <= 0 || r.Accounts.Pool > 2000 {
		return fmt.Errorf("run.accounts.pool must be between 1 and 2000")
	}

	total := 0
	for name, share := range r.Mix {
		switch name {
		case "bounce", "browse", "order":
		default:
			return fmt.Errorf("run.mix contains unknown journey %q", name)
		}
		if share < 0 {
			return fmt.Errorf("run.mix.%s must not be negative", name)
		}
		total += share
	}
	if total != 100 {
		return fmt.Errorf("run.mix must sum to 100, got %d", total)
	}
	return nil
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(b)
}
