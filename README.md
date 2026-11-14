# Weave

Weave is an incident-replay topology app for a Kubernetes application. It
constructs an observed workload graph from Cilium Hubble L4 flows and Grafana
Beyla HTTP spans, retains a time-bucketed history, reads the Argo CD desired
inventory read-only, and turns a manually selected incident window into a
bounded suspect region.

Weave is not an alerting system and does not claim a root cause. It is an
operator instrument: the graph and the evidence are the product.

## What is here

- `services/app/` — the Target App: Vite frontend behind NGINX, API Gateway, Auth, Catalog, Orders (Postgres), Payment, and the External Payment Provider.
- `services/simulator/` — the CLI that runs Bounce, Browse, and Order Journeys and writes a JSON report with the Expected Topology.
- `services/weave/` — the app: a Go topology/history service and incident API with a Vite/React operator screen.
- `compose.yaml` — local Docker Compose stack for the Target App.
- `deploy/` — Kubernetes: Helm charts, kind + Cilium bootstrap, Argo CD, Chaos Mesh, and Beyla. See [deploy/README.md](deploy/README.md).
- `expected-topology.yaml` — the healthy fixture's expected nodes and edges.
- `scripts/integration-test.sh` — unit tests, frontend build, Compose, contract checks, and a short Simulator run.
- `scripts/incident-replay.sh` — the 20-minute acceptance harness: 15-minute baseline, 2-minute Catalog fault, 3-minute recovery, then incident creation.
- `docs/adr/0005-incident-replay.md` — the incident-replay architecture.
- `docs/acceptance.md` — the recorded acceptance run and its evidence.

## Requirements

- Docker with Compose.
- Go 1.26+ for Weave; Go 1.24+ for the Target App and Simulator.
- Node 20+ for local frontend builds. The Weave image builds the frontend itself.
- `kind`, `kubectl`, and `helm` for the in-cluster path, with Cilium/Hubble,
  Argo CD, Chaos Mesh, and Beyla installed.

Weave builds with CGO because DuckDB is embedded. The image is a Debian build
and runtime, not a static Alpine binary.

## Run the Target App

```bash
docker compose up -d --build
docker compose ps
curl -k https://localhost:8443/health
```

Open <https://localhost:8443> and accept the self-signed certificate. Demo login:
`user0001` / `password`.

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

## Run Weave

Weave runs in `weave-system` and watches the Target App in `weave`. It reads
Hubble Relay and Beyla, not the Kubernetes API. It persists history in one
`weave-data` PVC and reads the Argo CD application `target-app-local` through a
read-only API token.

```bash
docker build -t weave-app:dev -f services/weave/Dockerfile services/weave
kind load docker-image weave-app:dev --name kind
helm upgrade --install weave deploy/charts/weave -n weave-system --create-namespace
kubectl -n weave-system rollout status deploy/weave --timeout=120s
kubectl -n weave-system port-forward svc/weave 8090:8080
```

The chart uses a 2 GiB PVC mounted at `/data`. The Deployment uses the
`Recreate` strategy because SQLite and DuckDB are single-writer stores on a
ReadWriteOnce volume. `persistence.accessMode` is configurable; use
`ReadWriteOncePod` where the storage class supports it.

Create the read-only Argo token before Weave starts. Without it Weave records
metrics but cannot reconcile declared/live lifecycle states. The exact
commands are in [deploy/platform/argocd/README.md](deploy/platform/argocd/README.md).

Beyla is a separate platform addon:

```bash
helm repo add grafana https://grafana.github.io/helm-charts
helm repo update grafana
helm upgrade --install beyla grafana/beyla \
  --namespace beyla-system --create-namespace \
  --version 1.16.17 \
  -f deploy/platform/observability/beyla-values.yaml
```

Beyla instruments only the `gateway` process. Gateway client spans cover
Gateway → Auth/Catalog/Orders/Payment; gateway server spans cover
Frontend → Gateway. It is a privileged eBPF DaemonSet and inserts no proxy.

Open <http://127.0.0.1:8090>. The live topology starts empty; drive traffic with
the Simulator after port-forwarding the Target App frontend:

```bash
kubectl -n weave port-forward svc/frontend 8443:8000
(cd services/simulator && go run ./cmd/sim run --config run.yaml)
```

### Weave API

| Endpoint | Purpose |
| --- | --- |
| `GET /api/topology` | Live cumulative snapshot plus 60 activity buckets |
| `GET /api/stream` | SSE topology snapshots every second |
| `POST /api/reset` | Clear Weave's live graph; not the Target App |
| `POST /v1/traces` | OTLP HTTP/protobuf receiver for Beyla |
| `POST /api/incidents` | Create and materialize an incident window |
| `GET /api/incidents` | List incidents |
| `GET /api/incidents/{id}` | Incident with its evidence snapshot |
| `PATCH /api/incidents/{id}` | Update notes or open/closed state |
| `POST /api/incidents/{id}/recompute` | Recompute evidence for the stored window |
| `GET /api/incidents/{id}/timeline` | Five-second suspect-edge frames for scrubbing |
| `GET /api/changes?from=&to=` | Lifecycle change events |
| `GET /health`, `/api/health` | Process and Relay status |

Seeded history: 5-second edge buckets for 24 hours, 1-minute rollups for 7
days, and change events for 30 days. Raw flows and spans are never retained.
See [the Weave contract](docs/weave-contract.md).

### Incident view

The `Topology` / `Incidents` switch opens the incident workspace. An incident
has a 15-minute baseline, a scrub timeline, a suspect-region canvas, per-edge
baseline/incident evidence, lifecycle changes, notes, close/reopen, and a live
topology link. The incident graph labels show error rates and quantities, not
protocol labels.

The flow heatmap at the top right of the topology view expands to the window
total, peak bucket, average, and active buckets. It is a live five-minute
instrument, not history.

![Weave topology with HTTP error rates and flow heatmap](docs/images/weave-topology.png)

![Weave incident workspace with scrub timeline](docs/images/weave-incident-workspace.png)

![Expanded flow heatmap](docs/images/weave-flow.png)

## Incident replay acceptance

The 20-minute replay is:

1. 15 minutes of healthy traffic.
2. Chaos Mesh `HTTPChaos` replacing Catalog `GET /api/products` with HTTP 500 for two minutes.
3. Three minutes of recovery.
4. An Incident created over the fault window.

```bash
./scripts/incident-replay.sh
```

The script expects the Target App port-forward on `:8443` and Weave on `:8090`.
It applies `deploy/platform/chaos/fixtures/catalog-http-500.yaml` at the
15-minute mark, removes it after two minutes, waits for the three-minute
recovery, and creates the incident through `POST /api/incidents`.

The expected result is a suspect region containing Catalog and
Gateway → Catalog, with Frontend → Gateway as the cascade. Auth and Payment must
be outside it. **The 2026-11-14 acceptance passed**: the suspect edges were
Frontend → Gateway and Gateway → Catalog, with both onsets in the first fault
bucket; Auth and Payment were outside. The recorded evidence and restart hashes
are in [docs/acceptance.md](docs/acceptance.md).

## Run the Simulator

The Simulator runs on the host, outside the Compose stack, so its traffic enters
from the host IP.

```bash
cd services/simulator
go run ./cmd/sim validate --config run.yaml
go run ./cmd/sim run --config run.yaml --report ../../reports/run.json
```

The default is the 10-minute, 50-user healthy Run. `testdata/run-short.yaml` is
the integration smoke run. `testdata/incident-replay.yaml` is the 20-minute
acceptance shape. The Expected Topology path is relative to the working
directory. `--log-requests` adds per-request details to the report.

## Test

```bash
./scripts/integration-test.sh
```

Weave's own checks:

```bash
(cd services/weave && go test ./...)
(cd services/weave/web && npm ci && npm run build)
./scripts/weave-integration-test.sh
```

Manual:

```bash
(cd services/app && go test ./...)
(cd services/simulator && go test ./...)
(cd services/app/frontend && npm ci && npm run build)
```

## Protocol and topology

- External: `Source IP → Frontend` HTTPS; `Payment → External Payment Provider` HTTPS.
- Internal: `Frontend → Gateway` HTTP; `Gateway → Auth/Catalog/Orders/Payment` HTTP; `Orders → Postgres` TCP.
- Weave groups pods into workloads, creates edges only from observed traffic, and folds replies into one ordered request edge.
- HTTP coverage is five of eight edges. `Source IP → Frontend` HTTPS,
  `Payment → External Payment Provider` HTTPS, and `Orders → Postgres` TCP are
  connection-level only.
- Four HTTP edges are client-observed; Frontend → Gateway is server-observed.
  The inspector labels the latency source per edge. Headers, query strings, and
  bodies are discarded.
- Kubelet probes are excluded by the observed-topology filter. The Source IP
  seen through a port-forward is the host endpoint.
- Declared Architecture is Argo CD object inventory only. It never creates an
  observed edge.
- The expected healthy topology is [expected-topology.yaml](expected-topology.yaml).

## Docs

- [Run spec](docs/run-spec.md)
- [Target App contract](docs/target-app-contract.md)
- [Weave contract](docs/weave-contract.md)
- [Acceptance](docs/acceptance.md)
- [Glossary](GLOSSARY.md)
- [Product](PRODUCT.md)
- [Design](DESIGN.md)
- [ADR-0001: real traffic only](docs/adr/0001-real-traffic-only.md)
- [ADR-0002: HTTP client, not browser](docs/adr/0002-http-client-not-browser.md)
- [ADR-0003: observed topology](docs/adr/0003-observed-topology.md)
- [ADR-0004: L7 observability](docs/adr/0004-l7-observability.md)
- [ADR-0005: incident replay](docs/adr/0005-incident-replay.md)

## Not included

- Authentication. The API is internal and port-forward only.
- An Agent UI. The read API is shared with a future Agent.
- Architecture/telemetry evolution beyond stored metrics and change events.
- A statistical single-root-cause claim. The output is a suspect region.
- Historical full-topology reconstruction at `at=...`; the incident timeline is
  built from suspect-edge metric frames, and `Live topology` opens the current
  graph.
- A supported phone layout. The visual layout holds at 720px, but the operator
  layout is designed for 1024px and wider.
- Generic all-connection host/probe acceptance. The current live filter keeps
  the Target App's ingress-scoped host behavior and fixture display names; the
  incident acceptance region is unaffected.
