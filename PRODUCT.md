# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

Platform and network engineers validating what traffic actually crosses a
Kubernetes application. Their working situation is a laptop beside a kind
cluster, a simulator driving the Target App, and Hubble as the only source of
truth about service-to-service traffic. They need to answer one question
quickly: which workloads talked to which, over what, and is that what the
application is supposed to do?

## Product Purpose

Weave constructs a live topology from observed network traffic. It ingests
Hubble L4 flows and HTTP observations from Grafana Beyla, groups
endpoints into workloads, and presents the resulting graph and edge telemetry
as an operator screen. Success for the first slice is a graph that, after a
Simulator run against the Target App, contains the nine Expected Topology nodes
and eight edges without reading the expected graph, with HTTP request, status,
latency, and route detail on the five HTTP edges.

## Positioning

Weave is observed-first. Traffic creates nodes and edges; Kubernetes metadata
is used only to name, group, and label what was already observed. It does not
render a declared service catalogue and call it a topology.

## Operating Context

- Target App namespace: `weave`.
- Weave runs in `weave-system` and watches one namespace at a time.
- Cilium 1.20.4 with Hubble Relay is installed in `kube-system`.
- Grafana Beyla runs as a separate privileged eBPF DaemonSet in
  `beyla-system`; it reads `gateway` client and server HTTP calls without
  inserting a proxy or changing the Target App.
- The Simulator runs outside the cluster and enters the Target App through a
  local port-forward.
- The first screen is used on a desktop while a run is active, often beside a
  terminal and the Simulator report.

## Capabilities and Constraints

- Nodes are logical Kubernetes workloads, plus a single `Source IP` node for
  traffic entering from outside the observed namespace.
- Edges are ordered workload pairs plus destination port. Replies fold into the
  request edge.
- State is in-memory: cumulative since start/reset, plus a five-minute activity
  window. There is no database.
- L4 plus HTTP L7 in v1. Hubble gives connection and flow-event observations,
  ports, verdicts, and first/last seen. Beyla adds HTTP requests, status
  classes, p50/p95 latency, and normalized routes for the five HTTP edges:
  Frontend → Gateway and Gateway → Auth, Catalog, Orders, Payment.
- L7 is additive. L4 connections/events remain separate from L7 requests;
  Weave never sums them.
- L7 is HTTP only. Headers, query strings, and bodies are discarded. Requests
  with no client span stay L4-only. Source IP → Frontend (HTTPS), Orders →
  Postgres (TCP), and Payment → External Payment Provider (HTTPS) remain L4-only.
- Four L7 edges are client-observed. `Frontend → Gateway` is server-observed
  because the gateway-only observer avoided the frontend instrumentation that
  exceeded the performance gate. The API and UI label the latency source.
- The Frontend Service is reached through a port-forward; Hubble reports the
  host endpoint as the source. Weave treats host traffic to `frontend:443` as
  the `Source IP` edge and drops host traffic to other workloads as kubelet
  probes.
- A Cilium L7 visibility policy was evaluated and rejected: its Envoy proxy
  added a 15.5% weighted p95 regression. Beyla was selected because it keeps
  the Target App data path unchanged and stayed within the 5% performance
  gate in the measured run.
- No authentication in v1. The surface is internal and reached through a local
  port-forward.
- The Target App is not modified by Weave or by the Beyla observer.

## Brand Commitments

- Product name: Weave.
- The Control AI case study at
  https://fuselabcreative.com/our-projects/control-ai/ is a binding
  composition and atmosphere reference.
- The visual world is an operator console at night: cool graphite, steel, a
  single signal-blue accent, hairlines and knots. Dark-only for v1.
- No existing logo or brand assets were supplied. The wordmark is the product
  name; the mark is a simple woven glyph.

## Evidence on Hand

- `expected-topology.yaml`: nine nodes and eight edges for the healthy v1.
- A 12-second, 20-user smoke run against the deployed Target App produced
  66 Bounce, 107 Browse, and 37 Order journeys with zero failures.
- The smoke run produced 8,096 Hubble flows. After folding replies and
  excluding kube-system DNS and kubelet probes, the observed edge set matched
  the expected eight edges.
- A 12-second smoke run produced 1,265 HTTP observations across all five
  L7-capable edges.
- A gateway-only, 60-second, 50-user Beyla run produced five L7 edges and 1,573
  HTTP observations with zero failed journeys and a 0.3% weighted p95
  regression against the same-run baseline. Per-step p95 is noisy at low
  counts; absolute changes were small.
- There is no production usage data. Do not invent request rates, latency,
  byte counts, customers, or benchmark claims.

## Product Principles

1. Observation over declaration. If no traffic created it, it is not in the
   graph.
2. The graph is evidence. Every visible node, edge, and figure must correspond
   to something Weave observed.
3. Be honest about the instrument. L4 observations are connections and events.
   L7 observations are HTTP requests from gateway client/server spans; they are
   never added to L4 counts, and the UI names whether latency is client- or
   server-observed.
4. Operator first. The screen is for a task, not a demo; atmosphere must not
   obscure state or selection.
5. Keep the Target App untouched. Weave reads; a separate eBPF observer reads;
   neither polices nor rewrites the application it observes.

## Accessibility & Inclusion

- Keyboard access to every control and to graph nodes and edges.
- Visible focus states on the dark surface.
- Text meets WCAG AA contrast; status is not carried by color alone.
- The graph has a textual inspector equivalent for selection and counts.
