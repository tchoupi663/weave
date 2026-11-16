# Data Model: Agentic investigation from detected incidents

This document describes the durable Weave data, the local run data, and the
validation rules the implementation must enforce. Storage choices are fixed by
`plan.md`; formats referenced here are detailed in `contracts/`.

## Durable Weave data (weave-history)

### Incident (existing, extended additively)

- `id`: 24-character hex string, generated per incident.
- `created_at`, `updated_at`: RFC3339 UTC.
- `window_start`, `window_end`: RFC3339 UTC; `window_end` must be after
  `window_start`.
- `baseline_start`, `baseline_end`: derived from `window_start` and the
  configured baseline (`BASELINE_MINUTES`, default 15).
- `symptom`: non-empty generated or operator text.
- `scope`: optional operator-supplied scope; not parsed.
- `status`: `open` or `closed`.
- `notes`: free text.
- `evidence`: materialized Evidence snapshot (see `weave-contract.md`); the
  localization algorithm and `incident-v1` evidence version are unchanged.
- `origin` (new): `manual` or `detection`. Defaults to `manual` for existing
  rows and manual creation; omitted from JSON when empty to preserve legacy
  JSON hashes.
- `trigger` (new, optional): present only for detection-originated incidents;
  omitted from JSON for manual incidents. Fields: `rule_id`, `rule_version`,
  `scope` (edge id or route), `metric`, `observed`, `threshold`,
  `breach_bucket`, `detected_at`.

**Validation**: a detected incident has a non-empty trigger and a generated
symptom; a manual incident has no trigger. `origin` is immutable after creation.

**State transitions**: `open → closed` only. Closing a detected incident sets
`window_end` to the last breaching bucket and recomputes evidence once. Manual
close/reopen behaviour is unchanged.

**Compatibility**: the stored evidence JSON is unchanged. New Incident fields
are additive and `omitempty`; existing recorded incidents decode with
`origin=manual` and no trigger.

### Alert (new)

- `id`: generated identifier.
- `incident_id`: nullable until grouped; foreign key to Incident.
- `rule_id`, `rule_version`: identity of the SLA rule that breached.
- `scope_kind`: `edge` or `route`.
- `edge_id`: nullable for route-scoped rules; the edge the rule applies to.
- `method`, `path`: nullable for edge-scoped rules; the normalized route.
- `metric`: `error_rate_5xx`, `error_rate`, or `p95_latency_ms`.
- `observed`, `threshold`: numeric values at the evaluation that completed the
  sustain requirement.
- `breach_bucket`: the first breaching bucket.
- `created_at`: when the Alert row was written.
- `grouped_at`: nullable; set when the Alert joins an Incident.

**Validation**: exactly one of edge or route scope is populated; metric is in
the allowed set; a grouped Alert's `incident_id` points at an existing incident;
Alerts are never deleted in this slice.

**Indexes**: `alerts(incident_id)`, `alerts(created_at)`, and
`alerts(rule_id, created_at)`.

### Recording gap (existing)

Used by detection suppression and by the agent's gap tool. No schema change.

### Edge and route metric buckets (existing)

No schema change. The metrics query API exposes them to the agent and the
benchmark. Detection reads `edge_metrics_5s` and `route_metrics_5s` only.

## Local run data (services/agent, not in Weave)

### Region Scope (per confined run)

Stored as JSON in the run directory, shared by all MCP server processes:

- `incident_id`: the incident under investigation.
- `mode`: `confined`; the unconfined arm has no scope file.
- `initial` and `current` entity sets:
  - `nodes`: observed node IDs (for example `workload:weave/catalog`).
  - `edges`: observed edge IDs.
  - `routes`: objects `{edge_id, method, path}`.
  - `workloads`: workload names derived from in-scope nodes.
  - `source_paths`: allowed source roots for in-scope workloads.
  - `changes`: implied by `workloads`.
- `time_context`: `{start, end}`; metrics and logs are clamped to this interval.
- `frontier`: list of `{node_id, directions}` where hidden observed neighbours
  exist; names of hidden neighbours are not included.
- `expansions`: ordered records of
  `{from_node, direction, rationale, added_nodes, added_edges, added_workloads, at}`.
- `limits`: `{max_expansions, max_added_nodes}`.
- `denials`: not persisted in the scope file; denials are recorded in the run
  transcript by the tool servers' error responses.

**Validation**: expansion `from_node` must be a current frontier node; direction
must be one of `upstream`, `downstream`, `both`; the rationale is non-empty; the
resulting node count must stay within limits; the time context end must not
precede start.

**State transitions**: initial scope is created from the evidence suspect
region (or the trigger endpoints if empty). Each successful expansion moves the
affected neighbours into the current sets and recomputes the frontier. The
initial scope is immutable and remains in the record.

### Region payload (treatment initial context)

A compact derived JSON document, not persisted in Weave:

- `incident`: id, window, baseline, symptom, status, trigger.
- `region`: suspect nodes and edges with per-edge baseline/incident metric
  summary, onset, reasons, and routes.
- `gaps`: recording gaps overlapping the incident context.
- `timeline`: downsampled frames (every 15s plus transitions) for suspect
  edges.
- `frontier`: boundary marker summary.
- `limits`: expansion limits.

Validation: derived only from the creation-time evidence and incident; total
size target ≤2k tokens; no out-of-scope entities may appear.

### Agent Run record

One JSON file per run plus a line in `runs.jsonl`:

- `run_id`, `incident_id`, `scenario_id`, `repetition`, `arm`
  (`confined`/`unconfined`), `order`.
- `model`: name, digest, temperature, num_ctx.
- `prompt_version` and prompt hash; caps.
- `started_at`, `ended_at`, `duration_ms`.
- `detection_latency_ms`, `end_to_end_ms` (from fault start, when known).
- `tokens`: `prompt`, `completion`, `total`; `tool_calls`; `tool_payload_bytes`.
- `search_breadth`: distinct nodes, edges, routes, workloads touched by
  successful queries.
- `expansions[]`: node, direction, rationale, added entities.
- `denials[]`: tool, requested entity, timestamp.
- `transcript`: path to the JSONL transcript of messages and tool results.
- `diagnosis`: parsed Diagnosis or null.
- `score`: primary exact-match, route/edge match, failure-mode match, failure
  reason if any.
- `status`: `completed`, `failed_parse`, `failed_cap`, `failed_tool`,
  `failed_model`.
- `region_recall`: whether the ground-truth workload was in the initial region
  (computed by the harness, not the agent).

**Validation**: both arms for one incident share `scenario_id`, `repetition`,
and incident; the arm's scope mode matches its enforcement; failed runs keep a
partial transcript and are scored as accuracy failures.

### Diagnosis

- `root_cause.workload`: one workload name.
- `root_cause.failure_mode`: `http_error`, `latency`,
  `dependency_unavailable`, `bad_deployment`, `other`.
- `affected.edge_id`, `affected.route`: optional strings for the failing edge
  and route.
- `evidence[]`: `{source, reference, observation}` entries; sources are the
  tool server names or evidence type.
- `confidence`: number in `[0,1]`.
- `summary`: one or two sentences.

**Validation**: workload must be from the observed or declared set; failure
mode must be in the enum; confidence is present; malformed JSON gets one repair
attempt and then fails the run.

### Fault Scenario

- `id`, `description`.
- `ground_truth`: `{root_cause_workloads[], failure_mode, affected_route,
  affected_edge, expected_initial_region_workloads[]}`.
- `injection`: mechanism (`http_chaos`, `pod_delete_loop`, `image_swap`), YAML
  path or parameters, fault duration, recovery wait.
- `evidence_expectation`: what the initial region is expected to contain and
  whether expansion is expected.
- `fixture_hash`: hash of the fixture YAML or image digest used.

**Validation**: root cause must be a known node/workload in the profile;
affected route or edge must exist in the profile's expected topology; the
injection mechanism must match one implemented by the harness.

### Target Profile

- `name`, `namespace`, `argo_application`.
- `traffic`: simulator config path, duration/matrix coverage, reset behaviour.
- `sla_rules_path`, `evidence_baseline_minutes`, `grouping_window_seconds`,
  `cooldown_seconds`.
- `workloads`: list of workload names, optional Kubernetes label selectors,
  and source roots.
- `expected_topology`: path to the expected nodes/edges fixture.
- `scenarios`: list of scenario IDs and paths.
- `cluster`: read-only kubeconfig role name, namespaces.

**Validation**: all referenced files exist; every scenario ground truth
workload exists in `workloads`; every rule scope route/edge exists in the
expected topology or observed edge set; baseline is at least one rule window;
profile version is present.

### Benchmark Report

- `run_id`, `started_at`, `ended_at`, `status` (`complete`/`incomplete`).
- `environment`: git SHA, branch, model digest, prompt hash, MCP versions,
  profile hash, rule-pack hash, fixture hashes, Simulator config hash,
  baseline, caps.
- `episodes[]`: scenario, repetition, incident id, fault window, detection
  latency, arm order, run ids, fault reverted, notes.
- `runs[]`: summary counters per run.
- `summary`: per-scenario/per-arm accuracy, median and range of time to root
  cause, detection latency, tokens, search breadth, expansions, denials,
  region recall, negative control.
- `caveats[]`: incomplete matrix, swapped fixtures, failures.

**Validation**: every run in `runs[]` has a transcript file; every episode has
both arms or an explicit failure; counts are internally consistent; a report
with any excluded run states why.

## Relationships

```text
Target Profile 1 ── * SLA Rule
Target Profile 1 ── * Fault Scenario
SLA Rule       1 ── * Alert
Alert          * ── 1 Incident (grouped)
Incident       1 ── 2 Agent Run (confined, unconfined)
Agent Run      1 ── 0..1 Diagnosis
Fault Scenario 1 ── * Episode (benchmark run of one scenario repetition)
Episode        1 ── 1 Incident
Episode        1 ── * Agent Run
Benchmark Report 1 ── * Episode
```

## Data retention

- Alerts follow incidents in SQLite; no prune in this slice. Existing history
  retention for metric buckets is unchanged.
- Benchmark artifacts are committed under `reports/agent-benchmark/` after a
  recorded run; transcripts may be large and are file artifacts, not database
  rows. The read-only kubeconfig token is never committed.
- Scope files live in the run directory and may be discarded after the run;
  the run record and transcript preserve the expansion history.
