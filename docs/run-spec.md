# Run Spec

The Simulator is a CLI plus a YAML Run spec. There is no UI. v1 is healthy-only: no crash, saturation, latency injection, or fault timeline.

## CLI

```text
sim run --config run.yaml
sim validate --config run.yaml
sim run --config run.yaml --log-requests
sim run --config run.yaml --report report.json
```

- `run` executes the Run and writes the JSON report to stdout or `--report`.
- `validate` parses the config and exits without traffic.
- `--log-requests` adds a full per-request log to the report; the default is aggregates plus failures.
- Exit `0` if every configured Journey and its required steps were exercised and no Journey failed; `1` if any Journey failed or a required step was not exercised; `2` on config or preflight error.

## YAML

```yaml 
run:
  id: auto
  seed: 42
  target: https://localhost:8443
  expected_topology: expected-topology.yaml
  duration: 10m
  concurrency: 50
  ramp: 30s
  think_time: {min: 1s, max: 5s}
  mix: {bounce: 30, browse: 50, order: 20}
  accounts: {pool: 2000}
  reset: true
  tls: {insecure_skip_verify: true}
```

| Field | Meaning | Default |
|---|---|---|
| `id` | Run identifier | generated |
| `seed` | Seed for all randomness | required |
| `target` | Frontend base URL | required |
| `expected_topology` | Path to the expected topology YAML | `expected-topology.yaml` |
| `duration` | Measured window after preflight and reset | `10m` |
| `concurrency` | Active Users | `50` |
| `ramp` | Linear ramp from 0 to `concurrency` | `30s` |
| `think_time` | Uniform pause between user-visible steps | `1s`–`5s` |
| `mix` | Journey shares; must sum to 100 | `30/50/20` |
| `accounts.pool` | Number of seeded accounts | `2000` |
| `reset` | Call `/admin/reset` before the measured window | `true` |
| `tls.insecure_skip_verify` | Accept the self-signed Frontend certificate | `true` |

There is no `faults` section in v1.

## Lifecycle

1. Validate the config.
2. Preflight: `GET /health` until 200 (60s timeout), then `GET /api/products` to verify the chain.
3. Reset: `POST /admin/reset` through the Frontend; wait for 200.
4. Start the measured window.
5. Ramp to `concurrency`.
6. Run for `duration`; stop admitting new Users at T.
7. Wait for active Users to finish, up to 60s; cancel the rest.
8. Write the report and exit.

Preflight and reset are setup traffic; they are outside `started_at`/`ended_at`.

## Journeys

Each User is assigned exactly one Journey by the seeded mix. The sequences are built in for v1.

### Bounce (30%)

1. `GET /` (HTML).
2. Fetch the assets referenced by the landing page.
3. Leave.

### Browse (50%)

1. `GET /` and assets.
2. `GET /api/products`.
3. For 2–4 products chosen from the list:
   - `GET /products/{id}` (HTML).
   - `GET /api/products/{id}`.
4. Leave.

Assets are fetched once per User at landing and reused from the client cache; later pages fetch HTML and API only.

### Order (20%)

1. `GET /` and assets.
2. `POST /api/auth/login` with an account from the pool; keep the token.
3. `GET /api/products`.
4. For 2–4 products:
   - `GET /products/{id}` (HTML).
   - `GET /api/products/{id}`.
5. `GET /cart` (HTML).
6. `GET /checkout` (HTML).
7. `POST /api/orders` with the items and total; keep `orderId`.
8. `GET /payment` (HTML).
9. `POST /api/payments` with `orderId`, amount, and a dummy card.
10. `GET /orders/{orderId}` (HTML).
11. `GET /api/orders/{orderId}`.
12. Leave.

Login happens before browsing in the confirmed Order Journey. `POST /api/orders` and `POST /api/payments` are never retried.

## Timing and concurrency

- A worker holds one active User; when the User finishes, the worker starts a new User until the duration ends.
- `ramp` spaces new Users linearly.
- `think_time` is uniform between user-visible steps; API calls within a page use a 100–500ms pause.
- Each User gets a deterministic seed derived from the run seed and the User index.
- Accounts are assigned round-robin from a seeded shuffle of the pool; the pool wraps when exhausted.

## Failure semantics

- Request timeout: 5s.
- Idempotent GET: one retry.
- `POST /api/orders` and `POST /api/payments`: no retry.
- A failed step aborts the Journey, records the failure, and releases the User; the worker starts a new User.
- A failed Journey increments `journeys.*.started` and `failed`, not `completed`.
- The Run exits `1` if any Journey fails or a configured Journey or required step is never exercised.

## Report

```json
{
  "run_id": "...",
  "seed": 42,
  "config": { "...": "..." },
  "started_at": "...",
  "ended_at": "...",
  "source_ip": "...",
  "journeys": {
    "bounce": {"started": 0, "completed": 0, "failed": 0},
    "browse": {"started": 0, "completed": 0, "failed": 0},
    "order":  {"started": 0, "completed": 0, "failed": 0}
  },
  "steps": {
    "landing": {"count": 0, "errors": 0, "p50_ms": 0, "p95_ms": 0, "bytes": 0}
  },
  "failures": [
    {"user": 1, "journey": "order", "step": "payment", "error": "...", "at": "..."}
  ],
  "expected_topology": {
    "nodes": [],
    "edges": []
  }
}
```

- `source_ip`: the laptop egress IP if detected, otherwise `null`. The comparator treats any non-cluster IP as the Source IP.
- `steps`: client-side metrics; the Simulator does not observe internal edges.
- `expected_topology`: embedded from the expected topology file.
- `--log-requests` adds `requests`, one entry per request (`step`, `method`, `url`, `status`, `ms`, `bytes`).

## Scope

- v1 generates healthy traffic only. Fault scenarios are a later iteration.
- The Simulator does not query Weave. A separate benchmark tool compares the report to Weave's `/api/topology` and `/api/telemetry`.
- The Simulator does not own the Target App deployment or reset internals; it calls the Frontend's `/admin/reset`.
