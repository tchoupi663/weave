package config

import (
	"testing"
	"time"
)

func TestLoadHistoryDetectionDefaults(t *testing.T) {
	cfg, err := LoadHistory()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.DetectionEvery != 5*time.Second {
		t.Fatalf("detection interval = %s", cfg.DetectionEvery)
	}
	if cfg.Baseline != 15*time.Minute {
		t.Fatalf("baseline = %s", cfg.Baseline)
	}
	if cfg.GroupingWindow != 2*time.Minute {
		t.Fatalf("grouping = %s", cfg.GroupingWindow)
	}
	if cfg.Cooldown != 5*time.Minute {
		t.Fatalf("cooldown = %s", cfg.Cooldown)
	}
	if cfg.SLAFile != "" {
		t.Fatalf("SLA file = %q", cfg.SLAFile)
	}
}

func TestLoadHistoryDetectionOverrides(t *testing.T) {
	t.Setenv("SLA_RULES_FILE", "/etc/weave/slas.yaml")
	t.Setenv("DETECTION_INTERVAL", "10s")
	t.Setenv("BASELINE_MINUTES", "5")
	t.Setenv("GROUPING_WINDOW", "90s")
	t.Setenv("COOLDOWN", "2m")
	cfg, err := LoadHistory()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SLAFile != "/etc/weave/slas.yaml" || cfg.DetectionEvery != 10*time.Second || cfg.Baseline != 5*time.Minute {
		t.Fatalf("cfg = %+v", cfg)
	}
	if cfg.GroupingWindow != 90*time.Second || cfg.Cooldown != 2*time.Minute {
		t.Fatalf("windows = %s/%s", cfg.GroupingWindow, cfg.Cooldown)
	}
}

func TestLoadHistoryRejectsBadDetectionValues(t *testing.T) {
	cases := []struct {
		key   string
		value string
	}{
		{"DETECTION_INTERVAL", "0s"},
		{"DETECTION_INTERVAL", "2m"},
		{"BASELINE_MINUTES", "0"},
		{"BASELINE_MINUTES", "121"},
		{"BASELINE_MINUTES", "abc"},
		{"GROUPING_WINDOW", "-1s"},
		{"COOLDOWN", "-1s"},
	}
	for _, testCase := range cases {
		t.Run(testCase.key+"="+testCase.value, func(t *testing.T) {
			t.Setenv(testCase.key, testCase.value)
			if _, err := LoadHistory(); err == nil {
				t.Fatalf("expected error for %s=%s", testCase.key, testCase.value)
			}
		})
	}
}
