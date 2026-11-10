package topology

import (
	"testing"
	"time"
)

func TestIngestL7AddsEdgeAndNormalizesRoutes(t *testing.T) {
	store := NewStore(testNamespace, 5*time.Minute)
	now := time.Now().UTC()
	frontend := endpoint(testNamespace, "frontend-abc", "frontend", "Deployment", "gen:io.cilium.k8s.named-ports=https.TCP.443")
	gateway := endpoint(testNamespace, "gateway-abc", "gateway", "Deployment", "gen:io.cilium.k8s.named-ports=http.TCP.8080")
	if !store.Ingest(tcpFlow(frontend, gateway, "10.244.1.130", "10.244.3.214", 8080, true, false, now, "l4-l7")) {
		t.Fatal("L4 edge was rejected")
	}

	if !store.IngestL7(L7Observation{
		SourceWorkload:  "frontend",
		SourceKind:      "Deployment",
		SourcePod:       "frontend-abc",
		SourceNamespace: testNamespace,
		TargetWorkload:  "gateway",
		TargetNamespace: testNamespace,
		Port:            8080,
		Method:          "GET",
		Path:            "/api/products/p001?coupon=secret",
		StatusCode:      200,
		Duration:        3 * time.Millisecond,
		Time:            now,
		Key:             "span-1",
	}) {
		t.Fatal("L7 observation was rejected")
	}
	if store.IngestL7(L7Observation{
		SourceWorkload:  "frontend",
		SourceNamespace: testNamespace,
		TargetWorkload:  "gateway",
		Port:            8080,
		Method:          "GET",
		Path:            "/api/products/p001",
		StatusCode:      200,
		Duration:        3 * time.Millisecond,
		Time:            now,
		Key:             "span-1",
	}) {
		t.Fatal("duplicate L7 observation was accepted")
	}

	snapshot := store.SnapshotAt(now)
	if snapshot.Totals.L7Edges != 1 || snapshot.Totals.L7Requests != 1 || snapshot.Totals.L7Responses != 1 {
		t.Fatalf("L7 totals = %+v", snapshot.Totals)
	}
	if snapshot.Totals.Connections != 1 || snapshot.Totals.Events != 1 {
		t.Fatalf("L7 observation changed L4 totals: %+v", snapshot.Totals)
	}
	if len(snapshot.Edges) != 1 {
		t.Fatalf("edges = %d, want 1", len(snapshot.Edges))
	}
	edge := snapshot.Edges[0]
	if edge.L7 == nil {
		t.Fatal("edge has no L7 stats")
	}
	if edge.L7.Requests != 1 || edge.L7.Responses != 1 {
		t.Fatalf("L7 requests/responses = %d/%d", edge.L7.Requests, edge.L7.Responses)
	}
	if edge.L7.Status.Success != 1 {
		t.Fatalf("status = %+v, want one 2xx", edge.L7.Status)
	}
	if edge.L7.LatencyMs.P50 <= 0 || edge.L7.LatencyMs.P95 <= 0 {
		t.Fatalf("latency = %+v", edge.L7.LatencyMs)
	}
	if len(edge.L7.Routes) != 1 || edge.L7.Routes[0].Path != "/api/products/:id" {
		t.Fatalf("routes = %+v", edge.L7.Routes)
	}
	if len(edge.L7.Window.Activity) != snapshotBuckets {
		t.Fatalf("window activity = %d, want %d", len(edge.L7.Window.Activity), snapshotBuckets)
	}
	if edge.L7.Window.Requests != 1 {
		t.Fatalf("window requests = %d", edge.L7.Window.Requests)
	}
}

func TestIngestL7ResolvesTargetFromL4Edge(t *testing.T) {
	store := NewStore(testNamespace, 5*time.Minute)
	now := time.Now().UTC()
	frontend := endpoint(testNamespace, "frontend-abc", "frontend", "Deployment", "gen:io.cilium.k8s.named-ports=https.TCP.443")
	gateway := endpoint(testNamespace, "gateway-abc", "gateway", "Deployment", "gen:io.cilium.k8s.named-ports=http.TCP.8080")
	if !store.Ingest(tcpFlow(frontend, gateway, "10.244.1.130", "10.244.3.214", 8080, true, false, now, "l4-1")) {
		t.Fatal("L4 edge was rejected")
	}

	if !store.IngestL7(L7Observation{
		SourceWorkload:  "frontend",
		SourceNamespace: testNamespace,
		Port:            8080,
		Method:          "GET",
		Path:            "/api/products",
		StatusCode:      200,
		Duration:        time.Millisecond,
		Time:            now,
		Key:             "span-resolve",
	}) {
		t.Fatal("L7 observation was not resolved from the L4 edge")
	}

	snapshot := store.SnapshotAt(now)
	if len(snapshot.Edges) != 1 {
		t.Fatalf("edges = %d, want 1", len(snapshot.Edges))
	}
	edge := snapshot.Edges[0]
	if edge.Source != "workload:weave/frontend" || edge.Target != "workload:weave/gateway" {
		t.Fatalf("edge endpoints = %s -> %s", edge.Source, edge.Target)
	}
	if edge.L7 == nil || edge.L7.Requests != 1 {
		t.Fatalf("L7 not attached: %+v", edge.L7)
	}
	if edge.Connections != 1 || edge.Events != 1 {
		t.Fatalf("L4 counters = %d/%d, want 1/1", edge.Connections, edge.Events)
	}
}

func TestL7StatusClassesAndReset(t *testing.T) {
	store := NewStore(testNamespace, 5*time.Minute)
	now := time.Now().UTC()
	gateway := endpoint(testNamespace, "gateway-abc", "gateway", "Deployment", "gen:io.cilium.k8s.named-ports=http.TCP.8080")
	catalog := endpoint(testNamespace, "catalog-abc", "catalog", "Deployment", "gen:io.cilium.k8s.named-ports=http.TCP.8082")
	if !store.Ingest(tcpFlow(gateway, catalog, "10.244.3.214", "10.244.2.137", 8082, true, false, now, "l4-status")) {
		t.Fatal("L4 edge was rejected")
	}
	codes := []uint32{200, 201, 404, 500}
	for i, code := range codes {
		ok := store.IngestL7(L7Observation{
			SourceWorkload:  "gateway",
			SourceNamespace: testNamespace,
			TargetWorkload:  "catalog",
			TargetNamespace: testNamespace,
			Port:            8082,
			Method:          "GET",
			Path:            "/api/products/01234567-89ab-cdef-0123-456789abcdef",
			StatusCode:      code,
			Duration:        time.Duration(i+1) * time.Millisecond,
			Time:            now,
			Key:             string(rune('a' + i)),
		})
		if !ok {
			t.Fatalf("L7 status %d rejected", code)
		}
	}
	snapshot := store.SnapshotAt(now)
	edge := snapshot.Edges[0]
	if edge.L7.Status.Success != 2 || edge.L7.Status.ClientError != 1 || edge.L7.Status.ServerError != 1 {
		t.Fatalf("status = %+v", edge.L7.Status)
	}
	if edge.L7.Routes[0].Path != "/api/products/:id" {
		t.Fatalf("route = %q", edge.L7.Routes[0].Path)
	}
	store.Reset()
	empty := store.SnapshotAt(now)
	if empty.Totals.L7Edges != 0 || empty.Totals.L7Requests != 0 || empty.Hubble.ObservedL7 != 0 {
		t.Fatalf("reset left L7 behind: %+v", empty.Totals)
	}
}

func TestIngestL7DoesNotInventAnL4Edge(t *testing.T) {
	store := NewStore(testNamespace, 5*time.Minute)
	now := time.Now().UTC()
	ok := store.IngestL7(L7Observation{
		SourceWorkload:  "frontend",
		SourceNamespace: testNamespace,
		TargetWorkload:  "gateway",
		TargetNamespace: testNamespace,
		Port:            8080,
		Method:          "GET",
		Path:            "/api/products",
		StatusCode:      200,
		Duration:        time.Millisecond,
		Time:            now,
		Key:             "orphan",
	})
	if ok {
		t.Fatal("L7 without an L4 edge was accepted")
	}
	if snapshot := store.SnapshotAt(now); len(snapshot.Edges) != 0 || snapshot.Totals.L7Edges != 0 {
		t.Fatalf("L7 invented topology: %+v", snapshot.Totals)
	}
}
