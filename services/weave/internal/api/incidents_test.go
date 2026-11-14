package api

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/tchoupi663/weave/services/weave/internal/history"
	"github.com/tchoupi663/weave/services/weave/internal/topology"
)

func TestIncidentAPIStoresAndReturnsIncident(t *testing.T) {
	store := topology.NewStore("weave", 5*time.Minute)
	hist, err := history.Open(t.TempDir(), "weave", store, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer hist.Close()
	server := httptest.NewServer(NewWithHistory(store, t.TempDir(), hist, slog.New(slog.NewTextHandler(io.Discard, nil))))
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
	var incident history.Incident
	if err := json.NewDecoder(response.Body).Decode(&incident); err != nil {
		t.Fatal(err)
	}
	if incident.ID == "" || incident.Status != history.IncidentStatusOpen {
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
	var loaded history.Incident
	if err := json.NewDecoder(get.Body).Decode(&loaded); err != nil {
		t.Fatal(err)
	}
	if loaded.ID != incident.ID || loaded.Symptom != "Catalog 500" {
		t.Fatalf("loaded = %+v", loaded)
	}
}
