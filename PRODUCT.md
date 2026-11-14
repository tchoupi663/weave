# Product

<!-- impeccable:product-schema 1 -->

## Platform

web

## Users

Platform and network engineers responding to a Kubernetes application incident.
Their working situation is a laptop beside a kind cluster, a simulator driving
the Target App, and Hubble and Beyla as the only sources of traffic truth. They
need to answer two questions quickly: did the graph change around the incident,
and where did the observed behavior start to diverge? They are not looking for
an alert, a dashboard of everything, or a causal verdict.

## Product Purpose

Weave constructs an observed topology from network traffic, retains a
time-bucketed history, and turns a manually selected incident window into a
bounded suspect region with evidence. It ingests Hubble L4 flows and HTTP
observations from Grafana Beyla, groups endpoints into workloads, reads the
Argo CD desired workload inventory read-only, and serves both the Operator
screen and a read API for a future Agent.

Success for the first slice is a 20-minute replay in which a two-minute Catalog
HTTP 500 incident produces the expected suspect region — Catalog and
Gateway → Catalog, with Frontend → Gateway as the cascade — while Auth and
Payment stay outside it. The incident, evidence, and history survive a Weave
restart.

## Positioning

Weave is observed-first. Traffic creates nodes and edges; Kubernetes metadata
names and groups what was already observed. Declared Architecture from Argo CD
is an object inventory for lifecycle comparison, never a source of observed
edges. The diagnosis is a suspect region, not a root cause.

## Operating Context

- Target App namespace: `weave`; Weave runs in `weave-system`.
- Cilium 1.20.4 with Hubble Relay is installed in `kube-system`.
- Grafana Beyla is a separate privileged eBPF DaemonSet in `beyla-system`; it
  instruments the gateway process without a proxy or Target App change.
- Argo CD manages the Target App from `deploy/envs/local` with a read-only token
  for Weave.
- Chaos Mesh injects the acceptance fault with an `HTTPChaos` object.
- The Simulator runs outside the cluster and enters through a local
  port-forward.
- Weave runs as four services in `weave-system`: `weave-live` (ingest and
  observed graph), `weave-history` (metrics, incidents, evidence, recording
  gaps), `weave-inventory` (declared workloads, lifecycle, change events), and
  `weave-api` (the built UI and the only browser origin). Each service has one
  owner and one upstream; `weave-history` and `weave-inventory` are the only
  stateful services, each on its own ReadWriteOnce PVC.
- The first screen is used on a desktop while a run is active, often beside a
  terminal and the Simulator report.

## Capabilities and Constraints

- Nodes are logical Kubernetes workloads plus a single `Source IP` boundary.
  Edges are ordered workload pairs plus destination port; replies fold into the
  request edge.
- The live graph is cumulative since start/reset with a five-minute activity
  window. History is separate: `weave-history` owns SQLite incidents, evidence,
  and recording gaps plus DuckDB edge metric buckets; `weave-inventory` owns
  SQLite workload state and change events.
- Retention: 5-second buckets for 24 hours, 1-minute rollups for 7 days, change
  events for 30 days. Raw flows and spans are not retained.
- L4 plus HTTP observations. Hubble gives connections, events, ports, verdicts,
  and first/last seen. Beyla adds requests, status classes, p50/p95 latency, and
  normalized routes on the HTTP edges.
- HTTP is additive. L4 connections/events are never summed with HTTP requests.
  Headers, query strings, and bodies are discarded.
- Five HTTP edges are observed: Frontend → Gateway and
  Gateway → Auth/Catalog/Orders/Payment. Source IP → Frontend (HTTPS),
  Orders → Postgres (TCP), and Payment → External Payment Provider (HTTPS) are
  connection-level only.
- Frontend → Gateway is server-observed; the four downstream HTTP edges are
  client-observed. The UI labels the latency source.
- An incident is a persisted, time-bounded investigation with a symptom,
  optional scope, status, notes, and a materialized evidence snapshot.
- Evidence uses a 15-minute baseline. An edge is anomalous when its 4xx or 5xx
  rate is at least 2× baseline and at least +5 percentage points, or when p95 is
  at least 2× baseline and at least +5 ms. The suspect region is the earliest
  connected anomalous region, not a root cause.
- The incident timeline is a 5-second scrub over suspect-edge frames. It starts
  at the 15-minute baseline so the transition from normal to affected is
  visible. It is not a full historical topology reconstruction.
- The declared source is Argo CD. Weave does not gain Kubernetes API RBAC.
- A service outage degrades only its own surface: no topology without
  `weave-live`, no incident data without `weave-history`, no change events
  without `weave-inventory`. Liveness and readiness never depend on another
  service. A history outage is recorded as a **Recording Gap**, not as zero
  traffic.
- The browser contract is frozen and served only by `weave-api`; services
  communicate over versioned internal HTTP contracts. Cilium Gateway API or an
  Ingress is a later front door for TLS and auth, not a change to the browser
  contract.
- No authentication in this slice. The surface is internal and reached through
  a local port-forward.
- The Target App is not modified by Weave or by the Beyla observer.

## Brand Commitments

- Product name: Weave.
- The Control AI case study at
  https://fuselabcreative.com/our-projects/control-ai/ is a binding composition
  and atmosphere reference.
- The visual world is an operator console at night: cool graphite, steel, a
  signal-blue accent, a restrained heat ramp for flow density, hairlines and
  knots. Dark-only for v1.
- No existing logo or brand assets were supplied. The wordmark is the product
  name; the mark is a simple woven glyph.

## Evidence on Hand

- `expected-topology.yaml`: nine nodes and eight edges for the healthy v1.
- A 12-second, 20-user smoke run produced 66 Bounce, 107 Browse, and 37 Order
  journeys with zero failures.
- Healthy runs produced 9 nodes, 8 edges, and 5 HTTP-bearing edges with about
  1,500 L7 requests.
- The 20-minute acceptance replay passed on 2026-11-14: 15 minutes healthy,
  2 minutes of Catalog `GET /api/products` HTTP 500, 3 minutes recovery.
- The suspect region was exactly Frontend → Gateway and Gateway → Catalog;
  Catalog, Frontend, and Gateway were the suspect nodes. Auth and Payment were
  outside. Both suspect edges had onset in the first fault bucket and reason
  `5xx rate`.
- Route evidence showed 2,008 5xx on `GET /api/products` for Frontend → Gateway
  and 2,004 for Gateway → Catalog; the other top-five routes had zero 5xx.
- The incident, its evidence, and its 24-frame timeline survived a Weave
  restart with identical JSON hashes. Restart history counts were
  `edge_5s=4134`, `edge_1m=514`, `changes=127`.
- A short run after restart restored 9 nodes, 8 edges, 5 HTTP edges, zero
  failures.
- The four-service cutover preserved the stored incident byte-identically:
  incident JSON hash
  `e950d501bf54733a1c16e4418da72e0207926627539d2e8eb9885cbbae9e2ad1` before and
  after; the timeline differs only by its additive `gaps` field.
- A short Simulator run against the four-service stack produced 9 nodes, 8 edges,
  5 HTTP edges, and 1,540 L7 requests with zero dropped observations.
- The degradation test passed: history down leaves topology up and incidents at
  502; inventory down leaves incidents and topology up and changes at 502; both
  recover, and the history outage becomes a Recording Gap.
- There is no production usage data. Do not invent request rates, latency,
  customers, or benchmark claims.

## Product Principles

1. Observation over declaration. A declared workload with no traffic is a ghost
   in lifecycle comparison, not an observed node or edge.
2. The graph is evidence. Every visible node, edge, and figure must correspond
   to something recorded.
3. Be honest about the instrument. Connections and events are not requests;
   latency names its observer; history names its bucket and retention.
4. A suspect is not a root cause. The output is a connected region and the
   evidence around its earliest onset.
5. Operator first. The screen is for a task, not a demo; atmosphere must not
   obscure state, selection, or the next action.
6. Keep the Target App untouched. Weave reads; the observer reads; the fault
   harness is the only writer and it lives outside the app.
7. Live and history are different instruments. The flow heatmap is the current
   five-minute window; the incident timeline is a recorded one.

## Accessibility & Inclusion

- Keyboard access to every control, graph node, edge, and incident action.
- Visible focus states on the dark surface.
- Text meets WCAG AA contrast; status is not carried by color alone.
- The graph and timeline have textual equivalents: the inspector, evidence
  panel, and labelled controls.
- The interface holds at 720px, but the working operator layout is 1024px and
  wider.
