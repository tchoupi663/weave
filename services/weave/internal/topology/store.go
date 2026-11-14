package topology

import (
	"sort"
	"strings"
	"sync"
	"time"

	flowpb "github.com/cilium/cilium/api/v1/flow"
)

const (
	snapshotBuckets = 60
	seenLimit       = 100_000
)

type nodeState struct {
	id          string
	name        string
	workload    string
	kind        string
	namespace   string
	firstSeen   time.Time
	lastSeen    time.Time
	pods        map[string]struct{}
	ips         map[string]struct{}
	connections uint64
	events      uint64
}

type edgeState struct {
	id          string
	source      string
	target      string
	protocol    string
	port        uint32
	firstSeen   time.Time
	lastSeen    time.Time
	connections uint64
	events      uint64
	activity    map[int64]uint64
	l7          *l7State
}

// Store is the in-memory topology constructor. It contains no declared graph:
// every node and edge in it was created by a flow that survived mapFlow.
type Store struct {
	mu              sync.RWMutex
	namespace       string
	window          time.Duration
	bucketSeconds   int64
	startedAt       time.Time
	nodes           map[string]*nodeState
	edges           map[string]*edgeState
	activity        map[int64]uint64
	seen            map[string]time.Time
	hubbleConnected bool
	hubbleError     string
	lastFlowAt      time.Time
	observed        uint64
	observedL7      uint64
	ingested        uint64
	recorder        ObservationRecorder
}

func NewStore(namespace string, window time.Duration) *Store {
	bucketSeconds := int64(window / snapshotBuckets / time.Second)
	if bucketSeconds < 1 {
		bucketSeconds = 1
	}
	return &Store{
		namespace:     namespace,
		window:        window,
		bucketSeconds: bucketSeconds,
		startedAt:     time.Now().UTC(),
		nodes:         make(map[string]*nodeState),
		edges:         make(map[string]*edgeState),
		activity:      make(map[int64]uint64),
		seen:          make(map[string]time.Time),
	}
}

func (s *Store) Namespace() string { return s.namespace }

func (s *Store) Window() time.Duration { return s.window }

// Ingest maps one Hubble flow into the store. It returns false for replies,
// control-plane traffic, probes, and duplicates.
func (s *Store) Ingest(f *flowpb.Flow) bool {
	mapped, ok := mapFlow(f, s.namespace)
	if !ok {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if _, duplicate := s.seen[mapped.EventKey]; duplicate {
		return false
	}
	s.seen[mapped.EventKey] = mapped.Time
	s.ingested++
	if s.ingested%1024 == 0 || len(s.seen) > seenLimit {
		s.pruneSeenLocked(time.Now())
	}

	if mapped.Time.After(s.lastFlowAt) {
		s.lastFlowAt = mapped.Time
	}
	s.observed++

	src := s.nodeLocked(mapped.Source, mapped.Time)
	dst := s.nodeLocked(mapped.Target, mapped.Time)
	src.events++
	dst.events++
	if mapped.Connection {
		src.connections++
		dst.connections++
	}

	edgeID := mapped.Source.ID + "->" + mapped.Target.ID + ":" + uitoa(mapped.Port)
	edge, exists := s.edges[edgeID]
	if !exists {
		edge = &edgeState{
			id:        edgeID,
			source:    mapped.Source.ID,
			target:    mapped.Target.ID,
			protocol:  mapped.Protocol,
			port:      mapped.Port,
			firstSeen: mapped.Time,
			activity:  make(map[int64]uint64),
		}
		s.edges[edgeID] = edge
	}
	if mapped.Time.Before(edge.firstSeen) {
		edge.firstSeen = mapped.Time
	}
	if mapped.Time.After(edge.lastSeen) {
		edge.lastSeen = mapped.Time
	}
	edge.events++
	if mapped.Connection {
		edge.connections++
	}
	bucket := s.bucketFor(mapped.Time)
	edge.activity[bucket]++
	s.activity[bucket]++

	if s.recorder != nil {
		s.recorder.ObserveL4(mapped.Source.Workload, mapped.Target.Workload, mapped.Protocol, mapped.Port, mapped.Connection, mapped.Time)
	}

	return true
}

func (s *Store) nodeLocked(ref endpointRef, at time.Time) *nodeState {
	node, ok := s.nodes[ref.ID]
	if !ok {
		node = &nodeState{
			id:        ref.ID,
			name:      ref.Name,
			workload:  ref.Workload,
			kind:      ref.Kind,
			namespace: ref.Namespace,
			firstSeen: at,
			pods:      make(map[string]struct{}),
			ips:       make(map[string]struct{}),
		}
		s.nodes[ref.ID] = node
	}
	if at.Before(node.firstSeen) {
		node.firstSeen = at
	}
	if (node.kind == "" || node.kind == "Pod" || node.kind == "Workload") && ref.Kind != "" && ref.Kind != "Workload" {
		node.kind = ref.Kind
	}
	if at.After(node.lastSeen) {
		node.lastSeen = at
	}
	if ref.Pod != "" {
		node.pods[ref.Pod] = struct{}{}
	}
	if ref.IP != "" && ref.IP != "unknown" {
		node.ips[ref.IP] = struct{}{}
	}
	return node
}

func (s *Store) bucketFor(t time.Time) int64 {
	return t.Unix() / s.bucketSeconds
}

func (s *Store) pruneSeenLocked(now time.Time) {
	cutoff := now.Add(-2 * s.window)
	for key, seenAt := range s.seen {
		if seenAt.Before(cutoff) {
			delete(s.seen, key)
		}
	}
	if len(s.seen) > seenLimit {
		// Keep the memory bound even during a flow storm. Losing a dedupe key
		// only risks double-counting one observation, never losing topology.
		type entry struct {
			key  string
			when time.Time
		}
		entries := make([]entry, 0, len(s.seen))
		for key, when := range s.seen {
			entries = append(entries, entry{key, when})
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].when.Before(entries[j].when) })
		for i := 0; i < len(entries)-seenLimit; i++ {
			delete(s.seen, entries[i].key)
		}
	}
}

func (s *Store) SetHubble(connected bool, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.hubbleConnected = connected
	if err != nil {
		s.hubbleError = err.Error()
	} else {
		s.hubbleError = ""
	}
}

func (s *Store) LastFlowAt() time.Time {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.lastFlowAt
}

func (s *Store) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.startedAt = time.Now().UTC()
	s.nodes = make(map[string]*nodeState)
	s.edges = make(map[string]*edgeState)
	s.activity = make(map[int64]uint64)
	s.seen = make(map[string]time.Time)
	s.lastFlowAt = time.Time{}
	s.observed = 0
	s.observedL7 = 0
	s.ingested = 0
}

func (s *Store) Snapshot() Snapshot {
	return s.SnapshotAt(time.Now())
}

func (s *Store) SnapshotAt(now time.Time) Snapshot {
	s.mu.RLock()
	defer s.mu.RUnlock()

	now = now.UTC()
	windowSeconds := int(s.window.Seconds())
	activity := s.activityLocked(now)
	edges := make([]Edge, 0, len(s.edges))
	globals := make([]uint64, snapshotBuckets)
	outdegree := make(map[string]int)
	totalConnections := uint64(0)
	totalEvents := uint64(0)
	totalL7Edges := 0
	totalL7Requests := uint64(0)
	totalL7Responses := uint64(0)

	for _, edge := range s.edges {
		outdegree[edge.source]++
		totalConnections += edge.connections
		totalEvents += edge.events
		l7 := edge.l7.snapshot(now, s.bucketSeconds)
		if l7 != nil {
			totalL7Edges++
			totalL7Requests += l7.Requests
			totalL7Responses += l7.Responses
		}
		edges = append(edges, Edge{
			ID:          edge.id,
			Source:      edge.source,
			Target:      edge.target,
			Protocol:    edge.protocol,
			Port:        edge.port,
			Connections: edge.connections,
			Events:      edge.events,
			FirstSeen:   edge.firstSeen.UTC(),
			LastSeen:    edge.lastSeen.UTC(),
			Activity:    edgeActivity(edge.activity, now, s.bucketSeconds),
			L7:          l7,
		})
	}
	for i, count := range activity {
		globals[i] = count
	}
	sort.Slice(edges, func(i, j int) bool {
		if edges[i].Source != edges[j].Source {
			return edges[i].Source < edges[j].Source
		}
		if edges[i].Target != edges[j].Target {
			return edges[i].Target < edges[j].Target
		}
		return edges[i].Port < edges[j].Port
	})

	nodes := make([]Node, 0, len(s.nodes))
	for _, node := range s.nodes {
		role := RoleService
		switch {
		case node.id == "source-ip":
			role = RoleSource
		case strings.EqualFold(node.kind, "StatefulSet"):
			role = RoleData
		case node.workload == "external":
			role = RoleExternal
		case outdegree[node.id] >= 3:
			role = RoleHub
		}
		nodes = append(nodes, Node{
			ID:          node.id,
			Workload:    node.workload,
			Name:        node.name,
			Kind:        node.kind,
			Namespace:   node.namespace,
			Role:        role,
			Pods:        sortedSet(node.pods),
			IPs:         sortedSet(node.ips),
			Connections: node.connections,
			Events:      node.events,
			FirstSeen:   node.firstSeen.UTC(),
			LastSeen:    node.lastSeen.UTC(),
		})
	}
	sort.Slice(nodes, func(i, j int) bool { return nodes[i].Name < nodes[j].Name })

	status := HubbleStatus{
		Connected:  s.hubbleConnected,
		LastError:  s.hubbleError,
		Observed:   s.observed,
		ObservedL7: s.observedL7,
	}
	if !s.lastFlowAt.IsZero() {
		last := s.lastFlowAt.UTC()
		status.LastFlowAt = &last
	}

	return Snapshot{
		GeneratedAt:   now,
		StartedAt:     s.startedAt.UTC(),
		Namespace:     s.namespace,
		WindowSeconds: windowSeconds,
		Nodes:         nodes,
		Edges:         edges,
		Activity:      globals,
		Totals: Totals{
			Nodes:       len(nodes),
			Edges:       len(edges),
			Connections: totalConnections,
			Events:      totalEvents,
			L7Edges:     totalL7Edges,
			L7Requests:  totalL7Requests,
			L7Responses: totalL7Responses,
		},
		Hubble: status,
	}
}

func (s *Store) activityLocked(now time.Time) []uint64 {
	return edgeActivity(s.activity, now, s.bucketSeconds)
}

func edgeActivity(activity map[int64]uint64, now time.Time, bucketSeconds int64) []uint64 {
	result := make([]uint64, snapshotBuckets)
	base := now.Unix() / bucketSeconds * bucketSeconds
	for i := 0; i < snapshotBuckets; i++ {
		offset := int64(snapshotBuckets-1-i) * bucketSeconds
		result[i] = activity[(base-offset)/bucketSeconds]
	}
	return result
}

func sortedSet(set map[string]struct{}) []string {
	values := make([]string, 0, len(set))
	for value := range set {
		values = append(values, value)
	}
	sort.Strings(values)
	return values
}

func uitoa(value uint32) string {
	if value == 0 {
		return "0"
	}
	var buf [10]byte
	i := len(buf)
	for value > 0 {
		i--
		buf[i] = byte('0' + value%10)
		value /= 10
	}
	return string(buf[i:])
}
