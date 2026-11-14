package history

import (
	"context"
	"testing"
	"time"

	"github.com/tchoupi663/weave/services/weave/internal/topology"
)

func TestCreateIncidentFindsConnectedSuspectRegion(t *testing.T) {
	dir := t.TempDir()
	h, err := Open(dir, "weave", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()

	start := time.Now().UTC().Truncate(5 * time.Second).Add(-30 * time.Second)
	baseline := start.Add(-10 * time.Minute)
	add := func(at time.Time, source, target, path string, status uint32, latency time.Duration) {
		h.ObserveL7(source, target, 8080, topology.L7Observation{
			SourceWorkload: source,
			TargetWorkload: target,
			Port:           8080,
			Method:         "GET",
			Path:           path,
			StatusCode:     status,
			LatencySource:  "client",
			Duration:       latency,
			Time:           at,
		})
	}
	add(baseline, "frontend", "gateway", "/api/products", 200, 3*time.Millisecond)
	add(baseline, "gateway", "catalog", "/api/products", 200, 2*time.Millisecond)
	add(start.Add(5*time.Second), "gateway", "catalog", "/api/products", 500, 2*time.Millisecond)
	add(start.Add(10*time.Second), "frontend", "gateway", "/api/products", 500, 4*time.Millisecond)
	if err := h.flushOnce(start.Add(20 * time.Second)); err != nil {
		t.Fatal(err)
	}

	incident, err := h.CreateIncident(context.Background(), CreateIncidentRequest{
		Start:   start,
		End:     start.Add(30 * time.Second),
		Symptom: "Catalog returns 500",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(incident.Evidence.SuspectEdges) != 2 {
		t.Fatalf("suspect edges = %v", incident.Evidence.SuspectEdges)
	}
	nodes := map[string]bool{}
	for _, node := range incident.Evidence.SuspectNodes {
		nodes[node] = true
	}
	for _, node := range []string{"frontend", "gateway", "catalog"} {
		if !nodes[node] {
			t.Fatalf("missing suspect node %q in %v", node, incident.Evidence.SuspectNodes)
		}
	}
	if len(incident.Evidence.Routes) == 0 {
		t.Fatal("expected route evidence")
	}
	loaded, err := h.GetIncident(context.Background(), incident.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Evidence.AlgorithmVersion != evidenceVersion {
		t.Fatalf("persisted version = %q", loaded.Evidence.AlgorithmVersion)
	}
}

func TestIncidentTimelineReturnsSuspectBuckets(t *testing.T) {
	dir := t.TempDir()
	h, err := Open(dir, "weave", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	start := time.Now().UTC().Truncate(5 * time.Second).Add(-30 * time.Second)
	add := func(at time.Time, source, target string, status uint32) {
		h.ObserveL7(source, target, 8080, topology.L7Observation{
			SourceWorkload: source,
			TargetWorkload: target,
			Port:           8080,
			Method:         "GET",
			Path:           "/api/products",
			StatusCode:     status,
			LatencySource:  "client",
			Duration:       2 * time.Millisecond,
			Time:           at,
		})
	}
	add(start.Add(-10*time.Minute), "gateway", "catalog", 200)
	add(start.Add(5*time.Second), "gateway", "catalog", 500)
	if err := h.flushOnce(start.Add(20 * time.Second)); err != nil {
		t.Fatal(err)
	}
	incident, err := h.CreateIncident(context.Background(), CreateIncidentRequest{
		Start:   start,
		End:     start.Add(30 * time.Second),
		Symptom: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	timeline, err := h.IncidentTimeline(context.Background(), incident.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(timeline.Frames) < 5 || timeline.BucketSeconds != 5 {
		t.Fatalf("timeline = %d frames at %ds", len(timeline.Frames), timeline.BucketSeconds)
	}
	if !timeline.Window.Start.Equal(incident.WindowStart) || !timeline.Window.End.Equal(incident.WindowEnd) {
		t.Fatalf("timeline incident window = %+v, want %+v", timeline.Window, incident.Evidence.Window)
	}
	if !timeline.Context.End.Equal(incident.WindowEnd) {
		t.Fatalf("timeline context end = %s, want %s", timeline.Context.End, incident.WindowEnd)
	}
	if timeline.Context.Start.After(incident.BaselineStart) {
		t.Fatalf("timeline context start = %s, want at or before baseline %s", timeline.Context.Start, incident.BaselineStart)
	}
	found := false
	preIncidentFrames := 0
	preIncidentSample := false
	for _, frame := range timeline.Frames {
		if frame.At.Before(incident.WindowStart) {
			preIncidentFrames++
			for _, edge := range frame.Edges {
				if edge.Requests > 0 && edge.Status5xx == 0 {
					preIncidentSample = true
				}
			}
		}
		for _, edge := range frame.Edges {
			if edge.Status5xx > 0 && edge.ErrorRate == 1 && edge.Anomaly {
				found = true
			}
		}
	}
	if preIncidentFrames < 180 {
		t.Fatalf("pre-incident frames = %d, want at least the 15-minute baseline", preIncidentFrames)
	}
	if !preIncidentSample {
		t.Fatal("expected a clean pre-incident sample in the timeline")
	}
	if !found {
		t.Fatal("expected an anomalous 500 bucket in the timeline")
	}
}
