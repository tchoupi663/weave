// Package benchmark runs paired confined and unconfined investigations.
package benchmark

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/tchoupi663/weave/services/agent/internal/investigation"
	"github.com/tchoupi663/weave/services/agent/internal/profile"
	"github.com/tchoupi663/weave/services/agent/internal/scope"
)

type Options struct {
	ProfilePath, WeaveURL, AgentBinary, OutDir, KubeConfig, ReadOnlyKubeConfig string
	ScenarioIDs                                                                []string
	Repetitions, MaxToolCalls, MaxTokens                                       int
	MaxWall                                                                    time.Duration
	WarmupSeconds                                                              int
	Pilot                                                                      bool
}

type Benchmark struct {
	opts        Options
	profile     *profile.Profile
	client      *http.Client
	outDir, dir string
}

type episode struct {
	ScenarioID         string `json:"scenario_id"`
	Repetition         int    `json:"repetition"`
	IncidentID         string `json:"incident_id,omitempty"`
	DetectionLatencyMS int64  `json:"detection_latency_ms,omitempty"`
	RegionRecall       bool   `json:"region_recall"`
	Status             string `json:"status"`
	Notes              string `json:"notes,omitempty"`
}

type runRecord struct {
	ScenarioID string `json:"scenario_id"`
	Repetition int    `json:"repetition"`
	IncidentID string `json:"incident_id"`
	Arm        string `json:"arm"`
	RunID      string `json:"run_id"`
	Status     string `json:"status"`
	Correct    bool   `json:"correct"`
	Tokens     int    `json:"tokens_total"`
	ToolCalls  int    `json:"tool_calls"`
	Denials    int    `json:"denials"`
	Expansions int    `json:"expansions"`
}

type incident struct {
	ID, Status, Origin                                            string
	CreatedAt, WindowStart, WindowEnd, BaselineStart, BaselineEnd time.Time
	Evidence                                                      json.RawMessage
}

func New(opts Options) *Benchmark {
	return &Benchmark{opts: opts, client: &http.Client{Timeout: 15 * time.Second}}
}

func (b *Benchmark) Run(ctx context.Context) error {
	if b.outDir == "" {
		b.outDir = b.opts.OutDir
	}
	if b.outDir == "" {
		b.outDir = filepath.Join("reports", "agent-benchmark", time.Now().UTC().Format("20060102T150405Z"))
	}
	if err := os.MkdirAll(b.outDir, 0o755); err != nil {
		return err
	}
	b.dir = b.outDir
	if b.opts.ProfilePath == "" {
		b.opts.ProfilePath = "deploy/targets/target-app/profile.yaml"
	}
	if b.opts.WeaveURL == "" {
		b.opts.WeaveURL = "http://127.0.0.1:8090"
	}
	if b.opts.AgentBinary == "" {
		b.opts.AgentBinary = ".cache/weave-agent"
	}
	if b.opts.Repetitions <= 0 {
		b.opts.Repetitions = 1
	}
	if b.opts.MaxToolCalls <= 0 {
		b.opts.MaxToolCalls = 20
	}
	if b.opts.MaxTokens <= 0 {
		b.opts.MaxTokens = 160000
	}
	if b.opts.MaxWall <= 0 {
		b.opts.MaxWall = 10 * time.Minute
	}
	profileValue, err := profile.Load(b.opts.ProfilePath)
	if err != nil {
		return err
	}
	b.profile = profileValue
	if _, err := os.Stat(b.opts.AgentBinary); err != nil {
		return err
	}
	if err := b.health(ctx); err != nil {
		return err
	}
	ids := b.opts.ScenarioIDs
	if len(ids) == 0 {
		ids = b.profile.Scenarios
	}
	if b.opts.Pilot {
		if len(ids) > 3 {
			ids = ids[:3]
		}
		b.opts.Repetitions = 1
	}
	scenarios := []profile.Scenario{}
	for _, id := range ids {
		scenario, err := b.profile.Scenario(id)
		if err != nil {
			return err
		}
		scenarios = append(scenarios, scenario)
	}
	if err := b.writeManifest("running"); err != nil {
		return err
	}
	for _, scenario := range scenarios {
		for repetition := 1; repetition <= b.opts.Repetitions; repetition++ {
			if err := b.runEpisode(ctx, scenario, repetition); err != nil {
				fmt.Printf("episode %s/%d: %v\n", scenario.ID, repetition, err)
			}
		}
	}
	if err := b.writeManifest("complete"); err != nil {
		return err
	}
	return b.writeSummary()
}

func (b *Benchmark) runEpisode(ctx context.Context, scenario profile.Scenario, repetition int) error {
	ep := episode{ScenarioID: scenario.ID, Repetition: repetition, Status: "failed"}
	if err := b.ensureClean(ctx); err != nil {
		ep.Notes = err.Error()
		return err
	}
	warmup := b.opts.WarmupSeconds
	if warmup <= 0 {
		warmup = b.profile.EvidenceBaselineMinutes * 60
	}
	fmt.Printf("%s r%d: warmup %ds\n", scenario.ID, repetition, warmup)
	if err := sleep(ctx, time.Duration(warmup)*time.Second); err != nil {
		return err
	}
	faultStart := time.Now().UTC()
	revert, hash, err := b.inject(ctx, scenario)
	if err != nil {
		ep.Notes = err.Error()
		return err
	}
	defer revert()
	inc, err := b.waitIncident(ctx, faultStart)
	if err != nil {
		ep.Notes = err.Error()
		return err
	}
	ep.IncidentID = inc.ID
	ep.DetectionLatencyMS = inc.CreatedAt.Sub(faultStart).Milliseconds()
	files, recall, err := b.prepare(ctx, scenario, inc)
	if err != nil {
		ep.Notes = err.Error()
		return err
	}
	ep.RegionRecall = recall
	arms := []string{investigation.ArmConfined, investigation.ArmUnconfined}
	if b.scenarioIndex(scenario.ID)%2 == 1 {
		arms[0], arms[1] = arms[1], arms[0]
	}
	for _, arm := range arms {
		runDir := filepath.Join(b.dir, "episodes", fmt.Sprintf("%s-r%d", scenario.ID, repetition), arm)
		record, err := b.runArm(ctx, scenario, repetition, inc, arm, runDir, files)
		if err != nil {
			record = runRecord{ScenarioID: scenario.ID, Repetition: repetition, IncidentID: inc.ID, Arm: arm, Status: "failed"}
		}
		record.ScenarioID, record.Repetition, record.IncidentID, record.Arm = scenario.ID, repetition, inc.ID, arm
		if err := appendJSONL(filepath.Join(b.dir, "runs.jsonl"), record); err != nil {
			return err
		}
	}
	_ = hash
	ep.Status = "complete"
	if err := sleep(ctx, time.Duration(scenario.RecoverySeconds)*time.Second); err != nil {
		return err
	}
	_ = ep
	return nil
}

type files struct{ scope, region string }

func (b *Benchmark) prepare(ctx context.Context, scenario profile.Scenario, inc incident) (files, bool, error) {
	rawIncident, err := b.get(ctx, "/api/incidents/"+url.PathEscape(inc.ID))
	if err != nil {
		return files{}, false, err
	}
	rawTimeline, _ := b.get(ctx, "/api/incidents/"+url.PathEscape(inc.ID)+"/timeline")
	rawTopology, err := b.get(ctx, "/api/topology")
	if err != nil {
		return files{}, false, err
	}
	var topology struct {
		Edges []struct{ ID, Source, Target string } `json:"edges"`
	}
	if err := json.Unmarshal(rawTopology, &topology); err != nil {
		return files{}, false, err
	}
	edges := make([]scope.Edge, 0, len(topology.Edges))
	for _, e := range topology.Edges {
		edges = append(edges, scope.Edge{ID: e.ID, Source: e.Source, Target: e.Target})
	}
	var evidence struct {
		SuspectEdges []string                                `json:"suspect_edges"`
		SuspectNodes []string                                `json:"suspect_nodes"`
		Routes       []struct{ EdgeID, Method, Path string } `json:"routes"`
	}
	if err := json.Unmarshal(inc.Evidence, &evidence); err != nil {
		return files{}, false, err
	}
	nodes, workloads := []string{}, []string{}
	for _, node := range evidence.SuspectNodes {
		full := node
		if !strings.HasPrefix(node, "workload:") {
			full = "workload:" + b.profile.Cluster.Namespace + "/" + node
		}
		nodes = append(nodes, full)
		if workload := scope.WorkloadFromNode(full); workload != "" {
			workloads = append(workloads, workload)
		}
	}
	sourceMap := b.profile.SourceMap()
	sourcePaths := []string{}
	for _, workload := range workloads {
		sourcePaths = append(sourcePaths, sourceMap[workload]...)
	}
	routes := []scope.Route{}
	for _, route := range evidence.Routes {
		if contains(evidence.SuspectEdges, route.EdgeID) {
			routes = append(routes, scope.Route{EdgeID: route.EdgeID, Method: route.Method, Path: route.Path})
		}
	}
	limits := scope.Limits{MaxExpansions: 3, MaxAddedNodes: 8}
	state, err := scope.New(inc.ID, scope.TimeRange{Start: inc.BaselineStart, End: inc.WindowEnd},
		scope.Entities{Nodes: nodes, Edges: evidence.SuspectEdges, Routes: routes, Workloads: unique(workloads), SourcePaths: unique(sourcePaths)}, limits, sourceMap, edges)
	if err != nil {
		return files{}, false, err
	}
	incidentDir := filepath.Join(b.dir, "incidents", inc.ID)
	if err := os.MkdirAll(incidentDir, 0o755); err != nil {
		return files{}, false, err
	}
	scopePath := filepath.Join(incidentDir, "scope.json")
	if err := scope.Save(scopePath, state); err != nil {
		return files{}, false, err
	}
	region, err := investigation.BuildRegion(rawIncident, rawTimeline, state.Frontier, limits)
	if err != nil {
		return files{}, false, err
	}
	regionPath := filepath.Join(incidentDir, "region.json")
	if err := os.WriteFile(regionPath, region, 0o644); err != nil {
		return files{}, false, err
	}
	return files{scope: scopePath, region: regionPath}, allIn(scenario.GroundTruth.RootCauseWorkloads, workloads), nil
}

func (b *Benchmark) runArm(ctx context.Context, scenario profile.Scenario, repetition int, inc incident, arm, runDir string, f files) (runRecord, error) {
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return runRecord{}, err
	}
	args := []string{"investigator", "--incident", inc.ID, "--arm", arm, "--run-dir", runDir,
		"--weave-url", b.opts.WeaveURL, "--max-tool-calls", fmt.Sprint(b.opts.MaxToolCalls),
		"--max-tokens", fmt.Sprint(b.opts.MaxTokens), "--max-wall", b.opts.MaxWall.String()}
	env := append(os.Environ(), "WEAVE_URL="+b.opts.WeaveURL, "WEAVE_NAMESPACE="+b.profile.Cluster.Namespace,
		"WEAVE_SOURCE_ROOTS="+strings.Join(b.profile.AllSourceRoots(), ","),
		"WEAVE_MAX_TOOL_CALLS="+fmt.Sprint(b.opts.MaxToolCalls), "WEAVE_MAX_TOKENS="+fmt.Sprint(b.opts.MaxTokens),
		"WEAVE_MAX_WALL="+b.opts.MaxWall.String())
	if b.opts.ReadOnlyKubeConfig != "" {
		env = append(env, "KUBECONFIG="+b.opts.ReadOnlyKubeConfig)
	}
	if arm == investigation.ArmConfined {
		env = append(env, "WEAVE_SCOPE_FILE="+f.scope)
		args = append(args, "--region-file", f.region)
	}
	cmd := exec.CommandContext(ctx, b.opts.AgentBinary, args...)
	cmd.Env = env
	log, err := os.Create(filepath.Join(runDir, "agent.log"))
	if err != nil {
		return runRecord{}, err
	}
	cmd.Stdout, cmd.Stderr = log, log
	runErr := cmd.Run()
	log.Close()
	raw, err := os.ReadFile(filepath.Join(runDir, "run.json"))
	if err != nil {
		return runRecord{}, fmt.Errorf("run.json missing: %v", runErr)
	}
	var record investigation.RunRecord
	if err := json.Unmarshal(raw, &record); err != nil {
		return runRecord{}, err
	}
	correct, _, _ := score(scenario.GroundTruth, record.Diagnosis)
	out := runRecord{ScenarioID: scenario.ID, Repetition: repetition, IncidentID: inc.ID, Arm: arm, RunID: record.RunID,
		Status: record.Status, Correct: correct, Tokens: record.Tokens.Total, ToolCalls: record.ToolCalls, Denials: record.Denials, Expansions: record.Expansions}
	return out, nil
}

func score(truth profile.GroundTruth, diagnosis *investigation.Diagnosis) (bool, bool, bool) {
	if diagnosis == nil {
		return false, false, false
	}
	correct := false
	for _, workload := range truth.RootCauseWorkloads {
		if strings.EqualFold(workload, diagnosis.RootCause.Workload) {
			correct = true
		}
	}
	routeMatch := truth.AffectedRoute == "" || normalize(diagnosis.Affected.Route) == normalize(truth.AffectedRoute)
	modeMatch := truth.FailureMode == "" || diagnosis.RootCause.FailureMode == truth.FailureMode
	return correct, routeMatch, modeMatch
}
func normalize(value string) string { return strings.Join(strings.Fields(value), " ") }

func (b *Benchmark) inject(ctx context.Context, scenario profile.Scenario) (func(), string, error) {
	switch scenario.Injection.Mechanism {
	case "http_chaos":
		if err := b.kubectl(ctx, "apply", "-f", scenario.Injection.Fixture); err != nil {
			return func() {}, "", err
		}
		return func() {
			_ = b.kubectl(context.Background(), "delete", "-f", scenario.Injection.Fixture, "--ignore-not-found")
		}, fileHash(scenario.Injection.Fixture), nil
	case "pod_delete_loop":
		stop := make(chan struct{})
		interval := scenario.Injection.DeleteIntervalSeconds
		if interval <= 0 {
			interval = 20
		}
		go func() {
			t := time.NewTicker(time.Duration(interval) * time.Second)
			defer t.Stop()
			for {
				select {
				case <-stop:
					return
				case <-t.C:
					_ = b.kubectl(context.Background(), "delete", "pod", "-l", "app="+scenario.Injection.Target, "--grace-period=0", "--force")
				}
			}
		}()
		_ = b.kubectl(ctx, "delete", "pod", "-l", "app="+scenario.Injection.Target, "--grace-period=0", "--force")
		return func() { close(stop) }, "pod_delete_loop:" + scenario.Injection.Target, nil
	case "image_swap":
		target := "deployment/" + scenario.Injection.Target
		original, err := b.kubectlOutput(ctx, "get", target, "-o", "jsonpath={.spec.template.spec.containers[0].image}")
		if err != nil {
			return func() {}, "", err
		}
		if err := b.kubectl(ctx, "set", "image", target, scenario.Injection.Target+"="+scenario.Injection.Image); err != nil {
			return func() {}, "", err
		}
		return func() {
			_ = b.kubectl(context.Background(), "set", "image", target, scenario.Injection.Target+"="+original)
		}, "image:" + scenario.Injection.Image, nil
	default:
		return func() {}, "", fmt.Errorf("unknown injection %q", scenario.Injection.Mechanism)
	}
}

func (b *Benchmark) waitIncident(ctx context.Context, start time.Time) (incident, error) {
	deadline := time.Now().Add(150 * time.Second)
	for time.Now().Before(deadline) {
		incidents, err := b.incidents(ctx)
		if err != nil {
			return incident{}, err
		}
		for _, inc := range incidents {
			if inc.Origin == "detection" && inc.CreatedAt.After(start.Add(-30*time.Second)) {
				return inc, nil
			}
		}
		if err := sleep(ctx, 5*time.Second); err != nil {
			return incident{}, err
		}
	}
	return incident{}, fmt.Errorf("no detected incident within 150s")
}

func (b *Benchmark) incidents(ctx context.Context) ([]incident, error) {
	raw, err := b.get(ctx, "/api/incidents?limit=100")
	if err != nil {
		return nil, err
	}
	var items []struct {
		ID, Status, Origin                                            string
		CreatedAt, WindowStart, WindowEnd, BaselineStart, BaselineEnd time.Time
		Evidence                                                      json.RawMessage
	}
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, err
	}
	result := []incident{}
	for _, item := range items {
		result = append(result, incident{ID: item.ID, Status: item.Status, Origin: item.Origin, CreatedAt: item.CreatedAt, WindowStart: item.WindowStart, WindowEnd: item.WindowEnd, BaselineStart: item.BaselineStart, BaselineEnd: item.BaselineEnd, Evidence: item.Evidence})
	}
	return result, nil
}

func (b *Benchmark) ensureClean(ctx context.Context) error {
	items, err := b.incidents(ctx)
	if err != nil {
		return err
	}
	for _, inc := range items {
		if inc.Origin == "detection" && inc.Status == "open" {
			return fmt.Errorf("open detected incident %s; close it first", inc.ID)
		}
	}
	return nil
}

func (b *Benchmark) health(ctx context.Context) error {
	raw, err := b.get(ctx, "/api/health")
	if err != nil {
		return err
	}
	var health map[string]any
	if err := json.Unmarshal(raw, &health); err != nil {
		return err
	}
	if health["detection_enabled"] != true {
		return fmt.Errorf("detection not enabled: %s", raw)
	}
	return nil
}
func (b *Benchmark) get(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(b.opts.WeaveURL, "/")+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := b.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("weave %s: %d", path, resp.StatusCode)
	}
	return io.ReadAll(resp.Body)
}
func (b *Benchmark) kubectl(ctx context.Context, args ...string) error {
	_, err := b.kubectlOutput(ctx, args...)
	return err
}
func (b *Benchmark) kubectlOutput(ctx context.Context, args ...string) (string, error) {
	base := []string{}
	if b.opts.KubeConfig != "" {
		base = append(base, "--kubeconfig", b.opts.KubeConfig)
	}
	out, err := exec.CommandContext(ctx, "kubectl", append(base, args...)...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("kubectl %v: %w: %s", args, err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}
func (b *Benchmark) writeManifest(status string) error {
	return writeJSON(filepath.Join(b.dir, "manifest.json"), map[string]any{"run_id": filepath.Base(b.dir), "status": status, "profile_sha256": b.profile.Hash(), "repetitions": b.opts.Repetitions, "caps": map[string]any{"tokens": b.opts.MaxTokens, "tool_calls": b.opts.MaxToolCalls}})
}
func (b *Benchmark) writeSummary() error {
	return os.WriteFile(filepath.Join(b.dir, "summary.md"), []byte("# Agent Benchmark\n\nArtifacts are in this directory. See runs.jsonl and manifest.json.\n"), 0o644)
}
func writeJSON(path string, value any) error {
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(raw, '\n'), 0o644)
}
func appendJSONL(path string, value any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(append(raw, '\n'))
	return err
}
func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}
func fileHash(path string) string {
	raw, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}
func unique(values []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, v := range values {
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
func contains(values []string, value string) bool {
	for _, v := range values {
		if v == value {
			return true
		}
	}
	return false
}
func allIn(values, set []string) bool {
	for _, v := range values {
		if !contains(set, v) {
			return false
		}
	}
	return true
}
func (b *Benchmark) scenarioIndex(id string) int {
	for i, v := range b.profile.Scenarios {
		if v == id {
			return i
		}
	}
	return 0
}
