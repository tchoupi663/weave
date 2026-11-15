#!/usr/bin/env bash
# Build the local agent and benchmark harness, then run a pilot or full matrix.
set -euo pipefail
cd "$(dirname "$0")/.."
ROOT="$PWD"
MODE="${1:---pilot}"
shift || true
AGENT_BIN="$ROOT/.cache/weave-agent"
BENCH_BIN="$ROOT/.cache/weave-bench"
KUBECONFIG_OUT="$ROOT/.cache/mcp-readonly.kubeconfig"

export GOCACHE="${GOCACHE:-$ROOT/.cache/go-build}"
export GOMODCACHE="${GOMODCACHE:-$ROOT/.cache/weave-go-mod}"
export GOPATH="${GOPATH:-$ROOT/.cache/weave-go-mod}"
export GOTMPDIR="${GOTMPDIR:-$ROOT/.cache/gotmp}"
export GOSUMDB="${GOSUMDB:-off}"
mkdir -p "$ROOT/.cache"

echo "==> Build local agent and benchmark harness"
(cd services/agent && go build -o "$AGENT_BIN" ./cmd/weave-agent && go build -o "$BENCH_BIN" ./cmd/weave-bench)

echo "==> Apply read-only RBAC and mint the MCP kubeconfig"
kubectl apply -f deploy/platform/mcp/readonly-rbac.yaml >/dev/null
TOKEN="$(kubectl -n weave create token weave-mcp-reader --duration=6h)"
python3 - "$TOKEN" "$KUBECONFIG_OUT" <<'PY'
import json, subprocess, sys, base64
token, out = sys.argv[1], sys.argv[2]
view = json.loads(subprocess.check_output(["kubectl", "config", "view", "--raw", "--minify", "-o", "json"]))
cluster = view["clusters"][0]["cluster"]
config = {
  "apiVersion": "v1", "kind": "Config", "current-context": "weave-mcp-reader",
  "clusters": [{"name": "kind", "cluster": {"server": cluster["server"], "certificate-authority-data": cluster.get("certificate-authority-data", "")}}],
  "users": [{"name": "weave-mcp-reader", "user": {"token": token}}],
  "contexts": [{"name": "weave-mcp-reader", "context": {"cluster": "kind", "user": "weave-mcp-reader"}}],
}
open(out, "w").write(json.dumps(config, indent=2))
PY
chmod 600 "$KUBECONFIG_OUT"

if [[ "$MODE" == "--full" || "$MODE" == "--with-crash-image" ]]; then
  echo "==> Build and load the crash-loop catalog image"
  export DOCKER_CONFIG="${DOCKER_CONFIG:-$ROOT/.cache/docker}"
  if [[ -S "$HOME/.docker/run/docker.sock" ]]; then export DOCKER_HOST="${DOCKER_HOST:-unix://$HOME/.docker/run/docker.sock}"; fi
  docker build --build-arg BUILD_TAGS=crashy -t weave-target-app:crashy -f services/app/Dockerfile services/app
  kind load docker-image weave-target-app:crashy --name kind
fi

if ! curl -skf -m 3 https://localhost:8443/health >/dev/null; then
  echo "Target App is not reachable on https://localhost:8443." >&2
  echo "Start the Target App port-forward and a Simulator run first:" >&2
  echo "  kubectl -n weave port-forward svc/frontend 8443:8000" >&2
  echo "  .cache/weave-sim run --config services/simulator/testdata/agent-benchmark.yaml" >&2
  exit 1
fi

ARGS=(--weave-url "${WEAVE_URL:-http://127.0.0.1:8090}" --agent-bin "$AGENT_BIN" --readonly-kubeconfig "$KUBECONFIG_OUT")
if [[ "$MODE" == "--full" ]]; then
  ARGS+=(--out "reports/agent-benchmark/$(date -u +%Y%m%dT%H%M%SZ)")
else
  ARGS+=(--pilot --out "reports/agent-benchmark/pilot-$(date -u +%Y%m%dT%H%M%SZ)")
fi
echo "==> Run benchmark ($MODE)"
"$BENCH_BIN" "${ARGS[@]}" "$@"
