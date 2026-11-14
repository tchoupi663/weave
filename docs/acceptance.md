# Incident-replay acceptance

This is the first-slice acceptance record for manual incident replay. It is an
evidence record, not a claim that the suspect region is the root cause.

## Configuration

| Item | Value |
|---|---|
| Cluster | kind, Cilium/Hubble, Argo CD, Chaos Mesh, Beyla |
| Target App | Argo `target-app-local`, namespace `weave` |
| Weave | `weave-app:dev`, namespace `weave-system`, 2 GiB PVC |
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

## Recorded result

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

### Timeline and persistence

- Timeline: 24 frames at 5 seconds, all 24 with suspect-edge samples.
- Weave restart: `history ready edge_5s=4134 edge_1m=514 changes=127`.
- Incident JSON hash before and after restart: identical
  (`21f247a31718d37626dad7ffc347ff1bc01b5cd88c6d4194117ed848441e275c`).
- Timeline JSON hash before and after restart: identical
  (`aaa719f2a6ea82f31703fe16d0303df9429d9ac5f4c6f72b1f11c93b2346343c`).
- The live topology was restored with a short run after restart: 9 nodes,
  8 edges, 5 HTTP edges, zero failures.

### Screenshots

- `docs/images/weave-incident-workspace.png` — acceptance incident with scrub
  timeline, suspect region, and evidence.
- `docs/images/weave-topology.png` — live topology with HTTP error rates and the
  flow heat panel.
- `docs/images/weave-flow.png` — expanded flow heat panel.

## Criteria

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
