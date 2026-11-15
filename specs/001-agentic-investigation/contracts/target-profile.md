# Target Profile and Fault Scenario contract

A Target Profile is the only place target-app-specific values live. Detection
reads the SLA rule file path from the Weave history configuration; the
benchmark reads the full profile. Slice 2 adds a new profile and does not
change Weave or the investigator.

## `profile.yaml`

```yaml
version: target-app-v1
name: target-app
cluster:
  namespace: weave
  weave_namespace: weave-system
  argo_application: target-app-local
  read_only_role: weave-mcp-reader
  read_only_kubeconfig: ""   # generated per run; never committed

traffic:
  driver: simulator
  config: services/simulator/testdata/agent-benchmark.yaml
  reset_target_app: true
  concurrency: 50

evidence_baseline_minutes: 5
grouping_window_seconds: 120
cooldown_seconds: 300
sla_rules: deploy/targets/target-app/slas.yaml
expected_topology: expected-topology.yaml

workloads:
  - name: frontend
    source_root: services/app
  - name: gateway
    source_root: services/app
  - name: auth
    source_root: services/app
  - name: catalog
    source_root: services/app
  - name: orders
    source_root: services/app
  - name: payment
    source_root: services/app
  - name: external
    source_root: services/app
  - name: postgres
    source_root: ""            # no source access

scenarios:
  - catalog-http-500
  - auth-login-latency
  - external-payment-outage
  - postgres-outage
  - catalog-crashloop

agents:
  confined:
    scope_limits: {max_expansions: 3, max_added_nodes: 8}
  unconfined: {}
```

### Validation

- `version`, `name`, `cluster.namespace`, `cluster.weave_namespace`,
  `traffic.config`, `sla_rules`, `expected_topology`, and at least one scenario
  are required.
- Every workload in a scenario's ground truth and expected initial region must
  exist in `workloads`.
- `evidence_baseline_minutes` must be at least one SLA rule window and at most
  60.
- Source roots must be relative paths inside the repository; benchmark,
  scenario, and ground-truth locations are never allowed as source roots.
- The simulator config must exist and declare the expected topology.
- The read-only kubeconfig path is generated at run time; the profile must not
  contain a token.

## Fault Scenario YAML

`deploy/targets/target-app/scenarios/<id>.yaml`:

```yaml
id: catalog-http-500
description: Catalog returns HTTP 500 on the product list route
fault_duration_seconds: 240
recovery_seconds: 120
injection:
  mechanism: http_chaos
  fixture: deploy/platform/chaos/fixtures/catalog-http-500.yaml
ground_truth:
  root_cause_workloads: [catalog]
  failure_mode: http_error
  affected_route: GET /api/products
  affected_edge: workload:weave/gateway->workload:weave/catalog:8082
  expected_initial_region_workloads: [catalog, gateway, frontend]
  expected_region_contains_root_cause: true
  expected_expansion: false
notes: >
  The HTTPChaos fixture matches the product list route only. Detail routes are
  unaffected.
```

External and Postgres outage scenarios use `mechanism: pod_delete_loop` with
`target: deployment/external` or `target: statefulset/postgres`, a
`delete_interval_seconds: 20`, and ground truth `dependency_unavailable`.
The crash scenario uses `mechanism: image_swap`, `image:
weave-target-app:crashy`, `target: deployment/catalog`, and ground truth
`bad_deployment`.

### Ground-truth rules

- `root_cause_workloads` must be exact lower-case workload names as they appear
  in Weave nodes.
- `failure_mode` is one of `http_error`, `latency`, `dependency_unavailable`,
  `bad_deployment`, `other`.
- `affected_route` uses `METHOD /normalized/path`.
- `expected_initial_region_workloads` is used only for reporting region recall;
  it is not hidden from the agent.
- `fixture_hash` is computed by the harness (SHA-256 of the fixture file or
  image digest) and stored in every run record.
- The harness refuses to score a scenario whose `fixture_hash` does not match
  the hash recorded at pilot validation.

## Stability

The profile and scenario files are part of the target slice configuration.
Changing an SLA threshold, fault duration, mechanism, ground truth, or source
root invalidates comparison with a previously recorded benchmark; the report
must name the changed file and hash.
