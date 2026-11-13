# ADR-0003: Construct topology from observed flows, not declarations

- **Status:** Accepted (L4 decision; L7 added in ADR-0004; incident replay, history, and declared inventory amended by ADR-0005)
- **Date:** 2026-11-09
- **Context:** Weave v1, the actual topology app

## Context

The Target App runs in the `weave` namespace and Cilium/Hubble observes its
traffic. The simulator emits the expected topology independently. Weave could
either:

1. enumerate Deployments, Services, and Endpoints and draw the declared
   application model, using flows only to color the result; or
2. create nodes and edges only when traffic was actually observed, using
   Kubernetes metadata only to name and group what Hubble saw.

The first option would always produce a plausible graph, including services
that never spoke, and would make the benchmark measure a rendering of the
Kubernetes API rather than topology inference. It would also obscure missing
edges, which are exactly what a network-observability product must expose.

## Decision

Weave is observed-first.

- A node is a logical workload that appears in at least one qualifying flow.
  Pods and IPs are evidence attached to that node, not nodes themselves.
- An edge is an ordered workload pair plus destination port. Replies and
  handshake packets fold into that edge.
- Nodes are grouped by the workload metadata carried by Hubble; no Kubernetes
  API access is used in v1.
- All non-cluster traffic into the exposed Frontend collapses to one
  `Source IP` node. Host traffic to other watched workloads is kubelet
  probe/control traffic and is dropped.
- The graph is cumulative in memory since start/reset, with a five-minute
  activity window. There is no database.
- The topology remains L4-first. Hubble flow records do not carry bytes,
  request counts, latency, or HTTP status. HTTP L7 evidence was added later by
  a separate eBPF observer; see ADR-0004. It is attached to the L4 edges and is
  never used to create edges that L4 did not observe.

## Consequences

- A silent service is absent, not grayed out. That is the intended signal.
- A service reached through a long-lived connection can have zero counted TCP
  opens while having observed events; the inspector reports both figures and
  explains them.
- The Source IP address seen through a local port-forward is the kind host
  endpoint, not the laptop. The node is still the correct topology boundary,
  and the inspector shows the observed address and the proxy situation.
- Weave cannot be used to inventory the cluster. It is a traffic map.
- The expected topology remains an external benchmark input; Weave never reads
  it.

## Amendment

ADR-0005 changes four parts of this decision for the incident-replay slice:

- host/world traffic is no longer restricted to the exposed Frontend;
- declared workloads from Argo CD are available as ghost nodes and lifecycle
  states, without becoming observed Nodes or Edges;
- persistent History and a database replace the in-memory-only statement; and
- Incidents, a suspect region, and evidence snapshots are added.

The core decision remains: the operational graph is built from observed
connections, and `expected-topology.yaml` is never the declared architecture.

## Alternatives considered

- **Declared topology with flow overlays.** Rejected: it would hide missing
  traffic and make the benchmark tautological.
- **Pod-level nodes.** Rejected: replica churn makes the topology unstable and
  would not match the workload-level expected graph.
- **One edge per flow.** Rejected: it produces thousands of edges and loses the
  topology.
- **Cilium L7 policy for request telemetry.** Rejected in ADR-0004: the Envoy
  proxy exceeded the performance gate and policy removal disrupted live
  connections.
- **Prometheus for bytes.** Deferred: the first slice has no metrics stack and
  the graph is about connectivity, not throughput.
