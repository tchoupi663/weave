# ADR-0009: Service-level fault classes

- **Status:** Accepted
- **Date:** 2026-11-15
- **Deciders:** Product and engineering
- **Amends:** ADR-0008

## Context

The benchmark started with five Fault Scenarios: catalog HTTP 500, auth login
latency, external provider outage, Postgres outage, and catalog crash-loop.
Those scenarios prove that Detection, Region Scope, and Agent diagnosis work,
but they cover only a narrow slice of the fault classes an on-call agent meets.

The project does not have code-level fault machinery: there is no source
mutation, no program-dependence analysis, and no configuration-editing agent.
The benchmark therefore cannot reproduce defects that live inside a program.
It can, however, exercise the service-level symptom classes that a production
agent observes, provided the classes are named honestly and the gaps are
explicit.

## Decision

The Target Profile declares seven **Fault Classes** in
`deploy/targets/target-app/templates.yaml`. A class is a design pattern; a
Fault Scenario is one concrete instance of a class. Every scenario names its
class in a `template` field.

Four classes are covered:

1. **Internal logic fault** — the service's own boundary returns errors or
   becomes slow. Covered by catalog HTTP 500 and auth login latency as
   service-level symptom proxies, not as code defects.
2. **External dependency handling** — a database or provider is unavailable and
   the caller does not handle it. Covered by external payment outage and
   Postgres outage.
3. **Deployment manifest error** — replicas set to zero, an unavailable image,
   or a crash-loop after a rollout. Covered by catalog crash-loop, orders
   replicas-zero, and payment ImagePullBackOff.
4. **Resource and infrastructure fault** — CPU saturation or network loss
   degrades a healthy workload. Covered by catalog CPU stress and catalog
   network loss.

Three classes are declared as **coming soon** and are not covered:

- **Data contract mismatch** — services disagree about fields, schemas, or
  payload shape. Needs source or protocol-level fault machinery.
- **Resource identity mismatch** — labels, selectors, or service identities
  point at the wrong resource. Needs a manifest/config change outside the
  current fixtures.
- **Configuration mismatch** — environment values, constants, or feature flags
  are wrong for the running code. Needs configuration injection and code-aware
  assertions.

The nine scenarios are:

| class | scenario |
| --- | --- |
| internal logic fault | catalog-http-500 |
| internal logic fault | auth-login-latency |
| external dependency handling | external-payment-outage |
| external dependency handling | postgres-outage |
| deployment manifest error | catalog-crashloop |
| deployment manifest error | orders-replicas-zero |
| deployment manifest error | payment-image-pull-backoff |
| resource and infrastructure fault | catalog-cpu-stress |
| resource and infrastructure fault | catalog-network-loss |

## Consequences

- Coverage is reported as **four of seven classes, at service level**. Weave
  does not claim code-level root-cause coverage or parity with any external
  benchmark.
- The benchmark report must aggregate by Fault Class as well as by scenario,
  and must show the coming-soon classes rather than silently dropping them.
- New failure modes are part of the diagnosis vocabulary:
  `resource_saturation` for stress and `network_fault` for network loss.
  `bad_deployment` continues to cover zero replicas, bad images, and
  crash-loops.
- A network fault keeps the affected workload as the Root Cause and names the
  affected Edge separately; the taxonomy does not introduce an edge-only
  root-cause entity.
- Adding a class or changing a class mapping is a benchmark revision: the
  report must name the changed registry and scenario hashes.
- The three coming-soon classes need real fixture and validation work before a
  scenario is added; declaring a scenario without end-to-end detection would
  repeat the failure mode this ADR is meant to prevent.

## Alternatives considered

- **Keep five scenarios and call the taxonomy complete.** Rejected: coverage
  would remain accidental and the gaps invisible.
- **Label service-level proxies as code-level faults.** Rejected: an injected
  HTTP 500 is not an internal logic bug; the claim would not survive review.
- **Add scenarios for all seven classes now.** Rejected: the three missing
  classes need fault machinery the project does not have, so they would be
  paper coverage.
