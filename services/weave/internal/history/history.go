package history

import (
	"context"
	"database/sql"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/duckdb/duckdb-go/v2"
	"github.com/tchoupi663/weave/services/weave/internal/contracts"
	"github.com/tchoupi663/weave/services/weave/internal/topology"
	_ "modernc.org/sqlite"
)

// History owns the durable row store and the analytical metric store. It no
// longer owns lifecycle or change events: those belong to weave-inventory.
type History struct {
	namespace string
	sqlite    *sql.DB
	duck      *sql.DB
	log       *slog.Logger
	grace     time.Duration

	mu               sync.Mutex
	pending5         map[metricKey]*metricBucket
	pending1         map[metricKey]*metricBucket
	pendingRoutes    map[routeKey]*routeBucket
	lastSeq          map[string]uint64
	lateObservations uint64
	flushedThrough   time.Time
	flushes          int
}

// Open creates or opens both stores under dataDir. grace is the flush grace
// that keeps late observations from overwriting an already-persisted bucket.
func Open(dataDir, namespace string, grace time.Duration, log *slog.Logger) (*History, error) {
	if strings.TrimSpace(dataDir) == "" {
		return nil, fmt.Errorf("data directory is empty")
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	if grace < 0 {
		return nil, fmt.Errorf("flush grace must not be negative")
	}

	sqliteDB, err := sql.Open("sqlite", filepath.Join(dataDir, "weave.db"))
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	sqliteDB.SetMaxOpenConns(1)
	if _, err := sqliteDB.Exec(`PRAGMA journal_mode=WAL;`); err != nil {
		sqliteDB.Close()
		return nil, fmt.Errorf("sqlite wal: %w", err)
	}
	if _, err := sqliteDB.Exec(`PRAGMA busy_timeout=5000;`); err != nil {
		sqliteDB.Close()
		return nil, fmt.Errorf("sqlite busy timeout: %w", err)
	}
	if err := execSchema(sqliteDB, sqliteSchema); err != nil {
		sqliteDB.Close()
		return nil, fmt.Errorf("sqlite schema: %w", err)
	}

	duckDB, err := sql.Open("duckdb", filepath.Join(dataDir, "metrics.duckdb"))
	if err != nil {
		sqliteDB.Close()
		return nil, fmt.Errorf("open duckdb: %w", err)
	}
	duckDB.SetMaxOpenConns(1)
	if err := execSchema(duckDB, duckDBEdgeSchema); err != nil {
		sqliteDB.Close()
		duckDB.Close()
		return nil, fmt.Errorf("duckdb schema: %w", err)
	}

	if log == nil {
		log = slog.New(slog.NewTextHandler(os.Stderr, nil))
	}
	h := &History{
		namespace:     namespace,
		sqlite:        sqliteDB,
		duck:          duckDB,
		log:           log,
		grace:         grace,
		pending5:      make(map[metricKey]*metricBucket),
		pending1:      make(map[metricKey]*metricBucket),
		pendingRoutes: make(map[routeKey]*routeBucket),
		lastSeq:       make(map[string]uint64),
	}
	return h, nil
}

func execSchema(db *sql.DB, schema string) error {
	for _, statement := range strings.Split(schema, ";") {
		statement = strings.TrimSpace(statement)
		if statement == "" {
			continue
		}
		if _, err := db.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}

// Start launches flushing and retention.
func (h *History) Start(ctx context.Context) {
	go h.flushLoop(ctx)
	go h.retentionLoop(ctx)
}

func (h *History) flushLoop(ctx context.Context) {
	ticker := time.NewTicker(bucket5Duration)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			if err := h.flushOnce(now); err != nil {
				h.log.Error("history flush failed", "error", err)
			}
		}
	}
}

func (h *History) retentionLoop(ctx context.Context) {
	h.prune(time.Now().UTC())
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			h.prune(now)
		}
	}
}

// RecordBatch applies one observation batch from weave-live. Batches from the
// same epoch with a non-increasing sequence are ignored. It returns the number
// of observations applied.
func (h *History) RecordBatch(ctx context.Context, batch contracts.ObservationBatch) (int, error) {
	applied := 0
	h.mu.Lock()
	if batch.Epoch != "" {
		if batch.Seq <= h.lastSeq[batch.Epoch] {
			h.mu.Unlock()
			return 0, nil
		}
		h.lastSeq[batch.Epoch] = batch.Seq
	}
	for _, observation := range batch.L4 {
		if h.recordL4Locked(observation.Source, observation.Target, observation.Protocol, observation.Port, observation.Connection, observation.Time) {
			applied++
		}
	}
	for _, observation := range batch.L7 {
		h.recordL7Locked(observation.Source, observation.Target, observation.Port, topology.L7Observation{
			SourceWorkload: observation.Source,
			TargetWorkload: observation.Target,
			Port:           observation.Port,
			Method:         observation.Method,
			Path:           observation.Path,
			StatusCode:     observation.Status,
			LatencySource:  observation.LatencySource,
			Duration:       time.Duration(observation.DurationMS * float64(time.Millisecond)),
			Time:           observation.Time,
		})
		applied++
	}
	h.mu.Unlock()

	for _, gap := range batch.Gaps {
		if !gap.To.After(gap.From) {
			continue
		}
		if err := h.insertGap(ctx, gap.From, gap.To); err != nil {
			return applied, err
		}
	}
	return applied, nil
}

// ObserveL4 is called after an accepted L4 observation. It is kept for tests
// and for a possible in-process path; the production path is RecordBatch.
func (h *History) ObserveL4(source, target, protocol string, port uint32, connection bool, at time.Time) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.recordL4Locked(source, target, protocol, port, connection, at)
}

// ObserveL7 is called after an accepted L7 observation.
func (h *History) ObserveL7(source, target string, port uint32, observation topology.L7Observation) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.recordL7Locked(source, target, port, observation)
}

func (h *History) recordL4Locked(source, target, protocol string, port uint32, connection bool, at time.Time) bool {
	if source == "" || target == "" || source == target || port == 0 {
		return false
	}
	at = at.UTC()
	if h.tooLateLocked(at) {
		return false
	}
	id := edgeID(h.namespace, source, target, port)
	h.bucket5(id, source, target, protocol, port, at).addL4(connection)
	h.bucket1(id, source, target, protocol, port, at).addL4(connection)
	return true
}

func (h *History) recordL7Locked(source, target string, port uint32, observation topology.L7Observation) {
	if source == "" || target == "" || source == target || port == 0 {
		return
	}
	at := observation.Time.UTC()
	if h.tooLateLocked(at) {
		return
	}
	id := edgeID(h.namespace, source, target, port)
	method := strings.ToUpper(strings.TrimSpace(observation.Method))
	path := topology.NormalizePath(observation.Path)
	h.bucket5(id, source, target, "HTTP", port, at).addL7(observation)
	h.bucket1(id, source, target, "HTTP", port, at).addL7(observation)
	key := routeKey{start: at.Truncate(bucket5Duration).UnixNano(), edgeID: id, method: method, path: path}
	route := h.pendingRoutes[key]
	if route == nil {
		route = &routeBucket{start: at.Truncate(bucket5Duration), edgeID: id, method: method, path: path}
		h.pendingRoutes[key] = route
	}
	route.requests++
	route.responses++
	route.status[statusClass(observation.StatusCode)]++
}

// tooLateLocked reports whether an observation belongs to a bucket that has
// already been flushed (or is within the unflushable past). Such observations
// would reopen a persisted bucket and overwrite it with partial counts.
func (h *History) tooLateLocked(at time.Time) bool {
	if h.flushedThrough.IsZero() {
		return false
	}
	bucketEnd := at.Truncate(bucket5Duration).Add(bucket5Duration)
	if !bucketEnd.After(h.flushedThrough) {
		h.lateObservations++
		return true
	}
	return false
}

func (h *History) bucket5(id, source, target, protocol string, port uint32, at time.Time) *metricBucket {
	start := at.Truncate(bucket5Duration)
	key := metricKey{start: start.UnixNano(), edgeID: id}
	bucket := h.pending5[key]
	if bucket == nil {
		bucket = &metricBucket{start: start, edgeID: id, source: source, target: target, protocol: protocol, port: port}
		h.pending5[key] = bucket
	}
	return bucket
}

func (h *History) bucket1(id, source, target, protocol string, port uint32, at time.Time) *metricBucket {
	start := at.Truncate(bucket1Duration)
	key := metricKey{start: start.UnixNano(), edgeID: id}
	bucket := h.pending1[key]
	if bucket == nil {
		bucket = &metricBucket{start: start, edgeID: id, source: source, target: target, protocol: protocol, port: port}
		h.pending1[key] = bucket
	}
	return bucket
}

func (h *History) flushOnce(now time.Time) error {
	now = now.UTC()
	h.mu.Lock()
	cutoff := now.Add(-h.grace)
	edges5 := make([]*metricBucket, 0)
	for key, bucket := range h.pending5 {
		if !now.Before(bucket.start.Add(bucket5Duration + h.grace)) {
			edges5 = append(edges5, bucket)
			delete(h.pending5, key)
		}
	}
	edges1 := make([]*metricBucket, 0)
	for key, bucket := range h.pending1 {
		if !now.Before(bucket.start.Add(bucket1Duration + h.grace)) {
			edges1 = append(edges1, bucket)
			delete(h.pending1, key)
		}
	}
	routes := make([]*routeBucket, 0)
	for key, bucket := range h.pendingRoutes {
		if !now.Before(bucket.start.Add(bucket5Duration + h.grace)) {
			routes = append(routes, bucket)
			delete(h.pendingRoutes, key)
		}
	}
	if cutoff.After(h.flushedThrough) {
		h.flushedThrough = cutoff
	}
	h.mu.Unlock()

	if len(edges5)+len(edges1)+len(routes) == 0 {
		return nil
	}

	tx, err := h.duck.Begin()
	if err != nil {
		return err
	}
	const edgeInsert = `INSERT OR REPLACE INTO edge_metrics_5s
(bucket_start, edge_id, source, target, protocol, port, connections, events, requests, responses,
 status_2xx, status_3xx, status_4xx, status_5xx, status_other, p50_ms, p95_ms, latency_source)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	const edge1Insert = `INSERT OR REPLACE INTO edge_metrics_1m
(bucket_start, edge_id, source, target, protocol, port, connections, events, requests, responses,
 status_2xx, status_3xx, status_4xx, status_5xx, status_other, p50_ms, p95_ms, latency_source)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	const routeInsert = `INSERT OR REPLACE INTO route_metrics_5s
(bucket_start, edge_id, method, path, requests, responses,
 status_2xx, status_3xx, status_4xx, status_5xx, status_other)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`

	writeEdge := func(table string, bucket *metricBucket) error {
		p50, p95 := bucket.latency.percentile(0.50), bucket.latency.percentile(0.95)
		source := bucket.latencySource
		if source == "" {
			source = "none"
		}
		var err error
		switch table {
		case "5s":
			_, err = tx.Exec(edgeInsert, bucket.start, bucket.edgeID, bucket.source, bucket.target, bucket.protocol, bucket.port,
				bucket.connections, bucket.events, bucket.requests, bucket.responses,
				bucket.status[status2xx], bucket.status[status3xx], bucket.status[status4xx], bucket.status[status5xx], bucket.status[statusOther],
				p50, p95, source)
		case "1m":
			_, err = tx.Exec(edge1Insert, bucket.start, bucket.edgeID, bucket.source, bucket.target, bucket.protocol, bucket.port,
				bucket.connections, bucket.events, bucket.requests, bucket.responses,
				bucket.status[status2xx], bucket.status[status3xx], bucket.status[status4xx], bucket.status[status5xx], bucket.status[statusOther],
				p50, p95, source)
		}
		return err
	}

	for _, bucket := range edges5 {
		if err := writeEdge("5s", bucket); err != nil {
			tx.Rollback()
			return err
		}
	}
	for _, bucket := range edges1 {
		if err := writeEdge("1m", bucket); err != nil {
			tx.Rollback()
			return err
		}
	}
	for _, route := range routes {
		if _, err := tx.Exec(routeInsert, route.start, route.edgeID, route.method, route.path, route.requests, route.responses,
			route.status[status2xx], route.status[status3xx], route.status[status4xx], route.status[status5xx], route.status[statusOther]); err != nil {
			tx.Rollback()
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	h.flushes++
	if h.flushes%12 == 0 {
		h.log.Info("history stored", "edges_5s", len(edges5), "edges_1m", len(edges1), "routes", len(routes))
	}
	return nil
}

func (h *History) prune(now time.Time) {
	now = now.UTC()
	if _, err := h.duck.Exec(`DELETE FROM edge_metrics_5s WHERE bucket_start < ?`, now.Add(-24*time.Hour)); err != nil {
		h.log.Error("prune 5s metrics", "error", err)
	}
	if _, err := h.duck.Exec(`DELETE FROM route_metrics_5s WHERE bucket_start < ?`, now.Add(-24*time.Hour)); err != nil {
		h.log.Error("prune route metrics", "error", err)
	}
	if _, err := h.duck.Exec(`DELETE FROM edge_metrics_1m WHERE bucket_start < ?`, now.Add(-7*24*time.Hour)); err != nil {
		h.log.Error("prune 1m metrics", "error", err)
	}
	if _, err := h.sqlite.Exec(`DELETE FROM recording_gaps WHERE to_ts < ?`, now.Add(-7*24*time.Hour).Format(time.RFC3339Nano)); err != nil {
		h.log.Error("prune recording gaps", "error", err)
	}

	// DuckDB does not reclaim file space on DELETE. CHECKPOINT flushes the WAL;
	// physical compaction remains a later maintenance task.
	if _, err := h.duck.Exec(`CHECKPOINT`); err != nil {
		h.log.Debug("duckdb checkpoint", "error", err)
	}
	if _, err := h.sqlite.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		h.log.Debug("sqlite wal checkpoint", "error", err)
	}
}

// Close flushes pending buckets and closes both databases.
func (h *History) Close() error {
	_ = h.flushOnce(time.Now().Add(24 * time.Hour))
	var err error
	if h.duck != nil {
		if closeErr := h.duck.Close(); closeErr != nil {
			err = closeErr
		}
	}
	if h.sqlite != nil {
		if closeErr := h.sqlite.Close(); closeErr != nil && err == nil {
			err = closeErr
		}
	}
	return err
}

// Counts reports persisted metric counts for tests and diagnostics.
func (h *History) Counts(ctx context.Context) (metrics5, metrics1 int64, err error) {
	if err = h.duck.QueryRowContext(ctx, `SELECT count(*) FROM edge_metrics_5s`).Scan(&metrics5); err != nil {
		return
	}
	err = h.duck.QueryRowContext(ctx, `SELECT count(*) FROM edge_metrics_1m`).Scan(&metrics1)
	return
}

// LateObservations is the number of observations dropped because their bucket
// was already flushed.
func (h *History) LateObservations() uint64 {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.lateObservations
}

// Gap is one persisted recording gap.
type Gap struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

func (h *History) insertGap(ctx context.Context, from, to time.Time) error {
	from, to = from.UTC(), to.UTC()
	var overlaps int
	if err := h.sqlite.QueryRowContext(ctx,
		`SELECT count(*) FROM recording_gaps WHERE from_ts <= ? AND to_ts >= ?`,
		to.Format(time.RFC3339Nano), from.Format(time.RFC3339Nano)).Scan(&overlaps); err != nil {
		return err
	}
	if overlaps > 0 {
		return nil
	}
	_, err := h.sqlite.ExecContext(ctx,
		`INSERT INTO recording_gaps (from_ts, to_ts) VALUES (?, ?)`,
		from.Format(time.RFC3339Nano), to.Format(time.RFC3339Nano))
	return err
}

// Gaps returns recording gaps overlapping the interval, ordered by start.
func (h *History) Gaps(ctx context.Context, from, to time.Time) ([]Gap, error) {
	rows, err := h.sqlite.QueryContext(ctx,
		`SELECT from_ts, to_ts FROM recording_gaps WHERE to_ts >= ? AND from_ts <= ? ORDER BY from_ts`,
		from.UTC().Format(time.RFC3339Nano), to.UTC().Format(time.RFC3339Nano))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]Gap, 0)
	for rows.Next() {
		var fromRaw, toRaw string
		if err := rows.Scan(&fromRaw, &toRaw); err != nil {
			return nil, err
		}
		fromAt, err := time.Parse(time.RFC3339Nano, fromRaw)
		if err != nil {
			return nil, err
		}
		toAt, err := time.Parse(time.RFC3339Nano, toRaw)
		if err != nil {
			return nil, err
		}
		result = append(result, Gap{From: fromAt.UTC(), To: toAt.UTC()})
	}
	return result, rows.Err()
}
