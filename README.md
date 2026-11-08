# Weave

Weave is a topology testbed: a dummy e-commerce **Target App** and a **Simulator** that drives it with real HTTP traffic. Weave, the topology builder, observes the traffic; the graph UI is out of scope here.

## What is here

- `services/app/` — the Target App: Vite frontend behind NGINX, API Gateway, Auth, Catalog, Orders (Postgres), Payment, and the External Payment Provider.
- `services/simulator/` — the CLI that runs the Bounce, Browse, and Order Journeys and writes a JSON report with the Expected Topology.
- `compose.yaml` — local Docker Compose stack for the Target App.
- `deploy/` — Kubernetes: Helm chart, kind cluster + Cilium bootstrap, Argo CD applications, per-environment values. See [deploy/README.md](deploy/README.md).
- `expected-topology.yaml` — the expected nodes and edges used by the inference benchmark.
- `scripts/integration-test.sh` — unit tests, frontend build, Compose, contract checks, and a short Simulator run.

## Requirements

- Docker with Compose
- Go 1.24+
- Node 20+ (for local frontend builds; Docker also builds the frontend)

## Run the Target App

```bash
docker compose up -d --build
docker compose ps
curl -k https://localhost:8443/health
```

Open <https://localhost:8443> and accept the self-signed certificate. Demo login: `user0001` / `password`.

The Frontend is the only published service. Everything else is internal to the Compose network:

| Service | Address | Notes |
|---|---|---|
| Frontend | `https://localhost:8443` | NGINX, TLS, SPA, `/api` proxy |
| Gateway | `gateway:8080` | routing and token validation |
| Auth | `auth:8081` | 2,000 in-memory accounts |
| Catalog | `catalog:8082` | 20 in-memory products |
| Orders | `orders:8083` | Postgres-backed |
| Payment | `payment:8084` | calls the External Payment Provider |
| External Payment Provider | `external:8443` | HTTPS, self-signed |
| Postgres | `postgres:5432` | `orders` database |

## Run the Simulator

The Simulator runs on the host, outside the Compose stack, so its traffic enters from the host IP.

```bash
cd services/simulator
go run ./cmd/sim validate --config run.yaml
go run ./cmd/sim run --config run.yaml --report ../../reports/run.json
```

That is the documented 10-minute, 50-user shape. The shorter `testdata/run-short.yaml` is used by the integration test. The Expected Topology path is relative to the working directory. `--log-requests` adds a per-request log to the report.

## Test

```bash
./scripts/integration-test.sh
```

The script runs the Go unit tests, the Vite build, `docker compose up --build`, HTTP contract checks, and a short Simulator run. It asserts that all three Journeys ran and that the Expected Topology has 9 nodes and 8 edges. Set `KEEP_STACK=1` to leave the stack running.

Manual checks:

```bash
(cd services/app && go test ./...)
(cd services/simulator && go test ./...)
(cd services/app/frontend && npm ci && npm run build)
```

## Protocol and topology

- External: `Source IP → Frontend` HTTPS; `Payment → External Payment Provider` HTTPS.
- Internal: `Frontend → Gateway` HTTP; `Gateway → Auth/Catalog/Orders/Payment` HTTP; `Orders → Postgres` TCP.
- NGINX proxies `/api/*` and `/admin/reset` to the Gateway.
- The Simulator talks only to the Frontend. The Source IP is the laptop's address; Weave displays it as an IP, not as a named `ext users` node.
- The expected topology is in [expected-topology.yaml](expected-topology.yaml).

## Docs

- [Run spec](docs/run-spec.md)
- [Target App contract](docs/target-app-contract.md)
- [Glossary](GLOSSARY.md)
- [ADR-0001: real traffic only](docs/adr/0001-real-traffic-only.md)
- [ADR-0002: HTTP client, not browser](docs/adr/0002-http-client-not-browser.md)

## Not included

- Fault injection (container crash and saturation) — the next iteration.
- Weave's graph UI and the benchmark comparison tool. The Simulator emits the ground truth; a separate tool compares it to Weave's `/api/topology` and `/api/telemetry`.
