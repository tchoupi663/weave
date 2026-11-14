# Weave topology contract

This is the contract between the Weave engine, the Operator UI, and the future
Agent/benchmark consumers. The live topology contract remains v1; history,
incidents, and the incident timeline are the v2 incident-replay contract.

## Runtime

- Weave runs as one Go process in `weave-system`.
- It follows `hubble-relay.kube-system.svc.cluster.local:80` over gRPC for L4.
- It accepts OTLP HTTP/protobuf traces at `POST /v1/traces` from Grafana Beyla for HTTP L7.
- It watches one namespace, `weave`, from `WATCH_NAMESPACE`.
- It does not call the Kubernetes API and requires no Kubernetes RBAC.
- It calls the Argo CD API read-only with a token to read the declared workload
  inventory for `target-app-local`. That token is the only declared-state input.
- It does not modify the Target App, install Cilium policies, or insert a proxy.
- The web build is served from `/app/web/dist` by the same process.
- It runs as one pod with one PVC mounted at `DATA_DIR`; SQLite and DuckDB live
  on that volume. The Deployment uses `Recreate` because both are single-writer.

## Topology semantics

**Node.** A logical workload observed in the watched namespace, or the single
`Source IP` endpoint. The ID is `workload:<namespace>/<workload>` for a
workload, and `source-ip` for the external entry.

**Edge.** One ordered `(source, target, destination port)` tuple. Replies are
discarded and folded into the request direction. Edge IDs are
`<source-id>-><target-id>:<port>`. An edge has L4 counters always, and may also
carry an `l7` object when a matching HTTP client span was observed.

**Protocol.** Inferred from Cilium's named-port label on the destination; falls
back to HTTPS for 443/8443, HTTP for common HTTP ports, and TCP otherwise.

**Connections.** Counted from non-reply TCP SYN observations. **Events** count
unique non-reply flows after deduplication. These are L4 observations, not
requests, bytes, latency, or HTTP status.

**Filtering.**

- Destination must be in the watched namespace.
- DNS/control-plane destinations are excluded with it.
- Replies are excluded.
- Same-workload traffic is excluded.
- `reserved:host` or `reserved:world` sources are accepted only when entering
  `frontend:443`; they become the aggregate `Source IP`.
- Host traffic to auth, catalog, orders, payment, external, or gateway is
  kubelet probe traffic and is dropped.
- Duplicate flow UUIDs are ignored.
- Only `FORWARDED` L3/L4 flows are eligible.

**Window.** Counters are cumulative from process start or the last reset. Each
edge also carries 60 activity buckets covering the five-minute window. A
reset clears both.

### HTTP L7 observations

L7 is observed by Grafana Beyla, a separate privileged eBPF DaemonSet. Beyla
does not sit in the Target App request path. It instruments only the `gateway`
Deployment. Gateway client spans cover Gateway → Auth/Catalog/Orders/Payment;
gateway server spans cover Frontend → Gateway. Weave accepts those spans and
attaches them to the already-observed L4 edge. A server span is accepted only
when its peer resolves to a watched workload, so kubelet probes are dropped.

- One accepted span is one completed request. `requests` and `responses` are
  therefore equal unless a future observer can record a request without a
  response.
- For a client span, the source workload comes from the span resource
  (`k8s.deployment.name`) and the target comes from the HTTP authority
  (`server.address`). For a server span, the resource is the target and the
  peer address resolves to the source workload. The L4 edge resolves an
  address or port when the span does not name the workload.
- `port` comes from `server.port`; `method`, path, status code, and client
  duration come from the span attributes.
- The route is method plus normalized path. Query strings, headers, and bodies
  are never stored. Dynamic path segments (UUIDs, numeric IDs, `pNNN` product
  IDs) normalize to `:id`; at most five routes are exposed, ordered by request
  count.
- Latency is **client-observed for four edges and server-observed for
  Frontend → Gateway**. `latency_source` is `client`, `server`, or `mixed`.
  It is not a Cilium proxy latency.
- Status classes are `2xx`, `3xx`, `4xx`, `5xx`, and `other`.
- Cumulative and five-minute values are separate. L4 events and L7 requests
  are never summed.

The Cilium L7 visibility policy was evaluated and rejected: its Envoy proxy
added a 15.5% weighted p95 regression. Do not reintroduce it as the L7 source.

## HTTP API

### `GET /api/topology`

Returns the full snapshot. Empty collections are `[]`, never `null`. Activity
arrays are shortened to five entries in this excerpt; the real arrays always
contain 60.

```json
{
  "generated_at": "2026-11-09T21:53:28Z",
  "started_at": "2026-11-09T21:52:50Z",
  "namespace": "weave",
  "window_seconds": 300,
  "nodes": [
    {
      "id": "workload:weave/gateway",
      "workload": "gateway",
      "name": "API Gateway",
      "kind": "Deployment",
      "namespace": "weave",
      "role": "hub",
      "pods": ["gateway-695dbf5dc9-xg8tc"],
      "ips": ["10.244.3.214"],
      "connections": 1481,
      "events": 7393,
      "first_seen": "2026-11-09T21:53:15Z",
      "last_seen": "2026-11-09T21:53:28Z"
    }
  ],
  "edges": [
    {
      "id": "workload:weave/frontend->workload:weave/gateway:8080",
      "source": "workload:weave/frontend",
      "target": "workload:weave/gateway",
      "protocol": "HTTP",
      "port": 8080,
      "connections": 1468,
      "events": 7340,
      "first_seen": "2026-11-09T21:53:15Z",
      "last_seen": "2026-11-09T21:53:28Z",
      "activity": [0, 0, 0, 12, 0],
      "l7": {
        "protocol": "HTTP",
        "latency_source": "server",
        "requests": 724,
        "responses": 724,
        "status": {"2xx": 724, "3xx": 0, "4xx": 0, "5xx": 0, "other": 0},
        "latency_ms": {"p50": 1.2, "p95": 4.4},
        "window": {
          "requests": 724,
          "responses": 724,
          "status": {"2xx": 724, "3xx": 0, "4xx": 0, "5xx": 0, "other": 0},
          "latency_ms": {"p50": 1.2, "p95": 4.4},
          "activity": [0, 0, 0, 12]
        },
        "routes": [
          {"method": "GET", "path": "/api/products/:id", "requests": 435, "responses": 435, "status": {"2xx": 435, "3xx": 0, "4xx": 0, "5xx": 0, "other": 0}}
        ],
        "first_seen": "2026-11-09T21:53:15Z",
        "last_seen": "2026-11-09T21:53:28Z"
      }
    }
  ],
  "activity": [0, 0, 0, 12, 0],
  "totals": {"nodes": 9, "edges": 8, "connections": 1491, "events": 7439, "l7_edges": 5, "l7_requests": 1573, "l7_responses": 1573},
  "hubble": {
    "connected": true,
    "last_flow_at": "2026-11-09T21:53:28Z",
    "observed_flows": 7439,
    "observed_l7_observations": 1573
  }
}
```

`activity` always has 60 entries. `last_error` is present when Relay has failed.

### `GET /api/stream`

Server-Sent Events. Sends `retry: 1500`, then `event: topology` with the same
snapshot payload every second. The browser reconnects automatically.

### `POST /api/reset`

Clears all topology state, including L7 observations, and restarts the
cumulative clock. Returns `{"status":"reset"}`. It does not reset the Target
App; that is the Simulator's `/admin/reset` path.

### `POST /v1/traces`

OTLP HTTP/protobuf trace receiver for Beyla. Weave reads only client spans in
the watched namespace, keeps their HTTP method, normalized path, status,
duration, source Deployment, and destination service/port, and discards
headers, query strings, and bodies. It returns an empty OTLP export response
with HTTP 200. Malformed payloads return 400; oversized payloads return 413.

### `GET /api/health` and `GET /health`

Always returns HTTP 200 while the process is alive:

```json
{"status":"ok","hubble_connected":true,"observed_flows":7439}
```

`status` is `degraded` when Relay is not connected. Kubernetes liveness does
not kill the process for a Relay outage.

### `POST /api/incidents`

Request:

```json
{"start":"2026-11-14T09:30:00Z","end":"2026-11-14T09:32:00Z","symptom":"Catalog returns 500","scope":"gateway->catalog"}
```

Creates an Incident, computes the evidence snapshot, stores it in SQLite, and
returns the Incident with HTTP 201.

### `GET /api/incidents`

Returns incidents newest first. `?limit=` defaults to 50, maximum 200.

### `GET /api/incidents/{id}`

Returns one Incident including its materialized evidence. Unknown id returns
404.

### `PATCH /api/incidents/{id}`

Accepts any subset of `{"status":"open|closed","notes":"..."}`. Returns the
updated Incident.

### `POST /api/incidents/{id}/recompute`

Recomputes evidence from the stored window and returns the updated Incident.
This is the normal action after history is backfilled.

### `GET /api/incidents/{id}/timeline`

Returns:

```json
{
  "window": {"start":"2026-11-14T10:11:35Z","end":"2026-11-14T10:13:35Z"},
  "context": {"start":"2026-11-14T09:56:35Z","end":"2026-11-14T10:13:35Z"},
  "bucket_seconds": 5,
  "suspect_edges": ["workload:weave/gateway->workload:weave/catalog:8082"],
  "frames": [
    {"at":"...","edges":[{"edge_id":"...","requests":12,"status_4xx":0,"status_5xx":0,"error_rate":0,"p95_ms":2.4,"anomaly":false}]},
    {"at":"...","edges":[{"edge_id":"...","requests":12,"status_4xx":0,"status_5xx":10,"error_rate":0.833,"p95_ms":2.4,"anomaly":true}]}
  ]
}
```

`window` is the incident's own window and marks the transition point in the
playback. `context` is the full frame span: the incident's 15-minute baseline
through the incident window end. The frames start at `context.start`, so the UI
can show the system working before the incident and then the affected period.
The UI scrubs these frames. It does not query `GET /api/topology?at=...`; that
historical full-graph resource remains deferred.

### `GET /api/changes?from=&to=&limit=`

Returns lifecycle change events in the interval, newest first. `from` and `to`
are RFC3339; default is the last 24 hours. Each event carries `source`
(`declared`, `live`, `observed`), `type`, `workload`, `kind`, and `details`.
Change Events are not L4 Events.

## Configuration

| Environment | Default | Meaning |
| --- | --- | --- |
| `LISTEN_ADDR` | `:8080` | HTTP listen address |
| `HUBBLE_RELAY_ADDR` | `hubble-relay.kube-system.svc.cluster.local:80` | Relay gRPC target |
| `WATCH_NAMESPACE` | `weave` | Observed namespace |
| `WINDOW` | `5m` | Activity window, 1m–1h |
| `WEB_DIR` | `web/dist` | Built UI directory |
| `DATA_DIR` | empty | Enable history at this directory; required for incidents |
| `ARGOCD_URL` | empty | Base URL of the Argo CD API |
| `ARGOCD_TOKEN` | empty | Read-only Argo API token |
| `ARGOCD_APP` | `target-app-local` | Argo Application to read |
| `ARGOCD_NAMESPACE` | watch namespace | Namespace used to filter declared workloads |

## UI contract

- Two workspaces, selected in the top bar: `Topology` and `Incidents`. The
  incident workspace is deep-linkable with `?view=incidents`.
- A node or edge click opens the inspector. The inspector shows only fields
  present in the snapshot.
- Live edge titles show `protocol · port` and, for HTTP edges, the error rate.
  They do not show L4/L7 jargon.
- The live flow heatmap is the five-minute Activity Window; its expanded state
  shows window total, peak bucket, average, and active buckets.
- An incident shows its symptom/window/status, a scrub timeline that starts
  with the baseline and marks the incident transition, a suspect graph with
  per-frame error rates and quantities, evidence, onset, changes, notes, and
  close/reopen controls.
- Incident graph edge labels carry error percentage and quantities, not
  protocol labels. Nodes are draggable; edges and nodes focus on hover; edges
  are selectable.
- The timeline is the only place a recorded frame is presented. `Live topology`
  opens the current graph, not a historical reconstruction.
- The UI never calls the Kubernetes API, Argo CD, the Target App, Cilium, or
  Beyla directly. It reads the Weave API only.
