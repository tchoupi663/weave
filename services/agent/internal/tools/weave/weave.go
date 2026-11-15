// Package weave exposes the read-only Weave incident context to the agent.
package weave

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/tchoupi663/weave/services/agent/internal/scope"
)

type Config struct {
	WeaveURL string
	Guard    *scope.Guard
}

type Client struct {
	baseURL string
	guard   *scope.Guard
	http    *http.Client
}

func New(cfg Config) *Client {
	return &Client{
		baseURL: strings.TrimRight(cfg.WeaveURL, "/"),
		guard:   cfg.Guard,
		http:    &http.Client{Timeout: 10 * time.Second},
	}
}

// Register adds the Weave incident tools to an MCP server.
func Register(server *mcp.Server, cfg Config) {
	client := New(cfg)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_incident",
		Description: "Read the incident metadata and detection trigger. The evidence snapshot is intentionally not exposed; use the incident context and observability tools.",
	}, client.getIncident)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_scope",
		Description: "Read the current investigation scope, frontier nodes, expansion limits, and expansion history.",
	}, client.getScope)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_topology",
		Description: "Read the observed topology. A confined run sees only the in-scope subgraph plus frontier markers.",
	}, client.getTopology)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_changes",
		Description: "Read lifecycle change events in a time range, filtered to in-scope workloads in a confined run.",
	}, client.getChanges)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "get_recording_gaps",
		Description: "Read recording gaps in a time range. Missing recording is not zero traffic.",
	}, client.getRecordingGaps)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "expand_scope",
		Description: "Confined runs only: expand the investigation scope by one observed hop from a frontier node. Requires a direction and a rationale.",
	}, client.expandScope)
}

// IncidentView is the safe subset of an incident exposed to the agent.
type IncidentView struct {
	ID            string    `json:"id"`
	WindowStart   time.Time `json:"window_start"`
	WindowEnd     time.Time `json:"window_end"`
	BaselineStart time.Time `json:"baseline_start"`
	BaselineEnd   time.Time `json:"baseline_end"`
	Symptom       string    `json:"symptom"`
	Status        string    `json:"status"`
	Origin        string    `json:"origin,omitempty"`
	Trigger       any       `json:"trigger,omitempty"`
}

type getIncidentIn struct {
	IncidentID string `json:"incident_id" jsonschema:"the incident id to read"`
}

func (c *Client) getIncident(ctx context.Context, _ *mcp.CallToolRequest, in getIncidentIn) (*mcp.CallToolResult, IncidentView, error) {
	if strings.TrimSpace(in.IncidentID) == "" {
		return nil, IncidentView{}, fmt.Errorf("incident_id is required")
	}
	state, err := c.guard.Load()
	if err != nil {
		return nil, IncidentView{}, err
	}
	if state != nil && state.IncidentID != in.IncidentID {
		return nil, IncidentView{}, scope.ErrDenied
	}
	var payload struct {
		ID            string          `json:"id"`
		WindowStart   time.Time       `json:"window_start"`
		WindowEnd     time.Time       `json:"window_end"`
		BaselineStart time.Time       `json:"baseline_start"`
		BaselineEnd   time.Time       `json:"baseline_end"`
		Symptom       string          `json:"symptom"`
		Status        string          `json:"status"`
		Origin        string          `json:"origin"`
		Trigger       json.RawMessage `json:"trigger"`
	}
	if err := c.getJSON(ctx, "/api/incidents/"+url.PathEscape(in.IncidentID), &payload); err != nil {
		return nil, IncidentView{}, err
	}
	var trigger any
	if len(payload.Trigger) > 0 {
		_ = json.Unmarshal(payload.Trigger, &trigger)
	}
	return nil, IncidentView{
		ID:            payload.ID,
		WindowStart:   payload.WindowStart,
		WindowEnd:     payload.WindowEnd,
		BaselineStart: payload.BaselineStart,
		BaselineEnd:   payload.BaselineEnd,
		Symptom:       payload.Symptom,
		Status:        payload.Status,
		Origin:        payload.Origin,
		Trigger:       trigger,
	}, nil
}

type ScopeView struct {
	Mode        string            `json:"mode"`
	IncidentID  string            `json:"incident_id,omitempty"`
	TimeContext scope.TimeRange   `json:"time_context,omitempty"`
	Current     scope.Entities    `json:"current,omitempty"`
	Frontier    []scope.Frontier  `json:"frontier,omitempty"`
	Expansions  []scope.Expansion `json:"expansions,omitempty"`
	Limits      scope.Limits      `json:"limits,omitempty"`
}

func scopeView(state *scope.State) ScopeView {
	if state == nil {
		return ScopeView{Mode: scope.ModeUnconfined}
	}
	return ScopeView{
		Mode:        state.Mode,
		IncidentID:  state.IncidentID,
		TimeContext: state.TimeContext,
		Current:     state.Current,
		Frontier:    state.Frontier,
		Expansions:  state.Expansions,
		Limits:      state.Limits,
	}
}

func (c *Client) getScope(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, ScopeView, error) {
	state, err := c.guard.Load()
	if err != nil {
		return nil, ScopeView{}, err
	}
	return nil, scopeView(state), nil
}

type TopologyNode struct {
	ID          string `json:"id"`
	Workload    string `json:"workload"`
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Role        string `json:"role,omitempty"`
	Connections uint64 `json:"connections"`
	Events      uint64 `json:"events"`
	LastSeen    string `json:"last_seen,omitempty"`
}

type TopologyEdge struct {
	ID          string `json:"id"`
	Source      string `json:"source"`
	Target      string `json:"target"`
	Protocol    string `json:"protocol"`
	Port        uint32 `json:"port"`
	Connections uint64 `json:"connections"`
	Events      uint64 `json:"events"`
	LastSeen    string `json:"last_seen,omitempty"`
}

type TopologyView struct {
	Nodes    []TopologyNode   `json:"nodes"`
	Edges    []TopologyEdge   `json:"edges"`
	Frontier []scope.Frontier `json:"frontier,omitempty"`
	Mode     string           `json:"mode"`
}

func (c *Client) getTopology(ctx context.Context, _ *mcp.CallToolRequest, _ struct{}) (*mcp.CallToolResult, TopologyView, error) {
	var payload struct {
		Nodes []TopologyNode `json:"nodes"`
		Edges []TopologyEdge `json:"edges"`
	}
	if err := c.getJSON(ctx, "/api/topology", &payload); err != nil {
		return nil, TopologyView{}, err
	}
	state, err := c.guard.Load()
	if err != nil {
		return nil, TopologyView{}, err
	}
	if state == nil {
		return nil, TopologyView{Nodes: payload.Nodes, Edges: payload.Edges, Mode: scope.ModeUnconfined}, nil
	}
	view := TopologyView{Nodes: make([]TopologyNode, 0), Edges: make([]TopologyEdge, 0), Frontier: state.Frontier, Mode: state.Mode}
	for _, node := range payload.Nodes {
		if state.NodeAllowed(node.ID) {
			view.Nodes = append(view.Nodes, node)
		}
	}
	for _, edge := range payload.Edges {
		if state.EdgeAllowed(edge.ID) {
			view.Edges = append(view.Edges, edge)
		}
	}
	return nil, view, nil
}

type getChangesIn struct {
	From string `json:"from,omitempty" jsonschema:"RFC3339 start; defaults to the incident context start"`
	To   string `json:"to,omitempty" jsonschema:"RFC3339 end; defaults to the incident context end"`
}

type ChangeView struct {
	ID         int64     `json:"id"`
	OccurredAt time.Time `json:"occurred_at"`
	Source     string    `json:"source"`
	Type       string    `json:"type"`
	Workload   string    `json:"workload"`
	Kind       string    `json:"kind"`
	Details    any       `json:"details,omitempty"`
}

func (c *Client) getChanges(ctx context.Context, _ *mcp.CallToolRequest, in getChangesIn) (*mcp.CallToolResult, []ChangeView, error) {
	state, err := c.guard.Load()
	if err != nil {
		return nil, nil, err
	}
	from, to, err := c.defaultRange(in.From, in.To, state)
	if err != nil {
		return nil, nil, err
	}
	from, to, err = c.guard.ClampTime(from, to)
	if err != nil {
		return nil, nil, err
	}
	query := url.Values{}
	query.Set("from", from.UTC().Format(time.RFC3339))
	query.Set("to", to.UTC().Format(time.RFC3339))
	query.Set("limit", "200")
	var payload []ChangeView
	if err := c.getJSON(ctx, "/api/changes?"+query.Encode(), &payload); err != nil {
		return nil, nil, err
	}
	if state == nil {
		return nil, payload, nil
	}
	filtered := make([]ChangeView, 0, len(payload))
	for _, change := range payload {
		if state.WorkloadAllowed(change.Workload) {
			filtered = append(filtered, change)
		}
	}
	return nil, filtered, nil
}

type getGapsIn struct {
	From string `json:"from,omitempty" jsonschema:"RFC3339 start; defaults to the incident context start"`
	To   string `json:"to,omitempty" jsonschema:"RFC3339 end; defaults to the incident context end"`
}

func (c *Client) getRecordingGaps(ctx context.Context, _ *mcp.CallToolRequest, in getGapsIn) (*mcp.CallToolResult, []scope.TimeRange, error) {
	state, err := c.guard.Load()
	if err != nil {
		return nil, nil, err
	}
	from, to, err := c.defaultRange(in.From, in.To, state)
	if err != nil {
		return nil, nil, err
	}
	from, to, err = c.guard.ClampTime(from, to)
	if err != nil {
		return nil, nil, err
	}
	query := url.Values{}
	query.Set("from", from.UTC().Format(time.RFC3339))
	query.Set("to", to.UTC().Format(time.RFC3339))
	var payload struct {
		Gaps []struct {
			From time.Time `json:"from"`
			To   time.Time `json:"to"`
		} `json:"gaps"`
	}
	if err := c.getJSON(ctx, "/api/gaps?"+query.Encode(), &payload); err != nil {
		return nil, nil, err
	}
	result := make([]scope.TimeRange, 0, len(payload.Gaps))
	for _, gap := range payload.Gaps {
		result = append(result, scope.TimeRange{Start: gap.From, End: gap.To})
	}
	return nil, result, nil
}

type expandIn struct {
	NodeID    string `json:"node_id" jsonschema:"an in-scope frontier node"`
	Direction string `json:"direction" jsonschema:"upstream, downstream, or both"`
	Rationale string `json:"rationale" jsonschema:"why the current scope is insufficient and which way the root cause may lie"`
}

type expandOut struct {
	AddedNodes          []string         `json:"added_nodes"`
	AddedEdges          []string         `json:"added_edges"`
	Frontier            []scope.Frontier `json:"frontier"`
	Limits              scope.Limits     `json:"limits"`
	RemainingExpansions int              `json:"remaining_expansions"`
}

func (c *Client) expandScope(ctx context.Context, _ *mcp.CallToolRequest, in expandIn) (*mcp.CallToolResult, expandOut, error) {
	state, err := c.guard.Load()
	if err != nil {
		return nil, expandOut{}, err
	}
	if state == nil {
		return nil, expandOut{}, fmt.Errorf("expand_scope is only available in a confined run")
	}
	var topology struct {
		Edges []TopologyEdge `json:"edges"`
	}
	if err := c.getJSON(ctx, "/api/topology", &topology); err != nil {
		return nil, expandOut{}, err
	}
	edges := make([]scope.Edge, 0, len(topology.Edges))
	for _, edge := range topology.Edges {
		edges = append(edges, scope.Edge{ID: edge.ID, Source: edge.Source, Target: edge.Target})
	}
	next, err := state.Expand(edges, in.NodeID, in.Direction, in.Rationale)
	if err != nil {
		return nil, expandOut{}, err
	}
	if err := scope.Save(c.guard.Path, next); err != nil {
		return nil, expandOut{}, err
	}
	expansion := next.Expansions[len(next.Expansions)-1]
	return nil, expandOut{
		AddedNodes:          expansion.AddedNodes,
		AddedEdges:          expansion.AddedEdges,
		Frontier:            next.Frontier,
		Limits:              next.Limits,
		RemainingExpansions: next.Limits.MaxExpansions - len(next.Expansions),
	}, nil
}

func (c *Client) defaultRange(fromRaw, toRaw string, state *scope.State) (time.Time, time.Time, error) {
	now := time.Now().UTC()
	var from, to time.Time
	if fromRaw == "" && toRaw == "" && state != nil {
		return state.TimeContext.Start, state.TimeContext.End, nil
	}
	if fromRaw == "" {
		from = now.Add(-time.Hour)
	} else {
		parsed, err := time.Parse(time.RFC3339, fromRaw)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("from must be RFC3339")
		}
		from = parsed
	}
	if toRaw == "" {
		to = now
	} else {
		parsed, err := time.Parse(time.RFC3339, toRaw)
		if err != nil {
			return time.Time{}, time.Time{}, fmt.Errorf("to must be RFC3339")
		}
		to = parsed
	}
	if !to.After(from) {
		return time.Time{}, time.Time{}, fmt.Errorf("to must be after from")
	}
	return from, to, nil
}

func (c *Client) getJSON(ctx context.Context, path string, out any) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	response, err := c.http.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		return fmt.Errorf("weave api %s returned %d: %s", path, response.StatusCode, strings.TrimSpace(string(body)))
	}
	return json.NewDecoder(response.Body).Decode(out)
}
