# ADR-0008: Confine an AI investigator to the suspect region

- **Status:** Accepted
- **Date:** 2026-11-15
- **Context:** Weave agentic-investigation slice
- **Related:** ADR-0005 (Incident and Agent read model), ADR-0007 (detected
  incidents)

## Context

Weave has a documented Agent read model but no Agent. The product thesis is
not another chatbot over a topology: it is that an Agent should investigate a
localised incident inside a bounded part of the graph, not search the whole
architecture. The claim only means something if confinement is enforced and
measured against an unconfined baseline.

The environment supplies a local GPU and Ollama with a tool-capable
Qwen3.5-4B model, no hosted model is required, and no MCP server exists yet.
The Target App is the current small fixture; a larger, realistic app and a
more realistic traffic simulation are a second slice. The first slice must
therefore prove the confinement mechanism and the measurement protocol on a
small graph, with an app-agnostic seam for the larger one.

## Decision

### Agent and tool surface

- The reference Agent is a local **investigator** process: an MCP client and a
  local-model client. The model is pinned (`qwen3.5:4b`, Qwen3.5-4B)
  with temperature 0 and fixed turn/token/time caps. The model digest, prompt
  hash, and caps are recorded per run. Any MCP-capable agent can replace the
  reference agent without changing the tool contract.
- The Agent has five read-only, production-shaped MCP servers: Kubernetes
  (workloads, pods, events, describe, logs), metrics (instant/range queries
  over Weave history), logs (label discovery and range queries over pod
  stdout), source (GitHub-shaped search/read scoped to Target App source), and
  Weave (incident metadata and trigger, topology, changes, recording gaps).
  Traces are absent because Weave retains aggregates, not spans.
- The Agent has no shell, filesystem, kubectl, or mutation tools. Tool servers
  use namespace-scoped read-only cluster credentials. Fault injection belongs
  to the external benchmark harness.

### Confinement

- A **Confined Run** begins with a **Region Scope**: the evidence Suspect
  Region (nodes and edges), all routes on those edges, and only those
  workloads, source directories, changes, and time context. If the region is
  empty, the trigger route's endpoints seed the scope.
- Every MCP server enforces the scope from shared scope state. Out-of-scope
  reads are refused without confirming that the hidden entity exists.
  Topology, declared inventory, metrics, logs, events, and source outside the
  scope are not merely discouraged; they are unavailable.
- **Scope Expansion** is the only way out. It is a recorded tool call from a
  Frontier node, with a mandatory rationale and a direction
  (downstream/upstream/both). It adds that node's immediate neighbours and the
  connecting edges, one hop at a time. Limits are three expansions and eight
  added nodes per run, configurable in the target profile. There is no bulk or
  full-graph expansion.
- An **Unconfined Run** has the full observed architecture and the same tool
  servers without scope enforcement. It is the benchmark baseline.
- The confined Agent receives a compact region payload (incident metadata,
  trigger, suspect subgraph and metrics, onsets, routes, gaps, downsampled
  timeline, frontier markers) before its first tool call. The unconfined Agent
  receives incident metadata only.

### Measurement

- Runs are paired on the same incident. The only difference between arms is
  confinement: same model, prompt skeleton, tool set, caps, and incident
  metadata.
- The headline outcome is **Time to Root Cause**: detection-complete Incident
  creation to the first correct Diagnosis. Detection latency and end-to-end
  time from fault injection are reported separately. Accuracy is scored
  against each Fault Scenario's ground-truth root cause, with route/edge and
  failure mode as secondary fields. Tokens, tool calls, payload sizes, search
  breadth, expansions, and out-of-scope denials are recorded.
- Ground truth is hidden from the Agent. Benchmark fixtures are validated
  before a recorded run; the healthy negative control and per-scenario region
  recall are reported.

### Scope and delivery

- The first slice now uses nine Fault Scenarios grouped into four covered
  service-level fault classes; see ADR-0009. They are catalog HTTP 500, auth
  login latency, external payment downtime, Postgres downtime, a crash-looping
  catalog image, orders scaled to zero, payment ImagePullBackOff, catalog CPU
  stress, and catalog network loss. The crash image is a benchmark-only build;
  the normal image and Target App source behavior are unchanged. Scenario
  ground truth includes the root cause, failure mode, affected route, and
  expected initial region.
- The first slice has no Agent UI and no persisted Agent Run in Weave. Results
  are committed benchmark artifacts. The browser surface remains the
  Operator's Incident View.
- A `TargetProfile` carries the namespace, Argo application, SLA rules, fault
  scenarios with ground truth, traffic driver, and expected topology. Slice 2
  replaces the Target App and the simulation by supplying a new profile;
  simulated MCP backends can be replaced by real Prometheus/Loki/Kubernetes/
  code-host servers without changing the Agent or the benchmark.
- The Agent does not remediate. There is no remediation tool, and the
  benchmark never asks it to fix anything.

## Considered options

- **Region as a hint only** (the Agent may query everything). Rejected: it
  does not prevent mass search and cannot support the product claim.
- **Prompt-only confinement.** Rejected: not auditable and trivially
  bypassable. Enforcement belongs in the tool layer.
- **Let a single expansion reveal the full graph.** Rejected: it reintroduces
  the mass-search failure mode. Expansion is one hop at a time.
- **Real observability stack now.** Deferred to slice 2; it adds platform work
  and would not improve the confinement experiment.
- **Persist Agent Runs and show them in the UI now.** Deferred; benchmark
  artifacts are the slice-1 deliverable and avoid cross-arm contamination.

## Consequences

- The scope policy adds a shared-state component across MCP servers and must
  be tested for denial, frontier correctness, and expansion limits.
- The confined arm is deliberately disadvantaged when the Root Cause lies
  outside the initial region; the benchmark must report region recall and
  expansions so those failures are attributable.
- The unconfined arm will usually be able to see more, so treatment wins are
  expected in tokens and search breadth, while accuracy may be equal; that is
  a result to record, not a target to engineer.
- The source server can incidentally expose service names inside an in-scope
  service's code (the gateway knows its upstreams). That is not data access;
  telemetry and objects outside the scope remain refused.
- A larger app in slice 2 will make expansion depth and scope policy limits a
  first-class tuning problem, not an afterthought.
