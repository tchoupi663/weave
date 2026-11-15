package investigation

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/tchoupi663/weave/services/agent/internal/scope"
)

func TestBuildRegionKeepsOnlySuspectEntities(t *testing.T) {
	incident := map[string]any{
		"id": "incident-1", "window_start": "2026-11-15T10:00:00Z", "window_end": "2026-11-15T10:02:00Z",
		"baseline_start": "2026-11-15T09:45:00Z", "baseline_end": "2026-11-15T10:00:00Z",
		"symptom": "Catalog 500", "status": "open", "origin": "detection",
		"trigger": map[string]any{"rule_id": "catalog-products-5xx"},
		"evidence": map[string]any{
			"edges": []map[string]any{
				{"edge_id": "gateway->catalog", "source": "gateway", "target": "catalog", "protocol": "HTTP", "port": 8082,
					"baseline": map[string]any{"requests": 100, "error_rate": 0.0, "p95_ms": 3.0},
					"incident": map[string]any{"requests": 50, "status_5xx": 50, "error_rate": 1.0, "p95_ms": 4.0},
					"anomaly":  true, "onset": "2026-11-15T10:00:05Z", "reasons": []string{"5xx rate"}},
				{"edge_id": "gateway->orders", "source": "gateway", "target": "orders", "protocol": "HTTP", "port": 8083,
					"anomaly": false},
			},
			"suspect_edges": []string{"gateway->catalog"},
			"suspect_nodes": []string{"gateway", "catalog"},
			"routes": []map[string]any{
				{"edge_id": "gateway->catalog", "method": "GET", "path": "/api/products", "requests": 50, "status_5xx": 50},
				{"edge_id": "gateway->orders", "method": "POST", "path": "/api/orders", "requests": 10},
			},
		},
	}
	timeline := map[string]any{
		"frames": []map[string]any{
			{"at": "2026-11-15T10:00:05Z", "edges": []map[string]any{
				{"edge_id": "gateway->catalog", "requests": 10, "error_rate": 1.0, "p95_ms": 4.0, "anomaly": true},
				{"edge_id": "gateway->orders", "requests": 2, "error_rate": 0.0, "p95_ms": 2.0, "anomaly": false},
			}},
		},
		"gaps": []map[string]any{{"from": "2026-11-15T10:00:30Z", "to": "2026-11-15T10:00:35Z"}},
	}
	incidentJSON, _ := json.Marshal(incident)
	timelineJSON, _ := json.Marshal(timeline)
	raw, err := BuildRegion(incidentJSON, timelineJSON, []scope.Frontier{{NodeID: "catalog", Directions: []string{"downstream"}}}, scope.Limits{MaxExpansions: 3, MaxAddedNodes: 8})
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if strings.Contains(text, "gateway->orders") {
		t.Fatalf("out-of-region edge leaked: %s", text)
	}
	var payload map[string]any
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatal(err)
	}
	if _, ok := payload["incident"]; !ok {
		t.Fatalf("payload = %+v", payload)
	}
	if !strings.Contains(text, "gateway->catalog") || !strings.Contains(text, "2026-11-15T10:00:30Z") {
		t.Fatalf("payload missing expected content: %s", text)
	}
	_ = time.Now
}
