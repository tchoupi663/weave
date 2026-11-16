# Implementation Plan: Agentic investigation from detected incidents

**Branch**: `main` (no feature branch created; feature directory `specs/001-agentic-investigation`)

**Date**: 2026-11-15 | **Spec**: [spec.md](spec.md)

**Input**: Feature specification from `specs/001-agentic-investigation/spec.md`

## Summary

Add SLA-driven incident detection to `weave-history`, then a local, region-confined
AI investigator that receives the bounded suspect region before its first tool
call and can leave it only through audited one-hop scope expansions. A
read-only set of production-shaped MCP servers exposes Kubernetes, metrics,
logs, target source, and Weave incident context to the investigator. A separate
benchmark harness runs five fault scenarios against paired confined and
unconfined arms, records time to root cause, tokens, accuracy, search breadth,
expansions, and denials, and writes committed artifacts. The current Target App
is driven by a target profile; slice 2 supplies a new profile without changing
Weave or the investigator.

## Technical Context

**Language/Version**: Go 1.26 for Weave and the new local agent/benchmark module
(existing services: `services/weave` Go 1.26, `services/app` and
`services/simulator` Go 1.24). TypeScript/React UI is unchanged in this slice.

**Primary Dependencies**: Go standard library; existing `weave-history` SQLite
and DuckDB stack; `sigs.k8s.io/yaml` (already in the Weave module); official
`github.com/modelcontextprotocol/go-sdk` v1.8.0 for MCP client and servers;
Ollama HTTP API (`/api/chat` with tool calls) for local inference; `kubectl`
CLI with a namespace-scoped read-only kubeconfig for the Kubernetes and log
tool servers. No Kubernetes client-go dependency, no new database, no new
observability platform.

**Storage**: Existing `weave.db` SQLite for Alerts, Incidents, and incident
trigger/origin columns; existing `metrics.duckdb` for edge and route buckets;
new local JSON scope state per investigation run; benchmark raw records and
reports as JSON/Markdown artifacts. No new persistent store.

**Testing**: Go unit tests for detection rules/evaluation/grouping/close,
scope enforcement, frontier/expansion, scoring, and MCP tool contracts; Go
integration tests with fake Ollama responses; shell acceptance for detection;
a pilot benchmark of one repetition over a small scenario subset before the
recorded matrix.

**Target Platform**: Operator laptop (macOS, Docker Desktop) plus the local
kind cluster with Weave in `weave-system` and the Target App in `weave`. The
investigator, MCP servers, and benchmark harness run on the laptop.

**Project Type**: Existing four-service backend plus new local CLI tooling:
one `weave-agent` binary (investigator and MCP server subcommands), one
`weave-bench` binary, and profile/scenario configuration.

**Performance Goals**: Detection within 30 seconds of the first breaching
bucket; a complete benchmark matrix (9 scenarios × 3 repetitions × 2 arms)
recorded with raw artifacts; each investigator run bounded by 20 tool calls,
10 minutes, and a fixed token budget; benchmark wall time approximately 3–4
hours, run as one managed session.

**Constraints**: No external alert stack or observability backend; the model
and agent run locally; all agent tool access is read-only; the confined arm is
enforced in the tool servers, not the prompt; no UI or persisted Agent Run in
Weave; no remediation; no Target App normal-image changes beyond the
benchmark-only crash build; the benchmark profile uses a recorded 5-minute
evidence baseline while the product default stays 15 minutes.

**Scale/Scope**: A 9-node observed graph today; 5 MCP servers; 5 fault
scenarios on 4 distinct routes; 15 incidents and 30 paired Agent Runs per
recorded matrix; 1 operator; single cluster.

## Constitution Check

*GATE: Must pass before Phase 0 research. Re-check after Phase 1 design.*

| Principle | Gate | Result |
| --- | --- | --- |
| I. Observation over declaration | Detection and regions derive only from observed L7 history and the observed topology; no fabricated metrics | PASS |
| II. Bounded, auditable investigation | Region Scope enforced by tool servers; only recorded one-hop expansions; no full-graph search; no remediation | PASS |
| III. Read-only by default | All agent tools read-only; namespace-scoped read-only cluster credentials; fault injection only in the external harness | PASS |
| IV. Honest instruments and failure isolation | Gap suppression prevents false incidents; detection and benchmark report incomplete data; no Weave service gains a cross-service readiness dependency | PASS |
| V. Evidence and measured claims | Benchmark artifacts record environment, versions, hashes, caps, and raw runs; no unmeasured performance claims | PASS |
| VI. Spec-driven delivery | This plan, tasks, and implementation trace to spec user stories and success criteria; ADR-0007/0008 record the hard decisions | PASS |

No gate violations. The new scope-state component and the simulated MCP
backends are justified in Complexity Tracking below.

**Post-design re-check (Phase 1 complete):** all gates still PASS. The
data model, tool contract, and benchmark artifacts keep observation-only data,
enforce confinement in the tool layer, preserve read-only credentials, report
incomplete matrices honestly, and record every reported number. No new
violation or unjustified complexity was introduced.

## Project Structure

### Documentation (this feature)

```text
specs/001-agentic-investigation/
├── plan.md
├── research.md
├── data-model.md
├── quickstart.md
├── contracts/
│   ├── history-api.md
│   ├── mcp-tools.md
│   ├── region-scope.md
│   ├── sla-rules.md
│   ├── target-profile.md
│   └── benchmark-artifacts.md
└── tasks.md                       # created by /speckit-tasks
```

### Source Code (repository root)

```text
services/weave/                    # existing Go module
├── cmd/history/main.go            # load SLA rules, start detection
├── internal/config/config.go      # SLA_RULES_FILE, DETECTION_*, BASELINE_MINUTES
├── internal/history/
│   ├── schema.go                  # alerts table; incidents origin/trigger columns
│   ├── incidents.go               # trigger/origin; configurable baseline; internal create/close
│   ├── detection.go               # rule pack load, evaluator loop, alert grouping/close
│   ├── http.go                    # additive incident fields, alerts, metrics query routes
│   ├── metrics_api.go             # edge/route metric range queries for agents/benchmark
│   └── evidence.go                # unchanged localization; baseline from incident
├── internal/contracts/contracts.go# additive DTOs: trigger, alert, metric range
└── internal/edge/edge.go          # proxy /api/metrics/* and /api/alerts to history

services/agent/                    # new Go module: local tooling, not deployed in cluster
├── go.mod
├── cmd/weave-agent/main.go        # investigator + MCP server subcommands
├── cmd/weave-bench/main.go        # benchmark harness
├── internal/model/                # Ollama chat/tools client; token accounting
├── internal/mcpclient/            # MCP client wrapper around the official SDK
├── internal/mcpserver/            # stdio server helpers; scope enforcement middleware
├── internal/scope/                # scope state file, frontier, expansion, denials
├── internal/tools/kube/           # kubectl-backed read-only tools
├── internal/tools/metrics/        # Weave metrics query tools
├── internal/tools/logs/           # kubectl logs tools; previous-container support
├── internal/tools/source/         # scoped local source search/read
├── internal/tools/weave/          # incident, topology, changes, gaps, expand_scope
├── internal/investigation/        # prompts, region payload, caps, transcript, diagnosis
├── internal/benchmark/            # profile load, scenarios, matrix, scoring, reports
│   └── artifacts/                 # run records, summaries
└── testdata/                      # fake model/tool fixtures

services/app/                      # existing Target App module
├── Dockerfile                     # BUILD_TAGS build argument for crashy variant
└── cmd/catalog/crashy.go          # test-only build tag; startup panic

deploy/
├── targets/target-app/
│   ├── profile.yaml               # namespace, Argo app, routes, source dirs, traffic
│   ├── slas.yaml                  # SLA rule pack
│   └── scenarios/                 # five fault scenarios with ground truth
├── platform/chaos/fixtures/       # catalog-http-500, auth-login-delay variants
├── platform/mcp/readonly-rbac.yaml# ServiceAccount, read-only Role/RoleBinding
└── charts/weave/...               # history env: SLA_RULES_FILE, DETECTION_*, BASELINE_MINUTES

scripts/
├── agent-benchmark.sh             # build agent/crash image, run pilot or full matrix
└── detection-acceptance.sh        # healthy run + catalog fault detection checks

reports/agent-benchmark/           # committed raw artifacts and summaries
docs/agent-benchmark.md            # recorded run and method
```

**Structure Decision**: Keep detection inside the existing `weave-history`
service because it already owns bucket semantics and evidence. Keep all local
agent, MCP server, and benchmark code in a new `services/agent` Go module so no
Weave image gains agent/model dependencies and the cluster deployment is
unchanged. The Target App gains only a build-tag crash fixture, not a normal
behavior change.

## Design Phases

- **Phase 1 — Detection and metric queries.** Rule pack loading, evaluator,
  Alerts table and grouping, auto-created incidents with trigger/origin,
  auto-close and evidence recompute, gap suppression, configurable baseline,
  additive `/api/metrics/*` and alerts endpoints, browser contract additions.
  Acceptance: healthy window creates no incident; catalog fault creates one
  incident; manual incidents unchanged.
- **Phase 2 — Scope, MCP servers, investigator.** Scope state and enforcement,
  five read-only tool servers, region payload, investigator loop with local
  model and caps, structured Diagnosis, transcripts and usage accounting,
  `expand_scope` and frontier logic, denial behavior. Acceptance: confined
  run cannot read outside scope; expansion works; diagnosis parses and scores.
- **Phase 3 — Benchmark.** Target Profile and scenarios, read-only RBAC,
  read-only kubeconfig generation, crash image build, fixture validation,
  harness matrix, paired arm execution, scoring, artifacts, report, pilot run.
  Acceptance: pilot completes and full matrix is recorded or explicitly
  reported incomplete.
- **Phase 4 — Documentation and verification.** Update glossary, browser and
  internal contracts, service docs, README, and acceptance/benchmark records;
  run Go tests, detection acceptance, pilot benchmark, and the read-only
  guarantees review.

## Complexity Tracking

| Violation | Why Needed | Simpler Alternative Rejected Because |
| --- | --- | --- |
| Shared Region Scope state across five MCP processes | Enforcement must survive a prompt-injection or a tool mistake in any server; the claim is auditable confinement | Filtering only in the agent loop is bypassable and cannot be verified independently of the model |
| Simulated MCP backends (kube/logs/metrics/source/weave) | The first slice has no real observability stack; production-shaped tools are the experiment's control surface | Installing Prometheus/Loki/GitHub MCPs adds platform work and dependencies unrelated to the confinement hypothesis |
| Configurable evidence baseline with a 5-minute benchmark profile | Keeps each incident's baseline uncontaminated without multi-hour warm-ups between episodes | A fixed 15-minute baseline would make the full matrix exceed 7 hours; a contaminated baseline would weaken region quality |
| Benchmark-only crash image | The requested code-error crash scenario needs a crashing image without changing normal Target App behavior | Killing pods or failing a command is not a code defect and would not exercise source/log diagnosis |
| Portable `weave-agent` and `weave-bench` binaries in a new module | Local agent and benchmark must not become cluster deployment units or bloat Weave images | Embedding them in `services/weave` couples tooling to the deployed service and its dependencies |

## Risks and Mitigations

- **Fixture reliability.** External and Postgres outages are simulated by
  repeated pod deletion; the pilot validates each scenario's detection and
  expected region before a recorded run. An unreliable fixture is swapped for a
  same-route equivalent and recorded before the matrix.
- **Local model tool-calling variance.** Tool-calling was probed successfully,
  but output can be malformed. One repair retry within the same caps; failures
  are recorded, never hidden.
- **Scope-state races.** One run at a time; atomic scope writes with a file
  lock; contract tests cover denial and expansion.
- **Region may exclude the root cause in dependency and crash scenarios.**
  That is an expected result; the benchmark reports region recall and expansions
  so treatment accuracy is attributable.
- **Long benchmark.** Estimated 3–4 hours. The pilot and full matrix run as a
  managed background process; partial artifacts are valid and reported as
  incomplete rather than extrapolated.
- **Baseline difference.** The benchmark's 5-minute baseline is recorded in the
  report; product defaults remain 15 minutes.
