package history

import (
	"context"
	"fmt"
	"sort"
	"time"
)

type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

type EvidenceThresholds struct {
	BaselineMinutes   int     `json:"baseline_minutes"`
	ErrorMultiplier   float64 `json:"error_multiplier"`
	ErrorDelta        float64 `json:"error_delta"`
	LatencyMultiplier float64 `json:"latency_multiplier"`
	LatencyDeltaMS    float64 `json:"latency_delta_ms"`
}

type MetricSummary struct {
	Requests      uint64  `json:"requests"`
	Responses     uint64  `json:"responses"`
	Status2xx     uint64  `json:"status_2xx"`
	Status3xx     uint64  `json:"status_3xx"`
	Status4xx     uint64  `json:"status_4xx"`
	Status5xx     uint64  `json:"status_5xx"`
	StatusOther   uint64  `json:"status_other"`
	ErrorRate4xx  float64 `json:"error_rate_4xx"`
	ErrorRate5xx  float64 `json:"error_rate_5xx"`
	ErrorRate     float64 `json:"error_rate"`
	P50MS         float64 `json:"p50_ms"`
	P95MS         float64 `json:"p95_ms"`
	LatencySource string  `json:"latency_source"`
}

type EvidenceEdge struct {
	EdgeID   string        `json:"edge_id"`
	Source   string        `json:"source"`
	Target   string        `json:"target"`
	Protocol string        `json:"protocol"`
	Port     uint32        `json:"port"`
	Baseline MetricSummary `json:"baseline"`
	Incident MetricSummary `json:"incident"`
	Anomaly  bool          `json:"anomaly"`
	Onset    *time.Time    `json:"onset,omitempty"`
	Reasons  []string      `json:"reasons,omitempty"`
}

type EvidenceRoute struct {
	EdgeID    string `json:"edge_id"`
	Method    string `json:"method"`
	Path      string `json:"path"`
	Requests  uint64 `json:"requests"`
	Responses uint64 `json:"responses"`
	Status4xx uint64 `json:"status_4xx"`
	Status5xx uint64 `json:"status_5xx"`
}

type Evidence struct {
	AlgorithmVersion string             `json:"algorithm_version"`
	Window           TimeRange          `json:"window"`
	Baseline         TimeRange          `json:"baseline"`
	Thresholds       EvidenceThresholds `json:"thresholds"`
	Edges            []EvidenceEdge     `json:"edges"`
	SuspectEdges     []string           `json:"suspect_edges"`
	SuspectNodes     []string           `json:"suspect_nodes"`
	Routes           []EvidenceRoute    `json:"routes"`
}

type edgeAggregate struct {
	EdgeID   string
	Source   string
	Target   string
	Protocol string
	Port     uint32
	Summary  MetricSummary
}

type bucketObservation struct {
	EdgeID    string
	Start     time.Time
	Requests  uint64
	Status4xx uint64
	Status5xx uint64
	P95MS     float64
}

func (h *History) computeEvidence(ctx context.Context, incident *Incident) (Evidence, error) {
	baseline := TimeRange{Start: incident.BaselineStart.UTC(), End: incident.BaselineEnd.UTC()}
	window := TimeRange{Start: incident.WindowStart.UTC(), End: incident.WindowEnd.UTC()}
	base, err := h.aggregateEdges(ctx, baseline.Start, baseline.End)
	if err != nil {
		return Evidence{}, fmt.Errorf("baseline metrics: %w", err)
	}
	current, err := h.aggregateEdges(ctx, window.Start, window.End)
	if err != nil {
		return Evidence{}, fmt.Errorf("incident metrics: %w", err)
	}
	observations, err := h.bucketObservations(ctx, window.Start, window.End)
	if err != nil {
		return Evidence{}, fmt.Errorf("incident buckets: %w", err)
	}

	edges := make(map[string]*EvidenceEdge)
	for id, aggregate := range current {
		edges[id] = &EvidenceEdge{
			EdgeID:   id,
			Source:   aggregate.Source,
			Target:   aggregate.Target,
			Protocol: aggregate.Protocol,
			Port:     aggregate.Port,
			Incident: aggregate.Summary,
		}
	}
	for id, aggregate := range base {
		edge := edges[id]
		if edge == nil {
			edge = &EvidenceEdge{EdgeID: id, Source: aggregate.Source, Target: aggregate.Target, Protocol: aggregate.Protocol, Port: aggregate.Port}
			edges[id] = edge
		}
		edge.Baseline = aggregate.Summary
	}

	onsets := make(map[string]*time.Time)
	reasons := make(map[string][]string)
	for _, observation := range observations {
		edge := edges[observation.EdgeID]
		if edge == nil {
			continue
		}
		baseSummary := edge.Baseline
		reasonsForBucket := anomalyReasons(observation, baseSummary)
		if len(reasonsForBucket) == 0 {
			continue
		}
		if onsets[observation.EdgeID] == nil {
			value := observation.Start.UTC()
			onsets[observation.EdgeID] = &value
			reasons[observation.EdgeID] = reasonsForBucket
		}
	}

	for id, onset := range onsets {
		if edge := edges[id]; edge != nil {
			edge.Anomaly = true
			edge.Onset = onset
			edge.Reasons = reasons[id]
		}
	}

	result := Evidence{
		AlgorithmVersion: evidenceVersion,
		Window:           window,
		Baseline:         baseline,
		Thresholds: EvidenceThresholds{
			BaselineMinutes:   int(incident.WindowStart.Sub(incident.BaselineStart) / time.Minute),
			ErrorMultiplier:   2,
			ErrorDelta:        0.05,
			LatencyMultiplier: 2,
			LatencyDeltaMS:    5,
		},
		Edges:        make([]EvidenceEdge, 0, len(edges)),
		SuspectEdges: []string{},
		SuspectNodes: []string{},
		Routes:       []EvidenceRoute{},
	}
	for _, edge := range edges {
		result.Edges = append(result.Edges, *edge)
	}
	sort.Slice(result.Edges, func(i, j int) bool {
		left, right := result.Edges[i], result.Edges[j]
		if left.Anomaly != right.Anomaly {
			return left.Anomaly
		}
		if left.Anomaly && !left.Onset.Equal(*right.Onset) {
			return left.Onset.Before(*right.Onset)
		}
		if left.Source != right.Source {
			return left.Source < right.Source
		}
		return left.Target < right.Target
	})

	suspectEdges := suspectComponent(result.Edges)
	suspectIDs := make(map[string]struct{}, len(suspectEdges))
	for _, edge := range suspectEdges {
		result.SuspectEdges = append(result.SuspectEdges, edge.EdgeID)
		suspectIDs[edge.EdgeID] = struct{}{}
	}
	nodes := make(map[string]struct{})
	for _, edge := range suspectEdges {
		nodes[edge.Source] = struct{}{}
		nodes[edge.Target] = struct{}{}
	}
	for node := range nodes {
		result.SuspectNodes = append(result.SuspectNodes, node)
	}
	sort.Strings(result.SuspectNodes)
	sort.Strings(result.SuspectEdges)

	if len(suspectIDs) > 0 {
		routes, err := h.suspectRoutes(ctx, window.Start, window.End, suspectIDs)
		if err != nil {
			return Evidence{}, err
		}
		result.Routes = routes
	}
	return result, nil
}

func (h *History) aggregateEdges(ctx context.Context, start, end time.Time) (map[string]edgeAggregate, error) {
	rows, err := h.duck.QueryContext(ctx, `
SELECT edge_id, source, target, protocol, port,
       sum(requests), sum(responses),
       sum(status_2xx), sum(status_3xx), sum(status_4xx), sum(status_5xx), sum(status_other),
       avg(p50_ms), avg(p95_ms), max(latency_source)
FROM edge_metrics_5s
WHERE bucket_start >= ? AND bucket_start < ?
GROUP BY edge_id, source, target, protocol, port`, start.UTC(), end.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make(map[string]edgeAggregate)
	for rows.Next() {
		var aggregate edgeAggregate
		var s2, s3, s4, s5, other uint64
		if err := rows.Scan(&aggregate.EdgeID, &aggregate.Source, &aggregate.Target, &aggregate.Protocol, &aggregate.Port,
			&aggregate.Summary.Requests, &aggregate.Summary.Responses,
			&s2, &s3, &s4, &s5, &other, &aggregate.Summary.P50MS, &aggregate.Summary.P95MS, &aggregate.Summary.LatencySource); err != nil {
			return nil, err
		}
		aggregate.Summary.Status2xx = s2
		aggregate.Summary.Status3xx = s3
		aggregate.Summary.Status4xx = s4
		aggregate.Summary.Status5xx = s5
		aggregate.Summary.StatusOther = other
		aggregate.Summary.ErrorRate4xx = rate(s4, aggregate.Summary.Requests)
		aggregate.Summary.ErrorRate5xx = rate(s5, aggregate.Summary.Requests)
		aggregate.Summary.ErrorRate = rate(s4+s5, aggregate.Summary.Requests)
		result[aggregate.EdgeID] = aggregate
	}
	return result, rows.Err()
}

func (h *History) bucketObservations(ctx context.Context, start, end time.Time) ([]bucketObservation, error) {
	rows, err := h.duck.QueryContext(ctx, `
SELECT edge_id, bucket_start, requests, status_4xx, status_5xx, p95_ms
FROM edge_metrics_5s
WHERE bucket_start >= ? AND bucket_start < ?
ORDER BY bucket_start`, start.UTC(), end.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []bucketObservation
	for rows.Next() {
		var observation bucketObservation
		if err := rows.Scan(&observation.EdgeID, &observation.Start, &observation.Requests, &observation.Status4xx, &observation.Status5xx, &observation.P95MS); err != nil {
			return nil, err
		}
		observation.Start = observation.Start.UTC()
		result = append(result, observation)
	}
	return result, rows.Err()
}

func anomalyReasons(observation bucketObservation, baseline MetricSummary) []string {
	if observation.Requests == 0 {
		return nil
	}
	var reasons []string
	if exceeds(rate(observation.Status4xx, observation.Requests), baseline.ErrorRate4xx) {
		reasons = append(reasons, "4xx rate")
	}
	if exceeds(rate(observation.Status5xx, observation.Requests), baseline.ErrorRate5xx) {
		reasons = append(reasons, "5xx rate")
	}
	if baseline.P95MS > 0 && observation.P95MS >= 2*baseline.P95MS && observation.P95MS-baseline.P95MS >= 5 {
		reasons = append(reasons, "latency p95")
	}
	return reasons
}

func exceeds(current, baseline float64) bool {
	return current >= 0.05 && (baseline == 0 || current >= 2*baseline)
}

func rate(count, total uint64) float64 {
	if total == 0 {
		return 0
	}
	return float64(count) / float64(total)
}

func suspectComponent(edges []EvidenceEdge) []EvidenceEdge {
	anomalous := make([]EvidenceEdge, 0)
	for _, edge := range edges {
		if edge.Anomaly {
			anomalous = append(anomalous, edge)
		}
	}
	if len(anomalous) == 0 {
		return nil
	}
	byNode := make(map[string][]int)
	for index, edge := range anomalous {
		byNode[edge.Source] = append(byNode[edge.Source], index)
		byNode[edge.Target] = append(byNode[edge.Target], index)
	}
	start := anomalous[0]
	seen := map[int]bool{}
	queue := []int{0}
	for len(queue) > 0 {
		index := queue[0]
		queue = queue[1:]
		if seen[index] {
			continue
		}
		seen[index] = true
		edge := anomalous[index]
		for _, node := range []string{edge.Source, edge.Target} {
			for _, next := range byNode[node] {
				if !seen[next] {
					queue = append(queue, next)
				}
			}
		}
	}
	result := make([]EvidenceEdge, 0, len(seen))
	for index := range seen {
		result = append(result, anomalous[index])
	}
	sort.Slice(result, func(i, j int) bool {
		if !result[i].Onset.Equal(*result[j].Onset) {
			return result[i].Onset.Before(*result[j].Onset)
		}
		return result[i].EdgeID < result[j].EdgeID
	})
	_ = start
	return result
}

func (h *History) suspectRoutes(ctx context.Context, start, end time.Time, suspects map[string]struct{}) ([]EvidenceRoute, error) {
	rows, err := h.duck.QueryContext(ctx, `
SELECT edge_id, method, path, sum(requests), sum(responses), sum(status_4xx), sum(status_5xx)
FROM route_metrics_5s
WHERE bucket_start >= ? AND bucket_start < ?
GROUP BY edge_id, method, path
ORDER BY sum(requests) DESC
LIMIT 20`, start.UTC(), end.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []EvidenceRoute
	for rows.Next() {
		var route EvidenceRoute
		if err := rows.Scan(&route.EdgeID, &route.Method, &route.Path, &route.Requests, &route.Responses, &route.Status4xx, &route.Status5xx); err != nil {
			return nil, err
		}
		if _, ok := suspects[route.EdgeID]; !ok {
			continue
		}
		result = append(result, route)
		if len(result) == 5 {
			break
		}
	}
	return result, rows.Err()
}
