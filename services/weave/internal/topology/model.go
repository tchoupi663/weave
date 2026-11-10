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
}

type Totals struct {
	Nodes       int    `json:"nodes"`
	Edges       int    `json:"edges"`
	Connections uint64 `json:"connections"`
	Events      uint64 `json:"events"`
}

type HubbleStatus struct {
	Connected  bool       `json:"connected"`
	LastFlowAt *time.Time `json:"last_flow_at,omitempty"`
	LastError  string     `json:"last_error,omitempty"`
	Observed   uint64     `json:"observed_flows"`
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
