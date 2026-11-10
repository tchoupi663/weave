# Weave

Weave is a topology testbed and the observed topology app that accompanies it.
A dummy e-commerce **Target App** generates real HTTP traffic through a
**Simulator**; Weave follows Cilium Hubble flows and constructs a live
workload-level map of what actually crossed the network.

## What is here

- `services/app/` — the Target App: Vite frontend behind NGINX, API Gateway, Auth, Catalog, Orders (Postgres), Payment, and the External Payment Provider.
- `services/simulator/` — the CLI that runs the Bounce, Browse, and Order Journeys and writes a JSON report with the Expected Topology.
- `services/weave/` — the actual app: a Go topology constructor that follows Hubble Relay and serves a Vite/React operator screen.
- `compose.yaml` — local Docker Compose stack for the Target App.
- `deploy/` — Kubernetes: Helm charts for the Target App and Weave, kind cluster + Cilium bootstrap, Argo CD applications, per-environment values. See [deploy/README.md](deploy/README.md).
- `expected-topology.yaml` — the expected nodes and edges used by the inference benchmark.
- `scripts/integration-test.sh` — unit tests, frontend build, Compose, contract checks, and a short Simulator run.

## Requirements

- Docker with Compose
- Go 1.26+ for Weave; Go 1.24+ for the Target App and Simulator
- Node 20+ (for local frontend builds; Docker also builds the frontend)
- `kind`, `kubectl`, and `helm` for the in-cluster path, with Cilium/Hubble
  installed

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

## Run Weave (kind + Hubble)

Weave runs in `weave-system` and watches the Target App in `weave`. It needs
the in-cluster Hubble Relay; it does not query the Kubernetes API. This path
assumes the Target App chart is installed in `weave` and Cilium has Hubble
enabled, as described in [deploy/README.md](deploy/README.md).

```bash
docker build -t weave-app:dev -f services/weave/Dockerfile services/weave
kind load docker-image weave-app:dev --name kind
helm upgrade --install weave deploy/charts/weave -n weave-system --create-namespace
kubectl -n weave-system rollout status deploy/weave --timeout=120s
kubectl -n weave-system port-forward svc/weave 8090:8080
```

Open <http://127.0.0.1:8090>. The screen is live; it will show an empty state
until Hubble reports a flow in `weave`.

To drive traffic, publish the Target App frontend and run the Simulator as
above:

```bash
kubectl -n weave port-forward svc/frontend 8443:8000
(cd services/simulator && go run ./cmd/sim run --config run.yaml)
```

Weave exposes:

| Endpoint | Purpose |
| --- | --- |
| `GET /api/topology` | Cumulative snapshot plus 60 activity buckets |
| `GET /api/stream` | SSE snapshots every second |
| `POST /api/reset` | Clear Weave's observed graph; not the Target App |
| `GET /health` | Liveness and Relay status |

The graph shows **connections** (TCP opens) and **events** (observed L4 flows),
not requests, bytes, or latency. See [the Weave contract](docs/weave-contract.md)
and [ADR-0003](docs/adr/0003-observed-topology.md).

![Weave topology screen](docs/images/weave-topology.png)

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

Weave's own tests:

```bash
(cd services/weave && go test ./...)
(cd services/weave/web && npm ci && npm run build)
./scripts/weave-integration-test.sh
```

The Weave integration script builds the image, deploys the chart, port-forwards
the app, runs the short Simulator config, and asserts that the constructed
graph is exactly 9 nodes and 8 edges. It does not tear down the Target App;
the Simulator resets its own test data as part of the run.

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
- The Simulator talks only to the Frontend. Source IP is the external boundary; through a local port-forward, Hubble reports the host endpoint. Weave displays the observed address, not a named `ext users` node.
- Weave groups pods into workloads, creates edges only from observed traffic, and folds replies into one ordered edge.
- Kubelet probes from the host to non-Frontend workloads are excluded. The Source IP observed through a port-forward is the host endpoint; the inspector says so.
- The expected topology is in [expected-topology.yaml](expected-topology.yaml).

## Docs

- [Run spec](docs/run-spec.md)
- [Target App contract](docs/target-app-contract.md)
- [Weave topology contract](docs/weave-contract.md)
- [Glossary](GLOSSARY.md)
- [Product](PRODUCT.md)
- [Design](DESIGN.md)
- [ADR-0001: real traffic only](docs/adr/0001-real-traffic-only.md)
- [ADR-0002: HTTP client, not browser](docs/adr/0002-http-client-not-browser.md)
- [ADR-0003: observed topology](docs/adr/0003-observed-topology.md)

## Not included

- Fault injection (container crash and saturation) — the next iteration.
- L7 request, latency, and status telemetry. v1 is L4-only.
- The benchmark comparison tool. The Simulator emits the ground truth; a
  separate tool compares it to Weave's `/api/topology`.
- A supported phone layout. Weave is designed for 1024px and wider.
