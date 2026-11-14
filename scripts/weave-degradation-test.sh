#!/usr/bin/env bash
# Verify the failure isolation promised by ADR-0006 against the deployed stack.
set -uo pipefail

cd "$(dirname "$0")/.."
ROOT="$PWD"
PORT="${WEAVE_PORT:-18093}"
PF_PID=0

export DOCKER_CONFIG="${DOCKER_CONFIG:-$ROOT/.cache/docker}"
if [[ -z "${DOCKER_HOST:-}" && -S "$HOME/.docker/run/docker.sock" ]]; then
  export DOCKER_HOST="unix://$HOME/.docker/run/docker.sock"
fi

cleanup() {
  kubectl -n weave-system scale deployment/weave-history --replicas=1 >/dev/null 2>&1 || true
  kubectl -n weave-system scale deployment/weave-inventory --replicas=1 >/dev/null 2>&1 || true
  if [[ "$PF_PID" != "0" ]]; then kill "$PF_PID" >/dev/null 2>&1 || true; fi
}
trap cleanup EXIT

api() { echo "http://127.0.0.1:${PORT}"; }
status() { curl -s -o /dev/null -w '%{http_code}' --max-time 5 "$1"; }
wait_status() {
  local url="$1" want="$2" seconds="${3:-30}"
  for _ in $(seq 1 "$seconds"); do
    [[ "$(status "$url")" == "$want" ]] && return 0
    sleep 1
  done
  return 1
}
wait_health() {
  local seconds="${1:-60}"
  for _ in $(seq 1 "$seconds"); do
    if curl -fsS --max-time 2 "$(api)/api/health" 2>/dev/null | python3 -c '
import json, sys
health = json.load(sys.stdin)
deps = health.get("dependencies", {})
raise SystemExit(0 if health.get("status") == "ok" and all(v == "ok" for v in deps.values()) else 1)
' 2>/dev/null; then
      return 0
    fi
    sleep 1
  done
  return 1
}

echo "==> Port-forward weave-api on ${PORT}"
kubectl -n weave-system port-forward svc/weave-api "${PORT}:8080" >"$ROOT/.cache/weave-degradation-portforward.log" 2>&1 &
PF_PID=$!
wait_status "$(api)/health" 200 20 || { echo "weave-api did not become reachable" >&2; exit 1; }
wait_health 60 || { echo "baseline health is not ok" >&2; exit 1; }

echo "==> Scale weave-history to zero"
kubectl -n weave-system scale deployment/weave-history --replicas=0 >/dev/null
wait_status "$(api)/api/incidents" 502 30 || { echo "incidents did not degrade to 502" >&2; exit 1; }
[[ "$(status "$(api)/api/topology")" == "200" ]] || { echo "topology must keep working without history" >&2; exit 1; }
curl -fsS "$(api)/api/health" | python3 -c '
import json, sys
health = json.load(sys.stdin)
assert health["status"] == "degraded", health
assert health["dependencies"]["history"] == "unavailable", health
assert health["dependencies"]["live"] == "ok", health
'
echo "history outage: topology ok, incidents 502, health degraded"

echo "==> Restore weave-history"
kubectl -n weave-system scale deployment/weave-history --replicas=1 >/dev/null
kubectl -n weave-system rollout status deployment/weave-history --timeout=120s >/dev/null
wait_status "$(api)/api/incidents" 200 60 || { echo "incidents did not recover" >&2; exit 1; }
wait_health 60 || { echo "health did not recover" >&2; exit 1; }

echo "==> Recording gap survives the history outage"
gaps=0
for _ in $(seq 1 15); do
  gaps=$(curl -fsS "$(api)/api/health" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("recording_gaps", 0))')
  [[ "$gaps" -ge 1 ]] && break
  sleep 1
done
if [[ "$gaps" -lt 1 ]]; then
  echo "expected at least one recording gap after a history outage" >&2
  exit 1
fi

echo "==> Scale weave-inventory to zero"
kubectl -n weave-system scale deployment/weave-inventory --replicas=0 >/dev/null
wait_status "$(api)/api/changes" 502 30 || { echo "changes did not degrade to 502" >&2; exit 1; }
[[ "$(status "$(api)/api/topology")" == "200" ]] || { echo "topology must keep working without inventory" >&2; exit 1; }
[[ "$(status "$(api)/api/incidents")" == "200" ]] || { echo "incidents must keep working without inventory" >&2; exit 1; }

echo "==> Restore weave-inventory"
kubectl -n weave-system scale deployment/weave-inventory --replicas=1 >/dev/null
kubectl -n weave-system rollout status deployment/weave-inventory --timeout=120s >/dev/null
wait_status "$(api)/api/changes" 200 30 || { echo "changes did not recover" >&2; exit 1; }

echo "==> Weave degradation test passed"
