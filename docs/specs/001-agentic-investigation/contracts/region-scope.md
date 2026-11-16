# Region scope and expansion contract

## Scope state file

The run directory contains `scope.json` for a confined run. All five MCP
servers read it. One investigator run at a time; writes use an advisory lock
and atomic rename.

```json
{
  "version": 1,
  "incident_id": "8a6a69ef4e60b57a1bbbf824",
  "mode": "confined",
  "time_context": {
    "start": "2026-11-15T12:00:00Z",
    "end": "2026-11-15T12:05:00Z"
  },
  "initial": {
    "nodes": [
      "workload:weave/frontend",
      "workload:weave/gateway",
      "workload:weave/catalog"
    ],
    "edges": [
      "workload:weave/frontend->workload:weave/gateway:8080",
      "workload:weave/gateway->workload:weave/catalog:8082"
    ],
    "routes": [
      {
        "edge_id": "workload:weave/gateway->workload:weave/catalog:8082",
        "method": "GET",
        "path": "/api/products"
      }
    ],
    "workloads": ["frontend", "gateway", "catalog"],
    "source_paths": [
      "services/app/internal/catalogsvc",
      "services/app/cmd/catalog"
    ]
  },
  "current": {
    "nodes": [
      "workload:weave/frontend",
      "workload:weave/gateway",
      "workload:weave/catalog"
    ],
    "edges": [
      "workload:weave/frontend->workload:weave/gateway:8080",
      "workload:weave/gateway->workload:weave/catalog:8082"
    ],
    "routes": [
      {
        "edge_id": "workload:weave/gateway->workload:weave/catalog:8082",
        "method": "GET",
        "path": "/api/products"
      }
    ],
    "workloads": ["frontend", "gateway", "catalog"],
    "source_paths": [
      "services/app/internal/catalogsvc",
      "services/app/cmd/catalog"
    ]
  },
  "frontier": [
    {
      "node_id": "workload:weave/catalog",
      "directions": ["upstream", "downstream"]
    },
    {
      "node_id": "workload:weave/gateway",
      "directions": ["upstream", "downstream"]
    }
  ],
  "expansions": [],
  "limits": {
    "max_expansions": 3,
    "max_added_nodes": 8
  }
}
```

### Field rules

- `mode` is `confined` for treatment runs. Unconfined runs have no scope file.
- `nodes` are Weave node IDs; `workloads` are derived from
  `workload:<namespace>/<name>` nodes and are the only Kubernetes/source
  identities the tools accept.
- `routes` are normalized method/path pairs on in-scope edges.
- `source_paths` are profile-derived roots for in-scope workloads; a path is
  allowed only if it is equal to or below one of these roots.
- `time_context.start` is the evidence baseline start; `end` is the incident
  window end as of the creation snapshot, extended if the incident window is
  updated on close. Queries outside are clamped or denied.
- `frontier` is recomputed after every scope change. A node is a frontier if it
  is in scope and has at least one observed neighbour outside the scope. Hidden
  neighbours are never named in the frontier entry.
- `directions` is derived from observed edge orientation: `downstream` means
  the node has outgoing observed edges outside scope, `upstream` means incoming.
- `expansions` is append-only and visible to the agent through `get_scope`.

## Expansion rules

`expand_scope(node_id, direction, rationale)`:

1. Require `mode=confined`; reject in the unconfined arm.
2. Require `node_id` in `current.nodes` and in `frontier`.
3. Require `direction` in `upstream|downstream|both` and permitted by the
   frontier entry.
4. Require a non-empty `rationale`; record it verbatim.
5. Compute immediate observed neighbours of `node_id` in the chosen direction
   that are not already in scope. Add those nodes, their connecting edges, and
   derived workloads/routes/source paths.
6. Reject if `len(expansions)+1 > max_expansions` or the resulting node count
   exceeds `max_added_nodes` above the initial count. No partial expansion.
7. Write the new scope under the lock; recompute frontier; return the added
   entities and remaining budget.

The initial scope is never modified. The expansion history remains even if a
later expansion is refused. An expansion adds only direct neighbours, never the
whole graph.

## Region payload

The harness passes this compact document to the investigator for treatment
runs, and writes it to the run directory for reproducibility. It is derived from
the creation-time incident evidence and must not contain out-of-scope entities.

```json
{
  "incident": {
    "id": "...",
    "window_start": "...",
    "window_end": "...",
    "baseline_start": "...",
    "baseline_end": "...",
    "symptom": "...",
    "status": "open",
    "trigger": {"rule_id": "...", "metric": "...", "observed": 1.0, "threshold": 0.01, "breach_bucket": "..."}
  },
  "region": {
    "nodes": [
      {"id": "workload:weave/catalog", "name": "Catalog", "role": "service"},
      {"id": "workload:weave/gateway", "name": "API Gateway", "role": "hub"}
    ],
    "edges": [
      {
        "edge_id": "workload:weave/gateway->workload:weave/catalog:8082",
        "source": "workload:weave/gateway",
        "target": "workload:weave/catalog",
        "protocol": "HTTP",
        "port": 8082,
        "baseline": {"requests": 900, "error_rate": 0.0, "p95_ms": 4.1},
        "incident": {"requests": 60, "error_rate": 1.0, "p95_ms": 5.0},
        "onset": "...",
        "reasons": ["5xx rate"]
      }
    ],
    "routes": [
      {"edge_id": "...", "method": "GET", "path": "/api/products"}
    ]
  },
  "gaps": [],
  "timeline": [
    {"at": "...", "edge_id": "...", "requests": 12, "error_rate": 0.0, "p95_ms": 4.1, "anomaly": false}
  ],
  "frontier": [
    {"node_id": "workload:weave/catalog", "directions": ["upstream", "downstream"]}
  ],
  "limits": {"max_expansions": 3, "max_added_nodes": 8}
}
```

Downsampling: one frame every 15 seconds plus the onset frame, the first/last
fault-visible frame, incident start, and incident end; cap at ~40 frames. If
the evidence suspect region is empty, the harness seeds the scope with the
trigger route's edge endpoints and marks it as such in the run record.

## Unconfined arm

No scope file is created. All servers return full observed data over the time
context. The agent is still read-only. The benchmark records the arm in every
run record.
