# ADR-0006: Decompose Weave into four cooperating services

- **Status:** Accepted; implemented 2026-11-14
- **Date:** 2026-11-14
- **Context:** Weave runtime and service boundaries
- **Supersedes:** the storage and deployment decision in ADR-0005 ("one Weave process and one PVC")

## Context

Weave is one Go process that follows Hubble Relay, accepts Beyla OTLP traces,
maintains the in-memory live graph, records SQLite and DuckDB history,
reconciles declared workloads from Argo CD, builds incident evidence, and serves
the HTTP/SSE API and the built React UI from the same image. It runs as one
Deployment with `replicaCount: 1` and `Recreate`, on one 2 GiB RWO PVC holding
`weave.db` and `metrics.duckdb`.

That shape was a correct fit for the incident-replay slice. It is a monolith by
deployment, code, and data: the recorder is called inside the topology write
lock, incident creation joins DuckDB metrics and the SQLite evidence store in
one operation, lifecycle reconciliation reads the live graph and writes the
same SQLite file as incidents, and the browser is served by the same process as
the API. None of those seams is visible or enforceable as a boundary.

The decision here is not a response to a scaling, team, or release bottleneck —
there is one operator, one laptop, and no production load. The goal is an
architecture demonstration: Weave's own system becomes a worked example of
service boundaries, private data ownership, network contracts, independent
deployability, and failure isolation. The cost is real and accepted: more
images, more pods, two PVCs, and more moving parts than a 5,000-line app needs.

Constraints that bound the decision:

- Runs on the operator's laptop beside the kind cluster.
- One command brings Weave up; one port-forward reaches the UI; the UI keeps a
  single same-origin browser contract.
- The 20-minute incident acceptance keeps the same verdict, and incidents,
  evidence, and the timeline survive a restart.
- No message broker, no new database server, no service mesh, no auth in this
  slice. HTTP + JSON; embedded stores per owner.
- The Target App and the Simulator are untouched.

## Decision

### Four services

| Service | Owns | Upstream | Durable data |
| --- | --- | --- | --- |
| `weave-live` | Hubble and Beyla ingest, the observed graph, snapshot/SSE/reset, the observed-workload feed, the observation feed | Hubble Relay, Beyla | none (memory only) |
| `weave-inventory` | Argo inventory reconciliation, lifecycle state, change events, `/api/changes` | Argo CD, `weave-live` | own SQLite (workload states, change events) |
| `weave-history` | The observation recorder, DuckDB metrics, incidents, evidence, timeline | `weave-live` observation feed | own DuckDB and SQLite (incidents, evidence, recording gaps) |
| `weave-api` | The built SPA and the browser contract (BFF proxy) | the browser and the three backends | none |

Each service owns exactly one store or none, and exactly one upstream system.
Only `weave-inventory` and `weave-history` are stateful, each with its own RWO
PVC and `Recreate` strategy. No database file is shared between services.

### One browser origin

`weave-api` serves the SPA and proxies the existing `/api/*` paths to the owning
service, so the UI keeps relative URLs, `EventSource('/api/stream')`, and
`connect-src 'self'`. The external contract — paths, JSON shapes, SSE semantics
— is frozen for this slice; only additive fields are permitted. Each backend
service owns its existing path slice. Cilium Gateway API or an Ingress is a
later front door for TLS, host routing, and auth; it replaces the port-forward,
not the browser contract.

### Internal contracts

Only two data contracts are new; both are HTTP + JSON under `/internal/v1` and are
never exposed by `weave-api`:

- **Observation feed** (`weave-live` → `weave-history`): batched, normalized L4
  and L7 observations. `weave-history` keeps bucket semantics, edge IDs, path
  normalization, and the DuckDB flush. Delivery is at-most-once: a failed batch
  is dropped and counted, no retry and no replay buffer. A history outage is a
  visible Recording Gap, not silent zero traffic. A 15-second grace window
  before a bucket is flushed keeps late observations from overwriting an
  already-persisted bucket; observations older than the grace are dropped and
  counted.
- **Observed-workload feed** (`weave-inventory` polls `weave-live` every 30 s):
  the workload set with kind and first/last seen. If `weave-live` is
  unreachable, inventory skips the reconcile exactly as it treats an Argo
  error today, and keeps serving stored change events.

The contracts are defined in
[`docs/weave-internal-api.md`](../weave-internal-api.md). A shared
`internal/contracts` Go package holds the DTOs so producer and consumer cannot
drift, and an import-boundary test keeps `weave-live` from importing the
history or inventory packages.

### Failure semantics

No cross-service readiness chains. Each service is ready when its own function
works, and its liveness probe checks only its own process. `weave-api`
aggregates dependency status in `/api/health` while preserving the fields
scripts already read (`hubble_connected`, `observed_flows`). The UI degrades
panel by panel with an explicit unavailable state and retry; the BFF keeps no
cache and never presents stale data as fresh.

| Down | Consequence |
| --- | --- |
| `weave-live` | Topology and heatmap unavailable; incident data still readable; history records nothing new |
| `weave-history` | Incident tab unavailable; topology keeps running; Recording Gap reported |
| `weave-inventory` | Changes section unavailable; incidents unaffected |
| `weave-api` | Whole UI unavailable (accepted consequence of one origin) |

### Deployment

One umbrella Helm chart `weave` with four subcharts and one release; one script
builds four images, loads them into kind, installs the chart, and forwards one
port to `weave-api` (8090, unchanged for the acceptance harness). Each subchart
has its own image and tag, so a `helm upgrade` of one image rolls only that
service's pod. `weave-history` keeps the existing `weave-data` PVC; inventory
gets its own small PVC. `weave-live` and `weave-api` are stateless. The old
single binary becomes `weave-live` and is deleted at the end of the migration.

### Migration

Strangler, in this order; every intermediate state is a working system:

1. `weave-api` in front of the monolith, proxying all `/api/*`.
2. `weave-inventory` extracted; the current process exposes the workload feed.
3. `weave-history` extracted; the monolith pushes the observation feed;
   `weave-data` is reassigned to `weave-history`.
4. The remainder becomes `weave-live`; Beyla's OTLP endpoint is repointed.

Checkpoints: the short integration after every step, the full 20-minute
acceptance after step 3 and after step 4, restart persistence at the end. The
recorded acceptance incident must survive step 3 byte-identically. Rollback is
redeploying the previous image and chart. Inventory change events start fresh
(the old tables remain behind in the history SQLite file, unused); continuity is
restored by rerunning the acceptance.

### Verification

- Per-service unit tests; existing tests move with the code.
- Contract tests for both internal endpoints against the shared DTO package,
  and the external API tests unchanged.
- Import-boundary test for service package directions.
- One-command integration: build and load four images, install the umbrella
  chart, assert the 9/8/5 topology and incident create/read through the BFF.
- Degradation test: scale `weave-history` and `weave-inventory` to zero, assert
  the per-endpoint behavior and health, restore.
- Full acceptance rerun and restart persistence across both stateful services;
  pre-split incident hash comparison.
- No CI pipeline; the scripts stay local.

### Non-goals

Cilium Gateway API/Ingress, TLS/auth, HA or multi-replica, a message broker,
Weave self-tracing, Argo CD managing Weave, and a Compose stack for Weave are
out of this slice.

## Consequences

- Weave gains three more images, three more pods, one more PVC, and a network
  boundary on the most sensitive paths (ingest and incident evidence). The
  laptop still fits in one namespace.
- The observation feed is best-effort. A `weave-history` outage now produces a
  Recording Gap instead of missing samples. That is honest, and it is still a
  regression compared with an in-process recorder.
- `weave-history` is the durability layer; `weave-live` and the UI survive its
  absence, which was the point of the split.
- Incidents no longer share a process with the live graph or lifecycle
  reconciliation, so their failures are isolated.
- Runtime and deployment documentation must change at cutover. Until then
  README, PRODUCT, the deploy README, and the acceptance record still describe
  the single process, and the external contract points at this ADR.
- The browser still has one origin, so `weave-api` is a new single point of
  failure for the UI. Accepted; Cilium Gateway later moves TLS and auth to the
  front door.

## Alternatives considered

- **Stay a modular monolith; deepen packages only.** Cheapest and operationally
  best for 5,000 lines. Rejected because the project wants a real service
  architecture, not only internal seams.
- **Two services (`live` + everything else).** Rejected: the UI would die with
  the history database, which defeats the failure-isolation goal.
- **Three services (fold `inventory` into `history`).** Viable fallback.
  Rejected: inventory then owns Argo, lifecycle, and incidents in one process
  with two upstreams, and its change-event domain is cleanly separable.
- **Five or more services (split metrics from incidents, or static UI from
  BFF).** Rejected: incident creation would become a chatty distributed join
  over the metric schema, and the BFF is a proxy plus static host, not a second
  business layer.
- **Message broker between live and history.** Rejected: a new piece of
  infrastructure for one stream, and replay would put the same idempotency
  problem behind a bus.
- **`weave-live` aggregates finalized buckets and history only stores.** This
  would make retries idempotent and allow replay. Rejected for this slice: it
  moves metric aggregation out of the domain that owns it and makes
  last-write-wins bucket replacement the contract. Kept as the upgrade path if
  replay becomes a requirement.
- **History pulls a subscription from live.** Rejected: inverts control and
  adds fan-out and slow-consumer handling without solving loss.
- **Keep one image with four run modes.** Rejected: not separate deployables,
  and it fails the stated bar.
- **Big-bang cutover.** Rejected: no acceptance signal until the end; the feed
  and bucket-race mistakes would surface late.
- **Cilium Gateway now or auth now.** Rejected for this slice; the portal
  boundary and the product contract are unchanged by the split.
- **Shared PVC or shared SQLite/DuckDB files between services.** Rejected: a
  distributed monolith with extra steps.
- **Argo CD manages Weave.** Rejected for this slice: Argo CD cannot load kind
  images, and the one-command Helm flow is the acceptance path.

## Implementation status

Implemented on 2026-11-14. The single process was replaced by `weave-live`,
`weave-history`, `weave-inventory`, and `weave-api`; the umbrella chart
`deploy/charts/weave` renders four Deployments, four Services, and two PVCs.
The external contract, the UI, and the Simulator were not modified.

The cutover itself was one deployment change, not the stepwise strangler order
described above: the four services were built and verified together against the
existing `weave-data` volume, and the external contract was exercised through
the BFF before and after. The stepwise order remains the plan if the migration
is ever repeated without a working pause.

Verification at cutover:

- Go unit tests pass; internal feed contract tests run on both ends.
- The integration script checks package import boundaries.
- The four-service stack reconstructs 9 nodes, 8 edges, and 5 HTTP edges from a
  short Simulator run through the BFF.
- The stored acceptance incident from the single-process deployment survives
  byte-identically: the incident JSON hash is
  `e950d501bf54733a1c16e4418da72e0207926627539d2e8eb9885cbbae9e2ad1`
  before and after; the timeline is identical apart from its additive `gaps`
  field.
- The degradation test passes: stopping `weave-history` leaves the topology up
  and returns 502 for incidents; stopping `weave-inventory` leaves incidents and
  topology up and returns 502 for changes; both recover, and the history outage
  is reported as a Recording Gap.
- A feed bug found during cutover (oversized observation batches rejected with
  413) was fixed before the acceptance rerun; `dropped_observations` stays 0
  during a Simulator run.
- The 2026-11-14 acceptance was rerun against the four-service stack through the
  BFF: incident `8a6a69ef4e60b57a1bbbf824`, fault window
  `2026-11-14T19:26:21Z`–`19:28:21Z`, suspect edges Frontend → Gateway and
  Gateway → Catalog with first-bucket onsets, Auth/Orders/Payment outside it,
  and 205 scrubbable frames with no Recording Gap in the incident context. The
  incident JSON hash was identical before and after a history/inventory restart
  (`c7ecb9fe...`), and the pre-split incident remained byte-identical
  (`e950d501...`). The record is in [`docs/acceptance.md`](../acceptance.md).

