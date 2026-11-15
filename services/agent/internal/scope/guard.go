package scope

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Guard is the read-side scope enforcement used by every MCP server. An empty
// path means the unconfined arm: all reads are allowed.
type Guard struct {
	Path string
}

// GuardFromEnv reads WEAVE_SCOPE_FILE.
func GuardFromEnv() *Guard {
	return &Guard{Path: strings.TrimSpace(os.Getenv("WEAVE_SCOPE_FILE"))}
}

// Confined reports whether a scope file is configured.
func (g *Guard) Confined() bool {
	return g != nil && strings.TrimSpace(g.Path) != ""
}

// Load returns the current scope, or nil for an unconfined run.
func (g *Guard) Load() (*State, error) {
	if !g.Confined() {
		return nil, nil
	}
	return Load(g.Path)
}

// RequireNode denies a node outside the current scope.
func (g *Guard) RequireNode(node string) (*State, error) {
	state, err := g.Load()
	if err != nil {
		return nil, err
	}
	if state == nil {
		return nil, nil
	}
	if !state.NodeAllowed(node) {
		return nil, ErrDenied
	}
	return state, nil
}

// RequireEdge denies an edge outside the current scope.
func (g *Guard) RequireEdge(edge string) (*State, error) {
	state, err := g.Load()
	if err != nil {
		return nil, err
	}
	if state == nil {
		return nil, nil
	}
	if !state.EdgeAllowed(edge) {
		return nil, ErrDenied
	}
	return state, nil
}

// RequireWorkload denies a workload outside the current scope.
func (g *Guard) RequireWorkload(workload string) (*State, error) {
	state, err := g.Load()
	if err != nil {
		return nil, err
	}
	if state == nil {
		return nil, nil
	}
	if !state.WorkloadAllowed(workload) {
		return nil, ErrDenied
	}
	return state, nil
}

// RequireRoute denies a route whose edge is outside the current scope.
func (g *Guard) RequireRoute(edgeID, method, path string) (*State, error) {
	state, err := g.RequireEdge(edgeID)
	if err != nil {
		return nil, err
	}
	if state == nil {
		return nil, nil
	}
	if !state.RouteAllowed(edgeID, method, path) {
		return nil, ErrDenied
	}
	return state, nil
}

// RequireSourcePath denies a source path outside the allowed roots.
func (g *Guard) RequireSourcePath(path string) (*State, error) {
	state, err := g.Load()
	if err != nil {
		return nil, err
	}
	if state == nil {
		return nil, nil
	}
	if !state.SourcePathAllowed(path) {
		return nil, ErrDenied
	}
	return state, nil
}

// ClampTime restricts a query to the incident context. It returns ErrDenied
// when the requested interval has no overlap with the context.
func (g *Guard) ClampTime(from, to time.Time) (time.Time, time.Time, error) {
	state, err := g.Load()
	if err != nil {
		return from, to, err
	}
	if state == nil {
		return from, to, nil
	}
	start, end := state.TimeContext.Start, state.TimeContext.End
	if to.Before(start) || from.After(end) {
		return from, to, ErrDenied
	}
	if from.Before(start) {
		from = start
	}
	if to.After(end) {
		to = end
	}
	if !to.After(from) {
		return from, to, fmt.Errorf("time range is empty after clamping")
	}
	return from, to, nil
}

// Workloads returns the in-scope workloads, or nil for an unconfined run.
func (g *Guard) Workloads() ([]string, error) {
	state, err := g.Load()
	if err != nil {
		return nil, err
	}
	if state == nil {
		return nil, nil
	}
	return append([]string(nil), state.Current.Workloads...), nil
}
