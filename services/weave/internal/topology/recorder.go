package topology

import "time"

// ObservationRecorder receives accepted observations after they have been
// deduplicated and attached to the live topology. Implementations must be
// non-blocking on the ingest path.
type ObservationRecorder interface {
	ObserveL4(source, target, protocol string, port uint32, connection bool, at time.Time)
	ObserveL7(source, target string, port uint32, observation L7Observation)
}

// ObservedWorkload is the lifecycle view of one observed node.
type ObservedWorkload struct {
	Workload  string
	Kind      string
	FirstSeen time.Time
	LastSeen  time.Time
}

func (s *Store) SetRecorder(recorder ObservationRecorder) {
	s.mu.Lock()
	s.recorder = recorder
	s.mu.Unlock()
}

// ObservedWorkloads returns the workloads currently present in the live graph,
// excluding the aggregate Source IP boundary.
func (s *Store) ObservedWorkloads() []ObservedWorkload {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]ObservedWorkload, 0, len(s.nodes))
	for _, node := range s.nodes {
		if node.id == "source-ip" || node.workload == "" {
			continue
		}
		result = append(result, ObservedWorkload{
			Workload:  node.workload,
			Kind:      node.kind,
			FirstSeen: node.firstSeen,
			LastSeen:  node.lastSeen,
		})
	}
	return result
}
