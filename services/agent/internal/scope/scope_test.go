package scope

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

func testScope() (*State, []Edge) {
	fullEdges := []Edge{
		{ID: "frontend->gateway", Source: "workload:weave/frontend", Target: "workload:weave/gateway"},
		{ID: "gateway->catalog", Source: "workload:weave/gateway", Target: "workload:weave/catalog"},
		{ID: "gateway->orders", Source: "workload:weave/gateway", Target: "workload:weave/orders"},
		{ID: "orders->postgres", Source: "workload:weave/orders", Target: "workload:weave/postgres"},
	}
	initial := Entities{
		Nodes:     []string{"workload:weave/frontend", "workload:weave/gateway", "workload:weave/orders"},
		Edges:     []string{"frontend->gateway", "gateway->orders"},
		Workloads: []string{"frontend", "gateway", "orders"},
		Routes: []Route{
			{EdgeID: "gateway->orders", Method: "POST", Path: "/api/orders"},
		},
	}
	state, err := New("incident-1", TimeRange{Start: time.Now().Add(-time.Hour), End: time.Now()}, initial, Limits{MaxExpansions: 2, MaxAddedNodes: 4}, nil, fullEdges)
	if err != nil {
		panic(err)
	}
	return state, fullEdges
}

func TestNewComputesFrontierWithoutLeakingNames(t *testing.T) {
	state, _ := testScope()
	if len(state.Frontier) != 2 {
		t.Fatalf("frontier = %+v", state.Frontier)
	}
	seen := map[string][]string{}
	for _, frontier := range state.Frontier {
		seen[frontier.NodeID] = frontier.Directions
	}
	if _, ok := seen["workload:weave/gateway"]; !ok {
		t.Fatalf("gateway not frontier: %+v", seen)
	}
	if _, ok := seen["workload:weave/orders"]; !ok {
		t.Fatalf("orders not frontier: %+v", seen)
	}
	if _, ok := seen["workload:weave/frontend"]; ok {
		t.Fatalf("frontend unexpectedly frontier: %+v", seen)
	}
	if state.NodeAllowed("workload:weave/catalog") || state.NodeAllowed("workload:weave/postgres") {
		t.Fatal("hidden node leaked into scope")
	}
}

func TestExpandAddsOneHopAndAudits(t *testing.T) {
	state, fullEdges := testScope()
	next, err := state.Expand(fullEdges, "workload:weave/orders", DirectionDownstream, "orders has no in-region dependency; inspect its datastore")
	if err != nil {
		t.Fatal(err)
	}
	if !next.NodeAllowed("workload:weave/postgres") || !next.EdgeAllowed("orders->postgres") {
		t.Fatalf("expansion did not add postgres: %+v", next.Current)
	}
	if !next.WorkloadAllowed("postgres") {
		t.Fatalf("workload missing: %+v", next.Current.Workloads)
	}
	if len(next.Expansions) != 1 || next.Expansions[0].Rationale == "" || next.Expansions[0].AddedNodes[0] != "workload:weave/postgres" {
		t.Fatalf("expansion = %+v", next.Expansions)
	}
	if state.NodeAllowed("workload:weave/postgres") {
		t.Fatal("original scope was mutated")
	}
	// A second expansion toward the other hidden neighbour is allowed.
	second, err := next.Expand(fullEdges, "workload:weave/gateway", DirectionDownstream, "inspect the catalog dependency")
	if err != nil {
		t.Fatal(err)
	}
	if !second.NodeAllowed("workload:weave/catalog") {
		t.Fatalf("catalog missing: %+v", second.Current.Nodes)
	}
}

func TestExpansionRejections(t *testing.T) {
	state, fullEdges := testScope()
	if _, err := state.Expand(fullEdges, "workload:weave/catalog", DirectionDownstream, "x"); !errors.Is(err, ErrDenied) {
		t.Fatalf("hidden node error = %v", err)
	}
	if _, err := state.Expand(fullEdges, "workload:weave/frontend", DirectionDownstream, "x"); !errors.Is(err, ErrNoExpansion) {
		t.Fatalf("non-frontier error = %v", err)
	}
	if _, err := state.Expand(fullEdges, "workload:weave/orders", DirectionDownstream, ""); err == nil {
		t.Fatal("empty rationale accepted")
	}
	if _, err := state.Expand(fullEdges, "workload:weave/orders", "sideways", "x"); err == nil {
		t.Fatal("bad direction accepted")
	}
}

func TestExpansionLimits(t *testing.T) {
	state, fullEdges := testScope()
	state.Limits.MaxExpansions = 0
	if _, err := state.Expand(fullEdges, "workload:weave/orders", DirectionDownstream, "x"); !errors.Is(err, ErrLimitReached) {
		t.Fatalf("max expansions error = %v", err)
	}
	state, fullEdges = testScope()
	state.Limits.MaxAddedNodes = 0
	if _, err := state.Expand(fullEdges, "workload:weave/orders", DirectionDownstream, "x"); !errors.Is(err, ErrLimitReached) {
		t.Fatalf("max nodes error = %v", err)
	}
}

func TestSaveLoadAndValidation(t *testing.T) {
	state, _ := testScope()
	path := filepath.Join(t.TempDir(), "scope.json")
	if err := Save(path, state); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.IncidentID != state.IncidentID || len(loaded.Current.Nodes) != len(state.Current.Nodes) {
		t.Fatalf("loaded = %+v", loaded)
	}
	loaded.Mode = ModeUnconfined
	if err := Save(path, loaded); err == nil {
		t.Fatal("unconfined scope saved")
	}
}

func TestWorkloadFromNode(t *testing.T) {
	if got := WorkloadFromNode("workload:weave/catalog"); got != "catalog" {
		t.Fatalf("workload = %q", got)
	}
	if got := WorkloadFromNode("source-ip"); got != "" {
		t.Fatalf("source ip workload = %q", got)
	}
}
