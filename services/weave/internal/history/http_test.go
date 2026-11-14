package history

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestIncidentAPIStoresAndReturnsIncident(t *testing.T) {
	hist, err := Open(t.TempDir(), "weave", 15*time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer hist.Close()
	server := httptest.NewServer(New(hist, slog.New(slog.NewTextHandler(io.Discard, nil))))
	defer server.Close()

	start := time.Now().UTC().Add(-time.Minute).Truncate(time.Second)
	end := start.Add(30 * time.Second)
	body, _ := json.Marshal(map[string]any{"start": start, "end": end, "symptom": "Catalog 500"})
	response, err := http.Post(server.URL+"/api/incidents", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusCreated {
		data, _ := io.ReadAll(response.Body)
		t.Fatalf("create status = %d: %s", response.StatusCode, data)
	}
	var incident Incident
	if err := json.NewDecoder(response.Body).Decode(&incident); err != nil {
		t.Fatal(err)
	}
	if incident.ID == "" || incident.Status != IncidentStatusOpen {
		t.Fatalf("incident = %+v", incident)
	}

	get, err := http.Get(server.URL + "/api/incidents/" + incident.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer get.Body.Close()
	if get.StatusCode != http.StatusOK {
		t.Fatalf("get status = %d", get.StatusCode)
	}
	var loaded Incident
	if err := json.NewDecoder(get.Body).Decode(&loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.ID != incident.ID || loaded.Symptom != "Catalog 500" {
		t.Fatalf("loaded = %+v", loaded)
	}
}

func TestObservationBatchAppliesAndReportsGaps(t *testing.T) {
	hist, err := Open(t.TempDir(), "weave", 15*time.Second, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}
	defer hist.Close()
	server := httptest.NewServer(New(hist, slog.New(slog.NewTextHandler(io.Discard, nil))))
	defer server.Close()

	now := time.Now().UTC()
	from := now.Add(-time.Minute).Truncate(time.Second)
	gapFrom := now.Add(-45 * time.Second)
	gapTo := now.Add(-30 * time.Second)
	batch := map[string]any{
		"epoch": "test",
		"seq":   1,
		"l4": []map[string]any{{
			"source": "frontend", "target": "gateway", "protocol": "HTTP", "port": 8080,
			"connection": true, "time": from,
		}},
		"l7": []map[string]any{{
			"source": "frontend", "target": "gateway", "port": 8080, "method": "GET",
			"path": "/api/products/p001", "status": 500, "latency_source": "client",
			"duration_ms": 3.2, "time": from,
		}},
		"gaps": []map[string]any{{"from": gapFrom, "to": gapTo}},
	}
	body, _ := json.Marshal(batch)
	response, err := http.Post(server.URL+"/internal/v1/observations", "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusAccepted {
		data, _ := io.ReadAll(response.Body)
		t.Fatalf("batch status = %d: %s", response.StatusCode, data)
	}
	if err := hist.flushOnce(now.Add(2 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	metrics5, _, err := hist.Counts(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if metrics5 != 1 {
		t.Fatalf("edge rows = %d", metrics5)
	}
	gaps, err := hist.Gaps(t.Context(), from.Add(-time.Minute), now)
	if err != nil {
		t.Fatal(err)
	}
	if len(gaps) != 1 || !gaps[0].From.Equal(gapFrom.UTC()) {
		t.Fatalf("gaps = %+v", gaps)
	}
}
