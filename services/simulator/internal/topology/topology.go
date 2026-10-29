package topology

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Node is a Target App node in the expected topology.
type Node struct {
	ID    string `yaml:"id" json:"id"`
	Kind  string `yaml:"kind" json:"kind"`
	Label string `yaml:"label" json:"label"`
}

// Edge is a directed connection in the expected topology.
type Edge struct {
	From     string `yaml:"from" json:"from"`
	To       string `yaml:"to" json:"to"`
	Protocol string `yaml:"protocol" json:"protocol"`
}

// Topology is the expected graph.
type Topology struct {
	Nodes []Node `yaml:"nodes" json:"nodes"`
	Edges []Edge `yaml:"edges" json:"edges"`
}

// Load reads and validates an expected topology file.
func Load(path string) (Topology, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Topology{}, err
	}
	var t Topology
	if err := yaml.Unmarshal(data, &t); err != nil {
		return Topology{}, err
	}
	if err := t.Validate(); err != nil {
		return Topology{}, err
	}
	return t, nil
}

// Validate checks node and edge references.
func (t Topology) Validate() error {
	if len(t.Nodes) == 0 {
		return fmt.Errorf("expected topology has no nodes")
	}
	nodes := make(map[string]struct{}, len(t.Nodes))
	for _, node := range t.Nodes {
		if node.ID == "" {
			return fmt.Errorf("expected topology has a node without an id")
		}
		if _, exists := nodes[node.ID]; exists {
			return fmt.Errorf("duplicate node %q", node.ID)
		}
		nodes[node.ID] = struct{}{}
	}
	if len(t.Edges) == 0 {
		return fmt.Errorf("expected topology has no edges")
	}
	edges := make(map[string]struct{}, len(t.Edges))
	for _, edge := range t.Edges {
		if _, ok := nodes[edge.From]; !ok {
			return fmt.Errorf("edge references unknown node %q", edge.From)
		}
		if _, ok := nodes[edge.To]; !ok {
			return fmt.Errorf("edge references unknown node %q", edge.To)
		}
		if edge.Protocol == "" {
			return fmt.Errorf("edge %s -> %s has no protocol", edge.From, edge.To)
		}
		key := edge.From + "->" + edge.To
		if _, exists := edges[key]; exists {
			return fmt.Errorf("duplicate edge %q", key)
		}
		edges[key] = struct{}{}
	}
	return nil
}
