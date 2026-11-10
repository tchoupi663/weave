package api

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/tchoupi663/weave/services/weave/internal/topology"
)

func testServer(t *testing.T) *httptest.Server {
	t.Helper()
	store := topology.NewStore("weave", 5*time.Minute)
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	server := httptest.NewServer(New(store, t.TempDir(), log))
	t.Cleanup(server.Close)
	return server
}

func TestTopologyAndReset(t *testing.T) {
	server := testServer(t)

	response, err := http.Get(server.URL + "/api/topology")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("topology status = %d", response.StatusCode)
	}
	var snapshot topology.Snapshot
	if err := json.NewDecoder(response.Body).Decode(&snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Namespace != "weave" {
		t.Fatalf("namespace = %q", snapshot.Namespace)
	}
	if snapshot.Nodes == nil || snapshot.Edges == nil || snapshot.Activity == nil {
		t.Fatal("topology arrays must not be null")
	}

	reset, err := http.Post(server.URL+"/api/reset", "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	reset.Body.Close()
	if reset.StatusCode != http.StatusOK {
		t.Fatalf("reset status = %d", reset.StatusCode)
	}
}

func TestHealthAndStream(t *testing.T) {
	server := testServer(t)

	response, err := http.Get(server.URL + "/api/health")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("health status = %d", response.StatusCode)
	}
	var health map[string]any
	if err := json.NewDecoder(response.Body).Decode(&health); err != nil {
		t.Fatal(err)
	}
	if health["status"] != "degraded" {
		t.Fatalf("health = %v, want degraded before a stream", health["status"])
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, server.URL+"/api/stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	stream, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer stream.Body.Close()
	if stream.StatusCode != http.StatusOK {
		t.Fatalf("stream status = %d", stream.StatusCode)
	}

	scanner := bufio.NewScanner(stream.Body)
	var data string
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "data: ") {
			data = strings.TrimPrefix(line, "data: ")
			break
		}
	}
	if data == "" {
		t.Fatal("stream did not send a topology event")
	}
	var snapshot topology.Snapshot
	if err := json.Unmarshal([]byte(data), &snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Namespace != "weave" {
		t.Fatalf("stream namespace = %q", snapshot.Namespace)
	}
}
