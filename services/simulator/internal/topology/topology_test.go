package topology

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad(t *testing.T) {
	path := filepath.Join(t.TempDir(), "topology.yaml")
	data := []byte(`nodes:
  - id: source-ip
    kind: external-client
    label: Source IP
  - id: frontend
    kind: frontend
    label: Frontend
edges:
  - from: source-ip
    to: frontend
    protocol: HTTPS
`)
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	topo, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(topo.Nodes) != 2 || len(topo.Edges) != 1 {
		t.Fatalf("nodes=%d edges=%d", len(topo.Nodes), len(topo.Edges))
	}
	if topo.Edges[0].Protocol != "HTTPS" {
		t.Fatalf("protocol = %q", topo.Edges[0].Protocol)
	}
}

func TestValidateUnknownEdge(t *testing.T) {
	topo := Topology{
		Nodes: []Node{{ID: "a"}},
		Edges: []Edge{{From: "a", To: "missing", Protocol: "HTTP"}},
	}
	if err := topo.Validate(); err == nil {
		t.Fatal("expected unknown edge error")
	}
}
