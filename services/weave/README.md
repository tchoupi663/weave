# Weave

The actual app: a Go process that follows Hubble Relay, constructs a
workload-level topology from observed L4 flows, and serves the Weave operator
screen.

The design and product truth live at the repository root:
[DESIGN.md](../../DESIGN.md) and [PRODUCT.md](../../PRODUCT.md). The API and
topology semantics are in [docs/weave-contract.md](../../docs/weave-contract.md).

## What it does

- `internal/hubble` follows `Observer.GetFlows` with `follow=true`.
- `internal/topology` filters, deduplicates, groups pods into workloads, and
  keeps cumulative counters plus five-minute activity buckets.
- `internal/api` serves `/api/topology`, `/api/stream`, `/api/reset`, and
  `/health`, and serves the built SPA.
- `web` is the Vite/React screen.

The process does not call the Kubernetes API and does not mutate the Target
App. It uses one environment-configured namespace and the in-cluster Relay.

## Local development

Run the backend:

```bash
kubectl -n kube-system port-forward svc/hubble-relay 4245:80
cd services/weave
HUBBLE_RELAY_ADDR=127.0.0.1:4245 WATCH_NAMESPACE=weave LISTEN_ADDR=:8090 \
  go run ./cmd/weave
```

Run the UI with a proxy to that backend:

```bash
cd services/weave/web
npm ci
VITE_API_PROXY=http://127.0.0.1:8090 npm run dev
```

The production image builds the UI and serves it from `/app/web/dist`.

## Tests

```bash
cd services/weave
go test ./...

cd web
npm ci
npm run build
```

The end-to-end script at the repository root builds the image, deploys the
chart, runs the Simulator, and asserts the nine-node/eight-edge graph:

```bash
./scripts/weave-integration-test.sh
```
