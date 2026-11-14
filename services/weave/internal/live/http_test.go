package live

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	flowpb "github.com/cilium/cilium/api/v1/flow"
	"github.com/tchoupi663/weave/services/weave/internal/contracts"
	"github.com/tchoupi663/weave/services/weave/internal/topology"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func TestWorkloadFeedAndHealth(t *testing.T) {
	store := topology.NewStore("weave", 5*time.Minute)
	now := time.Now().UTC()
	store.Ingest(&flowpb.Flow{
		Time:        timestamppb.New(now),
		Uuid:        "l4-feed",
		Verdict:     flowpb.Verdict_FORWARDED,
		Type:        flowpb.FlowType_L3_L4,
		IP:          &flowpb.IP{Source: "10.244.1.130", Destination: "10.244.3.214"},
		Source:      &flowpb.Endpoint{Namespace: "weave", PodName: "frontend-abc", Workloads: []*flowpb.Workload{{Name: "frontend", Kind: "Deployment"}}},
		Destination: &flowpb.Endpoint{Namespace: "weave", PodName: "gateway-abc", Workloads: []*flowpb.Workload{{Name: "gateway", Kind: "Deployment"}}},
		IsReply:     wrapperspb.Bool(false),
		L4: &flowpb.Layer4{Protocol: &flowpb.Layer4_TCP{TCP: &flowpb.TCP{
			DestinationPort: 8080,
			Flags:           &flowpb.TCPFlags{SYN: true},
		}}},
	})
	server := httptest.NewServer(testLiveHandler(t, store))
	defer server.Close()

	response, err := http.Get(server.URL + "/internal/v1/workloads")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("workloads status = %d", response.StatusCode)
	}
	var feed contracts.WorkloadsResponse
	if err := json.NewDecoder(response.Body).Decode(&feed); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, workload := range feed.Workloads {
		seen[workload.Workload] = true
	}
	if !seen["frontend"] || !seen["gateway"] {
		t.Fatalf("workloads = %+v", feed.Workloads)
	}

	healthResponse, err := http.Get(server.URL + "/health")
	if err != nil {
		t.Fatal(err)
	}
	defer healthResponse.Body.Close()
	var health map[string]any
	if err := json.NewDecoder(healthResponse.Body).Decode(&health); err != nil {
		t.Fatal(err)
	}
	if health["status"] != "degraded" {
		t.Fatalf("health status = %v, want degraded before the stream", health["status"])
	}
	if _, ok := health["history_connected"]; !ok {
		t.Fatalf("health is missing history_connected: %v", health)
	}
}
