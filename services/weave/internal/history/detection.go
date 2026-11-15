package history

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"sigs.k8s.io/yaml"
)

const (
	metricErrorRate    = "error_rate"
	metricErrorRate5xx = "error_rate_5xx"
	metricP95Latency   = "p95_latency_ms"

	scopeKindEdge  = "edge"
	scopeKindRoute = "route"

	defaultRuleWindowSeconds = 60
	defaultMinRequests       = 1
	defaultSustain           = 2
	defaultClear             = 12
)

// SLA rule pack types are kept private; the HTTP/agent contracts expose the
// resulting Alerts and incident triggers, not the rule file.
type slaRulePack struct {
	Version string    `json:"version"`
	Rules   []slaRule `json:"rules"`
}

type slaRule struct {
	ID                 string       `json:"id"`
	Description        string       `json:"description"`
	Severity           string       `json:"severity"`
	Scope              slaRuleScope `json:"scope"`
	Metric             string       `json:"metric"`
	Threshold          float64      `json:"threshold"`
	WindowSeconds      int          `json:"window_seconds"`
	MinRequests        uint64       `json:"min_requests"`
	SustainEvaluations int          `json:"sustain_evaluations"`
	ClearEvaluations   int          `json:"clear_evaluations"`
}

type slaRuleScope struct {
	Kind   string `json:"kind"`
	EdgeID string `json:"edge_id,omitempty"`
	Method string `json:"method,omitempty"`
	Path   string `json:"path,omitempty"`
}

// Alert is one persisted SLA breach. It is grouped into an Incident.
type Alert struct {
	ID           string       `json:"id"`
	IncidentID   string       `json:"incident_id,omitempty"`
	RuleID       string       `json:"rule_id"`
	RuleVersion  string       `json:"rule_version"`
	Scope        TriggerScope `json:"scope"`
	Metric       string       `json:"metric"`
	Observed     float64      `json:"observed"`
	Threshold    float64      `json:"threshold"`
	BreachBucket time.Time    `json:"breach_bucket"`
	CreatedAt    time.Time    `json:"created_at"`
	GroupedAt    time.Time    `json:"grouped_at,omitempty"`
}

// DetectionStatus is the additive detection health payload.
type DetectionStatus struct {
	Enabled          bool      `json:"detection_enabled"`
	RulesLoaded      int       `json:"rules_loaded"`
	RulePackVersion  string    `json:"rule_pack_version,omitempty"`
	AlertsCreated    uint64    `json:"alerts_created,omitempty"`
	IncidentsCreated uint64    `json:"incidents_created,omitempty"`
	SuppressedGaps   uint64    `json:"detection_suppressed_gaps,omitempty"`
	Evaluations      uint64    `json:"detection_evaluations,omitempty"`
	LastEvaluation   time.Time `json:"last_detection_evaluation,omitempty"`
}

type ruleRuntime struct {
	rule        slaRule
	breachRuns  int
	clearRuns   int
	active      bool
	suppressed  bool
	alertID     string
	incidentID  string
	breachStart time.Time
	lastBreach  time.Time
	lastEdges   []string
}

// Detector evaluates the configured SLA rules against flushed history.
type Detector struct {
	h         *History
	pack      slaRulePack
	rules     []*ruleRuntime
	interval  time.Duration
	grouping  time.Duration
	cooldown  time.Duration
	startedAt time.Time
	maxWindow time.Duration

	mu               sync.Mutex
	alertsCreated    uint64
	incidentsCreated uint64
	suppressedGaps   uint64
	evaluations      uint64
	lastEvaluation   time.Time
	lastBreach       map[string]time.Time

	skipWarmup bool // tests only
}

// ConfigureDetection loads the SLA rule pack and enables the detector. A
// malformed pack is reported to the caller; the caller may keep serving
// history without detection.
func (h *History) ConfigureDetection(path string, interval, grouping, cooldown time.Duration) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	pack, err := loadSLARulePack(path)
	if err != nil {
		return err
	}
	if interval <= 0 {
		interval = 5 * time.Second
	}
	if grouping <= 0 {
		grouping = 2 * time.Minute
	}
	if cooldown < 0 {
		cooldown = 0
	}
	detector := &Detector{
		h:          h,
		pack:       pack,
		interval:   interval,
		grouping:   grouping,
		cooldown:   cooldown,
		startedAt:  time.Now().UTC(),
		lastBreach: make(map[string]time.Time),
	}
	for _, rule := range pack.Rules {
		runtime := &ruleRuntime{rule: rule}
		detector.rules = append(detector.rules, runtime)
		window := time.Duration(rule.WindowSeconds) * time.Second
		if window > detector.maxWindow {
			detector.maxWindow = window
		}
	}
	h.detector = detector
	// A detection incident cannot keep its in-memory rule state across a
	// history restart. Close interrupted incidents at their stored window end
	// so a restart does not leave a permanently open investigation.
	if err := h.closeInterruptedDetections(context.Background()); err != nil {
		h.log.Warn("reconcile interrupted detections", "error", err)
	}
	return nil
}

func (h *History) closeInterruptedDetections(ctx context.Context) error {
	rows, err := h.sqlite.QueryContext(ctx, `SELECT id, window_end FROM incidents WHERE status = ? AND origin = ?`,
		IncidentStatusOpen, IncidentOriginDetect)
	if err != nil {
		return err
	}
	type candidate struct {
		id  string
		end string
	}
	var candidates []candidate
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.id, &item.end); err != nil {
			rows.Close()
			return err
		}
		candidates = append(candidates, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, item := range candidates {
		end, err := time.Parse(time.RFC3339Nano, item.end)
		if err != nil {
			continue
		}
		if err := h.closeDetectedIncident(ctx, item.id, end); err != nil {
			h.log.Warn("close interrupted detection", "incident", item.id, "error", err)
		}
	}
	return nil
}

// DetectionStatus reports detector health for /api/health.
func (h *History) DetectionStatus() DetectionStatus {
	detector := h.detector
	if detector == nil {
		return DetectionStatus{Enabled: false}
	}
	detector.mu.Lock()
	defer detector.mu.Unlock()
	return DetectionStatus{
		Enabled:          true,
		RulesLoaded:      len(detector.rules),
		RulePackVersion:  detector.pack.Version,
		AlertsCreated:    detector.alertsCreated,
		IncidentsCreated: detector.incidentsCreated,
		SuppressedGaps:   detector.suppressedGaps,
		Evaluations:      detector.evaluations,
		LastEvaluation:   detector.lastEvaluation,
	}
}

func loadSLARulePack(path string) (slaRulePack, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return slaRulePack{}, fmt.Errorf("read SLA rules: %w", err)
	}
	var pack slaRulePack
	if err := yaml.Unmarshal(raw, &pack); err != nil {
		return slaRulePack{}, fmt.Errorf("parse SLA rules: %w", err)
	}
	if err := validateSLARulePack(pack); err != nil {
		return slaRulePack{}, err
	}
	return pack, nil
}

func validateSLARulePack(pack slaRulePack) error {
	if strings.TrimSpace(pack.Version) == "" {
		return fmt.Errorf("SLA rules: version is required")
	}
	if len(pack.Rules) == 0 {
		return fmt.Errorf("SLA rules: at least one rule is required")
	}
	ids := map[string]bool{}
	for index := range pack.Rules {
		if err := validateSLARule(&pack.Rules[index]); err != nil {
			return err
		}
		if ids[pack.Rules[index].ID] {
			return fmt.Errorf("SLA rules: duplicate rule id %q", pack.Rules[index].ID)
		}
		ids[pack.Rules[index].ID] = true
	}
	return nil
}

func validateSLARule(rule *slaRule) error {
	if rule.ID == "" || strings.TrimSpace(rule.ID) == "" {
		return fmt.Errorf("SLA rules: rule id is required")
	}
	if rule.Severity != "critical" && rule.Severity != "warning" {
		return fmt.Errorf("SLA rule %q: severity must be critical or warning", rule.ID)
	}
	switch rule.Scope.Kind {
	case scopeKindEdge:
		if rule.Scope.EdgeID == "" {
			return fmt.Errorf("SLA rule %q: edge scope requires edge_id", rule.ID)
		}
	case scopeKindRoute:
		if rule.Scope.Method == "" || rule.Scope.Path == "" {
			return fmt.Errorf("SLA rule %q: route scope requires method and path", rule.ID)
		}
		rule.Scope.Method = strings.ToUpper(strings.TrimSpace(rule.Scope.Method))
		if !strings.HasPrefix(rule.Scope.Path, "/") {
			return fmt.Errorf("SLA rule %q: route path must start with /", rule.ID)
		}
		if rule.Scope.EdgeID != "" && !strings.Contains(rule.Scope.EdgeID, "->") {
			return fmt.Errorf("SLA rule %q: invalid edge_id", rule.ID)
		}
	default:
		return fmt.Errorf("SLA rule %q: scope.kind must be edge or route", rule.ID)
	}
	switch rule.Metric {
	case metricErrorRate, metricErrorRate5xx:
		if rule.Threshold < 0 || rule.Threshold > 1 {
			return fmt.Errorf("SLA rule %q: rate threshold must be between 0 and 1", rule.ID)
		}
	case metricP95Latency:
		if rule.Threshold <= 0 {
			return fmt.Errorf("SLA rule %q: p95 threshold must be positive", rule.ID)
		}
	default:
		return fmt.Errorf("SLA rule %q: metric must be error_rate, error_rate_5xx, or p95_latency_ms", rule.ID)
	}
	if rule.WindowSeconds == 0 {
		rule.WindowSeconds = defaultRuleWindowSeconds
	}
	if rule.WindowSeconds < 30 || rule.WindowSeconds > 900 {
		return fmt.Errorf("SLA rule %q: window_seconds must be between 30 and 900", rule.ID)
	}
	if rule.MinRequests == 0 {
		rule.MinRequests = defaultMinRequests
	}
	if rule.SustainEvaluations == 0 {
		rule.SustainEvaluations = defaultSustain
	}
	if rule.ClearEvaluations == 0 {
		rule.ClearEvaluations = defaultClear
	}
	if rule.SustainEvaluations < 1 {
		return fmt.Errorf("SLA rule %q: sustain_evaluations must be at least 1", rule.ID)
	}
	if rule.ClearEvaluations < 2 {
		return fmt.Errorf("SLA rule %q: clear_evaluations must be at least 2", rule.ID)
	}
	return nil
}

func (d *Detector) run(ctx context.Context) {
	ticker := time.NewTicker(d.interval)
	defer ticker.Stop()
	d.h.log.Info("sla detection enabled", "rules", len(d.rules), "version", d.pack.Version, "interval", d.interval.String())
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			d.tick(ctx, now.UTC())
		}
	}
}

func (d *Detector) tick(ctx context.Context, now time.Time) {
	if d.h.detectorStartupWarmup(now) {
		return
	}
	d.mu.Lock()
	d.evaluations++
	d.lastEvaluation = now
	d.mu.Unlock()
	for _, runtime := range d.rules {
		if err := d.evaluateRule(ctx, runtime, now); err != nil {
			d.h.log.Warn("sla rule evaluation failed", "rule", runtime.rule.ID, "error", err)
		}
	}
}

func (h *History) detectorStartupWarmup(now time.Time) bool {
	detector := h.detector
	if detector == nil || detector.skipWarmup {
		return false
	}
	return now.Sub(detector.startedAt) < detector.maxWindow+h.grace
}

type windowMetrics struct {
	Requests  uint64
	Status4xx uint64
	Status5xx uint64
	P95MS     float64
	EdgeIDs   []string
}

func (d *Detector) evaluateRule(ctx context.Context, runtime *ruleRuntime, now time.Time) error {
	rule := runtime.rule
	windowEnd := now.Add(-d.h.grace).Truncate(bucket5Duration)
	windowStart := windowEnd.Add(-time.Duration(rule.WindowSeconds) * time.Second)
	if !windowEnd.After(windowStart) {
		return nil
	}
	gaps, err := d.h.Gaps(ctx, windowStart, windowEnd)
	if err != nil {
		return err
	}
	if len(gaps) > 0 {
		d.mu.Lock()
		d.suppressedGaps++
		d.mu.Unlock()
		return nil
	}

	metrics, err := d.windowMetrics(ctx, rule, windowStart, windowEnd)
	if err != nil {
		return err
	}
	runtime.lastEdges = metrics.EdgeIDs
	breached, observed := ruleBreach(rule, metrics)
	var firstBreach time.Time
	if breached {
		firstBreach, err = d.earliestBreachingBucket(ctx, rule, windowStart, windowEnd)
		if err != nil {
			return err
		}
	}
	d.advance(ctx, runtime, breached, observed, rule.Threshold, firstBreach, windowStart, windowEnd)
	return nil
}

// earliestBreachingBucket finds the first 5-second bucket in the evaluation
// window that independently meets the objective. It is used for the alert's
// breach bucket and the incident's window start; it deliberately does not
// apply the window-level minimum volume, which is evaluated on the aggregate.
func (d *Detector) earliestBreachingBucket(ctx context.Context, rule slaRule, start, end time.Time) (time.Time, error) {
	if rule.Metric == metricP95Latency && rule.Scope.Kind == scopeKindRoute {
		return time.Time{}, nil
	}
	var query string
	var args []any
	if rule.Scope.Kind == scopeKindEdge {
		query = `SELECT bucket_start, requests, status_4xx, status_5xx, p95_ms FROM edge_metrics_5s
WHERE edge_id = ? AND bucket_start >= ? AND bucket_start < ? ORDER BY bucket_start`
		args = []any{rule.Scope.EdgeID, start.UTC(), end.UTC()}
	} else {
		query = `SELECT bucket_start, requests, status_4xx, status_5xx, 0 FROM route_metrics_5s
WHERE method = ? AND path = ? AND bucket_start >= ? AND bucket_start < ?`
		args = []any{rule.Scope.Method, rule.Scope.Path, start.UTC(), end.UTC()}
		if rule.Scope.EdgeID != "" {
			query += ` AND edge_id = ?`
			args = append(args, rule.Scope.EdgeID)
		}
		query += ` ORDER BY bucket_start`
	}
	rows, err := d.h.duck.QueryContext(ctx, query, args...)
	if err != nil {
		return time.Time{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var bucket time.Time
		var requests, status4xx, status5xx uint64
		var p95 float64
		if err := rows.Scan(&bucket, &requests, &status4xx, &status5xx, &p95); err != nil {
			return time.Time{}, err
		}
		if requests == 0 {
			continue
		}
		switch rule.Metric {
		case metricErrorRate:
			if ratio(status4xx+status5xx, requests) >= rule.Threshold {
				return bucket.UTC(), nil
			}
		case metricErrorRate5xx:
			if ratio(status5xx, requests) >= rule.Threshold {
				return bucket.UTC(), nil
			}
		case metricP95Latency:
			if p95 >= rule.Threshold {
				return bucket.UTC(), nil
			}
		}
	}
	return time.Time{}, rows.Err()
}

func ruleBreach(rule slaRule, metrics windowMetrics) (bool, float64) {
	if metrics.Requests < rule.MinRequests {
		return false, 0
	}
	switch rule.Metric {
	case metricErrorRate:
		value := ratio(metrics.Status4xx+metrics.Status5xx, metrics.Requests)
		return value >= rule.Threshold, value
	case metricErrorRate5xx:
		value := ratio(metrics.Status5xx, metrics.Requests)
		return value >= rule.Threshold, value
	case metricP95Latency:
		return metrics.P95MS >= rule.Threshold, metrics.P95MS
	default:
		return false, 0
	}
}

func (d *Detector) windowMetrics(ctx context.Context, rule slaRule, start, end time.Time) (windowMetrics, error) {
	if rule.Scope.Kind == scopeKindEdge {
		return d.edgeWindow(ctx, rule.Scope.EdgeID, rule.Metric, start, end)
	}
	return d.routeWindow(ctx, rule, start, end)
}

func (d *Detector) edgeWindow(ctx context.Context, edgeID, metric string, start, end time.Time) (windowMetrics, error) {
	row := d.h.duck.QueryRowContext(ctx, `
SELECT coalesce(sum(requests),0), coalesce(sum(status_4xx),0), coalesce(sum(status_5xx),0),
       coalesce(sum(p95_ms * requests),0)
FROM edge_metrics_5s
WHERE edge_id = ? AND bucket_start >= ? AND bucket_start < ?`, edgeID, start.UTC(), end.UTC())
	var requests, status4xx, status5xx uint64
	var weightedP95 float64
	if err := row.Scan(&requests, &status4xx, &status5xx, &weightedP95); err != nil {
		return windowMetrics{}, err
	}
	result := windowMetrics{Requests: requests, Status4xx: status4xx, Status5xx: status5xx, EdgeIDs: []string{edgeID}}
	if metric == metricP95Latency && requests > 0 {
		result.P95MS = weightedP95 / float64(requests)
	}
	return result, nil
}

func (d *Detector) routeWindow(ctx context.Context, rule slaRule, start, end time.Time) (windowMetrics, error) {
	args := []any{start.UTC(), end.UTC(), rule.Scope.Method, rule.Scope.Path}
	query := `SELECT coalesce(sum(requests),0), coalesce(sum(status_4xx),0), coalesce(sum(status_5xx),0)
FROM route_metrics_5s
WHERE bucket_start >= ? AND bucket_start < ? AND method = ? AND path = ?`
	if rule.Scope.EdgeID != "" {
		query += ` AND edge_id = ?`
		args = append(args, rule.Scope.EdgeID)
	}
	var requests, status4xx, status5xx uint64
	if err := d.h.duck.QueryRowContext(ctx, query, args...).Scan(&requests, &status4xx, &status5xx); err != nil {
		return windowMetrics{}, err
	}
	edges, err := d.routeEdges(ctx, rule, start, end)
	if err != nil {
		return windowMetrics{}, err
	}
	result := windowMetrics{Requests: requests, Status4xx: status4xx, Status5xx: status5xx, EdgeIDs: edges}
	if rule.Metric == metricP95Latency && len(edges) > 0 {
		placeholders := make([]string, 0, len(edges))
		edgeArgs := []any{start.UTC(), end.UTC()}
		for _, edge := range edges {
			placeholders = append(placeholders, "?")
			edgeArgs = append(edgeArgs, edge)
		}
		edgeQuery := `SELECT coalesce(sum(requests),0), coalesce(sum(p95_ms * requests),0) FROM edge_metrics_5s
WHERE bucket_start >= ? AND bucket_start < ? AND edge_id IN (` + strings.Join(placeholders, ",") + `)`
		var edgeRequests uint64
		var weightedP95 float64
		if err := d.h.duck.QueryRowContext(ctx, edgeQuery, edgeArgs...).Scan(&edgeRequests, &weightedP95); err != nil {
			return windowMetrics{}, err
		}
		if edgeRequests > 0 {
			result.P95MS = weightedP95 / float64(edgeRequests)
		}
	}
	return result, nil
}

func (d *Detector) routeEdges(ctx context.Context, rule slaRule, start, end time.Time) ([]string, error) {
	args := []any{start.UTC(), end.UTC(), rule.Scope.Method, rule.Scope.Path}
	query := `SELECT DISTINCT edge_id FROM route_metrics_5s
WHERE bucket_start >= ? AND bucket_start < ? AND method = ? AND path = ?`
	if rule.Scope.EdgeID != "" {
		query += ` AND edge_id = ?`
		args = append(args, rule.Scope.EdgeID)
	}
	query += ` ORDER BY edge_id`
	rows, err := d.h.duck.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var edge string
		if err := rows.Scan(&edge); err != nil {
			return nil, err
		}
		result = append(result, edge)
	}
	return result, rows.Err()
}

func (d *Detector) advance(ctx context.Context, runtime *ruleRuntime, breach bool, observed, threshold float64, firstBreach, windowStart, windowEnd time.Time) {
	rule := runtime.rule
	if breach {
		runtime.clearRuns = 0
		if runtime.breachRuns == 0 {
			runtime.breachStart = firstBreach
			if runtime.breachStart.IsZero() {
				runtime.breachStart = windowStart
			}
		}
		runtime.breachRuns++
		runtime.lastBreach = windowEnd
		if runtime.active {
			if runtime.incidentID != "" {
				d.mu.Lock()
				d.lastBreach[runtime.incidentID] = windowEnd
				d.mu.Unlock()
			}
			return
		}
		if runtime.breachRuns < rule.SustainEvaluations || runtime.suppressed {
			return
		}
		d.createAlert(ctx, runtime, observed, threshold, time.Now().UTC())
		return
	}

	runtime.breachRuns = 0
	runtime.clearRuns++
	if !runtime.active || runtime.clearRuns < rule.ClearEvaluations {
		return
	}
	runtime.active = false
	incidentID := runtime.incidentID
	runtime.alertID = ""
	runtime.incidentID = ""
	runtime.suppressed = false
	if incidentID == "" {
		return
	}
	if d.incidentHasActiveRule(incidentID) {
		return
	}
	d.mu.Lock()
	lastBreach := d.lastBreach[incidentID]
	delete(d.lastBreach, incidentID)
	d.mu.Unlock()
	if lastBreach.IsZero() {
		lastBreach = windowEnd
	}
	if err := d.h.closeDetectedIncident(ctx, incidentID, lastBreach); err != nil {
		d.h.log.Error("close detected incident", "incident", incidentID, "error", err)
		return
	}
	d.h.log.Info("detected incident closed", "incident", incidentID, "rule", runtime.rule.ID, "window_end", lastBreach.Format(time.RFC3339))
}

func (d *Detector) incidentHasActiveRule(incidentID string) bool {
	for _, other := range d.rules {
		if other.active && other.incidentID == incidentID {
			return true
		}
	}
	return false
}

func (d *Detector) createAlert(ctx context.Context, runtime *ruleRuntime, observed, threshold float64, now time.Time) {
	incident, err := d.findGroupIncident(ctx, runtime)
	if err != nil {
		d.h.log.Warn("find grouping incident", "rule", runtime.rule.ID, "error", err)
	}
	if incident == nil {
		if closed := d.recentlyClosedIncident(ctx, runtime); closed != nil {
			runtime.active = true
			runtime.suppressed = true
			return
		}
		incident, err = d.h.createDetectedIncident(ctx, runtime.breachStart, runtime.lastBreach, symptomForRule(runtime.rule), triggerForRule(runtime.rule, d.pack.Version, runtime.breachStart, observed, threshold, now))
		if err != nil {
			d.h.log.Error("create detected incident", "rule", runtime.rule.ID, "error", err)
			return
		}
		d.mu.Lock()
		d.incidentsCreated++
		d.mu.Unlock()
	}
	alert := Alert{
		ID:           newAlertID(),
		IncidentID:   incident.ID,
		RuleID:       runtime.rule.ID,
		RuleVersion:  d.pack.Version,
		Scope:        triggerScopeForRule(runtime.rule),
		Metric:       runtime.rule.Metric,
		Observed:     observed,
		Threshold:    threshold,
		BreachBucket: runtime.breachStart,
		CreatedAt:    now,
		GroupedAt:    now,
	}
	if err := d.insertAlert(ctx, alert); err != nil {
		d.h.log.Error("persist alert", "rule", runtime.rule.ID, "error", err)
		return
	}
	runtime.active = true
	runtime.alertID = alert.ID
	runtime.incidentID = incident.ID
	d.mu.Lock()
	d.alertsCreated++
	d.lastBreach[incident.ID] = runtime.lastBreach
	d.mu.Unlock()
	d.h.log.Info("SLA alert detected", "rule", runtime.rule.ID, "incident", incident.ID, "metric", runtime.rule.Metric, "observed", observed, "threshold", threshold, "breach_bucket", runtime.breachStart.Format(time.RFC3339))
}

func (d *Detector) findGroupIncident(ctx context.Context, runtime *ruleRuntime) (*Incident, error) {
	rows, err := d.h.sqlite.QueryContext(ctx, `SELECT id FROM incidents WHERE status = ? AND origin = ? AND created_at >= ? ORDER BY created_at DESC LIMIT 20`,
		IncidentStatusOpen, IncidentOriginDetect, time.Now().UTC().Add(-d.grouping).Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	rows.Close()
	for _, id := range ids {
		incident, err := d.h.GetIncident(ctx, id)
		if err != nil {
			continue
		}
		if d.scopeOverlapsIncident(runtime, incident) {
			return incident, nil
		}
	}
	return nil, nil
}

func (d *Detector) recentlyClosedIncident(ctx context.Context, runtime *ruleRuntime) *Incident {
	if d.cooldown <= 0 {
		return nil
	}
	rows, err := d.h.sqlite.QueryContext(ctx, `SELECT id FROM incidents WHERE status = ? AND origin = ? AND updated_at >= ? ORDER BY updated_at DESC LIMIT 20`,
		IncidentStatusClosed, IncidentOriginDetect, time.Now().UTC().Add(-d.cooldown).Format(time.RFC3339Nano))
	if err != nil {
		return nil
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil
		}
		ids = append(ids, id)
	}
	rows.Close()
	for _, id := range ids {
		incident, err := d.h.GetIncident(ctx, id)
		if err != nil {
			continue
		}
		if d.scopeOverlapsIncident(runtime, incident) {
			return incident
		}
	}
	return nil
}

func (d *Detector) scopeOverlapsIncident(runtime *ruleRuntime, incident *Incident) bool {
	scope := triggerScopeForRule(runtime.rule)
	suspectNodes := make(map[string]bool, len(incident.Evidence.SuspectNodes))
	for _, node := range incident.Evidence.SuspectNodes {
		suspectNodes[node] = true
	}
	for _, edge := range incident.Evidence.SuspectEdges {
		if scope.EdgeID != "" && scope.EdgeID == edge {
			return true
		}
		source, target, ok := edgeNodes(edge)
		if ok && (suspectNodes[source] || suspectNodes[target]) {
			for _, observed := range runtime.lastEdges {
				if observed == edge {
					return true
				}
			}
		}
	}
	if scope.Kind == scopeKindRoute {
		for _, route := range incident.Evidence.Routes {
			if route.Method == scope.Method && route.Path == scope.Path {
				return true
			}
		}
	}
	return false
}

func edgeNodes(edgeID string) (string, string, bool) {
	left, right, ok := strings.Cut(edgeID, "->")
	if !ok {
		return "", "", false
	}
	target, _, ok := strings.Cut(right, ":")
	if !ok {
		return "", "", false
	}
	return left, target, true
}

func triggerScopeForRule(rule slaRule) TriggerScope {
	return TriggerScope{Kind: rule.Scope.Kind, EdgeID: rule.Scope.EdgeID, Method: rule.Scope.Method, Path: rule.Scope.Path}
}

func symptomForRule(rule slaRule) string {
	description := strings.TrimSpace(rule.Description)
	if description == "" {
		description = rule.ID
	}
	return fmt.Sprintf("SLA breach: %s (%s)", description, rule.Metric)
}

func triggerForRule(rule slaRule, version string, breachBucket time.Time, observed, threshold float64, detectedAt time.Time) Trigger {
	return Trigger{
		RuleID:       rule.ID,
		RuleVersion:  version,
		Scope:        triggerScopeForRule(rule),
		Metric:       rule.Metric,
		Observed:     observed,
		Threshold:    threshold,
		BreachBucket: breachBucket.UTC(),
		DetectedAt:   detectedAt.UTC(),
	}
}

func (d *Detector) insertAlert(ctx context.Context, alert Alert) error {
	_, err := d.h.sqlite.ExecContext(ctx, `
INSERT INTO alerts
(id, incident_id, rule_id, rule_version, scope_kind, edge_id, method, path, metric, observed, threshold, breach_bucket, created_at, grouped_at)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		alert.ID, alert.IncidentID, alert.RuleID, alert.RuleVersion, alert.Scope.Kind, alert.Scope.EdgeID, alert.Scope.Method, alert.Scope.Path,
		alert.Metric, alert.Observed, alert.Threshold, alert.BreachBucket.Format(time.RFC3339Nano),
		alert.CreatedAt.Format(time.RFC3339Nano), alert.GroupedAt.Format(time.RFC3339Nano))
	return err
}

// AlertsForIncident returns the alerts grouped into an incident.
func (h *History) AlertsForIncident(ctx context.Context, incidentID string) ([]Alert, error) {
	rows, err := h.sqlite.QueryContext(ctx, `SELECT id, incident_id, rule_id, rule_version, scope_kind, edge_id, method, path, metric, observed, threshold, breach_bucket, created_at, grouped_at
FROM alerts WHERE incident_id = ? ORDER BY created_at, id`, incidentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Alert, 0)
	for rows.Next() {
		var alert Alert
		var incidentIDValue, breachBucket, createdAt, groupedAt string
		if err := rows.Scan(&alert.ID, &incidentIDValue, &alert.RuleID, &alert.RuleVersion, &alert.Scope.Kind, &alert.Scope.EdgeID, &alert.Scope.Method, &alert.Scope.Path,
			&alert.Metric, &alert.Observed, &alert.Threshold, &breachBucket, &createdAt, &groupedAt); err != nil {
			return nil, err
		}
		alert.IncidentID = incidentIDValue
		var parseErr error
		if alert.BreachBucket, parseErr = time.Parse(time.RFC3339Nano, breachBucket); parseErr != nil {
			return nil, parseErr
		}
		if alert.CreatedAt, parseErr = time.Parse(time.RFC3339Nano, createdAt); parseErr != nil {
			return nil, parseErr
		}
		if groupedAt != "" {
			if alert.GroupedAt, parseErr = time.Parse(time.RFC3339Nano, groupedAt); parseErr != nil {
				return nil, parseErr
			}
		}
		result = append(result, alert)
	}
	return result, rows.Err()
}

func newAlertID() string {
	var raw [8]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return fmt.Sprintf("alert-%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(raw[:])
}

func ratio(count, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return float64(count) / float64(total)
}
