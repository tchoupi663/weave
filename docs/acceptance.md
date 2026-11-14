# Incident-replay acceptance

This document records the incident-replay acceptance runs: the original
single-process run and the four-service run after ADR-0006. Both are evidence
records, not claims that the suspect region is the root cause.

## Configuration

| Item | Value |
|---|---|
| Cluster | kind, Cilium/Hubble, Argo CD, Chaos Mesh, Beyla |
| Target App | Argo `target-app-local`, namespace `weave` |
| Weave | four services (`weave-live`, `weave-history`, `weave-inventory`, `weave-api`), namespace `weave-system`, `weave-data` + `weave-inventory-data` PVCs |
| Replay | `services/simulator/testdata/incident-replay.yaml` |
| Traffic | 20 minutes, concurrency 50, 30/50/20 mix |
| Fault | Catalog `GET /api/products` → HTTP 500, 100%, 2 minutes |
| Baseline | 15 minutes healthy before the fault |
| Recovery | 3 minutes after the fault |
| Harness | `scripts/incident-replay.sh` |

## Expected result

The suspect region must contain Catalog and Gateway → Catalog, with
Frontend → Gateway as the later cascade. Auth and Payment must be outside the
region. The evidence must identify the fault route and show a clear error-rate
jump over baseline.

The incident, its evidence, and its timeline must remain readable after Weave
restarts. The 5-second bucket count must also survive the restart.

## Recorded result — single process (2026-11-14)

**Status: PASS.** Run on 2026-11-14.

| Field | Result |
|---|---|
| Simulator window | `2026-11-14T09:56:36Z` → `2026-11-14T10:17:16Z` |
| Fault window | `2026-11-14T10:11:35Z` → `2026-11-14T10:13:35Z` |
| Incident id | `27292d4e9697923bea97ecfe` |
| Incident status | open |
| Simulator exit | 1, expected because injected 500s failed journeys |
| Bounce | 1,766 started / 1,766 completed / 0 failed |
| Browse | 3,028 started / 2,033 completed / 995 failed |
| Order | 1,159 started / 783 completed / 376 failed |
| Total journey failures | 1,371, expected during the fault |

### Suspect region

```text
suspect_edges:
  workload:weave/frontend->workload:weave/gateway:8080
  workload:weave/gateway->workload:weave/catalog:8082

suspect_nodes:
  catalog
  frontend
  gateway
```

Both suspect edges have onset `2026-11-14T10:11:35Z` and reason `5xx rate`.
Auth and Payment are outside the region.

### Evidence

- Frontend → Gateway: 806 requests, 644 5xx in the incident window, 79.9% error rate.
- Gateway → Catalog: 1,448 requests, 1,429 5xx, 98.7% error rate.
- Route evidence: `GET /api/products` has 2,008 5xx on Frontend → Gateway and
  2,004 5xx on Gateway → Catalog. The other routes in the top five have zero
  5xx.
- Baseline: 15 minutes, 10,013 requests on the selected Frontend → Gateway edge,
  0.0% errors, 4.7 ms p95. The first incident frame is 34 requests, 53% errors,
  6.3 ms p95; later frames reach 67% on Catalog and 100% on some samples.

### Timeline semantics

- The scrubber uses a semantic green → amber → red ramp: green for a sampled
  bucket with no errors, amber for elevated error rates, red for a fault.
- A 5-second bucket with no requests is a sampling gap, not a healthy bucket.
  Inside a sampled run it inherits the adjacent observed state so a gap inside
  the fault remains red instead of turning white. A leading gap with no earlier
  sample stays neutral and reads `No sample` in the legend. The rule is visible
  at `11:12:22` local: inside the fault, a zero-request bucket keeps the red
  state rather than turning white.
- The timeline starts with the incident's 15-minute baseline. For this stored
  incident the current API returns 204 frames at 5 seconds: 180 pre-incident
  frames and 24 incident frames. Every frame carries suspect-edge buckets. The
  final two incident buckets have zero requests and inherit the last observed
  red state.
- The transition is visible in the scrubber: the last pre-incident frame
  (`10:11:30Z`) is 58 and 45 requests at 0% errors; the first incident frame
  (`10:11:35Z`) is 53% and 67% errors with 18 5xx on each suspect edge. The
  scrubber labels the current moment `Pre-incident` or `Incident` and marks the
  incident boundary, so the state is written, not carried by color alone.
- The incident window itself is entirely inside the injected fault. With the
  pre-incident context, the strip is green up to the boundary and red after it.

### Timeline and persistence

- Timeline: 24 incident frames at 5 seconds in the recorded run. The current API
  also returns the 180 pre-incident baseline frames from the same stored history
  (204 total).
- Weave restart: `history ready edge_5s=4134 edge_1m=514 changes=127`.
- Incident JSON hash before and after restart: identical
  (`21f247a31718d37626dad7ffc347ff1bc01b5cd88c6d4194117ed848441e275c`).
- Timeline JSON hash before and after restart: identical for the current
  pre-incident context response (`c1e46654ef679106b232c05cfe9d703c8627fd3a3d24ad780bb563b56566fc3f`).
  The original window-only response hash was
  `aaa719f2a6ea82f31703fe16d0303df9429d9ac5f4c6f72b1f11c93b2346343c`.
- The live topology was restored with a short run after restart: 9 nodes,
  8 edges, 5 HTTP edges, zero failures.

### Screenshots

- `docs/images/weave-incident-workspace.png` — acceptance incident with the
  pre-incident baseline, scrub timeline, suspect region, and evidence.
- `docs/images/weave-topology.png` — live topology with HTTP error rates and the
  flow heat panel.
- `docs/images/weave-flow.png` — expanded flow heat panel.

## Recorded result — four-service stack (2026-11-14)

**Status: PASS.** Run on 2026-11-14 against the umbrella chart: `weave-live`,
`weave-history`, `weave-inventory`, and `weave-api`, one replica each. The
history service reused the existing `weave-data` PVC, so the prior incident and
metric history were present throughout the run.

| Field | Result |
|---|---|
| Fault window | `2026-11-14T19:26:21Z` → `2026-11-14T19:28:21Z` |
| Incident id | `8a6a69ef4e60b57a1bbbf824` |
| Incident status | open |
| Simulator exit | 1, expected because injected 500s failed journeys |
| Bounce | 1,766 started / 1,766 completed / 0 failed |
| Browse | 3,027 started / 2,034 completed / 993 failed |
| Order | 1,159 started / 783 completed / 376 failed |
| Total journey failures | 1,369, expected during the fault |

### Suspect region

```text
suspect_edges:
  workload:weave/frontend->workload:weave/gateway:8080
  workload:weave/gateway->workload:weave/catalog:8082

suspect_nodes:
  catalog
  frontend
  gateway
```

Both suspect edges have onset `2026-11-14T19:26:25Z` (the first fault bucket)
and reason `5xx rate`. Auth, Orders, and Payment are outside the region.

### Evidence

- Frontend → Gateway: 3,245 requests, 2,711 5xx in the incident window.
- Gateway → Catalog: 2,787 requests, 2,712 5xx.
- Route evidence: `GET /api/products` has 2,711 5xx on Frontend → Gateway and
  2,712 on Gateway → Catalog; no other top route has 5xx.
- Timeline: 205 frames at 5 seconds from the baseline through the incident end;
  the additive `gaps` array is empty, so no Recording Gap fell inside the
  incident context.
- The observation feed reported zero dropped observations and zero recording
  gaps during the healthy portion of the run.

### Persistence and restart

- Restarting `weave-history` and `weave-inventory` pods preserved the incident:
  incident JSON hash `c7ecb9fec12bebb7d4617fb57754cbd35adad21fb49c0e7f6d136cdddf0e7ece`
  was identical before and after; timeline JSON hash
  `d8137022c3cb2f6accfab311416f8685b30505c38acd32ca692ae65420fe3bf7` was also
  identical.
- The incident recorded before the split is still byte-identical:
  `e950d501bf54733a1c16e4418da72e0207926627539d2e8eb9885cbbae9e2ad1`.
- History counters after restart: `edge_5s=11985`, `edge_1m=1482`.
- The deliberate history restart produced one Recording Gap and 11 dropped
  observations, reported by `weave-live`; no incident window overlaps it.

### Criteria — four-service stack

| Criterion | Result |
|---|---|
| Catalog in suspect region | pass |
| Gateway → Catalog in suspect region | pass |
| Frontend → Gateway cascade in suspect region | pass |
| Auth, Orders, Payment outside suspect region | pass |
| Fault route in evidence | pass |
| 5-second timeline is scrubbable | pass, 205 frames |
| No Recording Gap inside the incident context | pass |
| Incident survives restart | pass |
| Evidence survives restart | pass |
| Timeline survives restart | pass |
| Stored pre-split incident unchanged | pass |
| Browser contract unchanged through `weave-api` | pass |

## Criteria — single process

| Criterion | Result |
|---|---|
| Catalog in suspect region | pass |
| Gateway → Catalog in suspect region | pass |
| Frontend → Gateway cascade in suspect region | pass |
| Auth outside suspect region | pass |
| Payment outside suspect region | pass |
| Fault route in evidence | pass |
| 5-second timeline is scrubbable | pass |
| Incident survives Weave restart | pass |
| Evidence survives Weave restart | pass |
| Timeline survives Weave restart | pass |

## Notes and limits

- A non-zero Simulator exit is expected: the injected 500s fail journeys while
  the fault is active.
- The incident is created manually after the replay.
- The 20-minute run is one continuous Simulator run; the fixed grace timer means
  it does not stop after 60 seconds.
- The timeline shows recorded suspect-edge frames, not a full historical graph.
  `Live topology` opens the current graph.
- The generic all-connection host/probe acceptance remains deferred; the current
  filter keeps the Target App's ingress-scoped host behavior and fixture display
  names. The evaluated incident region is unaffected.
- The first four-service acceptance attempt exposed an oversized observation
  batch being rejected with HTTP 413 by the feed contract test. The batch
  splitting was fixed before the recorded four-service run above.
- The four-service run was driven through `svc/weave-api` on `:8090`, so the
  browser-facing paths, SSE stream, and incident API were exercised through the
  BFF rather than directly against the owning services.
