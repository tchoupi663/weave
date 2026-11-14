package history

import (
	"math"
	"strconv"
	"time"

	"github.com/tchoupi663/weave/services/weave/internal/topology"
)

const (
	bucket5Duration = 5 * time.Second
	bucket1Duration = time.Minute
	latencyBuckets  = 64
	latencyMinNS    = 100_000.0
	latencyMaxNS    = 10_000_000_000.0
)

type statusCounts [5]uint64

const (
	status2xx = iota
	status3xx
	status4xx
	status5xx
	statusOther
)

type latencyHistogram struct {
	counts [latencyBuckets]uint64
}

type metricKey struct {
	start  int64
	edgeID string
}

type routeKey struct {
	start  int64
	edgeID string
	method string
	path   string
}

type metricBucket struct {
	start         time.Time
	edgeID        string
	source        string
	target        string
	protocol      string
	port          uint32
	connections   uint64
	events        uint64
	requests      uint64
	responses     uint64
	status        statusCounts
	latency       latencyHistogram
	latencySource string
}

type routeBucket struct {
	start     time.Time
	edgeID    string
	method    string
	path      string
	requests  uint64
	responses uint64
	status    statusCounts
}

func edgeID(namespace, source, target string, port uint32) string {
	return "workload:" + namespace + "/" + source + "->workload:" + namespace + "/" + target + ":" + strconv.FormatUint(uint64(port), 10)
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

func (h *latencyHistogram) add(duration time.Duration) {
	ns := float64(duration.Nanoseconds())
	if ns <= 0 {
		return
	}
	index := int(math.Log(ns/latencyMinNS) / math.Log(latencyMaxNS/latencyMinNS) * latencyBuckets)
	if index < 0 {
		index = 0
	}
	if index >= latencyBuckets {
		index = latencyBuckets - 1
	}
	h.counts[index]++
}

func (h *latencyHistogram) percentile(fraction float64) float64 {
	var total uint64
	for _, count := range h.counts {
		total += count
	}
	if total == 0 {
		return 0
	}
	target := uint64(math.Ceil(float64(total) * fraction))
	var cumulative uint64
	for i, count := range h.counts {
		cumulative += count
		if cumulative >= target {
			ratio := math.Exp((float64(i+1) / latencyBuckets) * math.Log(latencyMaxNS/latencyMinNS))
			return math.Round((latencyMinNS*ratio/1_000_000)*100) / 100
		}
	}
	return 0
}

func (b *metricBucket) addL4(connection bool) {
	b.events++
	if connection {
		b.connections++
	}
}

func (b *metricBucket) addL7(observation topology.L7Observation) {
	class := statusClass(observation.StatusCode)
	b.requests++
	b.responses++
	b.status[class]++
	if observation.Duration > 0 {
		b.latency.add(observation.Duration)
	}
	if b.latencySource == "" {
		b.latencySource = observation.LatencySource
	} else if b.latencySource != observation.LatencySource {
		b.latencySource = "mixed"
	}
}
