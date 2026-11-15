# ADR-0007: Detect incidents from SLA rules inside Weave

- **Status:** Accepted
- **Date:** 2026-11-15
- **Context:** Weave agentic-investigation slice
- **Amends:** ADR-0005 — the "manually created only" Incident rule and the
  rejected "alerting stack" are superseded for detected incidents

## Context

The incident-replay slice creates Incidents only through
`POST /api/incidents`: the Operator or a harness selects a window and writes a
symptom. ADR-0005 justified this by saying an Incident "is not an alert and not
an anomaly", and deferred alerting.

The next slice needs an Incident to exist before a human selects anything: the
agentic investigation starts when the graph's own telemetry breaches a declared
objective. The environment has no Prometheus, Grafana, Loki, or Alertmanager,
the Target App exposes no `/metrics`, and Weave's only business-observable
telemetry is its own L7 edge and route history. `weave-history` already owns
the 5-second edge and route buckets, the 15-second flush grace, and the evidence
algorithm. Building an external alerting path would duplicate the metric truth
and add a platform the project explicitly avoids.

## Decision

- Detection is **Weave-native** and lives inside `weave-history`. No external
  alert stack, no new service, no new datastore.
- A **SLA Rule** is a declared, versioned service-level objective on observable
  traffic: scope (route or edge), metric (`error_rate_5xx`, `error_rate`,
  `p95_latency_ms`), threshold, evaluation window, minimum request volume, a
  sustain requirement, a clear requirement, and severity. Rule packs are part
  of the target profile, not hard-coded.
- The evaluator runs every 5 seconds over *flushed* buckets. A breach needs the
  window objective and minimum request volume for the sustain period. The first
  breach becomes a persisted **Alert** with the rule, observed value,
  threshold, and bucket.
- Alerts are grouped into **one Incident**. The first alert creates the
  Incident with the first breaching bucket as `window_start` and a generated
  symptom from the rule. Alerts whose scope shares a node or route with the
  open Incident join it; an open Incident suppresses new duplicates. After the
  incident closes, a cooldown prevents immediate re-creation.
- The Incident closes when all contributing rules have been clear for the
  clear requirement. `window_end` becomes the last breaching bucket, and the
  evidence snapshot is recomputed once so the Operator sees the full fault.
  The benchmark keeps the creation-time snapshot the Agent actually saw.
- Detection is suppressed when a Recording Gap overlaps the evaluation window.
  Missing data never becomes an incident, and the suppression is counted.
- Evaluation starts only after one rule window of flushed data exists after
  history start. Manual `POST /api/incidents` remains for replay and testing.
- Detected Incidents carry an additive `trigger` (rule id, metric, observed
  value, threshold, breach bucket, rule-pack version) and
  `origin: detection|manual`. Nothing else about the Incident contract changes.
- This slice detects only L7 rules. An edge that loses all requests (for
  example a crash-looping service behind a gateway) is detected through the
  downstream 5xx at the caller, not through an availability rule on the
  zero-request edge. Availability detection is deferred.

Accepted behavior: detection latency is roughly the flush grace plus
sustain/clear evaluation, about 15–30 seconds after the first breaching bucket.
Accuracy of detection is not scored in this slice; the agent benchmark measures
localization, tokens, and time. A healthy ten-minute run MUST create no
Incident.

## Considered options

- **Install kube-prometheus-stack and route Alertmanager webhooks into Weave.**
  Rejected: no metric source the stack can scrape, duplicate metric truth, and
  a new platform dependency for no detection quality gain.
- **A separate `weave-detect` service.** Rejected for now: it needs a new
  metrics-query contract, a new deployment, and a second owner of bucket
  semantics, while `weave-history` already computes the evidence the Incident
  needs.
- **Detect in `weave-live` for lower latency.** Rejected: live counters are
  in-memory, it would move Incident creation across a service boundary, and it
  would still wait for history to materialize the evidence.

## Consequences

- The Incident model changes meaning: it remains an investigation, but it can
  now be started by grouped Alerts. `GLOSSARY.md`, ADR-0005, the browser
  contract, and the API tests are updated.
- Detection is as good as L7 coverage. This is visible in the crash-loop
  scenario, where the initial region contains the caller but not the crashing
  dependency.
- The stable detection rule pack becomes target-app-specific configuration,
  which the slice-2 target profile must supply.
- External alert ingestion can be added later as an adapter feeding the same
  Alert and grouping path; it is not part of this decision.
