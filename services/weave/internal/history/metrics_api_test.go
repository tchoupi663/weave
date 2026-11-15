package history

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func openHTTPTestHistory(t *testing.T) (*History, *httptest.Server) {
	t.Helper()
	hist := openTestHistory(t)
	server := httptest.NewServer(New(hist, slog.New(slog.NewTextHandler(io.Discard, nil))))
	t.Cleanup(server.Close)
	return hist, server
}

func TestMetricsEdgesEndpoint(t *testing.T) {
	hist, server := openHTTPTestHistory(t)
	at := time.Now().UTC().Truncate(bucket5Duration).Add(-time.Minute)
	addL7(t, hist, at, "gateway", "catalog", "/api/products", 500)
	if err := hist.flushOnce(at.Add(2 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	from := at.Add(-time.Minute).Format(time.RFC3339)
	to := at.Add(time.Minute).Format(time.RFC3339)
	response, err := http.Get(server.URL + "/api/metrics/edges?" + url.Values{"from": {from}, "to": {to}, "bucket": {"5s"}}.Encode())
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(response.Body)
		t.Fatalf("status = %d: %s", response.StatusCode, data)
	}
	var payload struct {
		BucketSeconds int             `json:"bucket_seconds"`
		Rows          []edgeMetricRow `json:"rows"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.BucketSeconds != 5 || len(payload.Rows) != 1 {
		t.Fatalf("payload = %+v", payload)
	}
	row := payload.Rows[0]
	if row.EdgeID == "" || row.Requests != 1 || row.Status5xx != 1 || row.Source != "gateway" || row.Target != "catalog" {
		t.Fatalf("row = %+v", row)
	}

	filtered, err := http.Get(server.URL + "/api/metrics/edges?" + url.Values{"from": {from}, "to": {to}, "edge_id": {"workload:weave/gateway->workload:weave/catalog:8082"}}.Encode())
	if err != nil {
		t.Fatal(err)
	}
	defer filtered.Body.Close()
	var filteredPayload struct {
		Rows []edgeMetricRow `json:"rows"`
	}
	if err := json.NewDecoder(filtered.Body).Decode(&filteredPayload); err != nil {
		t.Fatal(err)
	}
	if len(filteredPayload.Rows) != 1 {
		t.Fatalf("filtered rows = %+v", filteredPayload.Rows)
	}
}

func TestMetricsRoutesEndpoint(t *testing.T) {
	hist, server := openHTTPTestHistory(t)
	at := time.Now().UTC().Truncate(bucket5Duration).Add(-time.Minute)
	addL7(t, hist, at, "gateway", "catalog", "/api/products/p001", 500)
	if err := hist.flushOnce(at.Add(2 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	from := at.Add(-time.Minute).Format(time.RFC3339)
	to := at.Add(time.Minute).Format(time.RFC3339)
	response, err := http.Get(server.URL + "/api/metrics/routes?" + url.Values{"from": {from}, "to": {to}}.Encode())
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(response.Body)
		t.Fatalf("status = %d: %s", response.StatusCode, data)
	}
	var payload struct {
		Rows []routeMetricRow `json:"rows"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if len(payload.Rows) != 1 || payload.Rows[0].Path != "/api/products/:id" || payload.Rows[0].Status5xx != 1 {
		t.Fatalf("rows = %+v", payload.Rows)
	}
}

func TestMetricsEndpointsRejectBadQueries(t *testing.T) {
	_, server := openHTTPTestHistory(t)
	cases := []string{
		"/api/metrics/edges",
		"/api/metrics/edges?from=bad&to=2026-01-01T00:01:00Z",
		"/api/metrics/edges?from=2026-01-01T00:01:00Z&to=2026-01-01T00:00:00Z",
		"/api/metrics/edges?from=2026-01-01T00:00:00Z&to=2026-01-01T00:01:00Z&bucket=2s",
		"/api/metrics/routes?from=2026-01-01T00:00:00Z&to=2026-01-01T00:01:00Z&bucket=1m",
	}
	for _, path := range cases {
		response, err := http.Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusBadRequest {
			t.Fatalf("%s status = %d, want 400", path, response.StatusCode)
		}
	}
}

func TestIncidentAlertsEndpoint(t *testing.T) {
	hist, server := openHTTPTestHistory(t)
	incident, err := hist.CreateIncident(t.Context(), CreateIncidentRequest{
		Start:   time.Now().UTC().Add(-time.Minute),
		End:     time.Now().UTC(),
		Symptom: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.Get(server.URL + "/api/incidents/" + incident.ID + "/alerts")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", response.StatusCode)
	}
	var payload struct {
		Alerts []Alert `json:"alerts"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	if payload.Alerts == nil || len(payload.Alerts) != 0 {
		t.Fatalf("alerts = %+v", payload.Alerts)
	}

	missing, err := http.Get(server.URL + "/api/incidents/missing/alerts")
	if err != nil {
		t.Fatal(err)
	}
	missing.Body.Close()
	if missing.StatusCode != http.StatusNotFound {
		t.Fatalf("missing status = %d", missing.StatusCode)
	}
}

func TestHealthReportsDetectionStatus(t *testing.T) {
	hist, server := openHTTPTestHistory(t)
	paths := []string{"/health", "/api/health"}
	for _, path := range paths {
		response, err := http.Get(server.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		var payload map[string]any
		if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
			response.Body.Close()
			t.Fatal(err)
		}
		response.Body.Close()
		if _, ok := payload["detection_enabled"]; !ok {
			t.Fatalf("%s payload lacks detection_enabled: %+v", path, payload)
		}
	}
	_ = hist
}
