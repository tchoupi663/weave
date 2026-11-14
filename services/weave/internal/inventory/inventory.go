package inventory

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/tchoupi663/weave/services/weave/internal/contracts"
	"github.com/tchoupi663/weave/services/weave/internal/httpx"
	_ "modernc.org/sqlite"
)

const (
	inactiveAfter       = 30 * time.Minute
	defaultLiveTimeout  = time.Second
	defaultPollInterval = 30 * time.Second
)

const inventorySchema = `
CREATE TABLE IF NOT EXISTS change_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  occurred_at TEXT NOT NULL,
  source TEXT NOT NULL,
  type TEXT NOT NULL,
  workload TEXT NOT NULL,
  kind TEXT NOT NULL DEFAULT '',
  details TEXT NOT NULL DEFAULT '{}'
);
CREATE INDEX IF NOT EXISTS change_events_occurred_at ON change_events(occurred_at);

CREATE TABLE IF NOT EXISTS workload_states (
  workload TEXT PRIMARY KEY,
  state TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
`

// Config is the weave-inventory configuration.
type Config struct {
	ListenAddr        string
	WatchNamespace    string
	DataDir           string
	LiveURL           string
	LiveTimeout       time.Duration
	ReconcileInterval time.Duration
	ArgoCDURL         string
	ArgoCDToken       string
	ArgoCDApp         string
	ArgoCDNamespace   string
}

// Inventory owns declared-architecture reconciliation, lifecycle state, and
// change events. It polls weave-live for the observed workload set.
type Inventory struct {
	cfg  Config
	log  *slog.Logger
	db   *sql.DB
	argo *ArgoClient
	http *http.Client

	mu            sync.Mutex
	states        map[string]string
	desired       map[string]string
	live          map[string]bool
	initialized   bool
	lastReconcile time.Time
	lastError     string
}

// New opens the inventory SQLite store and builds the reconciler.
func New(cfg Config, log *slog.Logger) (*Inventory, error) {
	if strings.TrimSpace(cfg.DataDir) == "" {
		return nil, fmt.Errorf("data directory is empty")
	}
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	db, err := sql.Open("sqlite", filepath.Join(cfg.DataDir, "inventory.db"))
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)
	if _, err := db.Exec(`PRAGMA journal_mode=WAL;`); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite wal: %w", err)
	}
	if _, err := db.Exec(`PRAGMA busy_timeout=5000;`); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite busy timeout: %w", err)
	}
	if err := execSchema(db, inventorySchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("sqlite schema: %w", err)
	}
	timeout := cfg.LiveTimeout
	if timeout <= 0 {
		timeout = defaultLiveTimeout
	}
	interval := cfg.ReconcileInterval
	if interval <= 0 {
		interval = defaultPollInterval
	}
	cfg.ReconcileInterval = interval
	inv := &Inventory{
		cfg:     cfg,
		log:     log,
		db:      db,
		http:    &http.Client{Timeout: timeout},
		states:  make(map[string]string),
		desired: make(map[string]string),
		live:    make(map[string]bool),
	}
	if cfg.ArgoCDURL != "" {
		inv.argo = NewArgoClient(cfg.ArgoCDURL, cfg.ArgoCDToken, cfg.ArgoCDApp, cfg.ArgoCDNamespace)
	}
	if err := inv.loadStates(); err != nil {
		db.Close()
		return nil, err
	}
	return inv, nil
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

func (i *Inventory) loadStates() error {
	rows, err := i.db.Query(`SELECT workload, state FROM workload_states`)
	if err != nil {
		return fmt.Errorf("load workload states: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var workload, state string
		if err := rows.Scan(&workload, &state); err != nil {
			return err
		}
		i.states[workload] = state
	}
	return rows.Err()
}

// Start launches the reconcile loop. Without an Argo client, the service still
// serves stored change events and reports itself healthy.
func (i *Inventory) Start(ctx context.Context) {
	if i.argo == nil {
		i.log.Warn("inventory reconcile disabled: ARGOCD_URL is empty")
		return
	}
	go i.loop(ctx)
}

func (i *Inventory) loop(ctx context.Context) {
	i.reconcile(ctx)
	ticker := time.NewTicker(i.cfg.ReconcileInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			i.reconcile(ctx)
		}
	}
}

func (i *Inventory) reconcile(ctx context.Context) {
	if i.argo == nil {
		return
	}
	desiredWorkloads, err := i.argo.Workloads(ctx)
	if err != nil {
		i.setError(fmt.Sprintf("read declared workloads: %v", err))
		i.log.Error("read declared workloads", "error", err)
		return
	}
	observed, err := i.observedWorkloads(ctx)
	if err != nil {
		i.setError(fmt.Sprintf("read observed workloads: %v", err))
		i.log.Error("read observed workloads", "error", err)
		return
	}
	i.setError("")
	i.reconcileLocked(desiredWorkloads, observed)
}

func (i *Inventory) observedWorkloads(ctx context.Context) (map[string]time.Time, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, i.cfg.LiveURL+"/internal/v1/workloads", nil)
	if err != nil {
		return nil, err
	}
	response, err := i.http.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("weave-live returned %s", response.Status)
	}
	var payload contracts.WorkloadsResponse
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode workloads: %w", err)
	}
	observed := make(map[string]time.Time, len(payload.Workloads))
	for _, workload := range payload.Workloads {
		observed[workload.Workload] = workload.LastSeen
	}
	return observed, nil
}

func (i *Inventory) reconcileLocked(desiredWorkloads []DeclaredWorkload, observed map[string]time.Time) {
	i.mu.Lock()
	defer i.mu.Unlock()
	now := time.Now().UTC()
	desired := make(map[string]DeclaredWorkload, len(desiredWorkloads))
	for _, workload := range desiredWorkloads {
		desired[workload.Name] = workload
	}
	first := !i.initialized
	if !first {
		for name, workload := range desired {
			if _, existed := i.desired[name]; !existed {
				i.recordChange(now, "declared", "declared_added", name, workload.Kind, map[string]any{"kind": workload.Kind})
			}
		}
		for name, kind := range i.desired {
			if _, exists := desired[name]; !exists {
				i.recordChange(now, "declared", "declared_removed", name, kind, map[string]any{"kind": kind})
			}
		}
	}

	for name, workload := range desired {
		lastSeen, observedBefore := observed[name]
		state := "Declared"
		if observedBefore {
			if now.Sub(lastSeen) <= inactiveAfter {
				state = "Active"
				if i.states[name] == "" {
					i.recordChange(now, "observed", "observed_first_seen", name, workload.Kind, map[string]any{"last_seen": lastSeen})
				}
			} else {
				state = "Inactive"
			}
		}
		if !workload.Live {
			state = "Removed"
			if first || i.live[name] {
				i.recordChange(now, "live", "live_missing", name, workload.Kind, map[string]any{"kind": workload.Kind})
			}
		}
		i.live[name] = workload.Live
		i.transition(now, name, workload.Kind, state)
	}

	for name, lastSeen := range observed {
		if _, declared := desired[name]; declared {
			continue
		}
		state := "Undeclared"
		if now.Sub(lastSeen) > inactiveAfter {
			state = "Removed"
		}
		i.transition(now, name, "", state)
	}

	for name := range i.desired {
		if _, exists := desired[name]; exists {
			continue
		}
		if _, observedNow := observed[name]; !observedNow {
			i.transition(now, name, i.desired[name], "Removed")
		}
	}

	for name, workload := range desired {
		i.desired[name] = workload.Kind
	}
	for name := range i.desired {
		if _, exists := desired[name]; !exists {
			delete(i.desired, name)
		}
	}
	i.initialized = true
	i.lastReconcile = now
}

func (i *Inventory) transition(now time.Time, workload, kind, state string) {
	previous := i.states[workload]
	if previous == state {
		return
	}
	i.states[workload] = state
	if _, err := i.db.Exec(
		`INSERT INTO workload_states (workload, state, updated_at) VALUES (?, ?, ?)
		 ON CONFLICT(workload) DO UPDATE SET state=excluded.state, updated_at=excluded.updated_at`,
		workload, state, now.Format(time.RFC3339Nano)); err != nil {
		i.log.Error("store workload state", "error", err, "workload", workload)
	}
	i.recordChange(now, lifecycleSource(state), "state_changed", workload, kind, map[string]any{
		"from": previous,
		"to":   state,
	})
}

func lifecycleSource(state string) string {
	switch state {
	case "Declared":
		return "declared"
	case "Undeclared":
		return "observed"
	case "Removed":
		return "live"
	default:
		return "observed"
	}
}

func (i *Inventory) recordChange(occurred time.Time, source, eventType, workload, kind string, details any) {
	data, _ := json.Marshal(details)
	if _, err := i.db.Exec(
		`INSERT INTO change_events (occurred_at, source, type, workload, kind, details) VALUES (?, ?, ?, ?, ?, ?)`,
		occurred.UTC().Format(time.RFC3339Nano), source, eventType, workload, kind, string(data)); err != nil {
		i.log.Error("record change", "error", err, "workload", workload, "type", eventType)
	}
}

func (i *Inventory) setError(message string) {
	i.mu.Lock()
	i.lastError = message
	i.mu.Unlock()
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

// Changes returns lifecycle change events in the interval, newest first.
func (i *Inventory) Changes(ctx context.Context, from, to time.Time, limit int) ([]ChangeEvent, error) {
	if limit <= 0 || limit > 5000 {
		limit = 1000
	}
	rows, err := i.db.QueryContext(ctx, `
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

// Handler returns the inventory HTTP handler.
func (i *Inventory) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/changes", i.changes)
	mux.HandleFunc("GET /health", i.health)
	mux.HandleFunc("GET /api/health", i.health)
	return httpx.SecurityHeaders(mux)
}

func (i *Inventory) health(w http.ResponseWriter, _ *http.Request) {
	i.mu.Lock()
	lastError := i.lastError
	lastReconcile := i.lastReconcile
	i.mu.Unlock()
	status := "ok"
	if lastError != "" {
		status = "degraded"
	}
	body := map[string]any{"status": status}
	if !lastReconcile.IsZero() {
		body["last_reconcile"] = lastReconcile
	}
	if lastError != "" {
		body["last_error"] = lastError
	}
	httpx.WriteJSON(w, http.StatusOK, body)
}

func (i *Inventory) changes(w http.ResponseWriter, r *http.Request) {
	to := time.Now().UTC()
	from := to.Add(-24 * time.Hour)
	if raw := r.URL.Query().Get("from"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "from must be RFC3339")
			return
		}
		from = parsed.UTC()
	}
	if raw := r.URL.Query().Get("to"); raw != "" {
		parsed, err := time.Parse(time.RFC3339, raw)
		if err != nil {
			httpx.WriteError(w, http.StatusBadRequest, "to must be RFC3339")
			return
		}
		to = parsed.UTC()
	}
	limit := 1000
	if raw := r.URL.Query().Get("limit"); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil {
			limit = parsed
		}
	}
	events, err := i.Changes(r.Context(), from, to, limit)
	if err != nil {
		httpx.WriteError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if events == nil {
		events = []ChangeEvent{}
	}
	httpx.WriteJSON(w, http.StatusOK, events)
}

// Close closes the SQLite store.
func (i *Inventory) Close() error { return i.db.Close() }
