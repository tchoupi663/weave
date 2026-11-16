# Feature Specification: Agentic investigation from detected incidents

**Feature Branch**: `001-agentic-investigation`

**Created**: 2026-11-15

**Status**: Draft

**Input**: User description: "The graph must be able to detect incidents from metric alerts and SLAs, then return the portion of the graph to a local AI agent for investigation. Performance — time to root cause, tokens, accuracy — is recorded and compared to runs without the bounded suspect region. The agent gets read-only MCP servers, logs, and everything needed to find the root cause, but cannot remediate. The current Target App is used first; a larger realistic app and more realistic simulation come in a second slice."

## User Scenarios & Testing *(mandatory)*

### User Story 1 - Detected incidents replace manually selected windows (Priority: P1)

An operator watches a running application. When observable traffic on a route
or edge breaches a declared service-level objective long enough to matter, Weave
creates one investigation automatically, with a generated symptom, the actual
breach window, and the triggering metric. The operator can open the incident
and see the same evidence the existing manual incidents show, including the
suspect region. Manual incident creation remains available for replay.

**Why this priority**: Every later story depends on an incident existing without
a human choosing a window. Without detection there is no agentic investigation.

**Independent Test**: Run ten minutes of healthy traffic and confirm no incident
is created. Inject the catalog HTTP 500 fault and confirm exactly one incident
appears within the expected detection latency, its window covers the fault, its
trigger names the breaching rule, and it closes after recovery. This can be
tested without any agent or benchmark harness.

**Acceptance Scenarios**:

1. **Given** a healthy Target App under steady traffic, **When** ten minutes
   elapse with no objective breach, **Then** Weave creates no incident.
2. **Given** the catalog returns HTTP 500 on the product route, **When** the
   breach is sustained past the rule's sustain requirement, **Then** exactly
   one incident is created with `window_start` at the first breaching bucket,
   a generated symptom, and a trigger naming the rule, metric, observed value,
   threshold, and rule-pack version.
3. **Given** an open detected incident whose contributing rules all clear,
   **When** the clear requirement elapses, **Then** the incident closes and its
   window ends at the last breaching bucket.
4. **Given** a second rule breaches while its scope is connected to an open
   incident's region, **When** it breaches, **Then** it joins that incident and
   no duplicate incident is created.
5. **Given** a Recording Gap overlaps an evaluation window, **When** the window
   would otherwise breach, **Then** no incident is created from missing data
   and the suppression is counted.
6. **Given** an operator or replay harness supplies a start, end, symptom, and
   scope, **When** the manual creation path is used, **Then** it behaves as it
   does today and creates no Alert records.

---

### User Story 2 - A confined agent investigates the suspect region (Priority: P1)

When a detected incident is created, a local AI investigator starts
automatically. Its first and only view of the architecture is the bounded
suspect region: the suspect nodes and edges, their routes, metrics, logs,
cluster objects, source, and changes, over the incident's time context. It
cannot read anything else. If the evidence inside that region indicates the
root cause lies outside, it can expand the boundary deliberately — one recorded
hop at a time from a boundary node — and continue. It never searches the whole
architecture, and it never changes anything.

**Why this priority**: This is the product thesis. Without enforced
confinement, the comparison with unconfined runs would measure prompt wording,
not the bounded-region mechanism.

**Independent Test**: Start an incident whose suspect region contains Catalog,
Gateway, and Frontend. Attempt to read an unrelated workload's logs, metrics,
source, cluster objects, topology, or changes; every attempt is denied without
revealing the hidden entity. Confirm a permitted in-region diagnosis is
possible. Then run a dependency-fault incident where the root cause is one hop
outside the region; confirm the agent can expand from the correct boundary node
and diagnose after expansion, and that the expansion appears in the run record.

**Acceptance Scenarios**:

1. **Given** a detected incident with a non-empty suspect region, **When** the
   investigator starts, **Then** its initial context contains the region and
   its tool access is limited to the region's nodes, edges, routes, workloads,
   source directories, changes, and incident time context.
2. **Given** a confined run, **When** it requests data for a node, edge, route,
   workload, pod, log stream, source file, or change outside the scope, **Then**
   the request is refused with a generic denial that does not confirm the
   hidden entity exists.
3. **Given** in-region evidence suggests the root cause is outside, **When** the
   agent expands scope from a boundary node with a rationale, **Then** that
   node's immediate neighbours and connecting edges become available, the
   expansion is recorded, and the previous scope remains intact.
4. **Given** an empty suspect region, **When** the incident is created, **Then**
   the trigger route's endpoints seed the scope and the investigator can start
   and expand from there.
5. **Given** the expansion limit is reached, **When** the agent requests
   another expansion, **Then** the request is refused and the run continues
   with the evidence already available.
6. **Given** any confined or unconfined run, **When** the agent attempts to
   mutate the Target App, cluster, or Weave state, **Then** no such capability
   exists in its tool access.

---

### User Story 3 - Benchmark compares confined and unconfined investigations (Priority: P2)

An engineer needs to know whether confinement helps. The same incidents are
investigated twice — once confined, once with the full architecture available —
using the same model, prompts, tools, and limits. Every run records time to
root cause, detection latency, tokens, tool calls, search breadth, expansions,
denials, and accuracy against the scenario's known root cause. A committed
report shows the paired comparison and the raw runs.

**Why this priority**: It turns the product claim into evidence. It is P2
because it depends on detection and confinement but not on any new operator
surface.

**Independent Test**: Run one scenario with both arms on the same incident and
verify that the report contains two complete run records with the full metric
set, the ground truth, the arm, the model and prompt versions, and the scenario
and fixture hashes; then repeat for the full matrix.

**Acceptance Scenarios**:

1. **Given** a fault scenario with known ground truth, **When** a detected
   incident exists, **Then** a confined run and an unconfined run are performed
   against that incident with the same model, prompt skeleton, tool surface,
   and resource caps.
2. **Given** a completed run, **When** it is scored, **Then** its diagnosis is
   compared to the scenario's ground-truth root-cause workload, affected route
   or edge, and failure mode, and correctness is recorded without human
   interpretation.
3. **Given** a full benchmark matrix, **When** it finishes, **Then** the report
   contains every raw run, per-scenario and per-arm accuracy, median and range
   of time to root cause, tokens, search breadth, expansion counts, denial
   counts, failures, a healthy negative control, and region recall.
4. **Given** a run that times out or exceeds its caps, **When** the benchmark
   completes, **Then** the failure is recorded as an accuracy failure with its
   partial transcript, not omitted from the report.
5. **Given** any reported benchmark number, **When** a reader opens the raw
   artifacts and environment metadata, **Then** the number is reproducible from
   the recorded command, versions, hashes, and configuration.

---

### User Story 4 - A target profile lets the next app be swapped without core changes (Priority: P3)

A later slice replaces the small Target App with a larger, realistic
application and a more realistic traffic simulation. That swap should be a new
target profile and benchmark scenarios, not a rewrite of detection, the
investigator, or the confinement mechanism. The current slice leaves that seam
in place and proves it with the current app.

**Why this priority**: It prevents the first slice from hard-coding the fixture
and makes the second slice a repeat rather than a redesign. It is P3 because
the first slice can demonstrate all outcomes without swapping the app.

**Independent Test**: Inspect the profile used for the current app: namespace,
declared application, SLA rules, fault scenarios with ground truth, traffic
driver, and expected topology are all profile data. Verify that no
target-app-specific name or route appears in the confinement, detection, or
benchmark logic.

**Acceptance Scenarios**:

1. **Given** the current Target App profile, **When** detection and the
   benchmark run, **Then** every app-specific value (namespace, routes, rules,
   scenarios, traffic, expected topology) comes from the profile.
2. **Given** a profile missing a required field, **When** the benchmark starts,
   **Then** it fails before traffic or fault injection with a message naming
   the missing field.
3. **Given** the region scope, the agent toolset is defined in terms of
   workload, edge, route, and source identity rather than fixture names, **When**
   a different profile supplies different identities, **Then** the confinement
   mechanism applies unchanged.

---

### Edge Cases

- A rule breaches on a route whose traffic is below the minimum request volume:
  no Alert is created; the breach is ignored rather than treated as an incident.
- A Recording Gap overlaps part or all of an evaluation window: detection is
  suppressed and counted; the gap is never interpreted as calm traffic.
- An incident opens and the fault recovers before the agent's first tool call:
  the incident is already closing, but the run uses the creation-time snapshot
  and completes against the recorded window.
- Two rules breach for the same fault on different edges: one incident, one
  trigger, the second alert linked to the same incident.
- The initial suspect region is empty because the anomaly is only visible on a
  route with no suspect edge: the trigger endpoints seed the scope.
- The root cause is outside the initial region (dependency outage, crash-looping
  service): the run is expected to require expansion; region recall is false
  and the report says so.
- The agent expands in the wrong direction, or repeatedly expands until the cap:
  expansion is recorded; no further access is granted; the run may fail
  accuracy and is still scored.
- The agent attempts an out-of-scope read one or many times: each denial is
  recorded, the hidden entity is never confirmed, and the run continues.
- The agent reaches its token, turn, or wall-time cap: the run is marked failed
  with a partial transcript and counts as an accuracy failure.
- The model returns malformed or unparseable output: one retry with the same
  caps is allowed; a second failure ends the run as failed and recorded.
- The crash-loop scenario produces no L7 requests on the failing edge: the
  incident is detected from the caller's 5xx; the initial region excludes the
  crashing workload and the agent must expand to find it.
- A fault fixture fails to apply or does not produce the expected symptom: the
  run is excluded from scoring, recorded with the failure, and the matrix is
  marked incomplete rather than silently shortened.
- The benchmark session is interrupted: partial raw runs remain valid
  artifacts and the report marks the matrix incomplete; no numbers are
  extrapolated.
- Source access inside an in-scope service incidentally names another service:
  no data access is granted outside the scope; the observation is treated as
  context, not a boundary breach.
- The cluster is unhealthy before a run (recording gaps, missing pods, stale
  history): the benchmark preflight fails and the run does not start.

## Requirements *(mandatory)*

### Functional Requirements

**Detection**

- **FR-001**: Weave MUST accept a versioned SLA rule pack per target profile.
  Each rule MUST declare scope (route or edge), metric, threshold, evaluation
  window, minimum request volume, sustain requirement, clear requirement, and
  severity.
- **FR-002**: The detector MUST evaluate rules against recorded L7 buckets on a
  fixed cadence, create an Alert on a sustained breach, and create no Alert
  when traffic is below the rule's minimum volume.
- **FR-003**: A detected Incident MUST be created from the first Alert of a
  group, MUST carry a generated symptom, `window_start` at the first breaching
  bucket, and an additive trigger with rule identity, metric, observed value,
  threshold, breach bucket, and rule-pack version.
- **FR-004**: Alerts MUST group into one Incident when their scope is connected
  by a shared node or route within the grouping window; an open Incident MUST
  suppress duplicate Incidents; a cooldown MUST prevent immediate re-creation
  after close.
- **FR-005**: A detected Incident MUST close when all contributing rules have
  been clear for the clear requirement; its `window_end` MUST be the last
  breaching bucket and its operator-facing evidence MUST be recomputed once for
  the full window.
- **FR-006**: Detection MUST be suppressed when a Recording Gap overlaps an
  evaluation window and MUST expose the suppression count; a healthy period
  MUST create no Incident.
- **FR-007**: The existing manual Incident creation, list, get, patch,
  recompute, and timeline behaviour MUST remain available and unchanged for
  manual incidents.
- **FR-008**: All Incident contract changes MUST be additive; existing browser
  and replay consumers MUST keep working without modification.

**Agent handoff and confinement**

- **FR-009**: A detected Incident MUST trigger a local investigator
  automatically, without human initiation, and every run MUST be bounded by
  fixed turn, token, and wall-time caps recorded in the run artifact.
- **FR-010**: A confined run MUST start from a compact region payload delivered
  before the first tool call: incident and trigger, suspect subgraph and
  per-edge metrics, onsets and reasons, routes, recording gaps, a downsampled
  timeline, and boundary markers. An unconfined run MUST receive incident
  metadata and trigger only.
- **FR-011**: The investigator MUST have read-only access to production-shaped
  evidence sources: cluster workload state, pods, events and logs; metrics over
  time; log search; target application source within scope; and Weave's incident
  context, topology, changes, and recording gaps. It MUST have no mutation,
  remediation, shell, or unrestricted filesystem capability.
- **FR-012**: A confined run's read access MUST be refused outside its Region
  Scope without confirming whether the hidden entity exists. The scope MUST
  cover nodes, edges, routes, edge and route metrics, workloads, pods, events,
  logs, source directories, changes, and the incident time context.
- **FR-013**: The initial Region Scope MUST be the suspect region; when that
  region is empty, the trigger route's endpoints MUST seed the scope.
- **FR-014**: Boundary nodes MUST be identified to the confined agent as the
  only expansion points. A Scope Expansion MUST require a rationale, MUST add
  only the chosen node's immediate neighbours in the chosen direction, and MUST
  be recorded. The run MUST enforce a configurable cap on expansions and added
  nodes and MUST refuse expansion beyond it.
- **FR-015**: The investigator MUST return a structured Diagnosis containing a
  primary root-cause workload, a failure mode, the affected route or edge,
  evidence references, a confidence, and a short summary. Malformed output MAY
  be retried once within the same caps.

**Benchmark**

- **FR-016**: Five Fault Scenarios MUST be defined with ground truth: catalog
  HTTP 500, auth login latency, external payment downtime, Postgres downtime,
  and a crash-looping catalog image. Ground truth MUST name the root-cause
  workload, failure mode, affected route or edge, and expected initial region.
- **FR-017**: The benchmark MUST pair a confined run and an unconfined run on
  the same Incident. Arms MUST share model, model digest, prompt skeleton, tool
  surface, and caps; only the confinement intervention and initial context may
  differ. Arm order MUST be balanced across repetitions.
- **FR-018**: The benchmark MUST run one continuous traffic session with a
  clean warm-up before the first fault, rotate scenarios so the same route
  recurs only after a clean baseline, validate each fixture before scoring, and
  never expose scenario ground truth or fault-injection metadata to the agent.
- **FR-019**: Every run MUST record time to root cause, detection latency,
  end-to-end time from fault injection, prompt/completion/total tokens, tool
  calls, tool payload sizes, search breadth, expansions, out-of-scope denials,
  accuracy fields, model digest, prompt hash, caps, and failure status.
- **FR-020**: The benchmark MUST produce a committed report and raw run records
  containing a healthy negative control, per-scenario and per-arm results,
  region recall, paired comparisons, and environment metadata sufficient to
  reproduce the run. It MUST report incomplete matrices as incomplete.
- **FR-021**: The crash-looping scenario MUST be produced by a benchmark-only
  build or deployment of the catalog image; the normal Target App image and
  normal source behaviour MUST remain unchanged, and the fixture MUST be
  reverted after the scenario.

**Target profile**

- **FR-022**: Detection, confinement, and benchmark configuration for the
  Target App MUST be supplied by a Target Profile containing namespace,
  declared application, SLA rules, fault scenarios with ground truth, traffic
  driver, and expected topology.
- **FR-023**: The confinement, detection, and benchmark logic MUST NOT contain
  target-app-specific workload names, routes, or fault assumptions; a malformed
  or incomplete profile MUST fail before traffic starts with a message naming
  the missing field.

**Documentation and contract**

- **FR-024**: The glossary, ADRs, browser contract, and service documentation
  MUST describe Alerts, SLA Rules, Region Scope, Scope Expansion, Confined and
  Unconfined Runs, and the benchmark measurement contract.
- **FR-025**: The read-only and no-remediation guarantees MUST be verifiable:
  tool inventory review confirms no mutation capability exists, and the
  cluster credentials used by read-only tools are namespace-scoped read-only.

### Key Entities

- **SLA Rule**: A declared service-level objective over observable traffic; has
  scope, metric, threshold, window, minimum volume, sustain, clear, severity,
  and version. Evaluated to produce Alerts.
- **Alert**: A sustained breach of one SLA Rule; has the rule, observed value,
  threshold, breach bucket, and rule-pack version. Alerts group into one
  Incident.
- **Incident**: A persisted, time-bounded investigation of a symptom. It can be
  manual or detection-originated, has a window, status, evidence snapshot, and
  optional trigger; it is never the Alert itself.
- **Trigger**: The detection provenance attached to a detected Incident.
- **Suspect Region**: The connected set of suspect nodes and edges around the
  earliest onset; the initial world of a confined investigator.
- **Region Scope**: The allowed set of nodes, edges, routes, workloads, source
  directories, changes, and time context for a confined run.
- **Frontier**: An in-scope node with out-of-scope observed neighbours; the only
  expansion point.
- **Scope Expansion**: A recorded one-hop enlargement from a Frontier node with
  a rationale, bounded by a cap.
- **Agent Run**: One investigator execution over an incident; records model,
  prompt, caps, arm, context, transcript, Diagnosis, time, tokens, tool calls,
  search breadth, expansions, denials, and score.
- **Diagnosis**: The investigator's structured root-cause hypothesis: primary
  workload, failure mode, affected route or edge, evidence references,
  confidence, and summary.
- **Root Cause**: The component or condition that produced the incident's
  symptoms; the ground-truth label for benchmark scenarios.
- **Fault Scenario**: A reproducible injected failure with ground truth, a
  fixture, a fault window, and expected initial-region membership.
- **Target Profile**: The target-app-specific configuration bundle for
  detection and benchmarking.
- **Benchmark Report**: The committed summary and raw run records produced by
  one benchmark matrix, with environment and version metadata.

## Success Criteria *(mandatory)*

### Measurable Outcomes

- **SC-001**: A healthy ten-minute traffic run produces zero detected
  incidents.
- **SC-002**: The catalog HTTP 500 fault produces exactly one detected incident,
  created within 30 seconds of the first breaching bucket, whose window covers
  the fault and whose trigger identifies the breaching rule.
- **SC-003**: A detected incident closes automatically within the defined clear
  window after recovery, and no duplicate incident is created for the same
  fault.
- **SC-004**: Every attempted out-of-scope read in a confined run is refused;
  zero out-of-scope results leak into the run transcript, and no denial
  confirms the hidden entity's existence.
- **SC-005**: Every confined run's search breadth is bounded by its initial
  scope plus at most the configured expansion cap; no full-architecture query
  is possible.
- **SC-006**: The benchmark completes and records the full five-scenario,
  three-repetition, two-arm matrix (30 runs) with complete raw artifacts and
  report, or records a partial matrix explicitly with the failure reason; no
  number is reported outside the artifacts.
- **SC-007**: Every run records time to root cause, detection latency, tokens,
  tool calls, search breadth, expansions, denials, and accuracy; medians and
  ranges are reported per scenario and arm.
- **SC-008**: The benchmark report includes accuracy against ground truth for
  every scenario, region recall for every incident, a healthy negative control,
  and a paired confined-versus-unconfined comparison.
- **SC-009**: The confined arm demonstrates at least one correct diagnosis
  reached through Scope Expansion in a dependency scenario, and the expansion
  is visible in the run record.
- **SC-010**: The normal Target App image and normal source behaviour are
  unchanged by the crash-loop fixture, and the fixture is reverted after the
  scenario.
- **SC-011**: The Target Profile supplies every app-specific value used by
  detection and benchmarking; a review of those code paths finds no
  target-app-specific name or route, and an incomplete profile fails before
  traffic starts.
- **SC-012**: Documentation and contract review confirms the read-only tool
  inventory, namespace-scoped read-only credentials, and the updated glossary,
  ADRs, and browser contract.

## Assumptions

- The local kind cluster, local model runtime, and the Target App are available
  and healthy for benchmark runs; there is no cloud dependency in this slice.
- The product evidence baseline remains 15 minutes and becomes profile
  configuration for detected incidents. The benchmark profile uses a shorter,
  recorded baseline (5 minutes) so each incident has an uncontaminated
  baseline without a multi-hour warm-up between every episode; the report
  records the baseline used and the difference is an explicit assumption, not
  a hidden change to product defaults.
- Fault fixtures for dependency outages and the crash loop are feasible with
  the existing fault-injection harness; the plan validates them before any
  recorded run, and a same-route equivalent may replace an unreliable fixture
  before recording, never after.
- The benchmark runs one investigation at a time; concurrent runs are out of
  scope.
- The first slice keeps the existing browser surface: detected incidents appear
  in the existing Incident View, but no Agent Run is persisted or displayed;
  benchmark results are committed artifacts.
- The slice-2 app and traffic simulation are not selected here; only the Target
  Profile seam and the criteria for that selection are in scope.
- Remediation is out of scope and no tool or prompt asks for it.
- The model's reasoning quality is not guaranteed; the benchmark records the
  result, including unflattering accuracy, rather than tuning the comparison.
