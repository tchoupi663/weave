package history

import (
	"context"
	"testing"
	"time"

	"github.com/tchoupi663/weave/services/weave/internal/topology"
)

func TestHistoryPersistsMetricsAcrossReopen(t *testing.T) {
	dir := t.TempDir()
	store := topology.NewStore("weave", 5*time.Minute)
	h, err := Open(dir, "weave", store, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC().Truncate(time.Minute)
	h.ObserveL4("frontend", "gateway", "HTTP", 8080, true, now)
	h.ObserveL7("frontend", "gateway", 8080, topology.L7Observation{
		SourceWorkload: "frontend",
		TargetWorkload: "gateway",
		Port:           8080,
		Method:         "GET",
		Path:           "/api/products/p001",
		StatusCode:     200,
		LatencySource:  "client",
		Duration:       3 * time.Millisecond,
		Time:           now,
	})
	if err := h.flushOnce(now.Add(10 * time.Second)); err != nil {
		t.Fatal(err)
	}
	metrics5, metrics1, changes, err := h.Counts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if metrics5 != 1 || metrics1 != 0 || changes != 0 {
		t.Fatalf("counts after 5s flush = %d/%d/%d", metrics5, metrics1, changes)
	}
	var routes, requests, success int64
	if err := h.duck.QueryRow(`SELECT count(*) FROM route_metrics_5s`).Scan(&routes); err != nil {
		t.Fatal(err)
	}
	if err := h.duck.QueryRow(`SELECT requests, status_2xx FROM edge_metrics_5s`).Scan(&requests, &success); err != nil {
		t.Fatal(err)
	}
	if routes != 1 || requests != 1 || success != 1 {
		t.Fatalf("route/edge detail = routes:%d requests:%d 2xx:%d", routes, requests, success)
	}
	if err := h.flushOnce(now.Add(2 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	metrics5, metrics1, _, err = h.Counts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if metrics5 != 1 || metrics1 != 1 {
		t.Fatalf("counts after 1m flush = %d/%d", metrics5, metrics1)
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(dir, "weave", store, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	metrics5, metrics1, changes, err = reopened.Counts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if metrics5 != 1 || metrics1 != 1 || changes != 0 {
		t.Fatalf("persisted counts = %d/%d/%d", metrics5, metrics1, changes)
	}
}

func TestHistoryDropsSelfEdges(t *testing.T) {
	dir := t.TempDir()
	store := topology.NewStore("weave", 5*time.Minute)
	h, err := Open(dir, "weave", store, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	now := time.Now().UTC().Truncate(bucket5Duration)
	h.ObserveL4("gateway", "gateway", "HTTP", 8080, true, now)
	h.ObserveL7("gateway", "gateway", 8080, topology.L7Observation{
		SourceWorkload: "gateway",
		TargetWorkload: "gateway",
		Port:           8080,
		Method:         "GET",
		Path:           "/health",
		StatusCode:     200,
		Time:           now,
	})
	if err := h.flushOnce(now.Add(10 * time.Second)); err != nil {
		t.Fatal(err)
	}
	metrics5, metrics1, changes, err := h.Counts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if metrics5 != 0 || metrics1 != 0 || changes != 0 {
		t.Fatalf("self edges were stored: %d/%d/%d", metrics5, metrics1, changes)
	}
}

func TestHistoryRetentionAndPrune(t *testing.T) {
	dir := t.TempDir()
	store := topology.NewStore("weave", 5*time.Minute)
	h, err := Open(dir, "weave", store, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer h.Close()
	now := time.Now().UTC()
	old := now.Add(-8 * 24 * time.Hour).Truncate(bucket5Duration)
	h.ObserveL4("frontend", "gateway", "HTTP", 8080, true, old)
	if err := h.flushOnce(old.Add(2 * time.Minute)); err != nil {
		t.Fatal(err)
	}
	if metrics5, metrics1, _, err := h.Counts(context.Background()); err != nil || metrics5 != 1 || metrics1 != 1 {
		t.Fatalf("before prune counts = %d/%d err=%v", metrics5, metrics1, err)
	}
	h.prune(now)
	metrics5, metrics1, _, err := h.Counts(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if metrics5 != 0 || metrics1 != 0 {
		t.Fatalf("after prune counts = %d/%d", metrics5, metrics1)
	}
}
