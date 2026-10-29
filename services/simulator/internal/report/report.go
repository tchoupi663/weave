package report

import (
	"math"
	"sort"
	"sync"
	"time"

	"github.com/tchoupi663/weave/services/simulator/internal/config"
	"github.com/tchoupi663/weave/services/simulator/internal/topology"
)

var journeyNames = []string{"bounce", "browse", "order"}

// JourneyStats counts one Journey.
type JourneyStats struct {
	Started   int `json:"started"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
}

// StepStats aggregates one step.
type StepStats struct {
	Count  int     `json:"count"`
	Errors int     `json:"errors"`
	P50MS  float64 `json:"p50_ms"`
	P95MS  float64 `json:"p95_ms"`
	Bytes  int64   `json:"bytes"`
}

// Failure is one failed Journey.
type Failure struct {
	User    int64     `json:"user"`
	Journey string    `json:"journey"`
	Step    string    `json:"step"`
	Error   string    `json:"error"`
	At      time.Time `json:"at"`
}

// Request is one optional per-request record.
type Request struct {
	User    int64   `json:"user"`
	Journey string  `json:"journey"`
	Step    string  `json:"step"`
	Method  string  `json:"method"`
	URL     string  `json:"url"`
	Status  int     `json:"status"`
	MS      float64 `json:"ms"`
	Bytes   int64   `json:"bytes"`
}

// Report is the Simulator's run artifact.
type Report struct {
	RunID            string                  `json:"run_id"`
	Seed             int64                   `json:"seed"`
	Config           config.Config           `json:"config"`
	StartedAt        time.Time               `json:"started_at"`
	EndedAt          time.Time               `json:"ended_at"`
	SourceIP         string                  `json:"source_ip"`
	Journeys         map[string]JourneyStats `json:"journeys"`
	Steps            map[string]StepStats    `json:"steps"`
	Failures         []Failure               `json:"failures"`
	ExpectedTopology topology.Topology       `json:"expected_topology"`
	Requests         []Request               `json:"requests,omitempty"`
}

type stepAccumulator struct {
	count     int
	errors    int
	bytes     int64
	durations []float64
}

// Aggregator collects thread-safe run metrics.
type Aggregator struct {
	mu          sync.Mutex
	logRequests bool
	journeys    map[string]*JourneyStats
	steps       map[string]*stepAccumulator
	failures    []Failure
	requests    []Request
}

// NewAggregator returns an Aggregator with all journey counters initialized.
func NewAggregator(logRequests bool) *Aggregator {
	journeys := make(map[string]*JourneyStats, len(journeyNames))
	for _, name := range journeyNames {
		journeys[name] = &JourneyStats{}
	}
	return &Aggregator{
		logRequests: logRequests,
		journeys:    journeys,
		steps:       make(map[string]*stepAccumulator),
	}
}

func (a *Aggregator) StartJourney(name string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.journeys[name].Started++
}

func (a *Aggregator) CompleteJourney(name string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.journeys[name].Completed++
}

func (a *Aggregator) FailJourney(name string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.journeys[name].Failed++
}

func (a *Aggregator) RecordStep(name string, status int, ms float64, bytes int64, failed bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	acc := a.steps[name]
	if acc == nil {
		acc = &stepAccumulator{}
		a.steps[name] = acc
	}
	acc.count++
	if failed {
		acc.errors++
	}
	acc.bytes += bytes
	if ms >= 0 {
		acc.durations = append(acc.durations, ms)
	}
}

func (a *Aggregator) RecordFailure(user int64, journey, step, message string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.failures = append(a.failures, Failure{
		User:    user,
		Journey: journey,
		Step:    step,
		Error:   message,
		At:      time.Now().UTC(),
	})
}

func (a *Aggregator) RecordRequest(req Request) {
	if !a.logRequests {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	a.requests = append(a.requests, req)
}

// Build freezes the metrics into a Report.
func (a *Aggregator) Build(
	runID string,
	seed int64,
	cfg config.Config,
	startedAt, endedAt time.Time,
	sourceIP string,
	expected topology.Topology,
) Report {
	a.mu.Lock()
	defer a.mu.Unlock()

	journeys := make(map[string]JourneyStats, len(a.journeys))
	for name, stats := range a.journeys {
		journeys[name] = *stats
	}
	steps := make(map[string]StepStats, len(a.steps))
	for name, acc := range a.steps {
		sorted := append([]float64(nil), acc.durations...)
		sort.Float64s(sorted)
		steps[name] = StepStats{
			Count:  acc.count,
			Errors: acc.errors,
			P50MS:  percentile(sorted, 0.50),
			P95MS:  percentile(sorted, 0.95),
			Bytes:  acc.bytes,
		}
	}
	failures := append([]Failure{}, a.failures...)
	requests := append([]Request{}, a.requests...)

	return Report{
		RunID:            runID,
		Seed:             seed,
		Config:           cfg,
		StartedAt:        startedAt,
		EndedAt:          endedAt,
		SourceIP:         sourceIP,
		Journeys:         journeys,
		Steps:            steps,
		Failures:         failures,
		ExpectedTopology: expected,
		Requests:         requests,
	}
}

func percentile(sorted []float64, p float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(math.Ceil(p*float64(len(sorted)))) - 1
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return math.Round(sorted[idx]*100) / 100
}
