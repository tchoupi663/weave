// Package profile loads and validates the target profile and fault scenarios
// used by detection and the agent benchmark.
package profile

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

type Profile struct {
	Version                 string         `yaml:"version"`
	Name                    string         `yaml:"name"`
	Cluster                 Cluster        `yaml:"cluster"`
	Traffic                 Traffic        `yaml:"traffic"`
	EvidenceBaselineMinutes int            `yaml:"evidence_baseline_minutes"`
	GroupingWindowSeconds   int            `yaml:"grouping_window_seconds"`
	CooldownSeconds         int            `yaml:"cooldown_seconds"`
	SLARules                string         `yaml:"sla_rules"`
	ExpectedTopology        string         `yaml:"expected_topology"`
	Templates               string         `yaml:"templates"`
	Workloads               []Workload     `yaml:"workloads"`
	Scenarios               []string       `yaml:"scenarios"`
	Agents                  map[string]any `yaml:"agents"`
	SourceRoot              string         `yaml:"source_root,omitempty"`
	path                    string
	scenarioCache           map[string]Scenario
	templateClasses         map[string]bool
}

type Cluster struct {
	Namespace          string `yaml:"namespace"`
	WeaveNamespace     string `yaml:"weave_namespace"`
	ArgoApplication    string `yaml:"argo_application"`
	ReadOnlyRole       string `yaml:"read_only_role"`
	ReadOnlyKubeconfig string `yaml:"read_only_kubeconfig"`
}

type Traffic struct {
	Driver         string `yaml:"driver"`
	Config         string `yaml:"config"`
	ResetTargetApp bool   `yaml:"reset_target_app"`
	Concurrency    int    `yaml:"concurrency"`
}

type Workload struct {
	Name        string   `yaml:"name"`
	SourceRoots []string `yaml:"source_roots"`
}

type Scenario struct {
	ID                   string      `yaml:"id"`
	Description          string      `yaml:"description"`
	Template             string      `yaml:"template"`
	FaultDurationSeconds int         `yaml:"fault_duration_seconds"`
	RecoverySeconds      int         `yaml:"recovery_seconds"`
	Injection            Injection   `yaml:"injection"`
	GroundTruth          GroundTruth `yaml:"ground_truth"`
	path                 string
}

type Injection struct {
	Mechanism             string `yaml:"mechanism"`
	Fixture               string `yaml:"fixture,omitempty"`
	Target                string `yaml:"target,omitempty"`
	DeleteIntervalSeconds int    `yaml:"delete_interval_seconds,omitempty"`
	Image                 string `yaml:"image,omitempty"`
	PodLabel              string `yaml:"pod_label,omitempty"`
}

type GroundTruth struct {
	RootCauseWorkloads              []string `yaml:"root_cause_workloads"`
	FailureMode                     string   `yaml:"failure_mode"`
	AffectedRoute                   string   `yaml:"affected_route"`
	AffectedEdge                    string   `yaml:"affected_edge"`
	ExpectedInitialRegionWorkloads  []string `yaml:"expected_initial_region_workloads"`
	ExpectedRegionContainsRootCause bool     `yaml:"expected_region_contains_root_cause"`
	ExpectedExpansion               bool     `yaml:"expected_expansion"`
}

// Load reads a profile and its scenario files. Relative paths resolve against
// the repository root (the working directory of the benchmark harness).
func Load(path string) (*Profile, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var profile Profile
	if err := yaml.Unmarshal(raw, &profile); err != nil {
		return nil, fmt.Errorf("parse profile: %w", err)
	}
	profile.path = path
	profile.scenarioCache = map[string]Scenario{}
	if err := profile.validate(); err != nil {
		return nil, err
	}
	if err := profile.loadTemplates(); err != nil {
		return nil, err
	}
	return &profile, nil
}

func (profile *Profile) Path() string { return profile.path }

// TemplatesHash returns the SHA-256 of the fault-class registry.
func (profile *Profile) TemplatesHash() string {
	path := profile.Templates
	if !filepath.IsAbs(path) {
		path = filepath.Join(filepath.Dir(profile.path), path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (profile *Profile) Hash() string {
	raw, err := os.ReadFile(profile.path)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (profile *Profile) SourceMap() map[string][]string {
	result := map[string][]string{}
	for _, workload := range profile.Workloads {
		roots := workload.SourceRoots
		if len(roots) == 0 && profile.SourceRoot != "" {
			roots = []string{filepath.Join(profile.SourceRoot, workload.Name)}
		}
		if len(roots) > 0 {
			result[workload.Name] = append([]string(nil), roots...)
		}
	}
	return result
}

func (profile *Profile) AllSourceRoots() []string {
	seen := map[string]bool{}
	result := []string{}
	for _, roots := range profile.SourceMap() {
		for _, root := range roots {
			if !seen[root] {
				seen[root] = true
				result = append(result, root)
			}
		}
	}
	if profile.SourceRoot != "" && len(result) == 0 {
		result = append(result, profile.SourceRoot)
	}
	return result
}

// Scenario loads a scenario by id. Resolved path is recorded for hashing.
func (profile *Profile) Scenario(id string) (Scenario, error) {
	if scenario, ok := profile.scenarioCache[id]; ok {
		return scenario, nil
	}
	path := filepath.Join(filepath.Dir(profile.path), "scenarios", id+".yaml")
	raw, err := os.ReadFile(path)
	if err != nil {
		return Scenario{}, fmt.Errorf("scenario %q: %w", id, err)
	}
	var scenario Scenario
	if err := yaml.Unmarshal(raw, &scenario); err != nil {
		return Scenario{}, fmt.Errorf("scenario %q: %w", id, err)
	}
	scenario.path = path
	if err := profile.validateScenario(scenario); err != nil {
		return Scenario{}, err
	}
	profile.scenarioCache[id] = scenario
	return scenario, nil
}

func (profile *Profile) ScenarioHash(scenario Scenario) string {
	raw, err := os.ReadFile(scenario.path)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func (profile *Profile) validate() error {
	if strings.TrimSpace(profile.Version) == "" {
		return fmt.Errorf("profile version is required")
	}
	if strings.TrimSpace(profile.Name) == "" {
		return fmt.Errorf("profile name is required")
	}
	if profile.Cluster.Namespace == "" || profile.Cluster.WeaveNamespace == "" {
		return fmt.Errorf("cluster namespace and weave_namespace are required")
	}
	if profile.Traffic.Config == "" {
		return fmt.Errorf("traffic.config is required")
	}
	if profile.SLARules == "" {
		return fmt.Errorf("sla_rules is required")
	}
	if profile.Templates == "" {
		return fmt.Errorf("templates is required")
	}
	if len(profile.Workloads) == 0 {
		return fmt.Errorf("at least one workload is required")
	}
	workloads := map[string]bool{}
	for _, workload := range profile.Workloads {
		if workload.Name == "" || workloads[workload.Name] {
			return fmt.Errorf("workloads must have unique names")
		}
		workloads[workload.Name] = true
	}
	if len(profile.Scenarios) == 0 {
		return fmt.Errorf("at least one scenario is required")
	}
	if profile.EvidenceBaselineMinutes < 1 || profile.EvidenceBaselineMinutes > 60 {
		return fmt.Errorf("evidence_baseline_minutes must be between 1 and 60")
	}
	return nil
}

type templateRegistry struct {
	Classes []struct {
		ID     string `yaml:"id"`
		Status string `yaml:"status"`
	} `yaml:"classes"`
}

func (profile *Profile) loadTemplates() error {
	path := profile.Templates
	if !filepath.IsAbs(path) {
		path = filepath.Join(filepath.Dir(profile.path), path)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read templates: %w", err)
	}
	var registry templateRegistry
	if err := yaml.Unmarshal(raw, &registry); err != nil {
		return fmt.Errorf("parse templates: %w", err)
	}
	profile.templateClasses = map[string]bool{}
	for _, class := range registry.Classes {
		if class.ID == "" {
			return fmt.Errorf("template class id is required")
		}
		if class.Status != "covered" && class.Status != "coming_soon" {
			return fmt.Errorf("template class %q: status must be covered or coming_soon", class.ID)
		}
		profile.templateClasses[class.ID] = true
	}
	if len(profile.templateClasses) == 0 {
		return fmt.Errorf("templates must declare at least one class")
	}
	return nil
}

func (profile *Profile) validateScenario(scenario Scenario) error {
	if strings.TrimSpace(scenario.ID) == "" {
		return fmt.Errorf("scenario id is required")
	}
	if strings.TrimSpace(scenario.Template) == "" {
		return fmt.Errorf("scenario %q: template is required", scenario.ID)
	}
	if len(profile.templateClasses) > 0 && !profile.templateClasses[scenario.Template] {
		return fmt.Errorf("scenario %q: unknown template %q", scenario.ID, scenario.Template)
	}
	if scenario.FaultDurationSeconds < 30 || scenario.RecoverySeconds < 0 {
		return fmt.Errorf("scenario %q: invalid fault/recovery duration", scenario.ID)
	}
	if len(scenario.GroundTruth.RootCauseWorkloads) == 0 {
		return fmt.Errorf("scenario %q: ground truth root cause is required", scenario.ID)
	}
	for _, workload := range scenario.GroundTruth.RootCauseWorkloads {
		found := false
		for _, profileWorkload := range profile.Workloads {
			if profileWorkload.Name == workload {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("scenario %q: ground-truth workload %q is not in the profile", scenario.ID, workload)
		}
	}
	switch scenario.Injection.Mechanism {
	case "http_chaos", "stress_chaos", "network_chaos":
		if scenario.Injection.Fixture == "" {
			return fmt.Errorf("scenario %q: fixture is required for %s", scenario.ID, scenario.Injection.Mechanism)
		}
	case "pod_delete_loop":
		if scenario.Injection.Target == "" {
			return fmt.Errorf("scenario %q: target is required for pod_delete_loop", scenario.ID)
		}
	case "scale_zero":
		if scenario.Injection.Target == "" {
			return fmt.Errorf("scenario %q: target is required for scale_zero", scenario.ID)
		}
	case "image_swap":
		if scenario.Injection.Target == "" || scenario.Injection.Image == "" {
			return fmt.Errorf("scenario %q: target and image are required for image_swap", scenario.ID)
		}
	default:
		return fmt.Errorf("scenario %q: unknown injection mechanism %q", scenario.ID, scenario.Injection.Mechanism)
	}
	return nil
}
