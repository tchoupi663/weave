# Research: Agentic investigation from detected incidents

All unknowns from the plan's Technical Context are resolved below. Each entry
records the decision, why it was chosen, and what was rejected.

## 1. MCP implementation

- **Decision**: Use the official `github.com/modelcontextprotocol/go-sdk` v1.8.0
  for both the MCP client (investigator) and the five stdio servers. One Go
  module, `services/agent`, builds `weave-agent` (subcommands `investigator`,
  `mcp kube`, `mcp metrics`, `mcp logs`, `mcp source`, `mcp weave`) and
  `weave-bench`.
- **Rationale**: The SDK is available and current, speaks stdio, and avoids a
  hand-rolled JSON-RPC implementation. Separate server processes keep the
  production-like surface visible while sharing one module and build. The
  binary can be pointed at by any MCP-capable agent in a later slice.
- **Alternatives**: Hand-rolled JSON-RPC (rejected: protocol drift risk);
  five separate modules (rejected: dependency/build duplication).

## 2. Scope enforcement

- **Decision**: A per-run `scope.json` written in the run directory. All five
  MCP servers receive `WEAVE_SCOPE_FILE` and check it before every call. A
  small `internal/scope` package owns read, atomic write under an advisory file
  lock, frontier calculation, and expansion. `weave-mcp` exposes
  `expand_scope`; other servers never mutate scope. Denials return a generic
  `scope_denied` error with no hidden-entity confirmation. The unconfined arm
  gets no scope file.
- **Rationale**: Enforces at the boundary regardless of prompt or model
  behavior; one state file is easy to inspect in transcripts and tests. One
  run at a time makes locking simple.
- **Alternatives**: Enforcement in the agent loop (rejected: bypassable and
  not reusable by other agents); separate scope service (rejected: unnecessary
  process for a local single-user run).

## 3. Detection evaluator placement and loop

- **Decision**: `weave-history` gains a detection goroutine started by
  `History.Start` when `SLA_RULES_FILE` is set. It ticks every 5 seconds,
  evaluates rules over the last flushed 5s edge/route buckets, and writes
  Alerts and Incidents in SQLite. Grouping, close, and evidence recompute reuse
  the existing incident/evidence code paths. Config: `SLA_RULES_FILE`,
  `DETECTION_INTERVAL` (default 5s), `BASELINE_MINUTES` (default 15).
- **Rationale**: History already owns bucket semantics, evidence, incidents,
  and flush timing. A separate service would duplicate those and add a
  deployment. A live detector would still wait for history to materialize
  evidence.
- **Alternatives**: Detector in `weave-live` (rejected in ADR-0007);
  `weave-detect` service (rejected for this slice); external Alertmanager
  (rejected: no metric source).

## 4. Detection rule semantics

- **Decision**: Rule fields are `id`, `description`, `severity`, `scope`
  (`edge_id` or `route` method/path), `metric` (`error_rate_5xx`,
  `error_rate`, `p95_latency_ms`), `threshold`, `window` (default 60s),
  `min_requests`, `sustain` (evaluations), `clear_after` (evaluations), and
  `version`. Evaluation sums counts/requests over the trailing window and uses
  the request-weighted mean of bucket p95 for latency; a breach requires the
  threshold and minimum volume for `sustain` consecutive evaluations. A
  persisted Alert is created on the first evaluation that completes the
  sustain requirement; its breach bucket is the first breaching bucket.
  Clear requires the complement condition for `clear_after` evaluations.
- **Rationale**: Simple, testable, and sufficient for a 100% fault; avoids
  burn-rate math and a PromQL parser while still expressing a service-level
  objective. The weighted-p95 approximation is documented in the rule contract.
- **Alternatives**: Per-bucket thresholds (rejected: too noisy on low-traffic
  payment/order routes); error budgets/burn rate (deferred); PromQL (deferred
  to real backends in slice 2).

## 5. Alert grouping and incident lifecycle

- **Decision**: The first Alert of a group creates the Incident with
  `origin: detection` and a generated symptom. Alerts whose scope shares a
  node with the open incident's suspect edges, or the same route within the
  grouping window (120s), join it; an open incident suppresses a new incident
  for the same scope; after close, a 5-minute cooldown prevents immediate
  re-creation. Close when every contributing rule is clear for `clear_after`
  evaluations: set `window_end` to the last breaching bucket, recompute the
  operator evidence snapshot once, and mark the incident closed. Detection is
  suppressed for any evaluation window intersecting a Recording Gap and the
  suppression count is exposed in `/api/health`.
- **Rationale**: One fault produces alerts on several edges (gateway and
  caller) and must produce one investigation; the operator evidence should
  cover the whole fault, while the Agent run retains the creation snapshot.
- **Alternatives**: One incident per Alert (rejected: duplicate investigations);
  never auto-close (rejected: suppresses later faults and misleads status);
  evidence recompute on every evaluation (rejected: expensive and churns the
  snapshot the agent saw).

## 6. Incident schema and API compatibility

- **Decision**: Additive SQLite migration on open: `alerts` table, and
  `incidents.origin TEXT NOT NULL DEFAULT 'manual'` plus
  `incidents.trigger_json TEXT NOT NULL DEFAULT ''`. Existing rows are
  upgraded in place. Incident JSON gains `origin` and `trigger` with
  `omitempty`; evidence JSON and existing timeline fields are unchanged.
  Existing manual-incident hashes remain stable because empty new fields are
  omitted. Add `GET /api/incidents/{id}/alerts` and additive
  `/api/metrics/edges` and `/api/metrics/routes` query endpoints.
- **Rationale**: Keeps browser and recorded incidents compatible, preserves the
  existing frozen contract, and gives the agent the detection provenance.
- **Alternatives**: New incidents table (rejected: migration churn); putting
  alerts in evidence (rejected: evidence is a localization snapshot, not
  alert history); changing evidence version (rejected: not needed for this
  additive data).

## 7. Metrics query API

- **Decision**: `GET /api/metrics/edges?from=&to=&bucket=5s|1m&edge_id=` and
  `GET /api/metrics/routes?from=&to=&bucket=5s&edge_id=&method=&path=` on
  `weave-history`, proxied by `weave-api`. Responses are arrays of bucket rows
  with the stored counters and p95/p50; no derived PromQL. The metrics MCP
  server reuses these endpoints and filters to the scope.
- **Rationale**: The unconfined arm needs full edge/route history to perform
  its own localization; the confined arm needs only in-scope series. Reusing
  the same API keeps the experiment fair.
- **Alternatives**: MCP server reading DuckDB (rejected: violates single-writer
  boundary); a new query service (rejected: unnecessary).

## 8. Evidence baseline configuration

- **Decision**: Replace the hard-coded 15-minute baseline with
  `BASELINE_MINUTES`, default 15, read by `weave-history`. The Target Profile
  carries `evidence_baseline_minutes`; the benchmark profile sets 5. The
  benchmark records this value and the report documents that product defaults
  remain 15. Episodes are spaced so at least the configured baseline is clean
  before each fault.
- **Rationale**: A fixed 15-minute baseline across 15 episodes produces a
  multi-hour warm-up and a contaminated baseline if episodes are close; a
  5-minute clean baseline keeps region quality defensible and the matrix
  feasible. The difference is explicit and same for both arms.
- **Alternatives**: Fixed 15 minutes with long gaps (rejected: >7 hours);
  contaminated baseline (rejected: confounds agent accuracy with region
  quality); per-benchmark database wipe (rejected: too slow, loses visibility).

## 9. Local model and agent loop

- **Decision**: Model `qwen3.5:4b` (Qwen3.5-4B, digest
  `7f3251fa6878`), temperature 0, fixed seed where Ollama honors it,
  `num_ctx: 32768`, and caps: 20 tool calls, 10 minutes wall time, 160k total
  tokens, 2k output tokens per response. The investigator sends OpenAI-style
  tool definitions via Ollama `/api/chat`, executes MCP `tools/call`, appends
  results, and stops on a JSON Diagnosis or a cap. A probe confirmed tool calls
  and `prompt_eval_count`/`eval_count` are returned. One malformed-output
  repair attempt is allowed inside the same caps.
- **Rationale**: Verified locally, fully local, exact token counts, and a
  deterministic, inspectable loop. Using DSH/Claude as the measurement
  instrument would add credential/routing and token-extraction complexity.
- **Alternatives**: DSH headless (deferred to interoperability testing); Claude
  Code headless (not local); building an agent framework dependency (rejected).

## 10. Tool server behavior

- **Decision**: Five servers, all stdio, all read-only:
  - `kube`: `list_pods`, `get_pod`, `list_deployments`, `get_deployment`,
    `list_statefulsets`, `get_statefulset`, `list_events`, `describe_workload`
    via `kubectl` with a namespace-scoped read-only kubeconfig.
  - `metrics`: `list_metrics`, `query_metric`, `query_metric_range`,
    `list_metric_labels` over the Weave metrics API.
  - `logs`: `list_log_sources`, `query_logs` (pod/workload, since, until,
    tail, filter, previous) via `kubectl logs`.
  - `source`: `search_source`, `read_file`, `list_directory` under the
    Target Profile's source roots only.
  - `weave`: `get_incident` (metadata and trigger), `get_scope`,
    `get_topology` (in-scope subgraph only), `get_changes` (in-scope workloads),
    `get_recording_gaps`, `expand_scope`.
  Scope filtering is applied before results are returned; denial text is
  generic. Traces are intentionally absent.
- **Rationale**: Mirrors the tool categories an on-call agent encounters in
  production while keeping every server read-only and replaceable later.
- **Alternatives**: A single combined server (rejected: less production-like,
  harder to swap backends independently); community Kubernetes MCP servers
  (rejected: often expose exec/apply and pull extra dependencies).

## 11. Region payload and frontier

- **Decision**: The harness builds a compact JSON payload (target ≤2k tokens)
  from the creation-time evidence: incident id/window/symptom/trigger, suspect
  nodes and edges, per-edge baseline/incident summary with onset and reasons,
  routes, recording gaps overlapping the window, a downsampled timeline (every
  15s plus transitions), and frontier markers with allowed directions. The
  investigator injects it into the treatment prompt; the control prompt
  receives incident metadata and trigger only. Frontier calculation joins the
  scope entity set to the full observed edge set; hidden neighbours are never
  named before expansion.
- **Rationale**: Keeps confinement honest and the token comparison meaningful;
  the full timeline would swamp a 4B context.
- **Alternatives**: Raw evidence JSON (rejected: too large and noisy);
  graph-only payload (rejected: agent would need many metric calls to start).

## 12. Read-only cluster credentials

- **Decision**: Add `deploy/platform/mcp/readonly-rbac.yaml`: ServiceAccount,
  Role, and RoleBinding in `weave` granting `get`, `list`, `watch` on pods,
  pods/log, events, deployments, statefulsets, replicasets, and services. The
  benchmark setup mints a token with `kubectl create token` and writes a
  short-lived kubeconfig under the run directory; `kube-mcp` and `logs-mcp`
  receive `KUBECONFIG`. No admin kubeconfig is handed to any MCP process.
- **Rationale**: Enforces the read-only guarantee at the credential layer, not
  only in tool code; matches the production RBAC assumption.
- **Alternatives**: Reuse the operator's admin context (rejected: violates
  least privilege and weakens the no-remediation claim); in-cluster MCP
  deployment (deferred: would need Ollama access from cluster).

## 13. Fault scenarios and fixture mechanisms

- **Decision**: Five scenarios, all reverted after use:
  - `catalog-http-500`: HTTPChaos replace 500 on `GET /api/products` for 4m.
  - `auth-login-latency`: HTTPChaos delay on `POST /api/auth/login` for 4m.
  - `external-payment-outage`: harness deletes the `external` pod every 20s
    during a 4m window, so Payment's calls fail; ground truth external.
  - `postgres-outage`: harness deletes the `postgres-0` pod every 20s during a
    4m window; Orders calls fail; ground truth Postgres.
  - `catalog-crashloop`: deploy a benchmark-only `weave-target-app:crashy`
    image to the catalog Deployment for 4m, then restore the original image.
  Each scenario YAML records ground truth (workload, failure mode, route or
  edge, expected initial-region membership) and the injection mechanism. The
  pilot validates detection and expected region for each before the recorded
  matrix. A same-route replacement is allowed before, never after, the recorded
  run.
- **Rationale**: Distinct routes, a mix of L7-direct and dependency root
  causes, and one code-error crash. HTTPChaos cannot reliably target HTTPS
  egress or make Postgres unavailable, so pod deletion is the honest mechanism
  for those scenarios.
- **Alternatives**: All HTTPChaos (rejected: cannot span L4 dependencies);
  network partition (rejected: target app clients have no timeouts and hang,
  which produces no clean detection signal); modifying Target App business code
  (rejected: normal behavior must stay unchanged).

## 14. Crash image build

- **Decision**: Add a `BUILD_TAGS` build argument to `services/app/Dockerfile`
  and a build-tagged `services/app/cmd/catalog/crashy.go` that panics at
  startup only when the tag is present. Build
  `weave-target-app:crashy`, kind-load it in the benchmark setup, patch the
  catalog Deployment image, wait for CrashLoopBackOff and the fault window,
  then restore the recorded original image. The normal image is unchanged.
- **Rationale**: A bad rollout is the realistic code-error failure; source and
  previous-container logs support the diagnosis. Argo's manual sync prevents
  self-healing during the scenario.
- **Alternatives**: Bad command override (rejected: not a code error); a
  separate tiny crash container (rejected: source/log mismatch); permanent
  crash code in normal image (rejected: breaks acceptance).

## 15. Benchmark harness

- **Decision**: `weave-bench` is a Go CLI driven by
  `deploy/targets/target-app/profile.yaml`. It:
  1. validates the profile and scenarios;
  2. starts or verifies a Simulator session with the profile's traffic config
     and a report path (duration computed to cover the matrix; faults cause
     expected journey failures);
  3. ensures a Weave API port-forward;
  4. warms up for the profile's baseline (5m benchmark), then for each episode:
     inject fault, poll for the detected incident, run both arms in balanced
     order against the same incident, revert the fault, wait for incident close
     and a clean baseline gap, then continue;
  5. writes one record per Agent Run, episode metadata, fixture hashes, summary
     JSON/Markdown, and environment metadata.
  A pilot mode runs one repetition over three scenarios. The full matrix is
  five scenarios × three repetitions × two arms.
- **Rationale**: A single orchestrator keeps fault timing, incident identity,
  arm pairing, and artifacts consistent; the simulator and kubectl are already
  the project's tools.
- **Alternatives**: Shell-only harness (rejected: JSON/scoring/fixture logic
  too error-prone); separate scripts per scenario (rejected: drift and partial
  artifacts); running each scenario in a fresh environment (rejected: hours of
  setup).

## 16. Scoring and artifacts

- **Decision**: Diagnosis JSON fields: `root_cause.workload`,
  `root_cause.failure_mode`, `affected.edge_id`, `affected.route`,
  `evidence[]`, `confidence`, `summary`. Primary accuracy is exact match of the
  root-cause workload against scenario ground truth; secondary fields are
  affected route/edge and failure mode. Time to root cause starts at Incident
  creation and stops at a correct parsed Diagnosis; detection latency and
  end-to-end time from fault injection are separate. Tokens, tool calls,
  payload bytes, distinct entities queried, expansions, and denials are
  recorded. Artifacts: per-run JSON + transcript, `runs.jsonl`, `summary.json`,
  `summary.md`, and `docs/agent-benchmark.md` for the recorded matrix. The
  healthy negative control is recorded. A run exceeding caps or malformed
  output twice is a failure.
- **Rationale**: Deterministic scoring, no LLM judge, and artifacts that make
  every reported number reproducible.
- **Alternatives**: LLM-as-judge (rejected: nondeterministic and expensive);
  manual scoring (rejected: not reproducible); time-to-resolution (rejected:
  no remediation).

## Risks and follow-ups

- Fixture validation is a required task before the recorded matrix; the pilot
  proves detection and region for each scenario.
- The local 4B model may produce weak or malformed diagnoses. The benchmark
  records that honestly; the plan does not promise treatment accuracy wins.
- Scope enforcement tests must cover all five servers, not just `weave`.
- The read-only kubeconfig token is short-lived and generated per run; it must
  not be committed.
