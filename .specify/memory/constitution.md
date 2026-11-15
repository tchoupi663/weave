<!--
Sync Impact Report
==================
Version change: (unratified template) → 1.0.0
Modified principles: none (first ratification; template placeholders replaced)
Added sections: Core Principles (I–VI), Additional Constraints, Development Workflow, Governance
Removed sections: none
Deferred TODOs: none
-->

# Weave Constitution

## Core Principles

### I. Observation over declaration

Observed traffic is the only source of graph nodes and edges. Declared
architecture is lifecycle evidence, never an observed edge and never a
substitute for traffic. Raw flows, spans, headers, query strings, and bodies
are not retained; only the aggregates the product documents. We MUST NOT
invent request rates, latencies, users, or benchmark numbers to make a view or
a report look complete. When the instrument was not recording, the product says
so instead of showing zero.

### II. Bounded, auditable investigation

An Incident is investigated inside a bounded Suspect Region. An Agent MUST NOT
be able to search or query the whole architecture to find a localised root
cause. Access outside the initial region is denied until an explicit, recorded
Scope Expansion along the observed topology, and every expansion is visible in
the investigation record. The Agent does not remediate. Localization remains a
suspect region, not a causal verdict; a Root Cause is a hypothesis supported by
evidence.

### III. Read-only by default

Weave, its API, and its Agent-facing tool servers are read-only. No Weave
service gains Kubernetes API write access; no Agent-facing tool mutates the
Target App, the cluster, or the investigation. Fault injection is owned by an
external benchmark harness, is scoped to a Fault Scenario, and is reverted
when the scenario ends. Authorization is least-privilege: any tool server that
reads cluster state uses a namespace-scoped read-only identity.

### IV. Honest instruments and failure isolation

A Recording Gap means the instrument was not recording, not that traffic was
zero. Each service degrades only its own surface; liveness and readiness never
depend on another service; aggregated health reports dependencies without
turning an outage into a false zero. Every claim about data completeness is
scoped to what the recording path actually observes, and missing evidence is
reported as missing.

### V. Evidence and measured claims

Every feature ships with reproducible acceptance evidence. Benchmark and
investigation claims report time to Root Cause, token use, accuracy, and search
breadth from recorded artifacts, never from memory or intuition. Every report
records the environment, model and prompt versions, rule-pack and scenario
versions, and the exact command that produced it. A claim that cannot be
reproduced from a committed artifact is not accepted.

### VI. Spec-driven delivery

Features are delivered in the order specification, plan, tasks, implementation.
The specification states what and why; the plan states how and justifies
complexity; tasks trace back to acceptance criteria. Hard-to-reverse,
surprising, or trade-off decisions are recorded as ADRs. The constitution
supersedes other practices; deviations require a documented, reviewed reason.

## Additional Constraints

- **Local-first, laptop-scale.** Acceptance and benchmarks run on the local
  kind cluster and the locally served model. Cloud services are optional
  replacements, never required for the first slice.
- **No new observability platform in the first agentic slice.** Simulated
  tool backends are acceptable, but the tool contract must let a real
  Prometheus, Loki, Kubernetes, or code-host MCP replace a simulated server
  without changing the Agent.
- **Single writer per store, versioned contracts.** `weave-history` owns bucket
  semantics; `weave-inventory` owns change events; `weave-live` owns
  observations. Cross-service contracts are versioned and additive.
- **Data retention is bounded and documented.** History keeps the documented
  aggregates; no raw flows or spans; tool transcripts must not contain secrets.
- **Benchmark integrity.** Ground truth stays hidden from the Agent; the
  control and treatment arms share the same model, prompt skeleton, tool
  surface, and resource caps except for the intervention under test; fault
  fixtures are validated before a recorded run.

## Development Workflow

- Run the Spec Kit workflow: constitution → specify → clarify (when needed) →
  plan → tasks → implement. A plan that violates a principle MUST be rejected or
  revised.
- Specification acceptance scenarios are the source of test and verification
  work. Unit, contract, and integration tests cover the seams named in the
  plan; recorded acceptance and benchmark runs cover the product outcomes.
- ADRs are required when a decision is hard to reverse, surprising without
  context, and the result of a real trade-off. ADRs are amended explicitly;
  they are never silently rewritten.
- Benchmark reports are committed with the run artifacts they summarize, and
  they include the healthy negative control and the region-recall measure.

## Governance

The constitution supersedes conflicting practices. Amendments require a
documented proposal, the affected ADRs and documents updated in the same
change, and a version bump by semantic versioning: MAJOR for an incompatible
removal or redefinition of a principle, MINOR for a new or materially expanded
principle, PATCH for clarification. Compliance is reviewed at specification,
plan, and task review; any deviation is recorded and justified in the plan's
Complexity Tracking. The constitution is ratified once and amended only by a
reviewed change with a migration note.

**Version**: 1.0.0 | **Ratified**: 2026-11-15 | **Last Amended**: 2026-11-15
