# SLA rule pack

The rule pack is YAML loaded by `weave-history` from `SLA_RULES_FILE` at
startup. It is part of the Target Profile. The file is versioned; changes
require a history restart in this slice.

## Format

```yaml
version: target-app-v2
rules:
  - id: catalog-products-5xx
    description: Product list availability
    severity: critical
    scope:
      kind: route
      method: GET
      path: /api/products
    metric: error_rate_5xx
    threshold: 0.01
    window_seconds: 60
    min_requests: 10
    sustain_evaluations: 2
    clear_evaluations: 12

  - id: catalog-products-p95
    description: Catalog product list latency
    severity: warning
    scope:
      kind: edge
      edge_id: workload:weave/gateway->workload:weave/catalog:8082
    metric: p95_latency_ms
    threshold: 40
    window_seconds: 60
    min_requests: 10
    sustain_evaluations: 2
    clear_evaluations: 12

  - id: frontend-gateway-products-5xx
    description: Caller-side product errors
    severity: critical
    scope:
      kind: route
      method: GET
      path: /api/products
    metric: error_rate_5xx
    threshold: 0.01
    window_seconds: 60
    min_requests: 10
    sustain_evaluations: 2
    clear_evaluations: 12
```

## Field rules

- `version`: non-empty rule-pack version copied into every Alert and trigger.
- `id`: unique, non-empty.
- `description`: operator-facing text; used to generate the Incident symptom
  when no operator text exists.
- `severity`: `critical` or `warning`.
- `scope.kind`: `edge` or `route`.
  - `edge` requires `edge_id`.
  - `route` requires `method` and normalized `path`, and an optional `edge_id`
    to restrict the rule to one observed edge.
- `metric`: `error_rate_5xx`, `error_rate`, or `p95_latency_ms`.
- `threshold`: a fraction in `[0,1]` for rates, milliseconds for latency.
- `window_seconds`: at least 30 and at most 900; default 60.
- `min_requests`: at least 1; the window is ignored below this volume.
- `sustain_evaluations`: at least 1; the number of consecutive evaluation
  ticks the condition must hold.
- `clear_evaluations`: at least 2; the number of consecutive clear ticks before
  the contributing Alert is considered recovered.

## Evaluation semantics

- Tick every `DETECTION_INTERVAL` (default 5s) after at least one window of
  flushed data plus the flush grace exists.
- Build the trailing window `[now - window, now]` from flushed 5s buckets. The
  history fetch uses `bucket_start < now - grace`.
- Error rate = `(sum(status_4xx) + sum(status_5xx)) / sum(requests)` for
  `error_rate`, and `sum(status_5xx) / sum(requests)` for `error_rate_5xx`.
  Requests below `min_requests` are not a breach.
- p95 = request-weighted mean of the bucket p95 values in the window. This is an
  approximation and is documented; it is not a true request-weighted
  percentile.
- An Alert is created once per rule episode, on the evaluation that completes
  `sustain_evaluations`; `breach_bucket` is the first bucket in the sustained
  run, not the tick time.
- A rule that stays in breach does not create a second Alert for the same
  incident. After the rule clears and the incident closes, the cooldown
  prevents immediate re-creation.
- If any Recording Gap intersects the window, the evaluation is skipped and
  counted; a gap never produces an Alert.
- Rule evaluation is disabled when `SLA_RULES_FILE` is empty.

## Incident grouping

- First Alert creates an Incident with `origin=detection`, generated symptom
  `SLA breach: <description> (<metric> <observed> vs <threshold>)`, and
  `window_start` = first Alert's `breach_bucket`.
- A later Alert joins the open Incident if its scope edge or route shares a
  node with the Incident's suspect edges, or if the route is identical, within
  `GROUPING_WINDOW_SECONDS` (default 120).
- An open Incident suppresses a second Incident for the same scope. After
  close, `COOLDOWN_SECONDS` (default 300) prevents immediate re-creation for
  the same rule/scope.
- Close when every contributing Alert's rule has been clear for its
  `clear_evaluations`. Set `window_end` to the last breaching bucket, recompute
  evidence once, and mark closed.

## Validation

- The file must parse, have a non-empty version, unique IDs, and valid scope,
  metric, threshold, window, volume, sustain, and clear values.
- A malformed rule pack prevents `weave-history` from starting detection; it
  does not prevent the service from serving existing incidents. The error names
  the invalid rule.
- Route rules whose path does not appear in recent route metrics are allowed;
  they simply never breach. The benchmark profile validation additionally
  checks that every scenario's affected route is covered by at least one rule.
