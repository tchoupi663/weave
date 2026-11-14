package live

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/tchoupi663/weave/services/weave/internal/contracts"
	"github.com/tchoupi663/weave/services/weave/internal/topology"
)

const defaultFeedTimeout = 2 * time.Second

// FeedStatus is the delivery state the live health endpoint reports.
type FeedStatus struct {
	Configured          bool
	HistoryConnected    bool
	GapActive           bool
	RecordingGaps       int
	DroppedObservations uint64
	Buffered            int
	LastError           string
}

// Feed implements topology.ObservationRecorder. It buffers accepted
// observations and delivers them to weave-history at most once. A failed
// delivery is dropped and later reported as a recording gap.
type Feed struct {
	url      string
	interval time.Duration
	maxBatch int
	timeout  time.Duration
	client   *http.Client
	log      *slog.Logger

	epoch string

	mu          sync.Mutex
	seq         uint64
	l4          []contracts.L4Observation
	l7          []contracts.L7Observation
	gaps        []contracts.Gap
	gapStart    time.Time
	gapActive   bool
	connected   bool
	lastError   string
	dropped     uint64
	recordGaps  int
	lastSuccess time.Time

	signal chan struct{}
	stop   chan struct{}
	once   sync.Once
}

// NewFeed builds the publisher. An empty url disables the feed.
func NewFeed(url string, interval time.Duration, maxBatch int, log *slog.Logger) *Feed {
	if interval <= 0 {
		interval = time.Second
	}
	if maxBatch <= 0 {
		maxBatch = contracts.MaxObservationsPerBatch
	}
	return &Feed{
		url:      url,
		interval: interval,
		maxBatch: maxBatch,
		timeout:  defaultFeedTimeout,
		client:   &http.Client{Timeout: defaultFeedTimeout},
		log:      log,
		epoch:    time.Now().UTC().Format(time.RFC3339Nano),
		signal:   make(chan struct{}, 1),
		stop:     make(chan struct{}),
	}
}

// Enabled reports whether a history endpoint is configured.
func (f *Feed) Enabled() bool { return f.url != "" }

// ObserveL4 implements topology.ObservationRecorder. It never blocks on the
// network and never acquires the history database.
func (f *Feed) ObserveL4(source, target, protocol string, port uint32, connection bool, at time.Time) {
	f.mu.Lock()
	f.l4 = append(f.l4, contracts.L4Observation{
		Source:     source,
		Target:     target,
		Protocol:   protocol,
		Port:       port,
		Connection: connection,
		Time:       at.UTC(),
	})
	full := len(f.l4)+len(f.l7) >= f.maxBatch
	f.mu.Unlock()
	if full {
		f.signalFlush()
	}
}

// ObserveL7 implements topology.ObservationRecorder.
func (f *Feed) ObserveL7(source, target string, port uint32, observation topology.L7Observation) {
	f.mu.Lock()
	f.l7 = append(f.l7, contracts.L7Observation{
		Source:        source,
		Target:        target,
		Port:          port,
		Method:        observation.Method,
		Path:          observation.Path,
		Status:        observation.StatusCode,
		LatencySource: observation.LatencySource,
		DurationMS:    float64(observation.Duration.Microseconds()) / 1000.0,
		Time:          observation.Time.UTC(),
	})
	full := len(f.l4)+len(f.l7) >= f.maxBatch
	f.mu.Unlock()
	if full {
		f.signalFlush()
	}
}

func (f *Feed) signalFlush() {
	select {
	case f.signal <- struct{}{}:
	default:
	}
}

// Run delivers batches every interval, on size threshold, and on shutdown.
func (f *Feed) Run(ctx context.Context) {
	ticker := time.NewTicker(f.interval)
	defer ticker.Stop()
	defer f.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-f.stop:
			return
		case <-ticker.C:
		case <-f.signal:
		}
		if err := f.flush(ctx); err != nil {
			f.log.Warn("observation feed delivery failed", "error", err)
		}
	}
}

// Stop flushes once with a short deadline and stops future runs.
func (f *Feed) Stop() {
	f.once.Do(func() {
		close(f.stop)
		ctx, cancel := context.WithTimeout(context.Background(), f.timeout)
		defer cancel()
		if err := f.flush(ctx); err != nil {
			f.log.Warn("final observation flush failed", "error", err)
		}
	})
}

func (f *Feed) flush(ctx context.Context) error {
	f.mu.Lock()
	hasData := len(f.l4) > 0 || len(f.l7) > 0
	hasGaps := len(f.gaps) > 0
	if !hasData && !hasGaps {
		f.mu.Unlock()
		return nil
	}
	// Keep a batch within the contract limit. Extra observations stay buffered
	// and are flushed on the next tick instead of being rejected with 413.
	takeL4 := len(f.l4)
	if takeL4 > f.maxBatch {
		takeL4 = f.maxBatch
	}
	remaining := f.maxBatch - takeL4
	takeL7 := len(f.l7)
	if takeL7 > remaining {
		takeL7 = remaining
	}
	batch := contracts.ObservationBatch{
		Epoch: f.epoch,
		Seq:   f.seq + 1,
		L4:    f.l4[:takeL4],
		L7:    f.l7[:takeL7],
		Gaps:  f.gaps,
	}
	f.seq++
	f.l4 = f.l4[takeL4:]
	f.l7 = f.l7[takeL7:]
	f.gaps = nil
	count := len(batch.L4) + len(batch.L7)
	hasMoreData := len(f.l4) > 0 || len(f.l7) > 0
	f.mu.Unlock()

	payload, err := json.Marshal(batch)
	if err != nil {
		return err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, f.url+"/internal/v1/observations", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	request.Header.Set("Content-Type", "application/json")

	response, err := f.client.Do(request)
	now := time.Now().UTC()
	if err != nil {
		f.recordFailure(count, now, err)
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		f.recordFailure(count, now, fmt.Errorf("history returned %s", response.Status))
		return fmt.Errorf("history returned %s", response.Status)
	}

	completedGap := false
	f.mu.Lock()
	f.connected = true
	f.lastSuccess = now
	if f.gapActive {
		f.gaps = append(f.gaps, contracts.Gap{From: f.gapStart, To: now})
		f.gapActive = false
		f.recordGaps++
		completedGap = true
	}
	f.mu.Unlock()
	if completedGap || hasMoreData {
		// Deliver the completed gap, or drain the remainder, promptly.
		f.signalFlush()
	}
	return nil
}

func (f *Feed) recordFailure(count int, now time.Time, err error) {
	f.mu.Lock()
	if !f.gapActive {
		f.gapActive = true
		f.gapStart = now
	}
	f.connected = false
	f.lastError = err.Error()
	f.dropped += uint64(count)
	f.mu.Unlock()
}

// Status returns a consistent snapshot for the health endpoint.
func (f *Feed) Status() FeedStatus {
	f.mu.Lock()
	defer f.mu.Unlock()
	buffered := len(f.l4) + len(f.l7)
	return FeedStatus{
		Configured:          f.Enabled(),
		HistoryConnected:    !f.Enabled() || f.connected,
		GapActive:           f.gapActive,
		RecordingGaps:       f.recordGaps,
		DroppedObservations: f.dropped,
		Buffered:            buffered,
		LastError:           f.lastError,
	}
}
