package weave

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/tchoupi663/weave/services/agent/internal/scope"
)

func testWeaveServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/incidents/incident-1":
			_, _ = w.Write([]byte(`{"id":"incident-1","window_start":"2026-11-15T10:00:00Z","window_end":"2026-11-15T10:02:00Z","baseline_start":"2026-11-15T09:45:00Z","baseline_end":"2026-11-15T10:00:00Z","symptom":"Catalog 500","status":"open","origin":"detection","trigger":{"rule_id":"catalog-products-5xx"},"evidence":{"suspect_edges":["gateway->catalog"],"suspect_nodes":["gateway","catalog"]}}`))
		case "/api/topology":
			_, _ = w.Write([]byte(`{"nodes":[{"id":"workload:weave/gateway","workload":"gateway","name":"API Gateway","kind":"Deployment"},{"id":"workload:weave/catalog","workload":"catalog","name":"Catalog","kind":"Deployment"},{"id":"workload:weave/orders","workload":"orders","name":"Orders","kind":"Deployment"},{"id":"workload:weave/postgres","workload":"postgres","name":"Postgres","kind":"StatefulSet"}],"edges":[{"id":"gateway->catalog","source":"workload:weave/gateway","target":"workload:weave/catalog","protocol":"HTTP","port":8082},{"id":"gateway->orders","source":"workload:weave/gateway","target":"workload:weave/orders","protocol":"HTTP","port":8083},{"id":"orders->postgres","source":"workload:weave/orders","target":"workload:weave/postgres","protocol":"TCP","port":5432}]}`))
		case "/api/changes":
			_, _ = w.Write([]byte(`[{"id":1,"occurred_at":"2026-11-15T10:00:10Z","source":"observed","type":"state_changed","workload":"catalog","kind":"Deployment"},{"id":2,"occurred_at":"2026-11-15T10:00:11Z","source":"observed","type":"state_changed","workload":"orders","kind":"Deployment"}]`))
		case "/api/gaps":
			_, _ = w.Write([]byte(`{"gaps":[{"from":"2026-11-15T10:00:30Z","to":"2026-11-15T10:00:35Z"}]}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func testGuard(t *testing.T) (*scope.Guard, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "scope.json")
	now := time.Now().UTC()
	state, err := scope.New("incident-1", scope.TimeRange{Start: now.Add(-time.Hour), End: now},
		scope.Entities{
			Nodes:     []string{"workload:weave/gateway", "workload:weave/catalog"},
			Edges:     []string{"gateway->catalog"},
			Workloads: []string{"gateway", "catalog"},
		}, scope.Limits{MaxExpansions: 3, MaxAddedNodes: 8}, nil,
		[]scope.Edge{
			{ID: "gateway->catalog", Source: "workload:weave/gateway", Target: "workload:weave/catalog"},
			{ID: "gateway->orders", Source: "workload:weave/gateway", Target: "workload:weave/orders"},
			{ID: "orders->postgres", Source: "workload:weave/orders", Target: "workload:weave/postgres"},
		})
	if err != nil {
		t.Fatal(err)
	}
	if err := scope.Save(path, state); err != nil {
		t.Fatal(err)
	}
	return &scope.Guard{Path: path}, path
}

func TestConfinedToolsFilterAndDeny(t *testing.T) {
	server := testWeaveServer(t)
	guard, _ := testGuard(t)
	client := New(Config{WeaveURL: server.URL, Guard: guard})
	ctx := context.Background()

	_, incident, err := client.getIncident(ctx, nil, getIncidentIn{IncidentID: "incident-1"})
	if err != nil {
		t.Fatal(err)
	}
	if incident.ID != "incident-1" || incident.Trigger == nil {
		t.Fatalf("incident = %+v", incident)
	}
	if _, _, err := client.getIncident(ctx, nil, getIncidentIn{IncidentID: "other"}); err == nil {
		t.Fatal("out-of-scope incident was allowed")
	}
	_, topology, err := client.getTopology(ctx, nil, struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	if len(topology.Nodes) != 2 || len(topology.Edges) != 1 {
		t.Fatalf("topology = %+v", topology)
	}
	if len(topology.Frontier) == 0 {
		t.Fatalf("frontier missing: %+v", topology)
	}
	_, changes, err := client.getChanges(ctx, nil, getChangesIn{})
	if err != nil {
		t.Fatal(err)
	}
	if len(changes) != 1 || changes[0].Workload != "catalog" {
		t.Fatalf("changes = %+v", changes)
	}
	_, gaps, err := client.getRecordingGaps(ctx, nil, getGapsIn{})
	if err != nil {
		t.Fatal(err)
	}
	if len(gaps) != 1 {
		t.Fatalf("gaps = %+v", gaps)
	}
}

func TestConfinedExpansion(t *testing.T) {
	server := testWeaveServer(t)
	guard, path := testGuard(t)
	client := New(Config{WeaveURL: server.URL, Guard: guard})
	_, out, err := client.expandScope(context.Background(), nil, expandIn{
		NodeID: "workload:weave/gateway", Direction: scope.DirectionDownstream, Rationale: "orders may explain the failure",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.AddedNodes) == 0 {
		t.Fatalf("expansion = %+v", out)
	}
	loaded, err := scope.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !loaded.NodeAllowed("workload:weave/orders") {
		t.Fatalf("orders not added: %+v", loaded.Current.Nodes)
	}
	if loaded.NodeAllowed("workload:weave/postgres") {
		t.Fatal("second hop leaked without an expansion")
	}
}
