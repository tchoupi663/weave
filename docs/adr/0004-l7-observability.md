# ADR-0004: Observe HTTP L7 with Beyla, not a Cilium proxy

- **Status:** Accepted
- **Date:** 2026-11-10
- **Context:** Weave v1, L7 telemetry
- **Supersedes:** the L7 deferral in ADR-0003

## Context

Weave's L4 topology already proves which workloads talked to which. Operators
also need to know what the HTTP traffic did: requests, status classes, client
latency, and routes.

Cilium can produce HTTP L7 observations with a `CiliumNetworkPolicy`. The policy
does not merely observe: it routes matching traffic through Cilium Envoy. The
first candidate selected `frontend` and `gateway` egress to the five HTTP
services. It produced the expected five L7 edges, but the Envoy hop measured a
15.5% weighted p95 regression at 60 seconds and 50 users. A second candidate
concentrated the proxy on `gateway` ingress and egress; it still measured 10.5%.
Both candidates had zero failed journeys.

That cost is not acceptable for a tool that must run concurrently with the
application and remain close to performance-neutral. The policy also has a
sharp operational edge: removing it left stale proxied connections until the
Gateway and Frontend were restarted.

## Decision

Use Grafana Beyla as a separate eBPF observer. Do not use a Cilium L7 policy
for Weave v1.

- Beyla runs as a privileged DaemonSet in `beyla-system`.
- It instruments only the `gateway` Deployment in `weave`. This is deliberate:
  instrumenting the frontend nginx process as well was measured above the
  performance gate.
- Gateway **client spans** cover Gateway → Auth/Catalog/Orders/Payment.
- Gateway **server spans** cover Frontend → Gateway. Weave accepts a server
  span only when its peer resolves to a watched workload, so kubelet probes and
  unknown callers are dropped.
- It exports OTLP HTTP/protobuf spans to Weave.
- Weave accepts spans at `POST /v1/traces` and attaches them to the
  already-observed L4 edge. L4 remains the source of truth for edge endpoints.
- Weave stores method, normalized path, status code, duration, source
  Deployment, destination service/port, and whether the duration is
  client- or server-observed. It discards headers, query strings, and bodies.
- L7 is additive: L4 connections/events and L7 requests are separate figures.
  They are never summed.
- L7 latency is **four client-observed edges plus one server-observed edge**
  (Frontend → Gateway). The UI labels the source per edge. It is not Cilium
  proxy latency.
- Coverage is the five HTTP edges: Frontend → Gateway and Gateway → Auth,
  Catalog, Orders, Payment. Source IP → Frontend, Orders → Postgres, and
  Payment → External Payment Provider remain L4-only.

The Cilium L7 policy artifact was removed. It must not be reintroduced as the
L7 source.

## Performance gate

The gate was 5% weighted p95 regression and zero failures on a 60-second,
50-user run against the same Target App state.

| Approach | Weighted p95 regression | Failures |
| --- | ---: | ---: |
| Cilium L7, Frontend + Gateway egress | +15.5% | 0 |
| Cilium L7, Gateway ingress + egress | +10.5% | 0 |
| Beyla eBPF, Frontend + Gateway | +4.2% / +7.5% / +9.5% (three trials) | 0 |
| Beyla eBPF, Gateway only | **+0.3%** | **0** |

The weighted figure is the Simulator's per-step p95 weighted by completed step
count. Individual steps are low-volume and noisy at this run length; the
absolute changes were small. The gate result is recorded with that method
rather than pretending a single 60-second sample has per-step precision it does
not have. The gateway-only configuration was selected because it was the only
observer configuration that held the gate across the controlled run while
still producing all five edges.

## Consequences

- The Target App data path is unchanged. Beyla uses eBPF probes, not a proxy.
- Weave gains an OTLP HTTP receiver and a small in-memory L7 aggregator. It
  still does not call the Kubernetes API and needs no RBAC.
- Beyla is privileged and is a separate platform dependency. It must be
  installed and upgraded independently of Weave.
- L7 has mixed provenance by construction: four edges are client-observed,
  and Frontend → Gateway is server-observed. The UI and API expose
  `latency_source` so the distinction is explicit.
- HTTPS and Postgres remain L4-only; they produce no HTTP span.
- Beyla spans are request events, not bytes. Weave does not claim byte counts.
- A missing Beyla or a dropped span degrades only L7; the L4 graph remains.

## Alternatives considered

- **Cilium L7 visibility policy.** Rejected: Envoy proxy cost exceeded the gate
  and policy removal disrupted live connections.
- **Application SDK instrumentation.** Rejected: it modifies the Target App and
  violates the network-observation model.
- **Beyla on both Frontend and Gateway.** Rejected for the final configuration:
  the five edges were present, but three measured trials exceeded the gate. The
  frontend nginx generic uprobes were the main added cost.
- **Sidecar service mesh.** Rejected: it inserts a proxy into every request,
  the same class of cost as the Cilium policy but larger.
- **Pixie.** Rejected for this cluster: not supported on kind, and its resource
  floor is high.
- **Tetragon / raw eBPF.** Rejected for v1: no turnkey HTTP parser and
  aggregation pipeline; too much bespoke work for this slice.
- **Coroot / DeepFlow.** Rejected: full observability platforms with a much
  larger footprint than the task needs.
- **Logs or ingress metrics.** Rejected: they cover only a subset of the five
  edges and do not provide a consistent client-observed view.
