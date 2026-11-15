# MCP tool contract

Five stdio MCP servers expose read-only tools. The investigator starts them
as separate processes and prefixes tools by server (`kube__list_pods`, and so
on). The unconfined arm runs the same servers with no scope file. The confined
arm passes `WEAVE_SCOPE_FILE` to every server; each server checks the scope
before returning data.

## Common rules

- No tool mutates cluster, Target App, files, Weave state, or benchmark state.
- Out-of-scope requests fail with a JSON-RPC tool error whose message contains
  `scope_denied` and does not confirm the hidden entity's existence.
- Empty results are `[]`; missing fields are omitted, never `null` unless the
  schema says so.
- Time arguments are RFC3339 UTC. The confined arm clamps or rejects times
  outside the scope's `time_context`.
- Every successful result includes the plain data only; no internal path names,
  tokens, or host details.

## `kube` server

Backed by `kubectl` with a namespace-scoped read-only kubeconfig. All results
are filtered to in-scope workloads before return.

| Tool | Arguments | Returns |
| --- | --- | --- |
| `list_pods` | `namespace?` | `{pods:[{name,workload,phase,restarts,started_at,labels}]}` |
| `get_pod` | `pod` | `{name,workload,phase,restarts,containers[{name,ready,restart_count,state}],started_at,labels}` |
| `list_deployments` | — | `{deployments:[{name,replicas,ready,image,updated_at}]}` |
| `get_deployment` | `deployment` | deployment detail including container images and conditions |
| `list_statefulsets` | — | `{statefulsets:[{name,replicas,ready,image}]}` |
| `get_statefulset` | `statefulset` | statefulset detail including container images and conditions |
| `list_events` | `workload?`, `since` | `{events:[{type,reason,message,workload,first_seen,last_seen,count}]}` |
| `describe_workload` | `workload` | `{workload,kind,status,conditions,containers,pods,recent_events}` |

## `metrics` server

Backed by `weave-history` via `/api/metrics/edges` and
`/api/metrics/routes`. In the confined arm, an edge or route outside the scope
is denied; series that contain out-of-scope edges are filtered.

| Tool | Arguments | Returns |
| --- | --- | --- |
| `list_metrics` | — | `{metrics:[{name,kind,description,labels}]}` |
| `query_metric` | `metric`, `from`, `to`, `filters?` | series with bucket rows for the exact range |
| `query_metric_range` | `metric`, `from`, `to`, `step?`, `aggregate?` | downsampled series (`sum`, `avg`, `max`) |
| `list_metric_labels` | `metric` | label names and example values |

Metric names: `weave_edge_requests`, `weave_edge_errors`, `weave_edge_p95_ms`,
`weave_route_requests`, `weave_route_errors`. Filters use edge id, source,
target, method, path, and status class. This is production-shaped, not a
PromQL implementation.

## `logs` server

Backed by `kubectl logs` with the same read-only kubeconfig. In the confined
arm, only pods of in-scope workloads are visible.

| Tool | Arguments | Returns |
| --- | --- | --- |
| `list_log_sources` | — | `{sources:[{workload,pods[]}]}` |
| `query_logs` | `workload` or `pod`, `since`, `until?`, `tail?`, `filter?`, `previous?` | `{entries:[{at,pod,container,line}],truncated}` |

`previous=true` reads the previous container instance and is required for the
crash-loop scenario. Lines are returned without modification except for a hard
per-line and total-size truncation; truncation is reported.

## `source` server

Read-only local source access, scoped to the Target Profile's source roots for
in-scope workloads. The benchmark profile and ground-truth files are outside
every allowed root.

| Tool | Arguments | Returns |
| --- | --- | --- |
| `list_directory` | `path` | `{path,entries:[{name,type,size?}]}` |
| `read_file` | `path`, `max_bytes?` | `{path,content,truncated}` |
| `search_source` | `query`, `path?`, `max_results?` | `{matches:[{path,line,text}]}` |

## `weave` server

Backed by `weave-api`. Read-only except `expand_scope`, which writes only the
local scope state, never Weave.

| Tool | Arguments | Returns |
| --- | --- | --- |
| `get_incident` | `incident_id` | incident metadata and trigger; never `evidence` in the confined arm, and never `evidence` via this tool in either arm |
| `get_scope` | — | current scope, frontier, limits, and expansion history |
| `get_topology` | — | observed subgraph restricted to in-scope nodes/edges in the confined arm; full observed graph in the unconfined arm |
| `get_changes` | `from`, `to`, `workload?` | lifecycle changes filtered to in-scope workloads in the confined arm |
| `get_recording_gaps` | `from`, `to` | gaps overlapping the incident context |
| `expand_scope` | `node_id`, `direction`, `rationale` | `{added_nodes,added_edges,frontier,limits,remaining_expansions}`; confined only |

`get_incident` intentionally never exposes the evidence snapshot: the treatment
receives the bounded region as an initial payload, and the control arm must not
be able to fetch it. `get_topology` is the only topology source; its confined
result is the in-scope subgraph plus frontier markers, never names or counts of
hidden entities.

## Scope-denial error shape

Tool errors carry an MCP error whose text includes:

```text
scope_denied: request outside the current investigation scope
```

No hidden node, edge, workload, pod, path, or count is included. The
investigator records each denial with the tool name and requested entity.

## Transport and lifecycle

- Stdio, one process per server, JSON-RPC per the MCP specification.
- The investigator initializes all five servers, requests `tools/list`, and
  enforces a global tool-call cap.
- Servers exit when stdin closes; the investigator closes them on run end.
- A server startup failure fails the run before the first model call.
