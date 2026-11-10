package topology

import (
	"testing"
	"time"

	flowpb "github.com/cilium/cilium/api/v1/flow"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"
)

const testNamespace = "weave"

func endpoint(ns, pod, workload, kind string, labels ...string) *flowpb.Endpoint {
	return &flowpb.Endpoint{
		Namespace: ns,
		PodName:   pod,
		Workloads: []*flowpb.Workload{{Name: workload, Kind: kind}},
		Labels:    labels,
	}
}

func hostEndpoint() *flowpb.Endpoint {
	return &flowpb.Endpoint{Labels: []string{"reserved:host"}}
}

func tcpFlow(src, dst *flowpb.Endpoint, srcIP, dstIP string, port uint32, syn, reply bool, at time.Time, uuid string) *flowpb.Flow {
	flags := &flowpb.TCPFlags{}
	if syn {
		flags.SYN = true
	} else {
		flags.ACK = true
	}
	return &flowpb.Flow{
		Time:        timestamppb.New(at),
		Uuid:        uuid,
		Verdict:     flowpb.Verdict_FORWARDED,
		Type:        flowpb.FlowType_L3_L4,
		IP:          &flowpb.IP{Source: srcIP, Destination: dstIP},
		Source:      src,
		Destination: dst,
		IsReply:     wrapperspb.Bool(reply),
		L4: &flowpb.Layer4{
			Protocol: &flowpb.Layer4_TCP{TCP: &flowpb.TCP{
				DestinationPort: port,
				Flags:           flags,
			}},
		},
	}
}

func TestIngestSourceIPAndInternalEdges(t *testing.T) {
	store := NewStore(testNamespace, 5*time.Minute)
	now := time.Now().UTC()

	frontend := endpoint(testNamespace, "frontend-6f8c7b9d4f-abcde", "frontend", "Deployment", "gen:io.cilium.k8s.named-ports=https.TCP.443")
	gateway := endpoint(testNamespace, "gateway-7d9f8c6b5d-abcde", "gateway", "Deployment", "gen:io.cilium.k8s.named-ports=http.TCP.8080")
	auth := endpoint(testNamespace, "auth-5f7d9c8b6a-abcde", "auth", "Deployment", "gen:io.cilium.k8s.named-ports=http.TCP.8081")
	postgres := endpoint(testNamespace, "postgres-0", "postgres", "StatefulSet", "gen:io.cilium.k8s.named-ports=postgres.TCP.5432")

	if !store.Ingest(tcpFlow(hostEndpoint(), frontend, "10.244.1.4", "10.244.1.130", 443, true, false, now.Add(-2*time.Second), "u1")) {
		t.Fatal("source flow was rejected")
	}
	if !store.Ingest(tcpFlow(frontend, gateway, "10.244.1.130", "10.244.3.214", 8080, true, false, now.Add(-time.Second), "u2")) {
		t.Fatal("frontend -> gateway was rejected")
	}
	if !store.Ingest(tcpFlow(gateway, auth, "10.244.3.214", "10.244.3.227", 8081, true, false, now.Add(-time.Second), "u3")) {
		t.Fatal("gateway -> auth was rejected")
	}
	if !store.Ingest(tcpFlow(gateway, postgres, "10.244.3.214", "10.244.3.166", 5432, true, false, now.Add(-time.Second), "u4")) {
		t.Fatal("gateway -> postgres was rejected")
	}

	snapshot := store.SnapshotAt(now)
	if snapshot.Totals.Nodes != 5 {
		t.Fatalf("nodes = %d, want 5", snapshot.Totals.Nodes)
	}
	if snapshot.Totals.Edges != 4 {
		t.Fatalf("edges = %d, want 4", snapshot.Totals.Edges)
	}
	if snapshot.Totals.Connections != 4 {
		t.Fatalf("connections = %d, want 4", snapshot.Totals.Connections)
	}

	var source *Node
	for i := range snapshot.Nodes {
		if snapshot.Nodes[i].ID == "source-ip" {
			source = &snapshot.Nodes[i]
		}
	}
	if source == nil {
		t.Fatal("source-ip node missing")
	}
	if source.Events != 1 || source.Connections != 1 {
		t.Fatalf("source totals = %d events / %d connections, want 1/1", source.Events, source.Connections)
	}
	if len(source.IPs) != 1 || source.IPs[0] != "10.244.1.4" {
		t.Fatalf("source IPs = %v, want [10.244.1.4]", source.IPs)
	}

	protocols := map[string]string{}
	for _, edge := range snapshot.Edges {
		protocols[edge.Source+"->"+edge.Target] = edge.Protocol
	}
	if protocols["source-ip->workload:weave/frontend"] != "HTTPS" {
		t.Fatalf("source protocol = %q, want HTTPS", protocols["source-ip->workload:weave/frontend"])
	}
	if protocols["workload:weave/frontend->workload:weave/gateway"] != "HTTP" {
		t.Fatalf("frontend protocol = %q, want HTTP", protocols["workload:weave/frontend->workload:weave/gateway"])
	}
	if protocols["workload:weave/gateway->workload:weave/postgres"] != "TCP" {
		t.Fatalf("postgres protocol = %q, want TCP", protocols["workload:weave/gateway->workload:weave/postgres"])
	}

	if len(snapshot.Activity) != snapshotBuckets {
		t.Fatalf("activity length = %d, want %d", len(snapshot.Activity), snapshotBuckets)
	}
}

func TestIngestDropsProbesRepliesAndControlPlane(t *testing.T) {
	store := NewStore(testNamespace, 5*time.Minute)
	now := time.Now().UTC()

	frontend := endpoint(testNamespace, "frontend-6f8c7b9d4f-abcde", "frontend", "Deployment", "gen:io.cilium.k8s.named-ports=https.TCP.443")
	auth := endpoint(testNamespace, "auth-5f7d9c8b6a-abcde", "auth", "Deployment", "gen:io.cilium.k8s.named-ports=http.TCP.8081")
	dns := endpoint("kube-system", "coredns-559f6c778d-prf74", "coredns", "Deployment", "gen:io.cilium.k8s.named-ports=dns.UDP.53")

	cases := []struct {
		name string
		flow *flowpb.Flow
	}{
		{
			name: "kubelet probe to auth",
			flow: tcpFlow(hostEndpoint(), auth, "10.244.1.4", "10.244.3.227", 8081, true, false, now, "probe"),
		},
		{
			name: "reply from frontend",
			flow: tcpFlow(frontend, hostEndpoint(), "10.244.1.130", "10.244.1.4", 443, false, true, now, "reply"),
		},
		{
			name: "dns to kube-system",
			flow: tcpFlow(frontend, dns, "10.244.1.130", "10.244.0.167", 53, true, false, now, "dns"),
		},
	}
	for _, tc := range cases {
		if store.Ingest(tc.flow) {
			t.Fatalf("%s was accepted", tc.name)
		}
	}
	if snapshot := store.SnapshotAt(now); snapshot.Totals.Nodes != 0 || snapshot.Totals.Edges != 0 {
		t.Fatalf("store is not empty after rejected flows: %+v", snapshot.Totals)
	}
}

func TestIngestDeduplicatesFlowUUID(t *testing.T) {
	store := NewStore(testNamespace, 5*time.Minute)
	now := time.Now().UTC()
	frontend := endpoint(testNamespace, "frontend-6f8c7b9d4f-abcde", "frontend", "Deployment", "gen:io.cilium.k8s.named-ports=https.TCP.443")

	flow := tcpFlow(hostEndpoint(), frontend, "10.244.1.4", "10.244.1.130", 443, true, false, now, "same")
	if !store.Ingest(flow) {
		t.Fatal("first flow rejected")
	}
	if store.Ingest(flow) {
		t.Fatal("duplicate flow accepted")
	}
	snapshot := store.SnapshotAt(now)
	if snapshot.Totals.Events != 1 || snapshot.Hubble.Observed != 1 {
		t.Fatalf("events = %d, observed = %d, want 1/1", snapshot.Totals.Events, snapshot.Hubble.Observed)
	}
}

func TestExpectedTopologyShape(t *testing.T) {
	store := NewStore(testNamespace, 5*time.Minute)
	now := time.Now().UTC()

	source := hostEndpoint()
	frontend := endpoint(testNamespace, "frontend-6f8c7b9d4f-abcde", "frontend", "Deployment", "gen:io.cilium.k8s.named-ports=https.TCP.443")
	gateway := endpoint(testNamespace, "gateway-7d9f8c6b5d-abcde", "gateway", "Deployment", "gen:io.cilium.k8s.named-ports=http.TCP.8080")
	auth := endpoint(testNamespace, "auth-5f7d9c8b6a-abcde", "auth", "Deployment", "gen:io.cilium.k8s.named-ports=http.TCP.8081")
	catalog := endpoint(testNamespace, "catalog-6df6b69b8d-abcde", "catalog", "Deployment", "gen:io.cilium.k8s.named-ports=http.TCP.8082")
	orders := endpoint(testNamespace, "orders-84dd9d5b5d-abcde", "orders", "Deployment", "gen:io.cilium.k8s.named-ports=http.TCP.8083")
	payment := endpoint(testNamespace, "payment-9f7c549d7-abcde", "payment", "Deployment", "gen:io.cilium.k8s.named-ports=http.TCP.8084")
	external := endpoint(testNamespace, "external-699949bcf9-abcde", "external", "Deployment", "gen:io.cilium.k8s.named-ports=https.TCP.8443")
	postgres := endpoint(testNamespace, "postgres-0", "postgres", "StatefulSet", "gen:io.cilium.k8s.named-ports=postgres.TCP.5432")

	type edge struct {
		src, dst *flowpb.Endpoint
		port     uint32
	}
	edges := []edge{
		{source, frontend, 443},
		{frontend, gateway, 8080},
		{gateway, auth, 8081},
		{gateway, catalog, 8082},
		{gateway, orders, 8083},
		{gateway, payment, 8084},
		{orders, postgres, 5432},
		{payment, external, 8443},
	}
	for i, e := range edges {
		uuid := "edge-" + string(rune('a'+i))
		if !store.Ingest(tcpFlow(e.src, e.dst, "10.0.0.1", "10.0.0.2", e.port, true, false, now.Add(-time.Duration(i)*time.Second), uuid)) {
			t.Fatalf("edge %d was rejected", i)
		}
	}

	snapshot := store.SnapshotAt(now)
	if snapshot.Totals.Nodes != 9 {
		t.Fatalf("nodes = %d, want 9", snapshot.Totals.Nodes)
	}
	if snapshot.Totals.Edges != 8 {
		t.Fatalf("edges = %d, want 8", snapshot.Totals.Edges)
	}
	if snapshot.Totals.Connections != 8 || snapshot.Totals.Events != 8 {
		t.Fatalf("totals = %+v, want 8 connections and 8 events", snapshot.Totals)
	}

	roles := map[string]string{}
	for _, node := range snapshot.Nodes {
		roles[node.ID] = node.Role
	}
	if roles["source-ip"] != RoleSource {
		t.Fatalf("source role = %q", roles["source-ip"])
	}
	if roles["workload:weave/gateway"] != RoleHub {
		t.Fatalf("gateway role = %q, want hub", roles["workload:weave/gateway"])
	}
	if roles["workload:weave/postgres"] != RoleData {
		t.Fatalf("postgres role = %q, want data", roles["workload:weave/postgres"])
	}
	if roles["workload:weave/external"] != RoleExternal {
		t.Fatalf("external role = %q, want external", roles["workload:weave/external"])
	}

	store.Reset()
	if snapshot := store.SnapshotAt(now); snapshot.Totals.Nodes != 0 || snapshot.Totals.Edges != 0 {
		t.Fatalf("reset left topology behind: %+v", snapshot.Totals)
	}
}
