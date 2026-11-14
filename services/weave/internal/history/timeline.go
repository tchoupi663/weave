package history

import (
	"context"
	"fmt"
	"sort"
	"time"
)

// TimelineEdge is one suspect edge inside one timeline bucket.
type TimelineEdge struct {
	EdgeID    string  `json:"edge_id"`
	Requests  uint64  `json:"requests"`
	Status4xx uint64  `json:"status_4xx"`
	Status5xx uint64  `json:"status_5xx"`
	ErrorRate float64 `json:"error_rate"`
	P95MS     float64 `json:"p95_ms"`
	Anomaly   bool    `json:"anomaly"`
}

// TimelineFrame is one scrubbable moment in an incident window.
type TimelineFrame struct {
	At    time.Time      `json:"at"`
	Edges []TimelineEdge `json:"edges"`
}

type IncidentTimeline struct {
	Window        TimeRange       `json:"window"`
	BucketSeconds int             `json:"bucket_seconds"`
	SuspectEdges  []string        `json:"suspect_edges"`
	Frames        []TimelineFrame `json:"frames"`
}

const maxTimelineFrames = 720

func (h *History) IncidentTimeline(ctx context.Context, id string) (*IncidentTimeline, error) {
	incident, err := h.GetIncident(ctx, id)
	if err != nil {
		return nil, err
	}
	window := TimeRange{Start: incident.WindowStart.UTC(), End: incident.WindowEnd.UTC()}
	bucket := 5 * time.Second
	table := "edge_metrics_5s"
	if window.End.Sub(window.Start) > 2*time.Hour {
		bucket = time.Minute
		table = "edge_metrics_1m"
	}
	// Keep the frame count bounded on long windows. The first slice is a
	// 20-minute window, so this normally stays at 5 s.
	if window.End.Sub(window.Start)/bucket > maxTimelineFrames {
		bucket = time.Minute
		table = "edge_metrics_1m"
	}
	suspects := incident.Evidence.SuspectEdges
	suspectSet := make(map[string]bool, len(suspects))
	for _, edgeID := range suspects {
		suspectSet[edgeID] = true
	}
	if len(suspects) == 0 {
		for _, edge := range incident.Evidence.Edges {
			suspects = append(suspects, edge.EdgeID)
			suspectSet[edge.EdgeID] = true
		}
	}

	start := window.Start.Truncate(bucket)
	frames := make([]TimelineFrame, 0)
	for at := start; at.Before(window.End); at = at.Add(bucket) {
		frames = append(frames, TimelineFrame{At: at.UTC(), Edges: []TimelineEdge{}})
		if len(frames) >= maxTimelineFrames {
			break
		}
	}
	if len(frames) == 0 {
		frames = []TimelineFrame{}
	}

	query := fmt.Sprintf(`
SELECT bucket_start, edge_id, sum(requests), sum(status_4xx), sum(status_5xx), avg(p95_ms)
FROM %s
WHERE bucket_start >= ? AND bucket_start < ?
GROUP BY bucket_start, edge_id
ORDER BY bucket_start, edge_id`, table)
	rows, err := h.duck.QueryContext(ctx, query, window.Start, window.End)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	baseline := make(map[string]MetricSummary)
	for _, edge := range incident.Evidence.Edges {
		baseline[edge.EdgeID] = edge.Baseline
	}
	for rows.Next() {
		var at time.Time
		var edge TimelineEdge
		if err := rows.Scan(&at, &edge.EdgeID, &edge.Requests, &edge.Status4xx, &edge.Status5xx, &edge.P95MS); err != nil {
			return nil, err
		}
		if !suspectSet[edge.EdgeID] {
			continue
		}
		edge.ErrorRate = rate(edge.Status4xx+edge.Status5xx, edge.Requests)
		edge.Anomaly = len(anomalyReasons(bucketObservation{
			EdgeID:    edge.EdgeID,
			Start:     at.UTC(),
			Requests:  edge.Requests,
			Status4xx: edge.Status4xx,
			Status5xx: edge.Status5xx,
			P95MS:     edge.P95MS,
		}, baseline[edge.EdgeID])) > 0
		index := int(at.UTC().Truncate(bucket).Sub(start) / bucket)
		if index < 0 || index >= len(frames) {
			continue
		}
		frames[index].Edges = append(frames[index].Edges, edge)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for index := range frames {
		sort.Slice(frames[index].Edges, func(i, j int) bool { return frames[index].Edges[i].EdgeID < frames[index].Edges[j].EdgeID })
	}
	sort.Strings(suspects)
	return &IncidentTimeline{
		Window:        window,
		BucketSeconds: int(bucket / time.Second),
		SuspectEdges:  suspects,
		Frames:        frames,
	}, nil
}
