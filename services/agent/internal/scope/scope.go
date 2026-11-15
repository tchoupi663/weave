// Package scope owns the Region Scope for a confined investigation run: the
// allowed entities, the frontier, and the audited one-hop expansions.
package scope

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"syscall"
	"time"
)

const (
	ModeConfined   = "confined"
	ModeUnconfined = "unconfined"

	DirectionUpstream   = "upstream"
	DirectionDownstream = "downstream"
	DirectionBoth       = "both"
)

var (
	ErrDenied       = errors.New("scope_denied: request outside the current investigation scope")
	ErrNoExpansion  = errors.New("no observed neighbour outside the current scope in that direction")
	ErrLimitReached = errors.New("scope expansion limit reached")
)

// Edge is the minimum observed-topology edge shape needed to compute a
// frontier and expand one hop.
type Edge struct {
	ID     string `json:"id"`
	Source string `json:"source"`
	Target string `json:"target"`
}

// Route is a normalized method/path pair on an edge.
type Route struct {
	EdgeID string `json:"edge_id"`
	Method string `json:"method"`
	Path   string `json:"path"`
}

// TimeRange is the incident context the tools clamp queries to.
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// Entities is one immutable set in the scope.
type Entities struct {
	Nodes       []string `json:"nodes"`
	Edges       []string `json:"edges"`
	Routes      []Route  `json:"routes"`
	Workloads   []string `json:"workloads"`
	SourcePaths []string `json:"source_paths"`
}

// Frontier marks an in-scope node that has hidden observed neighbours.
type Frontier struct {
	NodeID     string   `json:"node_id"`
	Directions []string `json:"directions"`
}

// Expansion is one audited Scope Expansion.
type Expansion struct {
	FromNode       string    `json:"from_node"`
	Direction      string    `json:"direction"`
	Rationale      string    `json:"rationale"`
	AddedNodes     []string  `json:"added_nodes"`
	AddedEdges     []string  `json:"added_edges"`
	AddedWorkloads []string  `json:"added_workloads"`
	At             time.Time `json:"at"`
}

// Limits bounds how far a run may expand.
type Limits struct {
	MaxExpansions int `json:"max_expansions"`
	MaxAddedNodes int `json:"max_added_nodes"`
}

// State is the per-run scope document shared by all MCP servers.
type State struct {
	Version         int                 `json:"version"`
	IncidentID      string              `json:"incident_id"`
	Mode            string              `json:"mode"`
	TimeContext     TimeRange           `json:"time_context"`
	Initial         Entities            `json:"initial"`
	Current         Entities            `json:"current"`
	Frontier        []Frontier          `json:"frontier"`
	Expansions      []Expansion         `json:"expansions"`
	Limits          Limits              `json:"limits"`
	WorkloadSources map[string][]string `json:"workload_sources,omitempty"`
}

// New builds a confined scope from the evidence suspect region. fullEdges is
// the observed topology at creation time and is used only to compute the
// frontier; its hidden names are never written into Frontier.
func New(incidentID string, timeContext TimeRange, initial Entities, limits Limits, sourceMap map[string][]string, fullEdges []Edge) (*State, error) {
	state := &State{
		Version:         1,
		IncidentID:      incidentID,
		Mode:            ModeConfined,
		TimeContext:     timeContext,
		Initial:         cloneEntities(initial),
		Current:         cloneEntities(initial),
		Limits:          limits,
		WorkloadSources: sourceMap,
	}
	if err := Validate(state); err != nil {
		return nil, err
	}
	state.recomputeFrontier(fullEdges)
	return state, nil
}

// Validate checks the invariants of a scope document.
func Validate(state *State) error {
	if state == nil {
		return fmt.Errorf("scope is nil")
	}
	if state.Mode != ModeConfined {
		return fmt.Errorf("scope mode must be confined")
	}
	if strings.TrimSpace(state.IncidentID) == "" {
		return fmt.Errorf("scope incident_id is required")
	}
	if !state.TimeContext.End.After(state.TimeContext.Start) {
		return fmt.Errorf("scope time context end must be after start")
	}
	if state.Limits.MaxExpansions < 0 || state.Limits.MaxAddedNodes < 0 {
		return fmt.Errorf("scope limits must not be negative")
	}
	if len(state.Initial.Nodes) == 0 && len(state.Initial.Edges) == 0 {
		return fmt.Errorf("scope must contain at least one node or edge")
	}
	if err := validateEntities(state.Initial, "initial"); err != nil {
		return err
	}
	if err := validateEntities(state.Current, "current"); err != nil {
		return err
	}
	for _, expansion := range state.Expansions {
		if strings.TrimSpace(expansion.FromNode) == "" || strings.TrimSpace(expansion.Rationale) == "" {
			return fmt.Errorf("scope expansion is incomplete")
		}
	}
	return nil
}

func validateEntities(entities Entities, label string) error {
	seen := map[string]bool{}
	for _, node := range entities.Nodes {
		if strings.TrimSpace(node) == "" || seen[node] {
			return fmt.Errorf("scope %s nodes are invalid or duplicated", label)
		}
		seen[node] = true
	}
	return nil
}

// Load reads and validates a scope file.
func Load(path string) (*State, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var state State
	if err := json.Unmarshal(raw, &state); err != nil {
		return nil, fmt.Errorf("parse scope: %w", err)
	}
	if err := Validate(&state); err != nil {
		return nil, err
	}
	return &state, nil
}

// Save writes a scope file atomically under an advisory lock.
func Save(path string, state *State) error {
	if err := Validate(state); err != nil {
		return err
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN) //nolint:errcheck
	raw, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Expand adds the immediate observed neighbours of a frontier node in the
// selected direction. It returns a new state; the receiver is not mutated.
func (state *State) Expand(fullEdges []Edge, nodeID, direction, rationale string) (*State, error) {
	if state == nil {
		return nil, fmt.Errorf("scope is nil")
	}
	if state.Mode != ModeConfined {
		return nil, fmt.Errorf("scope is not confined")
	}
	nodeID = strings.TrimSpace(nodeID)
	if nodeID == "" {
		return nil, fmt.Errorf("node_id is required")
	}
	if !state.currentNode(nodeID) {
		return nil, ErrDenied
	}
	if direction != DirectionUpstream && direction != DirectionDownstream && direction != DirectionBoth {
		return nil, fmt.Errorf("direction must be upstream, downstream, or both")
	}
	if strings.TrimSpace(rationale) == "" {
		return nil, fmt.Errorf("rationale is required for a scope expansion")
	}
	if len(state.Expansions)+1 > state.Limits.MaxExpansions {
		return nil, ErrLimitReached
	}

	inScope := map[string]bool{}
	for _, node := range state.Current.Nodes {
		inScope[node] = true
	}
	addedNodes := map[string]bool{}
	addedEdges := map[string]bool{}
	for _, edge := range fullEdges {
		connects := false
		switch direction {
		case DirectionDownstream:
			connects = edge.Source == nodeID
		case DirectionUpstream:
			connects = edge.Target == nodeID
		case DirectionBoth:
			connects = edge.Source == nodeID || edge.Target == nodeID
		}
		if !connects {
			continue
		}
		if inScope[edge.Source] && inScope[edge.Target] {
			continue
		}
		if !inScope[edge.Source] {
			addedNodes[edge.Source] = true
		}
		if !inScope[edge.Target] {
			addedNodes[edge.Target] = true
		}
		addedEdges[edge.ID] = true
	}
	if len(addedEdges) == 0 {
		return nil, ErrNoExpansion
	}

	next := state.clone()
	for node := range addedNodes {
		next.Current.Nodes = append(next.Current.Nodes, node)
	}
	for edge := range addedEdges {
		next.Current.Edges = append(next.Current.Edges, edge)
	}
	sort.Strings(next.Current.Nodes)
	sort.Strings(next.Current.Edges)

	addedWorkloads := map[string]bool{}
	for node := range addedNodes {
		if workload := WorkloadFromNode(node); workload != "" {
			addedWorkloads[workload] = true
			if !contains(next.Current.Workloads, workload) {
				next.Current.Workloads = append(next.Current.Workloads, workload)
			}
		}
	}
	sort.Strings(next.Current.Workloads)
	for workload := range addedWorkloads {
		for _, sourcePath := range state.WorkloadSources[workload] {
			if !contains(next.Current.SourcePaths, sourcePath) {
				next.Current.SourcePaths = append(next.Current.SourcePaths, sourcePath)
			}
		}
	}
	sort.Strings(next.Current.SourcePaths)

	totalAdded := len(next.Current.Nodes) - len(state.Initial.Nodes)
	if totalAdded > state.Limits.MaxAddedNodes {
		return nil, ErrLimitReached
	}

	expansion := Expansion{
		FromNode:       nodeID,
		Direction:      direction,
		Rationale:      rationale,
		AddedNodes:     sortedKeys(addedNodes),
		AddedEdges:     sortedKeys(addedEdges),
		AddedWorkloads: sortedKeys(addedWorkloads),
		At:             time.Now().UTC(),
	}
	next.Expansions = append(next.Expansions, expansion)
	next.recomputeFrontier(fullEdges)
	return next, nil
}

func (state *State) clone() *State {
	clone := *state
	clone.Initial = cloneEntities(state.Initial)
	clone.Current = cloneEntities(state.Current)
	clone.Frontier = append([]Frontier(nil), state.Frontier...)
	clone.Expansions = append([]Expansion(nil), state.Expansions...)
	if state.WorkloadSources != nil {
		clone.WorkloadSources = make(map[string][]string, len(state.WorkloadSources))
		for key, value := range state.WorkloadSources {
			clone.WorkloadSources[key] = append([]string(nil), value...)
		}
	}
	return &clone
}

func (state *State) recomputeFrontier(fullEdges []Edge) {
	inScope := map[string]bool{}
	for _, node := range state.Current.Nodes {
		inScope[node] = true
	}
	upstream := map[string]bool{}
	downstream := map[string]bool{}
	for _, edge := range fullEdges {
		if inScope[edge.Source] && !inScope[edge.Target] {
			downstream[edge.Source] = true
		}
		if inScope[edge.Target] && !inScope[edge.Source] {
			upstream[edge.Target] = true
		}
	}
	frontier := make([]Frontier, 0)
	for _, node := range state.Current.Nodes {
		directions := make([]string, 0, 2)
		if upstream[node] {
			directions = append(directions, DirectionUpstream)
		}
		if downstream[node] {
			directions = append(directions, DirectionDownstream)
		}
		if len(directions) > 0 {
			frontier = append(frontier, Frontier{NodeID: node, Directions: directions})
		}
	}
	sort.Slice(frontier, func(i, j int) bool { return frontier[i].NodeID < frontier[j].NodeID })
	state.Frontier = frontier
}

func (state *State) currentNode(node string) bool {
	for _, item := range state.Current.Nodes {
		if item == node {
			return true
		}
	}
	return false
}

// NodeAllowed reports whether a node is in the current scope.
func (state *State) NodeAllowed(node string) bool { return state.currentNode(node) }

// EdgeAllowed reports whether an edge is in the current scope.
func (state *State) EdgeAllowed(edge string) bool {
	for _, item := range state.Current.Edges {
		if item == edge {
			return true
		}
	}
	return false
}

// WorkloadAllowed reports whether a workload is in the current scope.
func (state *State) WorkloadAllowed(workload string) bool {
	for _, item := range state.Current.Workloads {
		if item == workload {
			return true
		}
	}
	return false
}

// RouteAllowed reports whether a route on an edge is in scope. Route
// membership follows the edge: a route list cannot be precomputed for
// neighbours revealed by a later expansion.
func (state *State) RouteAllowed(edgeID, _, _ string) bool {
	return state.EdgeAllowed(edgeID)
}

// SourcePathAllowed reports whether path is equal to or below an allowed source root.
func (state *State) SourcePathAllowed(path string) bool {
	clean, err := filepath.Abs(path)
	if err != nil {
		clean = filepath.Clean(path)
	}
	for _, root := range state.Current.SourcePaths {
		rootAbs, err := filepath.Abs(root)
		if err != nil {
			rootAbs = filepath.Clean(root)
		}
		if clean == rootAbs || strings.HasPrefix(clean, rootAbs+string(filepath.Separator)) {
			return true
		}
	}
	return false
}

// TimeAllowed clamps or rejects a query outside the incident context.
func (state *State) TimeAllowed(from, to time.Time) bool {
	return !from.Before(state.TimeContext.Start) && !to.After(state.TimeContext.End)
}

// WorkloadFromNode extracts the workload name from a Weave node ID.
func WorkloadFromNode(node string) string {
	trimmed := strings.TrimPrefix(node, "workload:")
	if trimmed == node {
		return ""
	}
	_, name, ok := strings.Cut(trimmed, "/")
	if !ok {
		return ""
	}
	return name
}

func cloneEntities(entities Entities) Entities {
	return Entities{
		Nodes:       append([]string(nil), entities.Nodes...),
		Edges:       append([]string(nil), entities.Edges...),
		Routes:      append([]Route(nil), entities.Routes...),
		Workloads:   append([]string(nil), entities.Workloads...),
		SourcePaths: append([]string(nil), entities.SourcePaths...),
	}
}

func contains(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

func sortedKeys(set map[string]bool) []string {
	result := make([]string, 0, len(set))
	for key := range set {
		result = append(result, key)
	}
	sort.Strings(result)
	return result
}
