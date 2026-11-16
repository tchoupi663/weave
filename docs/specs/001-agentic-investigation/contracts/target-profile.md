# Target Profile and Fault Scenario contract

A Target Profile is the only place target-app-specific values live. Detection
reads the SLA rule file path from the Weave history configuration; the
benchmark reads the full profile. A different Target App is a new profile, not
a code change in Weave or the investigator.

## `profile.yaml`

```yaml
version: target-app-v2
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
sla_rules: deploy/charts/weave/charts/history/files/slas.yaml
expected_topology: expected-topology.yaml
templates: templates.yaml

workloads:
  - name: frontend
    source_roots: [services/app/frontend/src, services/app/frontend/nginx.conf]
  - name: gateway
    source_roots: [services/app/cmd/gateway, services/app/internal/gatewaysvc, services/app/internal/httpx]
  - name: auth
    source_roots: [services/app/cmd/auth, services/app/internal/authsvc]
  - name: catalog
    source_roots: [services/app/cmd/catalog, services/app/internal/catalogsvc]
  - name: orders
    source_roots: [services/app/cmd/orders, services/app/internal/orderssvc]
  - name: payment
    source_roots: [services/app/cmd/payment, services/app/internal/paymentsvc]
  - name: external
    source_roots: [services/app/cmd/external, services/app/internal/externalsvc]
  - name: postgres
    source_roots: []

scenarios:
  - catalog-http-500
  - auth-login-latency
  - external-payment-outage
  - postgres-outage
  - catalog-crashloop
  - orders-replicas-zero
  - payment-image-pull-backoff
  - catalog-cpu-stress
  - catalog-network-loss

agents:
  confined:
    scope_limits: {max_expansions: 3, max_added_nodes: 8}
  unconfined: {}
```

### Validation

- `version`, `name`, `cluster.namespace`, `cluster.weave_namespace`,
  `traffic.config`, `sla_rules`, and at least one scenario are required.
- Every workload in a scenario's ground truth and expected initial region must
  exist in `workloads`.
- Every scenario must declare a `template` that exists in the profile's
  `templates.yaml`.
- `evidence_baseline_minutes` must be at least one SLA rule window and at most
  60.
- Source roots must be relative paths inside the repository; benchmark,
  scenario, ground-truth, and template locations are never allowed as source
  roots.
- The simulator config must exist and declare the expected topology.
- The read-only kubeconfig path is generated at run time; the profile must not
  contain a token.

## Fault classes (`templates.yaml`)

Seven service-level fault classes are declared. A class has an `id`, a display
name, a `status` of `covered` or `coming_soon`, a problem-class description, and
the scenarios that cover it. Classes are design patterns, not fixtures.

```yaml
version: target-app-v2
classes:
  - id: internal_logic_bug
    status: covered
    scenarios: [catalog-http-500, auth-login-latency]
  - id: external_dependency_handling
    status: covered
    scenarios: [external-payment-outage, postgres-outage]
  - id: deployment_manifest_error
    status: covered
    scenarios: [catalog-crashloop, orders-replicas-zero, payment-image-pull-backoff]
  - id: resource_infrastructure_fault
    status: covered
    scenarios: [catalog-cpu-stress, catalog-network-loss]
  - id: data_contract_mismatch
    status: coming_soon
  - id: resource_identity_mismatch
    status: coming_soon
  - id: configuration_mismatch
    status: coming_soon
```

The current claim is **four of seven classes, at service level**. The
coming-soon classes do not have scenarios yet and must not be counted as
covered.

## Fault Scenario YAML

`deploy/targets/target-app/scenarios/<id>.yaml`:

```yaml
id: catalog-http-500
description: Catalog returns HTTP 500 on the product list route
template: internal_logic_bug
fault_duration_seconds: 240
recovery_seconds: 120
injection:
  mechanism: http_chaos
  fixture: deploy/platform/chaos/fixtures/bench/catalog-http-500.yaml
ground_truth:
  root_cause_workloads: [catalog]
  failure_mode: http_error
  affected_route: GET /api/products
  affected_edge: workload:weave/gateway->workload:weave/catalog:8082
  expected_initial_region_workloads: [catalog, gateway, frontend]
  expected_region_contains_root_cause: true
  expected_expansion: false
```

### Injection mechanisms

| mechanism | fields | use |
| --- | --- | --- |
| `http_chaos` | `fixture` | HTTP error or delay on a route |
| `stress_chaos` | `fixture` | CPU stress in a workload |
| `network_chaos` | `fixture` | packet loss or delay on a workload path |
| `scale_zero` | `target` | replicas set to zero (`deployment/...` or `statefulset/...`) |
| `image_swap` | `target`, `image` | image that crash-loops or cannot be pulled |
| `pod_delete_loop` | `target`, `delete_interval_seconds` | legacy pod churn |

The harness restores the original replicas, image, and any Chaos object on
cleanup. `image_swap` scales the old pod to zero first so a single-replica
rollout produces an outage instead of leaving the old pod serving.

### Ground-truth rules

- `root_cause_workloads` must be exact lower-case workload names as they appear
  in Weave nodes.
- `failure_mode` is one of `http_error`, `latency`, `dependency_unavailable`,
  `bad_deployment`, `resource_saturation`, `network_fault`, `other`.
- `affected_route` uses `METHOD /normalized/path`.
- `affected_edge` uses `workload:<namespace>/<name>->workload:<namespace>/<name>:<port>`.
- `expected_initial_region_workloads` is used only for reporting region recall;
  it is not hidden from the agent.
- A network fault keeps the affected workload as the Root Cause and names the
  affected Edge separately. The contract does not define an edge-only root
  cause.
- `fixture_hash` is computed by the harness (SHA-256 of the fixture file or
  image digest) and stored in every run record.
- The harness refuses to score a scenario whose `fixture_hash` does not match
  the hash recorded at pilot validation.

## Stability

The profile, scenario, and template files are part of the target slice
configuration. Changing an SLA threshold, fault duration, mechanism, ground
truth, template mapping, or source root invalidates comparison with a
previously recorded benchmark; the report must name the changed file and hash.
