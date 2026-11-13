# ADR-0005: Incident replay from observed history and declared inventory

- **Status:** Accepted
- **Date:** 2026-11-13
- **Context:** Weave incident-response slice
- **Amends:** ADR-0003 (observed-only truth, no database, host filtering)

## Context

Weave v1 is a live, in-memory topology view. It has no retained history, no
incident object, no declared architecture, and no way to replay a past window.
It is also hard-coded to the Target App: the external ingress rule, display
names, the `external` role, and the integration assertions all assume this
fixture.

The next product direction is different: an incident responder should open a
time-bounded investigation, see the graph change around that window, and be
taken to a suspect region instead of searching the entire graph. Architecture
and telemetry evolution are the second feature, not the first.

The current cluster has no Argo CD, no Chaos Mesh, no history database, and no
fault-injection harness. The live Target App was installed manually by Helm
with chart defaults; the repository's local environment declares a different
Frontend Service type.

## Decision

### Generic observed topology

- Weave remains a single-namespace tool. The Target App is only the fixture.
- Every observed connection is eligible, including host and world traffic.
  There is no entrypoint declaration and no hard-coded `frontend:443`
  exception. Kubelet/probe edges may be visible.
- Non-cluster sources aggregate into one `Source IP` boundary node.
- Workload names come from Kubernetes labels or annotations; roles are inferred
  from topology and workload kind. The Target App display-name map and
  `external` literal are not product logic.
- Declared architecture does not create observed edges. L4 remains the source
  of truth for the graph.

### Declared architecture

- The declared source is the **Argo CD desired state** for the Target App,
  read through the Argo CD API with a read-only token. Weave does not gain
  Kubernetes API RBAC.
- Declared architecture is an **object inventory**, not a call graph. The chart
  declares workloads and Services; it does not declare edges.
- A declared workload with no observed traffic is a `Declared` ghost in the
  architecture/comparison view. It is not a Node in the incident canvas.
- Lifecycle states are `Declared`, `Active`, `Inactive`, `Undeclared`, and
  `Removed`.
  - `Inactive`: no qualifying traffic for 30 minutes while still declared/live.
  - `Removed`: no longer declared or no longer live. Silence alone never means
    removed.
- Change Events are separate from L4 Events and keep their provenance:
  `declared`, `live`, or `observed`.

### History and storage

- One Weave process and one PVC.
- SQLite stores Incidents, Change Events, and declared/workload state.
- DuckDB stores edge metric buckets.
- Retention: 5-second buckets for 24 hours, 1-minute rollups for 7 days, Change
  Events for 30 days. No raw flows or spans are retained.
- Incident evidence is a **versioned, materialized snapshot** in SQLite. It
  references DuckDB buckets rather than copying them. It records algorithm
  version, baseline, per-edge anomaly intervals, onset order, and the suspect
  region.

### Incident and localization

- An Incident is a persisted, manually created, time-bounded investigation with
  a symptom, optional structured scope, status, and notes.
- An Incident is not an alert and not an anomaly.
- Baseline: the 15 minutes before the Incident Window.
- Anomaly: 4xx or 5xx rate at least 2× baseline and at least +5 percentage
  points, or latency p95 at least 2× baseline. L4-only edges contribute
  connectivity and traffic evidence, not HTTP error rates.
- Localization: find the earliest anomalous 5-second bucket; seed the suspect
  region with those edges; expand through connected anomalous edges. Ties
  within one bucket stay in the region. Order by onset, then severity, then
  topology depth for display only.
- The result is a connected suspect region of Nodes and Edges. It is not a
  single root cause.
- Route detail (`method` + normalized path) is supporting evidence; route-level
  anomalies are not first-class in this slice.
- Default incident view: suspect-region canvas, onset/evidence timeline, a
  compact lifecycle change strip, and expandable full graph. Minimal controls:
  timeline scrub, expand neighbourhood, note, close/reopen.
- No separate architecture-evolution screen and no Agent UI in this slice.

### API and consumers

- Operator and Agent share one read-only, incident-centric REST model:
  - `POST /api/incidents`
  - `GET /api/incidents`
  - `GET /api/incidents/{id}`
  - `PATCH /api/incidents/{id}`
  - `POST /api/incidents/{id}/recompute`
  - `GET /api/topology?at=...`
  - `GET /api/changes?from=...&to=...`
- No authentication in this slice; the API is internal/port-forward only. Add a
  token before an Agent leaves that boundary.
- The Operator UI and Agent consume the same incident/evidence resources.

### Fault replay and delivery

- First fault: Catalog `GET /api/products` returns HTTP 500 for two minutes at
  100%, after a 15-minute healthy baseline. Total replay: 20 minutes.
- Injection: minimal Chaos Mesh `HTTPChaos`; no Target App source change.
- Traffic: fix the Simulator's grace timer, which currently ends every run at
  60 seconds regardless of duration, and run one continuous replay.
- Accept the result when the suspect region contains Catalog and
  Gateway → Catalog, with Frontend → Gateway as the later cascade; Payment and
  Auth are outside it.
- Deployment: minimal Argo CD (no HA, Dex, or dashboard) and minimal Chaos Mesh
  (no dashboard). Target App is reinstalled through Argo from
  `deploy/envs/local`, replacing the manual Helm release. Orders Postgres data
  may be discarded. Weave and Beyla remain platform releases.

## Consequences

- Weave gains persistent state and a database migration path. The in-memory
  live path remains for the current view.
- Argo CD and Chaos Mesh become platform dependencies with their own resource
  cost and failure modes.
- The Target App will be recreated under Argo, with a deliberate NodePort and
  fresh data.
- Host/probe edges become visible because all connections count.
- Declared edges are absent; edge drift cannot be detected until a later
  explicit graph declaration exists.
- Payment → External remains L4-only and cannot provide HTTP error rates. The
  first fault uses an L7-observed edge.
- The localization output is evidence and a suspect region, not a causal root
  cause.
- No auth; the API is safe only inside the port-forward boundary.

## Alternatives considered

- **Stay observed-only and live-only.** Rejected: no history, no incident, no
  evolution.
- **Use declared architecture as the graph base.** Rejected: unobserved
  services would become architecture and missing traffic would be hidden.
- **Use `expected-topology.yaml` as declared architecture.** Rejected: it is a
  Simulator benchmark oracle, not a general declaration.
- **Declare edges in the first slice.** Rejected: Kubernetes objects do not
  declare calls; an explicit graph is a later feature.
- **Retain raw flows and spans.** Rejected: high cardinality and storage cost;
  bucketed aggregates and evidence snapshots suffice for incident replay.
- **Alert-driven incidents.** Rejected for the first slice: no alerting stack
  and no stable incident trigger.
- **Statistical causal inference.** Rejected: overclaims causality from this
  telemetry.
- **Cilium L7 proxy for edge status.** Rejected in ADR-0004 for performance and
  connection disruption.
- **Reuse the healthy Simulator.** Rejected: it has no faults; a separate
  harness with ground truth is required.
- **Adopt the existing Helm resources into Argo.** Rejected for this slice:
  clean ownership is more valuable than preserving testbed data.
