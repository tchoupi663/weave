package topology

import (
	"strings"
	"time"
)

// Roles are presentational classes inferred from the observed graph, not from
// a declared application model. They keep the screen's vocabulary small.
const (
	RoleSource   = "source"
	RoleHub      = "hub"
	RoleService  = "service"
	RoleExternal = "external"
	RoleData     = "data"
)

// Node is one logical workload, or the aggregate Source IP endpoint. A node's
// existence is earned by at least one observed flow.
type Node struct {
	ID          string    `json:"id"`
	Workload    string    `json:"workload"`
	Name        string    `json:"name"`
	Kind        string    `json:"kind"`
	Namespace   string    `json:"namespace"`
	Role        string    `json:"role"`
	Pods        []string  `json:"pods"`
	IPs         []string  `json:"ips"`
	Connections uint64    `json:"connections"`
	Events      uint64    `json:"events"`
	FirstSeen   time.Time `json:"first_seen"`
	LastSeen    time.Time `json:"last_seen"`
}

// Edge is one ordered workload pair at one destination port. Replies and
// handshake packets fold into it rather than becoming separate edges.
type Edge struct {
	ID          string    `json:"id"`
	Source      string    `json:"source"`
	Target      string    `json:"target"`
	Protocol    string    `json:"protocol"`
	Port        uint32    `json:"port"`
	Connections uint64    `json:"connections"`
	Events      uint64    `json:"events"`
	FirstSeen   time.Time `json:"first_seen"`
	LastSeen    time.Time `json:"last_seen"`
	Activity    []uint64  `json:"activity"`
	L7          *L7Stats  `json:"l7,omitempty"`
}

// L7Stats is HTTP telemetry attached to the L4 edge that connects the same
// two workloads. LatencySource says whether the duration came from the client
// or server span. It is additive: L4 counters are not replaced or summed with
// L7 request counts.
type L7Stats struct {
	Protocol      string             `json:"protocol"`
	LatencySource string             `json:"latency_source"`
	Requests      uint64             `json:"requests"`
	Responses     uint64             `json:"responses"`
	Status        L7StatusCounts     `json:"status"`
	LatencyMs     LatencyPercentiles `json:"latency_ms"`
	Window        L7Window           `json:"window"`
	Routes        []L7Route          `json:"routes"`
	FirstSeen     time.Time          `json:"first_seen"`
	LastSeen      time.Time          `json:"last_seen"`
}

type L7StatusCounts struct {
	Success     uint64 `json:"2xx"`
	Redirect    uint64 `json:"3xx"`
	ClientError uint64 `json:"4xx"`
	ServerError uint64 `json:"5xx"`
	Other       uint64 `json:"other"`
}

type LatencyPercentiles struct {
	P50 float64 `json:"p50"`
	P95 float64 `json:"p95"`
}

type L7Window struct {
	Requests  uint64             `json:"requests"`
	Responses uint64             `json:"responses"`
	Status    L7StatusCounts     `json:"status"`
	LatencyMs LatencyPercentiles `json:"latency_ms"`
	Activity  []uint64           `json:"activity"`
}

type L7Route struct {
	Method    string         `json:"method"`
	Path      string         `json:"path"`
	Requests  uint64         `json:"requests"`
	Responses uint64         `json:"responses"`
	Status    L7StatusCounts `json:"status"`
}

type Totals struct {
	Nodes       int    `json:"nodes"`
	Edges       int    `json:"edges"`
	Connections uint64 `json:"connections"`
	Events      uint64 `json:"events"`
	L7Edges     int    `json:"l7_edges"`
	L7Requests  uint64 `json:"l7_requests"`
	L7Responses uint64 `json:"l7_responses"`
}

type HubbleStatus struct {
	Connected  bool       `json:"connected"`
	LastFlowAt *time.Time `json:"last_flow_at,omitempty"`
	LastError  string     `json:"last_error,omitempty"`
	Observed   uint64     `json:"observed_flows"`
	ObservedL7 uint64     `json:"observed_l7_observations"`
}

type Snapshot struct {
	GeneratedAt   time.Time    `json:"generated_at"`
	StartedAt     time.Time    `json:"started_at"`
	Namespace     string       `json:"namespace"`
	WindowSeconds int          `json:"window_seconds"`
	Nodes         []Node       `json:"nodes"`
	Edges         []Edge       `json:"edges"`
	Activity      []uint64     `json:"activity"`
	Totals        Totals       `json:"totals"`
	Hubble        HubbleStatus `json:"hubble"`
}

var displayNames = map[string]string{
	"frontend": "Frontend",
	"gateway":  "API Gateway",
	"auth":     "Auth",
	"catalog":  "Catalog",
	"orders":   "Orders",
	"payment":  "Payment",
	"external": "External Payment Provider",
	"postgres": "Postgres",
}

func displayName(workload string) string {
	if name, ok := displayNames[workload]; ok {
		return name
	}
	return titleCase(workload)
}

func titleCase(s string) string {
	if s == "" {
		return "Unknown"
	}
	parts := strings.FieldsFunc(s, func(r rune) bool { return r == '-' || r == '_' })
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	if len(parts) == 0 {
		return s
	}
	return strings.Join(parts, " ")
}
