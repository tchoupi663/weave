package topology

import (
	"fmt"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	latencyBucketCount = 64
	latencyMinNS       = 100_000.0
	latencyMaxNS       = 10_000_000_000.0
	maxL7Routes        = 64
)

type statusCounts [5]uint64

const (
	status2xx = iota
	status3xx
	status4xx
	status5xx
	statusOther
)

// L7Observation is one completed HTTP span. It is deliberately header-free
// and query-free: only method, path, status, duration, latency source, and the
// observed workload endpoints cross this boundary.
type L7Observation struct {
	SourceWorkload  string
	SourceKind      string
	SourcePod       string
	SourceNamespace string
	TargetWorkload  string
	TargetNamespace string
	TargetIP        string
	Port            uint32
	Method          string
	Path            string
	StatusCode      uint32
	LatencySource   string
	Duration        time.Duration
	Time            time.Time
	Key             string
}

type l7State struct {
	requests      uint64
	responses     uint64
	status        statusCounts
	latency       latencyHistogram
	windowStatus  map[int64]statusCounts
	windowLatency map[int64]*latencyHistogram
	activity      map[int64]uint64
	routes        map[string]*routeState
	latencySource string
	firstSeen     time.Time
	lastSeen      time.Time
}

type routeState struct {
	method    string
	path      string
	requests  uint64
	responses uint64
	status    statusCounts
}

type latencyHistogram struct {
	counts [latencyBucketCount]uint64
}

// IngestL7 adds one client-observed HTTP span to the matching L4 edge. The
// target may be absent when the observer only has an unresolved address; in
// that case the existing L4 graph resolves it by source workload and port.
func (s *Store) IngestL7(obs L7Observation) bool {
	if obs.SourceWorkload == "" || obs.SourceNamespace != s.namespace {
		return false
	}
	if obs.TargetNamespace == "" {
		obs.TargetNamespace = s.namespace
	}
	if obs.TargetNamespace != s.namespace || obs.Port == 0 || obs.Method == "" || obs.Path == "" {
		return false
	}
	if obs.Time.IsZero() {
		obs.Time = time.Now()
	}
	if obs.Duration <= 0 {
		return false
	}
	if obs.SourceKind == "" {
		obs.SourceKind = "Workload"
	}
	if obs.LatencySource == "" {
		obs.LatencySource = "client"
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	sourceID := "workload:" + s.namespace + "/" + obs.SourceWorkload
	targetID := ""
	if obs.TargetWorkload != "" {
		targetID = "workload:" + s.namespace + "/" + obs.TargetWorkload
	} else {
		targetID = s.resolveL7TargetLocked(sourceID, obs.Port, obs.TargetIP)
	}
	if targetID == "" || targetID == sourceID {
		return false
	}

	edgeID := sourceID + "->" + targetID + ":" + uitoa(obs.Port)
	edge, exists := s.edges[edgeID]
	if !exists {
		// L4 remains the source of truth. An L7 span without a matching L4
		// edge is dropped rather than inventing topology from HTTP metadata.
		return false
	}

	key := obs.Key
	if key == "" {
		key = fmt.Sprintf("%s|%s|%d|%s|%s|%d|%d", sourceID, targetID, obs.Port, obs.Method, obs.Path, obs.StatusCode, obs.Time.UnixNano())
	}
	key = "l7:" + key
	if _, duplicate := s.seen[key]; duplicate {
		return false
	}
	s.seen[key] = obs.Time
	s.ingested++
	if s.ingested%1024 == 0 || len(s.seen) > seenLimit {
		s.pruneSeenLocked(time.Now())
		s.pruneL7WindowsLocked(obs.Time)
	}
	if obs.Time.After(s.lastFlowAt) {
		s.lastFlowAt = obs.Time
	}
	s.observedL7++

	if edge.l7 == nil {
		edge.l7 = &l7State{
			windowStatus:  make(map[int64]statusCounts),
			windowLatency: make(map[int64]*latencyHistogram),
			activity:      make(map[int64]uint64),
			routes:        make(map[string]*routeState),
		}
	}
	edge.l7.add(obs, s.bucketFor(obs.Time))

	return true
}

func (s *Store) resolveL7TargetLocked(sourceID string, port uint32, targetIP string) string {
	targets := make(map[string]struct{})
	for _, edge := range s.edges {
		if edge.source == sourceID && edge.port == port {
			targets[edge.target] = struct{}{}
		}
	}
	if len(targets) == 1 {
		for id := range targets {
			return id
		}
	}
	if targetIP != "" {
		for id, node := range s.nodes {
			for ip := range node.ips {
				if ip == targetIP {
					targets[id] = struct{}{}
				}
			}
		}
	}
	if len(targets) == 1 {
		for id := range targets {
			return id
		}
	}
	return ""
}

func (s *Store) pruneL7WindowsLocked(now time.Time) {
	cutoff := s.bucketFor(now.Add(-2 * s.window))
	for _, edge := range s.edges {
		l7 := edge.l7
		if l7 == nil {
			continue
		}
		for bucket := range l7.activity {
			if bucket < cutoff {
				delete(l7.activity, bucket)
			}
		}
		for bucket := range l7.windowStatus {
			if bucket < cutoff {
				delete(l7.windowStatus, bucket)
			}
		}
		for bucket := range l7.windowLatency {
			if bucket < cutoff {
				delete(l7.windowLatency, bucket)
			}
		}
	}
}

func (l *l7State) add(obs L7Observation, bucket int64) {
	if l.activity == nil {
		l.activity = make(map[int64]uint64)
	}
	if l.windowStatus == nil {
		l.windowStatus = make(map[int64]statusCounts)
	}
	if l.windowLatency == nil {
		l.windowLatency = make(map[int64]*latencyHistogram)
	}
	if l.routes == nil {
		l.routes = make(map[string]*routeState)
	}

	if l.latencySource == "" {
		l.latencySource = obs.LatencySource
	} else if l.latencySource != obs.LatencySource {
		l.latencySource = "mixed"
	}
	class := statusClass(obs.StatusCode)
	l.requests++
	l.responses++
	l.status[class]++
	l.latency.add(obs.Duration)
	l.activity[bucket]++

	windowStatus := l.windowStatus[bucket]
	windowStatus[class]++
	l.windowStatus[bucket] = windowStatus

	windowLatency := l.windowLatency[bucket]
	if windowLatency == nil {
		windowLatency = &latencyHistogram{}
		l.windowLatency[bucket] = windowLatency
	}
	windowLatency.add(obs.Duration)

	if l.firstSeen.IsZero() || obs.Time.Before(l.firstSeen) {
		l.firstSeen = obs.Time
	}
	if obs.Time.After(l.lastSeen) {
		l.lastSeen = obs.Time
	}

	method := strings.ToUpper(strings.TrimSpace(obs.Method))
	path := normalizePath(obs.Path)
	key := method + " " + path
	route, exists := l.routes[key]
	if !exists && len(l.routes) >= maxL7Routes {
		key = "* (other)"
		route = l.routes[key]
		if route == nil {
			route = &routeState{method: "*", path: "(other)"}
			l.routes[key] = route
		}
	}
	if route == nil {
		route = &routeState{method: method, path: path}
		l.routes[key] = route
	}
	route.requests++
	route.responses++
	route.status[class]++
}

func (l *l7State) snapshot(now time.Time, bucketSeconds int64) *L7Stats {
	if l == nil || l.requests == 0 {
		return nil
	}
	winStatus := statusCounts{}
	var windowRequests uint64
	minBucket := now.Add(-time.Duration(snapshotBuckets)*time.Duration(bucketSeconds)*time.Second).Unix() / bucketSeconds
	for bucket, count := range l.activity {
		if bucket >= minBucket {
			windowRequests += count
		}
	}
	for bucket, counts := range l.windowStatus {
		if bucket >= minBucket {
			winStatus.merge(counts)
		}
	}
	windowLatency := latencyHistogram{}
	for bucket, histogram := range l.windowLatency {
		if bucket >= minBucket {
			windowLatency.merge(*histogram)
		}
	}

	routes := make([]L7Route, 0, len(l.routes))
	for _, route := range l.routes {
		routes = append(routes, L7Route{
			Method:    route.method,
			Path:      route.path,
			Requests:  route.requests,
			Responses: route.responses,
			Status:    route.status.public(),
		})
	}
	sort.Slice(routes, func(i, j int) bool {
		if routes[i].Requests != routes[j].Requests {
			return routes[i].Requests > routes[j].Requests
		}
		if routes[i].Path != routes[j].Path {
			return routes[i].Path < routes[j].Path
		}
		return routes[i].Method < routes[j].Method
	})
	if len(routes) > 5 {
		routes = routes[:5]
	}

	latencySource := l.latencySource
	if latencySource == "" {
		latencySource = "client"
	}
	return &L7Stats{
		Protocol:      "HTTP",
		LatencySource: latencySource,
		Requests:      l.requests,
		Responses:     l.responses,
		Status:        l.status.public(),
		LatencyMs:     percentiles(l.latency),
		Window: L7Window{
			Requests:  windowRequests,
			Responses: winStatus.total(),
			Status:    winStatus.public(),
			LatencyMs: percentiles(windowLatency),
			Activity:  l7Activity(l.activity, now, bucketSeconds),
		},
		Routes:    routes,
		FirstSeen: l.firstSeen.UTC(),
		LastSeen:  l.lastSeen.UTC(),
	}
}

func l7Activity(activity map[int64]uint64, now time.Time, bucketSeconds int64) []uint64 {
	return edgeActivity(activity, now, bucketSeconds)
}

func percentiles(histogram latencyHistogram) LatencyPercentiles {
	return LatencyPercentiles{
		P50: roundMS(histogram.percentile(0.50)),
		P95: roundMS(histogram.percentile(0.95)),
	}
}

func roundMS(value float64) float64 {
	return math.Round(value*100) / 100
}

func (h *latencyHistogram) add(duration time.Duration) {
	ns := float64(duration.Nanoseconds())
	if ns <= 0 {
		return
	}
	index := int(math.Log(ns/latencyMinNS) / math.Log(latencyMaxNS/latencyMinNS) * latencyBucketCount)
	if index < 0 {
		index = 0
	}
	if index >= latencyBucketCount {
		index = latencyBucketCount - 1
	}
	h.counts[index]++
}

func (h *latencyHistogram) merge(other latencyHistogram) {
	for i, count := range other.counts {
		h.counts[i] += count
	}
}

func (h latencyHistogram) percentile(fraction float64) float64 {
	total := h.total()
	if total == 0 {
		return 0
	}
	target := uint64(math.Ceil(float64(total) * fraction))
	var cumulative uint64
	for i, count := range h.counts {
		cumulative += count
		if cumulative >= target {
			return latencyBucketUpperMS(i)
		}
	}
	return latencyBucketUpperMS(latencyBucketCount - 1)
}

func (h latencyHistogram) total() uint64 {
	var total uint64
	for _, count := range h.counts {
		total += count
	}
	return total
}

func latencyBucketUpperMS(index int) float64 {
	ratio := math.Exp((float64(index+1) / latencyBucketCount) * math.Log(latencyMaxNS/latencyMinNS))
	return (latencyMinNS * ratio) / 1_000_000
}

func (s statusCounts) total() uint64 {
	var total uint64
	for _, count := range s {
		total += count
	}
	return total
}

func (s *statusCounts) merge(other statusCounts) {
	for i, count := range other {
		s[i] += count
	}
}

func (s statusCounts) public() L7StatusCounts {
	return L7StatusCounts{
		Success:     s[status2xx],
		Redirect:    s[status3xx],
		ClientError: s[status4xx],
		ServerError: s[status5xx],
		Other:       s[statusOther],
	}
}

func statusClass(code uint32) int {
	switch code / 100 {
	case 2:
		return status2xx
	case 3:
		return status3xx
	case 4:
		return status4xx
	case 5:
		return status5xx
	default:
		return statusOther
	}
}

var idSegment = regexp.MustCompile(`(?i)^(?:[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}|[0-9]+|p[0-9]+|[0-9a-f]{16,})$`)

func normalizePath(raw string) string {
	if raw == "" {
		return "/"
	}
	if parsed, err := url.Parse(raw); err == nil && parsed.Path != "" {
		raw = parsed.Path
	} else if index := strings.IndexByte(raw, '?'); index >= 0 {
		raw = raw[:index]
	}
	if !strings.HasPrefix(raw, "/") {
		raw = "/" + raw
	}
	segments := strings.Split(raw, "/")
	for i, segment := range segments {
		if idSegment.MatchString(segment) {
			segments[i] = ":id"
		}
	}
	normalized := strings.Join(segments, "/")
	if normalized == "" {
		return "/"
	}
	return normalized
}

// ResolveWorkloadByIP maps an observed endpoint address back to a watched
// workload. It is used for server spans, where the observer names the server
// but only exposes the client address.
func (s *Store) ResolveWorkloadByIP(ip string) (string, bool) {
	if ip == "" {
		return "", false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, node := range s.nodes {
		if node.id == "source-ip" || node.workload == "" {
			continue
		}
		for candidate := range node.ips {
			if candidate == ip {
				return node.workload, true
			}
		}
	}
	return "", false
}
