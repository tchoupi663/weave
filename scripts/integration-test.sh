#!/usr/bin/env bash
set -euo pipefail

cd "$(dirname "$0")/.."
ROOT="$PWD"

export GOCACHE="${GOCACHE:-$ROOT/.cache/go-build}"
export GOMODCACHE="${GOMODCACHE:-$ROOT/.cache/go-mod}"
export GOTMPDIR="${GOTMPDIR:-$ROOT/.cache/gotmp}"
export GOSUMDB="${GOSUMDB:-off}"
export npm_config_cache="${npm_config_cache:-$ROOT/.cache/npm}"
mkdir -p "$GOCACHE" "$GOMODCACHE" "$GOTMPDIR" "$npm_config_cache" "$ROOT/reports"

cleanup() {
  if [[ "${KEEP_STACK:-0}" != "1" ]]; then
    docker compose down -v --remove-orphans >/dev/null 2>&1 || true
  fi
}
trap cleanup EXIT

echo "==> Go unit tests (Target App)"
(cd services/app && go test ./...)

echo "==> Go unit tests (Simulator)"
(cd services/simulator && go test ./...)

echo "==> Frontend build"
(cd services/app/frontend && npm ci --no-audit --no-fund && npm run build)

echo "==> Docker Compose up"
docker compose down -v --remove-orphans >/dev/null 2>&1 || true
docker compose up -d --build

echo "==> Waiting for the Target App"
ready=0
for _ in $(seq 1 120); do
  if curl -ksf https://localhost:8443/health >/dev/null 2>&1 \
     && curl -ksf https://localhost:8443/api/products >/dev/null 2>&1; then
    ready=1
    break
  fi
  sleep 1
done
if [[ "$ready" != "1" ]]; then
  echo "target app did not become ready" >&2
  docker compose ps >&2
  docker compose logs --tail=100 >&2
  exit 1
fi

echo "==> HTTP contract checks"
bad_status=$(curl -ks -o /tmp/weave-bad-login.json -w '%{http_code}' \
  -X POST https://localhost:8443/api/auth/login \
  -H 'Content-Type: application/json' \
  -d '{"username":"user0001","password":"wrong"}')
if [[ "$bad_status" != "401" ]]; then
  echo "invalid login returned $bad_status, want 401" >&2
  exit 1
fi
curl -ksf -X POST https://localhost:8443/admin/reset >/dev/null
curl -ksf -X POST https://localhost:8443/admin/reset >/dev/null
curl -ksf https://localhost:8443/api/products >/dev/null

echo "==> Simulator run"
(
  cd services/simulator
  go run ./cmd/sim run --config testdata/run-short.yaml --report "$ROOT/reports/integration.json"
)

echo "==> Report assertions"
python3 - "$ROOT/reports/integration.json" <<'PY'
import json
import sys

path = sys.argv[1]
with open(path, encoding="utf-8") as handle:
    report = json.load(handle)

assert report["run_id"] == "integration", report["run_id"]
for name in ("bounce", "browse", "order"):
    stats = report["journeys"][name]
    assert stats["started"] > 0, f"{name} never started"
    assert stats["failed"] == 0, f"{name} failed"
assert report["failures"] == [], report["failures"][:3]
assert len(report["expected_topology"]["nodes"]) == 9, report["expected_topology"]["nodes"]
assert len(report["expected_topology"]["edges"]) == 8, report["expected_topology"]["edges"]
for step in ("landing", "products.list", "auth.login", "order.create", "payment.create"):
    assert report["steps"][step]["count"] > 0, f"step {step} never exercised"
print("report ok:", {name: report["journeys"][name]["started"] for name in ("bounce", "browse", "order")})
PY

echo "==> Integration test passed"
