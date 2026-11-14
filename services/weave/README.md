# Weave

The observed-topology and incident-replay app, split into four services:
`weave-live`, `weave-history`, `weave-inventory`, and `weave-api`. The design,
product truth, and contracts live at the repository root:
[DESIGN.md](../../DESIGN.md), [PRODUCT.md](../../PRODUCT.md),
[docs/weave-contract.md](../../docs/weave-contract.md), and
[docs/weave-internal-api.md](../../docs/weave-internal-api.md). The split and
its trade-offs are recorded in
[ADR-0006](../../docs/adr/0006-weave-service-decomposition.md).

## Services

- `cmd/live` + `internal/live` — follows Hubble Relay for L4, accepts Grafana
  Beyla OTLP spans for L7, builds the in-memory workload graph, serves the
  snapshot/SSE/reset paths, and batches accepted observations to history.
- `cmd/history` + `internal/history` — owns DuckDB edge metrics and SQLite
  incidents, evidence, timeline frames, and recording gaps.
- `cmd/inventory` + `internal/inventory` — reconciles the Argo CD declared
  workload inventory, owns lifecycle state and change events, and polls live for
  observed workloads.
- `cmd/api` + `internal/edge` — the built SPA and the single browser origin. It
  proxies the frozen `/api/*` contract and aggregates dependency health.
- `internal/contracts` — the only package shared across service boundaries.
- `internal/topology` and `internal/hubble` are shared by live only; `web` is
  the Vite/React screen.

No service calls the Kubernetes API, and no service mutates the Target App.

## Local development

Run the Target App port-forwards first (frontend `:8443` for traffic, Hubble
Relay `:4245` for live):

```bash
kubectl -n weave port-forward svc/frontend 8443:8000
kubectl -n kube-system port-forward svc/hubble-relay 4245:80
```

Then the services, each in its own terminal. `weave-history` needs a writable
directory; the others need the URLs of their upstreams:

```bash
# history
cd services/weave
DATA_DIR=$(mktemp -d) LISTEN_ADDR=:8081 go run ./cmd/history

# live
HUBBLE_RELAY_ADDR=127.0.0.1:4245 WATCH_NAMESPACE=weave LISTEN_ADDR=:8082 \
  HISTORY_URL=http://127.0.0.1:8081 go run ./cmd/live

# inventory (Argo optional; without ARGOCD_URL lifecycle is disabled)
DATA_DIR=$(mktemp -d) LISTEN_ADDR=:8083 LIVE_URL=http://127.0.0.1:8082 \
  ARGOCD_URL=http://argocd-server.argocd.svc.cluster.local ARGOCD_TOKEN=... \
  ARGOCD_APP=target-app-local go run ./cmd/inventory

# api
LISTEN_ADDR=:8090 WEB_DIR=web/dist \
  LIVE_URL=http://127.0.0.1:8082 HISTORY_URL=http://127.0.0.1:8081 \
  INVENTORY_URL=http://127.0.0.1:8083 go run ./cmd/api
```

Run the UI with a proxy to the API:

```bash
cd services/weave/web
npm ci
VITE_API_PROXY=http://127.0.0.1:8090 npm run dev
```

The production images are built from `services/weave/Dockerfile` with
`--target live|history|inventory|api`.

## Tests

```bash
cd services/weave
go test ./...

cd web
npm ci
npm run build
```

The root scripts cover the deployed system:
`scripts/weave-up.sh` (one-command bring-up),
`scripts/weave-integration-test.sh` (images, chart, Simulator, graph
assertions, import boundaries), and
`scripts/weave-degradation-test.sh` (history/inventory outage behaviour).
