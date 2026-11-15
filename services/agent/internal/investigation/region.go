package investigation

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"time"

	"github.com/tchoupi663/weave/services/agent/internal/scope"
)

// Incident is the safe incident shape used to build an investigation context.
type Incident struct {
	ID            string          `json:"id"`
	WindowStart   time.Time       `json:"window_start"`
	WindowEnd     time.Time       `json:"window_end"`
	BaselineStart time.Time       `json:"baseline_start"`
	BaselineEnd   time.Time       `json:"baseline_end"`
	Symptom       string          `json:"symptom"`
	Status        string          `json:"status"`
	Origin        string          `json:"origin,omitempty"`
	Trigger       json.RawMessage `json:"trigger,omitempty"`
	Evidence      json.RawMessage `json:"evidence,omitempty"`
}

type metricSummary struct {
	Requests  uint64  `json:"requests"`
	Responses uint64  `json:"responses"`
	Status4xx uint64  `json:"status_4xx"`
	Status5xx uint64  `json:"status_5xx"`
	P95MS     float64 `json:"p95_ms"`
	ErrorRate float64 `json:"error_rate"`
}

type regionEdge struct {
	EdgeID   string        `json:"edge_id"`
	Source   string        `json:"source"`
	Target   string        `json:"target"`
	Protocol string        `json:"protocol"`
	Port     uint32        `json:"port"`
	Baseline metricSummary `json:"baseline"`
	Incident metricSummary `json:"incident"`
	Onset    time.Time     `json:"onset,omitempty"`
	Reasons  []string      `json:"reasons,omitempty"`
}

type regionRoute struct {
	EdgeID    string `json:"edge_id"`
	Method    string `json:"method"`
	Path      string `json:"path"`
	Requests  uint64 `json:"requests"`
	Status5xx uint64 `json:"status_5xx"`
}

type timelinePoint struct {
	At        time.Time `json:"at"`
	EdgeID    string    `json:"edge_id"`
	Requests  uint64    `json:"requests"`
	ErrorRate float64   `json:"error_rate"`
	P95MS     float64   `json:"p95_ms"`
	Anomaly   bool      `json:"anomaly"`
}

type Region struct {
	Incident map[string]any    `json:"incident"`
	Region   map[string]any    `json:"region"`
	Gaps     []scope.TimeRange `json:"gaps"`
	Timeline []timelinePoint   `json:"timeline"`
	Frontier []scope.Frontier  `json:"frontier"`
	Limits   scope.Limits      `json:"limits"`
}

// BuildRegion builds the compact region-first context from Weave's incident
// JSON and optional timeline JSON. The result excludes every entity outside the
// Suspect Region.
func BuildRegion(incidentJSON, timelineJSON []byte, frontier []scope.Frontier, limits scope.Limits) ([]byte, error) {
	var incident Incident
	if err := json.Unmarshal(incidentJSON, &incident); err != nil {
		return nil, fmt.Errorf("parse incident: %w", err)
	}
	var evidence struct {
		Edges []struct {
			EdgeID   string        `json:"edge_id"`
			Source   string        `json:"source"`
			Target   string        `json:"target"`
			Protocol string        `json:"protocol"`
			Port     uint32        `json:"port"`
			Baseline metricSummary `json:"baseline"`
			Incident metricSummary `json:"incident"`
			Anomaly  bool          `json:"anomaly"`
			Onset    *time.Time    `json:"onset"`
			Reasons  []string      `json:"reasons"`
		} `json:"edges"`
		SuspectEdges []string `json:"suspect_edges"`
		SuspectNodes []string `json:"suspect_nodes"`
		Routes       []struct {
			EdgeID    string `json:"edge_id"`
			Method    string `json:"method"`
			Path      string `json:"path"`
			Requests  uint64 `json:"requests"`
			Status5xx uint64 `json:"status_5xx"`
		} `json:"routes"`
	}
	if err := json.Unmarshal(incident.Evidence, &evidence); err != nil {
		return nil, fmt.Errorf("parse evidence: %w", err)
	}
	suspectEdges := make(map[string]bool, len(evidence.SuspectEdges))
	for _, edgeID := range evidence.SuspectEdges {
		suspectEdges[edgeID] = true
	}
	edges := make([]regionEdge, 0)
	for _, edge := range evidence.Edges {
		if !suspectEdges[edge.EdgeID] {
			continue
		}
		item := regionEdge{
			EdgeID: edge.EdgeID, Source: edge.Source, Target: edge.Target,
			Protocol: edge.Protocol, Port: edge.Port,
			Baseline: edge.Baseline, Incident: edge.Incident,
			Reasons: edge.Reasons,
		}
		if edge.Onset != nil {
			item.Onset = edge.Onset.UTC()
		}
		edges = append(edges, item)
	}
	sort.Slice(edges, func(i, j int) bool { return edges[i].EdgeID < edges[j].EdgeID })
	routes := make([]regionRoute, 0)
	for _, route := range evidence.Routes {
		if !suspectEdges[route.EdgeID] {
			continue
		}
		routes = append(routes, regionRoute{EdgeID: route.EdgeID, Method: route.Method, Path: route.Path, Requests: route.Requests, Status5xx: route.Status5xx})
	}
	incidentSummary := map[string]any{
		"id": incident.ID, "window_start": incident.WindowStart, "window_end": incident.WindowEnd,
		"baseline_start": incident.BaselineStart, "baseline_end": incident.BaselineEnd,
		"symptom": incident.Symptom, "status": incident.Status, "origin": incident.Origin,
	}
	if len(incident.Trigger) > 0 {
		incidentSummary["trigger"] = json.RawMessage(incident.Trigger)
	}

	gaps := make([]scope.TimeRange, 0)
	timeline := make([]timelinePoint, 0)
	if len(timelineJSON) > 0 {
		var rawTimeline struct {
			Frames []struct {
				At    time.Time `json:"at"`
				Edges []struct {
					EdgeID    string  `json:"edge_id"`
					Requests  uint64  `json:"requests"`
					ErrorRate float64 `json:"error_rate"`
					P95MS     float64 `json:"p95_ms"`
					Anomaly   bool    `json:"anomaly"`
				} `json:"edges"`
			} `json:"frames"`
			Gaps []struct {
				From time.Time `json:"from"`
				To   time.Time `json:"to"`
			} `json:"gaps"`
		}
		if err := json.Unmarshal(timelineJSON, &rawTimeline); err == nil {
			for _, gap := range rawTimeline.Gaps {
				gaps = append(gaps, scope.TimeRange{Start: gap.From, End: gap.To})
			}
			for frameIndex, frame := range rawTimeline.Frames {
				for _, edge := range frame.Edges {
					if !suspectEdges[edge.EdgeID] {
						continue
					}
					if frameIndex%3 != 0 && !edge.Anomaly && frameIndex != 0 && frameIndex != len(rawTimeline.Frames)-1 {
						continue
					}
					timeline = append(timeline, timelinePoint{
						At: frame.At, EdgeID: edge.EdgeID, Requests: edge.Requests,
						ErrorRate: edge.ErrorRate, P95MS: edge.P95MS, Anomaly: edge.Anomaly,
					})
				}
			}
		}
	}
	if len(timeline) > 60 {
		timeline = timeline[:60]
	}
	region := map[string]any{
		"nodes":  evidence.SuspectNodes,
		"edges":  edges,
		"routes": routes,
	}
	payload := Region{
		Incident: incidentSummary,
		Region:   region,
		Gaps:     gaps,
		Timeline: timeline,
		Frontier: frontier,
		Limits:   limits,
	}
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(payload); err != nil {
		return nil, err
	}
	return bytes.TrimSpace(buffer.Bytes()), nil
}
