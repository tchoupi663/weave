#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
ROOT="$PWD"

export GOCACHE="${GOCACHE:-$ROOT/.cache/go-build}"
export GOMODCACHE="${GOMODCACHE:-$ROOT/.cache/go-mod}"
export GOTMPDIR="${GOTMPDIR:-$ROOT/.cache/gotmp}"
export GOSUMDB="${GOSUMDB:-off}"
export DOCKER_CONFIG="${DOCKER_CONFIG:-$ROOT/.cache/docker}"
if [[ -z "${DOCKER_HOST:-}" && -S "$HOME/.docker/run/docker.sock" ]]; then
  export DOCKER_HOST="unix://$HOME/.docker/run/docker.sock"
fi
export HELM_CACHE_HOME="${HELM_CACHE_HOME:-$ROOT/.cache/helm/cache}"
export HELM_CONFIG_HOME="${HELM_CONFIG_HOME:-$ROOT/.cache/helm/config}"
export HELM_DATA_HOME="${HELM_DATA_HOME:-$ROOT/.cache/helm/data}"
mkdir -p "$GOCACHE" "$GOMODCACHE" "$GOTMPDIR" "$DOCKER_CONFIG" "$HELM_CACHE_HOME" "$HELM_CONFIG_HOME" "$HELM_DATA_HOME" "$ROOT/.cache"

WEAVE_PORT="${WEAVE_PORT:-18090}"
TARGET_PORT="${TARGET_PORT:-8443}"
PF_TARGET_PID=0
PF_WEAVE_PID=0

cleanup() {
  if [[ "$PF_WEAVE_PID" != "0" ]]; then kill "$PF_WEAVE_PID" >/dev/null 2>&1 || true; fi
  if [[ "$PF_TARGET_PID" != "0" ]]; then kill "$PF_TARGET_PID" >/dev/null 2>&1 || true; fi
}
trap cleanup EXIT

for command in docker kind kubectl helm go npm curl python3; do
  command -v "$command" >/dev/null || { echo "missing $command" >&2; exit 1; }
done

echo "==> Check Target App in weave"
kubectl -n weave get service/frontend >/dev/null
kubectl -n weave get pods >/dev/null

if ! kubectl -n beyla-system get daemonset/beyla >/dev/null 2>&1; then
  echo "Beyla is not installed. See deploy/platform/observability/README.md." >&2
  exit 1
fi

if kubectl -n weave get ciliumnetworkpolicies.cilium.io 2>/dev/null | grep -q .; then
  echo "A CiliumNetworkPolicy exists in weave; L7 must not use the Cilium proxy." >&2
  kubectl -n weave get ciliumnetworkpolicies.cilium.io >&2
  exit 1
fi

if ! curl -ksf "https://localhost:${TARGET_PORT}/health" >/dev/null 2>&1; then
  echo "==> Port-forward Target App on ${TARGET_PORT}"
  kubectl -n weave port-forward svc/frontend "${TARGET_PORT}:8000" \
    >"$ROOT/.cache/weave-target-portforward.log" 2>&1 &
  PF_TARGET_PID=$!
  ready=0
  for _ in $(seq 1 40); do
    if curl -ksf "https://localhost:${TARGET_PORT}/health" >/dev/null 2>&1; then
      ready=1
      break
    fi
    sleep 0.5
  done
  if [[ "$ready" != "1" ]]; then
    echo "Target App port-forward did not become ready" >&2
    cat "$ROOT/.cache/weave-target-portforward.log" >&2
    exit 1
  fi
fi

echo "==> Go tests"
(cd services/weave && go test ./...)

echo "==> Build image"
docker build -t weave-app:dev -f services/weave/Dockerfile services/weave
kind load docker-image weave-app:dev --name kind

echo "==> Deploy Weave"
helm upgrade --install weave deploy/charts/weave -n weave-system --create-namespace --set image.tag=dev
kubectl -n weave-system rollout restart deploy/weave
kubectl -n weave-system rollout status deploy/weave --timeout=120s

echo "==> Port-forward Weave on ${WEAVE_PORT}"
kubectl -n weave-system port-forward svc/weave "${WEAVE_PORT}:8080" \
  >"$ROOT/.cache/weave-portforward.log" 2>&1 &
PF_WEAVE_PID=$!
ready=0
for _ in $(seq 1 40); do
  if curl -fsS "http://127.0.0.1:${WEAVE_PORT}/api/health" >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 0.5
done
if [[ "$ready" != "1" ]]; then
  echo "Weave port-forward did not become ready" >&2
  cat "$ROOT/.cache/weave-portforward.log" >&2
  exit 1
fi

connected=0
for _ in $(seq 1 60); do
  if curl -fsS "http://127.0.0.1:${WEAVE_PORT}/api/health" | grep -q '"hubble_connected":true'; then
    connected=1
    break
  fi
  sleep 0.5
done
if [[ "$connected" != "1" ]]; then
  echo "Weave could not connect to Hubble Relay" >&2
  kubectl -n weave-system logs deploy/weave --tail=100 >&2
  exit 1
fi

echo "==> Reset observed graph"
curl -fsS -X POST "http://127.0.0.1:${WEAVE_PORT}/api/reset" >/dev/null
sleep 1

echo "==> Simulator run"
(
  cd services/simulator
  go run ./cmd/sim run --config testdata/run-short.yaml --report "$ROOT/.cache/weave-integration.json"
)

echo "==> Assert constructed topology"
sleep 2
curl -fsS "http://127.0.0.1:${WEAVE_PORT}/api/topology" -o "$ROOT/.cache/weave-topology.json"
python3 - "$ROOT/.cache/weave-topology.json" <<'PY'
import json
import sys

with open(sys.argv[1], encoding="utf-8") as handle:
    snapshot = json.load(handle)

assert snapshot["namespace"] == "weave", snapshot["namespace"]
assert snapshot["hubble"]["connected"] is True, snapshot["hubble"]
assert snapshot["totals"]["nodes"] == 9, snapshot["totals"]
assert snapshot["totals"]["edges"] == 8, snapshot["totals"]
assert snapshot["totals"]["l7_edges"] == 5, snapshot["totals"]
assert snapshot["totals"]["l7_requests"] > 0, snapshot["totals"]
assert snapshot["totals"]["l7_responses"] > 0, snapshot["totals"]

by_id = {node["id"]: node for node in snapshot["nodes"]}
expected = {
    ("Source IP", "Frontend"): "HTTPS",
    ("Frontend", "API Gateway"): "HTTP",
    ("API Gateway", "Auth"): "HTTP",
    ("API Gateway", "Catalog"): "HTTP",
    ("API Gateway", "Orders"): "HTTP",
    ("API Gateway", "Payment"): "HTTP",
    ("Orders", "Postgres"): "TCP",
    ("Payment", "External Payment Provider"): "HTTPS",
}
actual = {}
for edge in snapshot["edges"]:
    source = by_id[edge["source"]]["name"]
    target = by_id[edge["target"]]["name"]
    actual[(source, target)] = edge["protocol"]

assert actual == expected, f"edges do not match expected topology:\nactual={actual}\nexpected={expected}"
assert len(snapshot["activity"]) == 60, len(snapshot["activity"])

l7_expected = {
    ("Frontend", "API Gateway"): "server",
    ("API Gateway", "Auth"): "client",
    ("API Gateway", "Catalog"): "client",
    ("API Gateway", "Orders"): "client",
    ("API Gateway", "Payment"): "client",
}
l7_seen = set()
for edge in snapshot["edges"]:
    source = by_id[edge["source"]]["name"]
    target = by_id[edge["target"]]["name"]
    pair = (source, target)
    if pair in l7_expected:
        assert "l7" in edge and edge["l7"]["requests"] > 0, f"missing L7 for {pair}: {edge}"
        l7_seen.add(pair)
        l7 = edge["l7"]
        assert l7["latency_source"] == l7_expected[pair], (pair, l7["latency_source"])
        assert len(l7["window"]["activity"]) == 60, l7["window"]["activity"]
        assert len(l7["routes"]) <= 5, l7["routes"]
        assert "headers" not in l7 and "query" not in l7 and "body" not in l7, l7
    else:
        assert "l7" not in edge, f"unexpected L7 on L4-only edge {pair}: {edge}"

assert l7_seen == set(l7_expected), (l7_seen, set(l7_expected))
print("weave topology ok:", snapshot["totals"])
PY

echo "==> Weave integration test passed"
