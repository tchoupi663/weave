package history

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

var ErrIncidentNotFound = errors.New("incident not found")

const (
	IncidentStatusOpen   = "open"
	IncidentStatusClosed = "closed"
	IncidentOriginManual = "manual"
	IncidentOriginDetect = "detection"
	evidenceVersion      = "incident-v1"
)

// Trigger records the detection provenance of an automatically created
// Incident. It is omitted for manually created incidents.
type Trigger struct {
	RuleID       string       `json:"rule_id"`
	RuleVersion  string       `json:"rule_version"`
	Scope        TriggerScope `json:"scope"`
	Metric       string       `json:"metric"`
	Observed     float64      `json:"observed"`
	Threshold    float64      `json:"threshold"`
	BreachBucket time.Time    `json:"breach_bucket"`
	DetectedAt   time.Time    `json:"detected_at"`
}

// TriggerScope identifies what the breached rule covers.
type TriggerScope struct {
	Kind   string `json:"kind"`
	EdgeID string `json:"edge_id,omitempty"`
	Method string `json:"method,omitempty"`
	Path   string `json:"path,omitempty"`
}

// Incident is a persisted, time-bounded investigation.
type Incident struct {
	ID            string    `json:"id"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	WindowStart   time.Time `json:"window_start"`
	WindowEnd     time.Time `json:"window_end"`
	BaselineStart time.Time `json:"baseline_start"`
	BaselineEnd   time.Time `json:"baseline_end"`
	Symptom       string    `json:"symptom"`
	Scope         string    `json:"scope,omitempty"`
	Status        string    `json:"status"`
	Notes         string    `json:"notes"`
	Origin        string    `json:"origin,omitempty"`
	Trigger       *Trigger  `json:"trigger,omitempty"`
	Evidence      Evidence  `json:"evidence"`
}

type CreateIncidentRequest struct {
	Start   time.Time `json:"start"`
	End     time.Time `json:"end"`
	Symptom string    `json:"symptom"`
	Scope   string    `json:"scope"`
}

type PatchIncidentRequest struct {
	Status *string `json:"status"`
	Notes  *string `json:"notes"`
}

func newIncidentID() (string, error) {
	var raw [12]byte
	if _, err := rand.Read(raw[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw[:]), nil
}

// SetBaseline configures the evidence baseline for incidents created after
// this call. It exists so the history service and the benchmark profile can
// use different baselines without changing the product default.
func (h *History) SetBaseline(baseline time.Duration) error {
	if baseline <= 0 {
		return fmt.Errorf("baseline must be positive")
	}
	h.baseline = baseline
	return nil
}

func (h *History) baselineDuration() time.Duration {
	if h.baseline <= 0 {
		return defaultBaseline
	}
	return h.baseline
}

func (h *History) CreateIncident(ctx context.Context, request CreateIncidentRequest) (*Incident, error) {
	return h.createIncident(ctx, request, IncidentOriginManual, nil)
}

// createDetectedIncident creates an incident from grouped Alerts. It is not
// exposed over HTTP; only the detector calls it.
func (h *History) createDetectedIncident(ctx context.Context, start, end time.Time, symptom string, trigger Trigger) (*Incident, error) {
	return h.createIncident(ctx, CreateIncidentRequest{Start: start, End: end, Symptom: symptom}, IncidentOriginDetect, &trigger)
}

func (h *History) createIncident(ctx context.Context, request CreateIncidentRequest, origin string, trigger *Trigger) (*Incident, error) {
	now := time.Now().UTC()
	start := request.Start.UTC()
	end := request.End.UTC()
	if end.IsZero() {
		end = now
	}
	if !end.After(start) {
		return nil, fmt.Errorf("incident end must be after start")
	}
	id, err := newIncidentID()
	if err != nil {
		return nil, err
	}
	if origin == IncidentOriginManual {
		origin = ""
	}
	incident := &Incident{
		ID:            id,
		CreatedAt:     now,
		UpdatedAt:     now,
		WindowStart:   start,
		WindowEnd:     end,
		BaselineStart: start.Add(-h.baselineDuration()),
		BaselineEnd:   start,
		Symptom:       request.Symptom,
		Scope:         request.Scope,
		Status:        IncidentStatusOpen,
		Origin:        origin,
		Trigger:       trigger,
	}
	if incident.Symptom == "" {
		incident.Symptom = "SLA breach detected"
	}
	evidence, err := h.computeEvidence(ctx, incident)
	if err != nil {
		return nil, err
	}
	incident.Evidence = evidence
	if err := h.saveIncident(ctx, incident); err != nil {
		return nil, err
	}
	return incident, nil
}

// closeDetectedIncident ends an automatically detected incident at the last
// breaching bucket and recomputes its operator-facing evidence once.
func (h *History) closeDetectedIncident(ctx context.Context, id string, end time.Time) error {
	incident, err := h.GetIncident(ctx, id)
	if err != nil {
		return err
	}
	if incident.Origin != IncidentOriginDetect {
		return fmt.Errorf("incident %s is not detection-originated", id)
	}
	end = end.UTC()
	if !end.After(incident.WindowStart) {
		end = incident.WindowStart.Add(bucket5Duration)
	}
	incident.WindowEnd = end
	evidence, err := h.computeEvidence(ctx, incident)
	if err != nil {
		return err
	}
	incident.Evidence = evidence
	incident.Status = IncidentStatusClosed
	incident.UpdatedAt = time.Now().UTC()
	return h.saveIncident(ctx, incident)
}

func (h *History) saveIncident(ctx context.Context, incident *Incident) error {
	payload, err := json.Marshal(incident.Evidence)
	if err != nil {
		return err
	}
	trigger := ""
	if incident.Trigger != nil {
		raw, err := json.Marshal(incident.Trigger)
		if err != nil {
			return err
		}
		trigger = string(raw)
	}
	origin := incident.Origin
	if origin == "" {
		origin = IncidentOriginManual
	}
	_, err = h.sqlite.ExecContext(ctx, `
INSERT INTO incidents
(id, created_at, updated_at, window_start, window_end, baseline_start, baseline_end, symptom, scope, status, notes, evidence_json, origin, trigger_json)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
ON CONFLICT(id) DO UPDATE SET
  updated_at=excluded.updated_at,
  window_start=excluded.window_start,
  window_end=excluded.window_end,
  baseline_start=excluded.baseline_start,
  baseline_end=excluded.baseline_end,
  symptom=excluded.symptom,
  scope=excluded.scope,
  status=excluded.status,
  notes=excluded.notes,
  evidence_json=excluded.evidence_json,
  origin=excluded.origin,
  trigger_json=excluded.trigger_json`,
		incident.ID, incident.CreatedAt.Format(time.RFC3339Nano), incident.UpdatedAt.Format(time.RFC3339Nano),
		incident.WindowStart.Format(time.RFC3339Nano), incident.WindowEnd.Format(time.RFC3339Nano),
		incident.BaselineStart.Format(time.RFC3339Nano), incident.BaselineEnd.Format(time.RFC3339Nano),
		incident.Symptom, incident.Scope, incident.Status, incident.Notes, string(payload), origin, trigger)
	return err
}

func (h *History) GetIncident(ctx context.Context, id string) (*Incident, error) {
	row := h.sqlite.QueryRowContext(ctx, `SELECT id, created_at, updated_at, window_start, window_end, baseline_start, baseline_end, symptom, scope, status, notes, evidence_json, origin, trigger_json FROM incidents WHERE id = ?`, id)
	return scanIncident(row)
}

func (h *History) ListIncidents(ctx context.Context, limit int) ([]Incident, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := h.sqlite.QueryContext(ctx, `SELECT id, created_at, updated_at, window_start, window_end, baseline_start, baseline_end, symptom, scope, status, notes, evidence_json, origin, trigger_json FROM incidents ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Incident
	for rows.Next() {
		incident, err := scanIncident(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, *incident)
	}
	return result, rows.Err()
}

type incidentScanner interface {
	Scan(dest ...any) error
}

func scanIncident(scanner incidentScanner) (*Incident, error) {
	var incident Incident
	var createdAt, updatedAt, windowStart, windowEnd, baselineStart, baselineEnd, evidenceJSON, origin, triggerJSON string
	if err := scanner.Scan(&incident.ID, &createdAt, &updatedAt, &windowStart, &windowEnd, &baselineStart, &baselineEnd, &incident.Symptom, &incident.Scope, &incident.Status, &incident.Notes, &evidenceJSON, &origin, &triggerJSON); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrIncidentNotFound
		}
		return nil, err
	}
	var err error
	if incident.CreatedAt, err = time.Parse(time.RFC3339Nano, createdAt); err != nil {
		return nil, err
	}
	if incident.UpdatedAt, err = time.Parse(time.RFC3339Nano, updatedAt); err != nil {
		return nil, err
	}
	if incident.WindowStart, err = time.Parse(time.RFC3339Nano, windowStart); err != nil {
		return nil, err
	}
	if incident.WindowEnd, err = time.Parse(time.RFC3339Nano, windowEnd); err != nil {
		return nil, err
	}
	if incident.BaselineStart, err = time.Parse(time.RFC3339Nano, baselineStart); err != nil {
		return nil, err
	}
	if incident.BaselineEnd, err = time.Parse(time.RFC3339Nano, baselineEnd); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(evidenceJSON), &incident.Evidence); err != nil {
		return nil, err
	}
	if origin == "" || origin == IncidentOriginManual {
		origin = ""
	}
	incident.Origin = origin
	if triggerJSON != "" {
		var trigger Trigger
		if err := json.Unmarshal([]byte(triggerJSON), &trigger); err != nil {
			return nil, err
		}
		incident.Trigger = &trigger
	}
	return &incident, nil
}

func (h *History) UpdateIncident(ctx context.Context, id string, patch PatchIncidentRequest) (*Incident, error) {
	incident, err := h.GetIncident(ctx, id)
	if err != nil {
		return nil, err
	}
	if patch.Status != nil {
		if *patch.Status != IncidentStatusOpen && *patch.Status != IncidentStatusClosed {
			return nil, fmt.Errorf("status must be open or closed")
		}
		incident.Status = *patch.Status
	}
	if patch.Notes != nil {
		incident.Notes = *patch.Notes
	}
	incident.UpdatedAt = time.Now().UTC()
	if err := h.saveIncident(ctx, incident); err != nil {
		return nil, err
	}
	return incident, nil
}

func (h *History) RecomputeIncident(ctx context.Context, id string) (*Incident, error) {
	incident, err := h.GetIncident(ctx, id)
	if err != nil {
		return nil, err
	}
	evidence, err := h.computeEvidence(ctx, incident)
	if err != nil {
		return nil, err
	}
	incident.Evidence = evidence
	incident.UpdatedAt = time.Now().UTC()
	if err := h.saveIncident(ctx, incident); err != nil {
		return nil, err
	}
	return incident, nil
}
