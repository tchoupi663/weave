package history

import (
	"database/sql"
	"path/filepath"
	"testing"
)

func TestOpenMigratesLegacyIncidentTable(t *testing.T) {
	dir := t.TempDir()
	legacy, err := sql.Open("sqlite", filepath.Join(dir, "weave.db"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`
CREATE TABLE incidents (
  id TEXT PRIMARY KEY,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  window_start TEXT NOT NULL,
  window_end TEXT NOT NULL,
  baseline_start TEXT NOT NULL,
  baseline_end TEXT NOT NULL,
  symptom TEXT NOT NULL DEFAULT '',
  scope TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'open',
  notes TEXT NOT NULL DEFAULT '',
  evidence_json TEXT NOT NULL DEFAULT '{}'
)`); err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`INSERT INTO incidents
(id, created_at, updated_at, window_start, window_end, baseline_start, baseline_end, symptom, status, evidence_json)
VALUES ('legacy', '2026-11-14T10:00:00Z', '2026-11-14T10:01:00Z', '2026-11-14T10:00:00Z', '2026-11-14T10:01:00Z', '2026-11-14T09:45:00Z', '2026-11-14T10:00:00Z', 'legacy', 'open', '{}')`); err != nil {
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	hist, err := Open(dir, "weave", 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer hist.Close()

	rows, err := hist.sqlite.Query(`PRAGMA table_info(incidents)`)
	if err != nil {
		t.Fatal(err)
	}
	columns := map[string]bool{}
	for rows.Next() {
		var cid int
		var name, kind string
		var notnull, pk int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &kind, &notnull, &defaultValue, &pk); err != nil {
			rows.Close()
			t.Fatal(err)
		}
		columns[name] = true
	}
	rows.Close()
	if !columns["origin"] || !columns["trigger_json"] {
		t.Fatalf("migrated columns = %+v", columns)
	}

	incident, err := hist.GetIncident(t.Context(), "legacy")
	if err != nil {
		t.Fatal(err)
	}
	if incident.Origin != "" || incident.Trigger != nil {
		t.Fatalf("legacy incident = %+v", incident)
	}

	manual, err := hist.CreateIncident(t.Context(), CreateIncidentRequest{
		Start:   incident.WindowStart,
		End:     incident.WindowEnd,
		Symptom: "manual after migration",
	})
	if err != nil {
		t.Fatal(err)
	}
	if manual.Origin != "" || manual.Trigger != nil {
		t.Fatalf("manual incident after migration = %+v", manual)
	}
}
