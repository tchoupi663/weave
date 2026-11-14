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
	baselineDuration     = 15 * time.Minute
	evidenceVersion      = "incident-v1"
)

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

func (h *History) CreateIncident(ctx context.Context, request CreateIncidentRequest) (*Incident, error) {
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
	incident := &Incident{
		ID:            id,
		CreatedAt:     now,
		UpdatedAt:     now,
		WindowStart:   start,
		WindowEnd:     end,
		BaselineStart: start.Add(-baselineDuration),
		BaselineEnd:   start,
		Symptom:       request.Symptom,
		Scope:         request.Scope,
		Status:        IncidentStatusOpen,
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

func (h *History) saveIncident(ctx context.Context, incident *Incident) error {
	payload, err := json.Marshal(incident.Evidence)
	if err != nil {
		return err
	}
	_, err = h.sqlite.ExecContext(ctx, `
INSERT INTO incidents
(id, created_at, updated_at, window_start, window_end, baseline_start, baseline_end, symptom, scope, status, notes, evidence_json)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
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
  evidence_json=excluded.evidence_json`,
		incident.ID, incident.CreatedAt.Format(time.RFC3339Nano), incident.UpdatedAt.Format(time.RFC3339Nano),
		incident.WindowStart.Format(time.RFC3339Nano), incident.WindowEnd.Format(time.RFC3339Nano),
		incident.BaselineStart.Format(time.RFC3339Nano), incident.BaselineEnd.Format(time.RFC3339Nano),
		incident.Symptom, incident.Scope, incident.Status, incident.Notes, string(payload))
	return err
}

func (h *History) GetIncident(ctx context.Context, id string) (*Incident, error) {
	row := h.sqlite.QueryRowContext(ctx, `SELECT id, created_at, updated_at, window_start, window_end, baseline_start, baseline_end, symptom, scope, status, notes, evidence_json FROM incidents WHERE id = ?`, id)
	return scanIncident(row)
}

func (h *History) ListIncidents(ctx context.Context, limit int) ([]Incident, error) {
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	rows, err := h.sqlite.QueryContext(ctx, `SELECT id, created_at, updated_at, window_start, window_end, baseline_start, baseline_end, symptom, scope, status, notes, evidence_json FROM incidents ORDER BY created_at DESC LIMIT ?`, limit)
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
	var createdAt, updatedAt, windowStart, windowEnd, baselineStart, baselineEnd, evidenceJSON string
	if err := scanner.Scan(&incident.ID, &createdAt, &updatedAt, &windowStart, &windowEnd, &baselineStart, &baselineEnd, &incident.Symptom, &incident.Scope, &incident.Status, &incident.Notes, &evidenceJSON); err != nil {
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
