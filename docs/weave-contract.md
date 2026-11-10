# Weave topology contract

This is the v1 contract between the Weave engine, its UI, and the future
benchmark comparator.

## Runtime

- Weave runs as one Go process in `weave-system`.
- It follows `hubble-relay.kube-system.svc.cluster.local:80` over gRPC.
- It watches one namespace, `weave`, from `WATCH_NAMESPACE`.
- It does not call the Kubernetes API and requires no RBAC.
- It does not modify the Target App or install Cilium policies.
- The web build is served from `/app/web/dist` by the same process.

## Topology semantics

**Node.** A logical workload observed in the watched namespace, or the single
`Source IP` endpoint. The ID is `workload:<namespace>/<workload>` for a
workload, and `source-ip` for the external entry.

**Edge.** One ordered `(source, target, destination port)` tuple. Replies are
discarded and folded into the request direction. Edge IDs are
`<source-id>-><target-id>:<port>`.

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
      "activity": [0, 0, 0, 12, 0]
    }
  ],
  "activity": [0, 0, 0, 12, 0],
  "totals": {"nodes": 9, "edges": 8, "connections": 1491, "events": 7439},
  "hubble": {
    "connected": true,
    "last_flow_at": "2026-11-09T21:53:28Z",
    "observed_flows": 7439
  }
}
```

`activity` always has 60 entries. `last_error` is present when Relay has failed.

### `GET /api/stream`

Server-Sent Events. Sends `retry: 1500`, then `event: topology` with the same
snapshot payload every second. The browser reconnects automatically.

### `POST /api/reset`

Clears all topology state and restarts the cumulative clock. Returns
`{"status":"reset"}`. It does not reset the Target App; that is the Simulator's
`/admin/reset` path.

### `GET /api/health` and `GET /health`

Always returns HTTP 200 while the process is alive:

```json
{"status":"ok","hubble_connected":true,"observed_flows":7439}
```

`status` is `degraded` when Relay is not connected. Kubernetes liveness does
not kill the process for a Relay outage.

## Configuration

| Environment | Default | Meaning |
| --- | --- | --- |
| `LISTEN_ADDR` | `:8080` | HTTP listen address |
| `HUBBLE_RELAY_ADDR` | `hubble-relay.kube-system.svc.cluster.local:80` | Relay gRPC target |
| `WATCH_NAMESPACE` | `weave` | Observed namespace |
| `WINDOW` | `5m` | Activity window, 1m–1h |
| `WEB_DIR` | `web/dist` | Built UI directory |

## UI contract

- One route, `/`.
- A node or edge click opens the inspector.
- The graph is cumulative; the ribbon is the five-minute window.
- The inspector shows only fields present in the snapshot.
- The UI never calls the Kubernetes API, the Target App, or Cilium directly.
