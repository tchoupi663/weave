package inventory

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestArgoClientReadsDesiredAndLiveWorkloads(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/v1/applications/target-app-local/manifests":
			_ = json.NewEncoder(w).Encode(map[string]any{"manifests": []string{
				"apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: gateway\n  namespace: weave\n",
				"apiVersion: apps/v1\nkind: Deployment\nmetadata:\n  name: catalog\n  namespace: weave\n",
				"apiVersion: v1\nkind: Service\nmetadata:\n  name: gateway\n  namespace: weave\n",
			}})
		case "/api/v1/applications/target-app-local":
			_ = json.NewEncoder(w).Encode(map[string]any{"status": map[string]any{"resources": []map[string]any{
				{"kind": "Deployment", "name": "gateway", "namespace": "weave", "health": map[string]string{"status": "Healthy"}},
				{"kind": "Deployment", "name": "catalog", "namespace": "weave", "health": map[string]string{"status": "Missing"}},
			}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	client := NewArgoClient(server.URL, "test-token", "target-app-local", "weave")
	workloads, err := client.Workloads(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(workloads) != 2 {
		t.Fatalf("workloads = %+v", workloads)
	}
	byName := make(map[string]DeclaredWorkload)
	for _, workload := range workloads {
		byName[workload.Name] = workload
	}
	if !byName["gateway"].Live || byName["catalog"].Live {
		t.Fatalf("live states = %+v", byName)
	}
}
