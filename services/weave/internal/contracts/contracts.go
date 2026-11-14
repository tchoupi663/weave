// Package contracts defines the internal HTTP contracts between the Weave
// services. It is the only package shared across service boundaries; service
// packages must not import each other.
package contracts

import "time"

const (
	// Version is the path version used by the internal endpoints.
	Version = "v1"

	// MaxObservationsPerBatch bounds one observation batch.
	MaxObservationsPerBatch = 500

	// MaxBatchBytes bounds one observation request body.
	MaxBatchBytes = 4 << 20
)

// ObservationBatch is one delivery from weave-live to weave-history. Batches
// are sent at most once: a failed request is dropped and reported as a gap.
type ObservationBatch struct {
	Epoch string          `json:"epoch"`
	Seq   uint64          `json:"seq"`
	L4    []L4Observation `json:"l4"`
	L7    []L7Observation `json:"l7"`
	Gaps  []Gap           `json:"gaps,omitempty"`
}

// L4Observation is one accepted, deduplicated L4 flow observation.
type L4Observation struct {
	Source     string    `json:"source"`
	Target     string    `json:"target"`
	Protocol   string    `json:"protocol"`
	Port       uint32    `json:"port"`
	Connection bool      `json:"connection"`
	Time       time.Time `json:"time"`
}

// L7Observation is one accepted HTTP observation. Path is raw here;
// weave-history owns normalization and route cardinality.
type L7Observation struct {
	Source        string    `json:"source"`
	Target        string    `json:"target"`
	Port          uint32    `json:"port"`
	Method        string    `json:"method"`
	Path          string    `json:"path"`
	Status        uint32    `json:"status"`
	LatencySource string    `json:"latency_source"`
	DurationMS    float64   `json:"duration_ms"`
	Time          time.Time `json:"time"`
}

// Gap is an interval in which live observations could not be delivered and
// therefore were not recorded. It is not zero traffic.
type Gap struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

// Workload is one workload observed in the live graph.
type Workload struct {
	Workload  string    `json:"workload"`
	Kind      string    `json:"kind"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
}

// WorkloadsResponse is the observed-workload feed consumed by weave-inventory.
type WorkloadsResponse struct {
	Workloads []Workload `json:"workloads"`
}
