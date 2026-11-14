package history

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	_ "github.com/duckdb/duckdb-go/v2"
	"github.com/tchoupi663/weave/services/weave/internal/topology"
	_ "modernc.org/sqlite"
)

// History owns the durable row store and the analytical metric store.
type History struct {
	namespace string
	sqlite    *sql.DB
	duck      *sql.DB
	argo      *ArgoClient
	store     *topology.Store
	log       *slog.Logger

	mu            sync.Mutex
	pending5      map[metricKey]*metricBucket
	pending1      map[metricKey]*metricBucket
	pendingRoutes map[routeKey]*routeBucket
	states        map[string]string
	desired       map[string]string
	live          map[string]bool
	lifecycleInit bool

	flushes int
}

// Open creates or opens both stores under dataDir.
func Open(dataDir, namespace string, store *topology.Store, argo *ArgoClient, log *slog.Logger) (*History, error) {
	if strings.TrimSpace(dataDir) == "" {
		return nil, fmt.Errorf("data directory is empty")
	}
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
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
		argo:          argo,
		store:         store,
		log:           log,
		pending5:      make(map[metricKey]*metricBucket),
		pending1:      make(map[metricKey]*metricBucket),
		pendingRoutes: make(map[routeKey]*routeBucket),
		states:        make(map[string]string),
		desired:       make(map[string]string),
		live:          make(map[string]bool),
	}
	if err := h.loadStates(); err != nil {
		h.Close()
		return nil, err
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

func (h *History) loadStates() error {
	rows, err := h.sqlite.Query(`SELECT workload, state FROM workload_states`)
	if err != nil {
		return fmt.Errorf("load workload states: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var workload, state string
		if err := rows.Scan(&workload, &state); err != nil {
			return err
		}
		h.states[workload] = state
	}
	return rows.Err()
}

// Start launches flushing, lifecycle reconciliation, and retention.
func (h *History) Start(ctx context.Context) {
	go h.flushLoop(ctx)
	go h.lifecycleLoop(ctx)
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

// ObserveL4 is called after an accepted L4 observation.
func (h *History) ObserveL4(source, target, protocol string, port uint32, connection bool, at time.Time) {
	if source == "" || target == "" || source == target || port == 0 {
		return
	}
	at = at.UTC()
	id := edgeID(h.namespace, source, target, port)
	h.mu.Lock()
	defer h.mu.Unlock()
	h.bucket5(id, source, target, protocol, port, at).addL4(connection)
	h.bucket1(id, source, target, protocol, port, at).addL4(connection)
}

// ObserveL7 is called after an accepted L7 observation.
func (h *History) ObserveL7(source, target string, port uint32, observation topology.L7Observation) {
	if source == "" || target == "" || source == target || port == 0 {
		return
	}
	at := observation.Time.UTC()
	id := edgeID(h.namespace, source, target, port)
	method := strings.ToUpper(strings.TrimSpace(observation.Method))
	path := topology.NormalizePath(observation.Path)
	h.mu.Lock()
	defer h.mu.Unlock()
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
	edges5 := make([]*metricBucket, 0)
	for key, bucket := range h.pending5 {
		if !now.Before(bucket.start.Add(bucket5Duration)) {
			edges5 = append(edges5, bucket)
			delete(h.pending5, key)
		}
	}
	edges1 := make([]*metricBucket, 0)
	for key, bucket := range h.pending1 {
		if !now.Before(bucket.start.Add(bucket1Duration)) {
			edges1 = append(edges1, bucket)
			delete(h.pending1, key)
		}
	}
	routes := make([]*routeBucket, 0)
	for key, bucket := range h.pendingRoutes {
		if !now.Before(bucket.start.Add(bucket5Duration)) {
			routes = append(routes, bucket)
			delete(h.pendingRoutes, key)
		}
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
	if _, err := h.sqlite.Exec(`DELETE FROM change_events WHERE occurred_at < ?`, now.Add(-30*24*time.Hour).Format(time.RFC3339Nano)); err != nil {
		h.log.Error("prune change events", "error", err)
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

func (h *History) recordChange(ctx context.Context, occurred time.Time, source, eventType, workload, kind string, details any) {
	data, _ := json.Marshal(details)
	if _, err := h.sqlite.ExecContext(ctx,
		`INSERT INTO change_events (occurred_at, source, type, workload, kind, details) VALUES (?, ?, ?, ?, ?, ?)`,
		occurred.UTC().Format(time.RFC3339Nano), source, eventType, workload, kind, string(data)); err != nil {
		h.log.Error("record change", "error", err, "workload", workload, "type", eventType)
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

// Counts reports persisted metric/change counts for tests and diagnostics.
func (h *History) Counts(ctx context.Context) (metrics5, metrics1, changes int64, err error) {
	if err = h.duck.QueryRowContext(ctx, `SELECT count(*) FROM edge_metrics_5s`).Scan(&metrics5); err != nil {
		return
	}
	if err = h.duck.QueryRowContext(ctx, `SELECT count(*) FROM edge_metrics_1m`).Scan(&metrics1); err != nil {
		return
	}
	err = h.sqlite.QueryRowContext(ctx, `SELECT count(*) FROM change_events`).Scan(&changes)
	return
}

// ChangeEvent is one persisted lifecycle transition.
type ChangeEvent struct {
	ID         int64           `json:"id"`
	OccurredAt time.Time       `json:"occurred_at"`
	Source     string          `json:"source"`
	Type       string          `json:"type"`
	Workload   string          `json:"workload"`
	Kind       string          `json:"kind"`
	Details    json.RawMessage `json:"details"`
}

func (h *History) Changes(ctx context.Context, from, to time.Time, limit int) ([]ChangeEvent, error) {
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	rows, err := h.sqlite.QueryContext(ctx, `
SELECT id, occurred_at, source, type, workload, kind, details
FROM change_events
WHERE occurred_at >= ? AND occurred_at < ?
ORDER BY occurred_at DESC, id DESC
LIMIT ?`, from.UTC().Format(time.RFC3339Nano), to.UTC().Format(time.RFC3339Nano), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []ChangeEvent
	for rows.Next() {
		var event ChangeEvent
		var occurred, details string
		if err := rows.Scan(&event.ID, &occurred, &event.Source, &event.Type, &event.Workload, &event.Kind, &details); err != nil {
			return nil, err
		}
		event.OccurredAt, err = time.Parse(time.RFC3339Nano, occurred)
		if err != nil {
			return nil, err
		}
		event.Details = json.RawMessage(details)
		result = append(result, event)
	}
	return result, rows.Err()
}
