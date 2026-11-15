package history

import (
	"context"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/tchoupi663/weave/services/weave/internal/topology"
)

func writeRulePack(t *testing.T, contents string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "slas.yaml")
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func openTestHistory(t *testing.T) *History {
	t.Helper()
	hist, err := Open(t.TempDir(), "weave", 0, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = hist.Close() })
	return hist
}

func configureTestDetector(t *testing.T, hist *History, rules string) *Detector {
	t.Helper()
	path := writeRulePack(t, rules)
	if err := hist.ConfigureDetection(path, 5*time.Second, 120*time.Second, 300*time.Second); err != nil {
		t.Fatal(err)
	}
	hist.detector.skipWarmup = true
	return hist.detector
}

func addL7(t *testing.T, hist *History, at time.Time, source, target, path string, status uint32) {
	t.Helper()
	port := uint32(8082)
	if source == "frontend" {
		port = 8080
	}
	hist.ObserveL7(source, target, port, topology.L7Observation{
		SourceWorkload: source,
		TargetWorkload: target,
		Port:           port,
		Method:         "GET",
		Path:           path,
		StatusCode:     status,
		LatencySource:  "client",
		Duration:       2 * time.Millisecond,
		Time:           at,
	})
}

func TestLoadSLARulePackValidation(t *testing.T) {
	valid := `version: test-v1
rules:
  - id: catalog-5xx
    description: catalog errors
    severity: critical
    scope: {kind: route, method: get, path: /api/products}
    metric: error_rate_5xx
    threshold: 0.01
    window_seconds: 60
    min_requests: 10
    sustain_evaluations: 2
    clear_evaluations: 12
`
	pack, err := loadSLARulePack(writeRulePack(t, valid))
	if err != nil {
		t.Fatal(err)
	}
	if pack.Version != "test-v1" || len(pack.Rules) != 1 || pack.Rules[0].Scope.Method != "GET" {
		t.Fatalf("pack = %+v", pack)
	}

	cases := map[string]string{
		"missing version": `rules: [{id: r, severity: critical, scope: {kind: edge, edge_id: a->b:1}, metric: error_rate, threshold: 0.1}]`,
		"duplicate ids": valid + `  - id: catalog-5xx
    severity: warning
    scope: {kind: edge, edge_id: a->b:1}
    metric: error_rate
    threshold: 0.1
`,
		"bad scope": `version: v
rules: [{id: r, severity: critical, scope: {kind: route}, metric: error_rate, threshold: 0.1}]`,
		"bad metric": `version: v
rules: [{id: r, severity: critical, scope: {kind: edge, edge_id: a->b:1}, metric: nope, threshold: 0.1}]`,
		"bad rate": `version: v
rules: [{id: r, severity: critical, scope: {kind: edge, edge_id: a->b:1}, metric: error_rate, threshold: 2}]`,
		"bad p95": `version: v
rules: [{id: r, severity: critical, scope: {kind: edge, edge_id: a->b:1}, metric: p95_latency_ms, threshold: 0}]`,
		"bad sustain": `version: v
rules: [{id: r, severity: critical, scope: {kind: edge, edge_id: a->b:1}, metric: error_rate, threshold: 0.1, sustain_evaluations: -1, clear_evaluations: 2}]`,
		"bad window": `version: v
rules: [{id: r, severity: critical, scope: {kind: edge, edge_id: a->b:1}, metric: error_rate, threshold: 0.1, window_seconds: 5}]`,
		"bad severity": `version: v
rules: [{id: r, severity: info, scope: {kind: edge, edge_id: a->b:1}, metric: error_rate, threshold: 0.1}]`,
	}
	for name, contents := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := loadSLARulePack(writeRulePack(t, contents)); err == nil {
				t.Fatalf("expected validation error for %s", name)
			}
		})
	}
}

func TestDetectorCreatesAlertAndIncidentThenCloses(t *testing.T) {
	hist := openTestHistory(t)
	rules := `version: test-v1
rules:
  - id: catalog-5xx
    description: Catalog product errors
    severity: critical
    scope: {kind: route, method: GET, path: /api/products}
    metric: error_rate_5xx
    threshold: 0.01
    window_seconds: 60
    min_requests: 5
    sustain_evaluations: 1
    clear_evaluations: 2
`
	detector := configureTestDetector(t, hist, rules)
	ctx := context.Background()
	breachBucket := time.Now().UTC().Truncate(bucket5Duration).Add(-2 * time.Minute)
	for i := 0; i < 20; i++ {
		addL7(t, hist, breachBucket.Add(time.Duration(i)*100*time.Millisecond), "gateway", "catalog", "/api/products", 500)
	}
	if err := hist.flushOnce(breachBucket.Add(2 * time.Minute)); err != nil {
		t.Fatal(err)
	}

	tick := breachBucket.Add(time.Minute)
	detector.tick(ctx, tick)

	incidents, err := hist.ListIncidents(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(incidents) != 1 {
		t.Fatalf("incidents = %d", len(incidents))
	}
	incident := incidents[0]
	if incident.Origin != IncidentOriginDetect {
		t.Fatalf("origin = %q", incident.Origin)
	}
	if incident.Trigger == nil || incident.Trigger.RuleID != "catalog-5xx" {
		t.Fatalf("trigger = %+v", incident.Trigger)
	}
	if !incident.WindowStart.Equal(breachBucket) {
		t.Fatalf("window start = %s, want %s", incident.WindowStart, breachBucket)
	}
	alerts, err := hist.AlertsForIncident(ctx, incident.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(alerts) != 1 || alerts[0].Metric != metricErrorRate5xx || alerts[0].Observed < 0.99 {
		t.Fatalf("alerts = %+v", alerts)
	}

	// Two clear evaluations close the incident.
	detector.tick(ctx, tick.Add(2*time.Minute))
	detector.tick(ctx, tick.Add(2*time.Minute+5*time.Second))
	closed, err := hist.GetIncident(ctx, incident.ID)
	if err != nil {
		t.Fatal(err)
	}
	if closed.Status != IncidentStatusClosed {
		t.Fatalf("status = %q", closed.Status)
	}
	if closed.Origin != IncidentOriginDetect || closed.Trigger == nil {
		t.Fatalf("closed incident lost detection provenance: %+v", closed)
	}
}

func TestDetectorHonorsMinRequestsAndRecordingGaps(t *testing.T) {
	hist := openTestHistory(t)
	rules := `version: test-v1
rules:
  - id: catalog-5xx
    description: Catalog product errors
    severity: critical
    scope: {kind: route, method: GET, path: /api/products}
    metric: error_rate_5xx
    threshold: 0.01
    window_seconds: 60
    min_requests: 10
    sustain_evaluations: 1
    clear_evaluations: 2
`
	detector := configureTestDetector(t, hist, rules)
	ctx := context.Background()
	at := time.Now().UTC().Truncate(bucket5Duration).Add(-2 * time.Minute)
	addL7(t, hist, at, "gateway", "catalog", "/api/products", 500)
	if err := hist.flushOnce(at.Add(2 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	detector.tick(ctx, at.Add(time.Minute))
	if incidents, _ := hist.ListIncidents(ctx, 10); len(incidents) != 0 {
		t.Fatalf("low-volume incident = %+v", incidents)
	}

	for i := 0; i < 20; i++ {
		addL7(t, hist, at.Add(time.Duration(i)*100*time.Millisecond), "gateway", "catalog", "/api/products", 500)
	}
	if err := hist.flushOnce(at.Add(3 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := hist.insertGap(ctx, at.Add(time.Minute), at.Add(90*time.Second)); err != nil {
		t.Fatal(err)
	}
	detector.tick(ctx, at.Add(2*time.Minute))
	if incidents, _ := hist.ListIncidents(ctx, 10); len(incidents) != 0 {
		t.Fatalf("gap incident = %+v", incidents)
	}
	if status := hist.DetectionStatus(); status.SuppressedGaps == 0 {
		t.Fatalf("gap suppression not counted: %+v", status)
	}
}

func TestDetectorGroupsTwoRulesIntoOneIncident(t *testing.T) {
	hist := openTestHistory(t)
	rules := `version: test-v1
rules:
  - id: catalog-5xx
    description: Catalog product errors
    severity: critical
    scope: {kind: route, method: GET, path: /api/products}
    metric: error_rate_5xx
    threshold: 0.01
    window_seconds: 60
    min_requests: 5
    sustain_evaluations: 1
    clear_evaluations: 2
  - id: gateway-5xx
    description: Gateway caller errors
    severity: critical
    scope: {kind: edge, edge_id: 'workload:weave/frontend->workload:weave/gateway:8080'}
    metric: error_rate_5xx
    threshold: 0.01
    window_seconds: 60
    min_requests: 5
    sustain_evaluations: 1
    clear_evaluations: 2
`
	detector := configureTestDetector(t, hist, rules)
	ctx := context.Background()
	at := time.Now().UTC().Truncate(bucket5Duration).Add(-2 * time.Minute)
	for i := 0; i < 20; i++ {
		addL7(t, hist, at.Add(time.Duration(i)*100*time.Millisecond), "gateway", "catalog", "/api/products", 500)
		addL7(t, hist, at.Add(time.Duration(i)*100*time.Millisecond), "frontend", "gateway", "/api/products", 500)
	}
	if err := hist.flushOnce(at.Add(2 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	detector.tick(ctx, at.Add(time.Minute))
	incidents, err := hist.ListIncidents(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(incidents) != 1 {
		t.Fatalf("incidents = %d, want 1", len(incidents))
	}
	alerts, err := hist.AlertsForIncident(ctx, incidents[0].ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(alerts) != 2 {
		t.Fatalf("alerts = %d, want 2: %+v", len(alerts), alerts)
	}
}

func TestManualIncidentOmitsDetectionFields(t *testing.T) {
	hist := openTestHistory(t)
	incident, err := hist.CreateIncident(context.Background(), CreateIncidentRequest{
		Start:   time.Now().UTC().Add(-time.Minute),
		End:     time.Now().UTC(),
		Symptom: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	if incident.Origin != "" || incident.Trigger != nil {
		t.Fatalf("manual incident has detection fields: %+v", incident)
	}
}

func TestDetectorSustainsAndDetectsLatency(t *testing.T) {
	hist := openTestHistory(t)
	rules := `version: test-v1
rules:
  - id: auth-p95
    description: Login latency
    severity: warning
    scope: {kind: edge, edge_id: 'workload:weave/gateway->workload:weave/auth:8081'}
    metric: p95_latency_ms
    threshold: 50
    window_seconds: 60
    min_requests: 5
    sustain_evaluations: 2
    clear_evaluations: 2
`
	detector := configureTestDetector(t, hist, rules)
	ctx := context.Background()
	at := time.Now().UTC().Truncate(bucket5Duration).Add(-2 * time.Minute)
	for bucket := 0; bucket < 2; bucket++ {
		for i := 0; i < 10; i++ {
			hist.ObserveL7("gateway", "auth", 8081, topology.L7Observation{
				SourceWorkload: "gateway",
				TargetWorkload: "auth",
				Port:           8081,
				Method:         "POST",
				Path:           "/api/auth/login",
				StatusCode:     200,
				LatencySource:  "client",
				Duration:       100 * time.Millisecond,
				Time:           at.Add(time.Duration(bucket)*bucket5Duration + time.Duration(i)*100*time.Millisecond),
			})
		}
	}
	if err := hist.flushOnce(at.Add(2 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	// First evaluation meets sustain=1 of 2; no alert yet.
	detector.tick(ctx, at.Add(time.Minute))
	if incidents, _ := hist.ListIncidents(ctx, 10); len(incidents) != 0 {
		t.Fatalf("alert before sustain: %+v", incidents)
	}
	detector.tick(ctx, at.Add(time.Minute+5*time.Second))
	incidents, err := hist.ListIncidents(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(incidents) != 1 || incidents[0].Trigger == nil || incidents[0].Trigger.Metric != metricP95Latency {
		t.Fatalf("incidents = %+v", incidents)
	}
}
