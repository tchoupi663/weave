package live

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	flowpb "github.com/cilium/cilium/api/v1/flow"
	"github.com/tchoupi663/weave/services/weave/internal/topology"
	collectortrace "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

func stringKV(key, value string) *commonpb.KeyValue {
	return &commonpb.KeyValue{Key: key, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: value}}}
}

func TestOTLPTraceIngest(t *testing.T) {
	store := topology.NewStore("weave", 5*time.Minute)
	now := time.Now().UTC()
	frontend := &flowpb.Endpoint{
		Namespace: "weave",
		PodName:   "frontend-abc",
		Workloads: []*flowpb.Workload{{Name: "frontend", Kind: "Deployment"}},
		Labels:    []string{"k8s:app=frontend", "gen:io.cilium.k8s.named-ports=https.TCP.443"},
	}
	gateway := &flowpb.Endpoint{
		Namespace: "weave",
		PodName:   "gateway-abc",
		Workloads: []*flowpb.Workload{{Name: "gateway", Kind: "Deployment"}},
		Labels:    []string{"k8s:app=gateway", "gen:io.cilium.k8s.named-ports=http.TCP.8080"},
	}
	store.Ingest(&flowpb.Flow{
		Time:        timestamppb.New(now),
		Uuid:        "l4-frontend-gateway",
		Verdict:     flowpb.Verdict_FORWARDED,
		Type:        flowpb.FlowType_L3_L4,
		IP:          &flowpb.IP{Source: "10.244.1.130", Destination: "10.244.3.214"},
		Source:      frontend,
		Destination: gateway,
		IsReply:     wrapperspb.Bool(false),
		L4: &flowpb.Layer4{Protocol: &flowpb.Layer4_TCP{TCP: &flowpb.TCP{
			DestinationPort: 8080,
			Flags:           &flowpb.TCPFlags{SYN: true},
		}}},
	})
	server := httptest.NewServer(testLiveHandler(t, store))
	defer server.Close()

	request := &collectortrace.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{{
			Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{
				stringKV("k8s.namespace.name", "weave"),
				stringKV("k8s.deployment.name", "frontend"),
				stringKV("k8s.pod.name", "frontend-abc"),
				stringKV("service.name", "frontend"),
			}},
			ScopeSpans: []*tracepb.ScopeSpans{{
				Spans: []*tracepb.Span{{
					TraceId:           []byte{1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
					SpanId:            []byte{1, 2, 3, 4, 5, 6, 7, 8},
					Name:              "GET /api/products",
					Kind:              tracepb.Span_SPAN_KIND_CLIENT,
					StartTimeUnixNano: uint64(now.Add(-3 * time.Millisecond).UnixNano()),
					EndTimeUnixNano:   uint64(now.UnixNano()),
					Attributes: []*commonpb.KeyValue{
						stringKV("http.request.method", "GET"),
						stringKV("url.path", "/api/products/p001"),
						stringKV("server.address", "gateway"),
						stringKV("server.port", "8080"),
						stringKV("http.response.status_code", "200"),
					},
				}},
			}},
		}},
	}
	body, err := proto.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.Post(server.URL+"/v1/traces", "application/x-protobuf", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("OTLP status = %d", response.StatusCode)
	}

	topologyResponse, err := http.Get(server.URL + "/api/topology")
	if err != nil {
		t.Fatal(err)
	}
	defer topologyResponse.Body.Close()
	var snapshot topology.Snapshot
	if err := json.NewDecoder(topologyResponse.Body).Decode(&snapshot); err != nil {
		t.Fatal(err)
	}
	if snapshot.Totals.L7Edges != 1 || snapshot.Totals.L7Requests != 1 {
		t.Fatalf("L7 totals = %+v", snapshot.Totals)
	}
	if len(snapshot.Edges) != 1 || snapshot.Edges[0].L7 == nil {
		t.Fatalf("L7 edge missing: %+v", snapshot.Edges)
	}
	edge := snapshot.Edges[0]
	if edge.L7.Routes[0].Path != "/api/products/:id" || edge.L7.Status.Success != 1 {
		t.Fatalf("L7 edge = %+v", edge.L7)
	}
}

func TestOTLPServerSpanMapsPeerToSourceWorkload(t *testing.T) {
	store := topology.NewStore("weave", 5*time.Minute)
	now := time.Now().UTC()
	frontend := &flowpb.Endpoint{
		Namespace: "weave",
		PodName:   "frontend-abc",
		Workloads: []*flowpb.Workload{{Name: "frontend", Kind: "Deployment"}},
		Labels:    []string{"k8s:app=frontend", "gen:io.cilium.k8s.named-ports=https.TCP.443"},
	}
	gateway := &flowpb.Endpoint{
		Namespace: "weave",
		PodName:   "gateway-abc",
		Workloads: []*flowpb.Workload{{Name: "gateway", Kind: "Deployment"}},
		Labels:    []string{"k8s:app=gateway", "gen:io.cilium.k8s.named-ports=http.TCP.8080"},
	}
	store.Ingest(&flowpb.Flow{
		Time:        timestamppb.New(now),
		Uuid:        "l4-server",
		Verdict:     flowpb.Verdict_FORWARDED,
		Type:        flowpb.FlowType_L3_L4,
		IP:          &flowpb.IP{Source: "10.244.1.130", Destination: "10.244.3.214"},
		Source:      frontend,
		Destination: gateway,
		IsReply:     wrapperspb.Bool(false),
		L4: &flowpb.Layer4{Protocol: &flowpb.Layer4_TCP{TCP: &flowpb.TCP{
			DestinationPort: 8080,
			Flags:           &flowpb.TCPFlags{SYN: true},
		}}},
	})

	server := httptest.NewServer(testLiveHandler(t, store))
	defer server.Close()

	request := &collectortrace.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{{
			Resource: &resourcepb.Resource{Attributes: []*commonpb.KeyValue{
				stringKV("k8s.namespace.name", "weave"),
				stringKV("k8s.deployment.name", "gateway"),
				stringKV("k8s.pod.name", "gateway-abc"),
				stringKV("service.name", "gateway"),
			}},
			ScopeSpans: []*tracepb.ScopeSpans{{
				Spans: []*tracepb.Span{{
					TraceId:           []byte{2, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16},
					SpanId:            []byte{2, 2, 3, 4, 5, 6, 7, 8},
					Name:              "GET /api/products",
					Kind:              tracepb.Span_SPAN_KIND_SERVER,
					StartTimeUnixNano: uint64(now.Add(-2 * time.Millisecond).UnixNano()),
					EndTimeUnixNano:   uint64(now.UnixNano()),
					Attributes: []*commonpb.KeyValue{
						stringKV("http.request.method", "GET"),
						stringKV("url.path", "/api/products"),
						stringKV("client.address", "frontend"),
						stringKV("server.address", "gateway"),
						stringKV("server.port", "8080"),
						stringKV("http.response.status_code", "200"),
					},
				}},
			}},
		}},
	}
	body, err := proto.Marshal(request)
	if err != nil {
		t.Fatal(err)
	}
	response, err := http.Post(server.URL+"/v1/traces", "application/x-protobuf", bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("OTLP status = %d", response.StatusCode)
	}

	topologyResponse, err := http.Get(server.URL + "/api/topology")
	if err != nil {
		t.Fatal(err)
	}
	defer topologyResponse.Body.Close()
	var snapshot topology.Snapshot
	if err := json.NewDecoder(topologyResponse.Body).Decode(&snapshot); err != nil {
		t.Fatal(err)
	}
	if len(snapshot.Edges) != 1 || snapshot.Edges[0].L7 == nil {
		t.Fatalf("L7 edge missing: %+v", snapshot.Edges)
	}
	edge := snapshot.Edges[0]
	if edge.Source != "workload:weave/frontend" || edge.Target != "workload:weave/gateway" {
		t.Fatalf("server span mapped to %s -> %s", edge.Source, edge.Target)
	}
	if edge.L7.LatencySource != "server" || edge.L7.Requests != 1 {
		t.Fatalf("server L7 = %+v", edge.L7)
	}
}

func testLiveHandler(t *testing.T, store *topology.Store) http.Handler {
	t.Helper()
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	feed := NewFeed("", time.Second, 100, log)
	return (&Live{log: log, store: store, feed: feed}).Handler()
}
