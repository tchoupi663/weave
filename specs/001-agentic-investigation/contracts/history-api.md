# History API additions

These additions are additive to the frozen browser contract in
`docs/weave-contract.md`. Existing paths, fields, and incident JSON remain
valid. `weave-api` proxies `/api/*` to `weave-history` as it does today.

## Incident additions

`GET /api/incidents`, `GET /api/incidents/{id}`, `POST /api/incidents`, and
`PATCH /api/incidents/{id}` responses gain two optional fields:

```json
{
  "origin": "detection",
  "trigger": {
    "rule_id": "catalog-products-5xx",
    "rule_version": "target-app-v1",
    "scope": {"kind": "route", "method": "GET", "path": "/api/products"},
    "metric": "error_rate_5xx",
    "observed": 1.0,
    "threshold": 0.01,
    "breach_bucket": "2026-11-15T12:00:05Z",
    "detected_at": "2026-11-15T12:00:21Z"
  }
}
```

- Manual incidents omit both fields (no change to their legacy JSON shape).
- `origin` is `manual` or `detection`; absent means `manual` for readers.
- `trigger` is immutable after creation and is only present when
  `origin=detection`.
- `evidence` and timeline fields are unchanged.

## Alerts

`GET /api/incidents/{id}/alerts` returns the Alerts grouped into the incident,
oldest first:

```json
{
  "alerts": [
    {
      "id": "a1b2c3d4e5f60718",
      "incident_id": "8a6a69ef4e60b57a1bbbf824",
      "rule_id": "catalog-products-5xx",
      "rule_version": "target-app-v1",
      "scope": {"kind": "route", "method": "GET", "path": "/api/products"},
      "metric": "error_rate_5xx",
      "observed": 1.0,
      "threshold": 0.01,
      "breach_bucket": "2026-11-15T12:00:05Z",
      "created_at": "2026-11-15T12:00:21Z",
      "grouped_at": "2026-11-15T12:00:21Z"
    }
  ]
}
```

Unknown incident returns 404. Empty list is `{"alerts":[]}`, never `null`.

## Edge metric range

`GET /api/metrics/edges?from=RFC3339&to=RFC3339&bucket=5s|1m&edge_id=...`

- `from` and `to` are required, RFC3339 UTC; `from` must precede `to`.
- `bucket` defaults to `5s`; `1m` reads the rollup table.
- `edge_id` is optional and repeatable; when absent all edges are returned.
- Results are ordered by `bucket_start`, then `edge_id`.

```json
{
  "bucket_seconds": 5,
  "from": "2026-11-15T11:45:00Z",
  "to": "2026-11-15T12:05:00Z",
  "rows": [
    {
      "bucket_start": "2026-11-15T12:00:05Z",
      "edge_id": "workload:weave/gateway->workload:weave/catalog:8082",
      "source": "workload:weave/gateway",
      "target": "workload:weave/catalog",
      "protocol": "HTTP",
      "port": 8082,
      "connections": 4,
      "events": 12,
      "requests": 12,
      "responses": 12,
      "status_2xx": 0,
      "status_3xx": 0,
      "status_4xx": 0,
      "status_5xx": 12,
      "status_other": 0,
      "p50_ms": 2.1,
      "p95_ms": 4.0,
      "latency_source": "client"
    }
  ]
}
```

## Route metric range

`GET /api/metrics/routes?from=RFC3339&to=RFC3339&bucket=5s&edge_id=...&method=...&path=...`

- Same time and ordering rules; only the 5-second route table exists.
- `edge_id`, `method`, and `path` are optional filters.
- Base path uses the normalized route (for example `/api/products/:id`).

```json
{
  "bucket_seconds": 5,
  "from": "2026-11-15T11:45:00Z",
  "to": "2026-11-15T12:05:00Z",
  "rows": [
    {
      "bucket_start": "2026-11-15T12:00:05Z",
      "edge_id": "workload:weave/gateway->workload:weave/catalog:8082",
      "method": "GET",
      "path": "/api/products",
      "requests": 12,
      "responses": 12,
      "status_2xx": 0,
      "status_3xx": 0,
      "status_4xx": 0,
      "status_5xx": 12,
      "status_other": 0
    }
  ]
}
```

## Errors

- `400` for malformed or missing timestamps, `from >= to`, unknown bucket, or
  an invalid filter.
- `404` for an unknown incident on the alerts route.
- `503` when the database is unavailable, consistent with the existing
  history behaviour.

## Detection health

`GET /api/health` on `weave-history` gains additive fields:

```json
{
  "status": "ok",
  "detection_enabled": true,
  "rules_loaded": 6,
  "rule_pack_version": "target-app-v1",
  "alerts_created": 12,
  "incidents_created": 3,
  "detection_suppressed_gaps": 0
}
```

`weave-api` may include these in the aggregated health payload under
`dependencies.history_detail` in a later change; this slice keeps the existing
aggregate shape unchanged.
