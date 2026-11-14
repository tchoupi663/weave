#!/usr/bin/env bash
# 20-minute Catalog incident acceptance replay.
# Prerequisites: Target App port-forward on :8443, Weave port-forward on :8090,
# Argo-managed Target App, Chaos Mesh, and the current Weave image deployed.
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
SIM_DIR="$ROOT/services/simulator"
API="${WEAVE_API:-http://127.0.0.1:8090}"
TARGET="${TARGET_URL:-https://localhost:8443}"
BASELINE_SECONDS="${BASELINE_SECONDS:-900}"
FAULT_SECONDS="${FAULT_SECONDS:-120}"
REPORT="${REPORT:-$ROOT/.cache/stage6-report.json}"
INCIDENT_OUT="${INCIDENT_OUT:-$ROOT/.cache/stage6-incident.json}"
CHAOS="$ROOT/deploy/platform/chaos/fixtures/catalog-http-500.yaml"
LOG="$ROOT/.cache/stage6-sim.log"

mkdir -p "$ROOT/.cache"
rm -f "$LOG" "$REPORT" "$INCIDENT_OUT" "$ROOT/.cache/stage6-sim.exit"

cleanup() {
  kubectl -n weave delete httpchaos catalog-http-500 --ignore-not-found >/dev/null 2>&1 || true
}
trap cleanup EXIT INT TERM

curl -fsS -k "$TARGET/health" >/dev/null
curl -fsS "$API/api/health" >/dev/null
kubectl -n weave delete httpchaos catalog-http-500 --ignore-not-found >/dev/null 2>&1 || true

(
  cd "$SIM_DIR"
  GOCACHE="${GOCACHE:-$ROOT/.cache/go-build}" \
  GOMODCACHE="${GOMODCACHE:-$ROOT/.cache/go-mod}" \
  GOTMPDIR="${GOTMPDIR:-$ROOT/.cache/gotmp}" \
  GOSUMDB=off \
  go run ./cmd/sim run --config testdata/incident-replay.yaml --report "$REPORT" >"$LOG" 2>&1
  echo $? >"$ROOT/.cache/stage6-sim.exit"
) &
sim_pid=$!

printf 'Healthy baseline: %ss\n' "$BASELINE_SECONDS"
sleep "$BASELINE_SECONDS"
fault_start="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
kubectl apply -f "$CHAOS"
printf 'Fault window started: %s\n' "$fault_start"
sleep "$FAULT_SECONDS"
fault_end="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
cleanup
printf 'Fault window ended: %s\n' "$fault_end"

wait "$sim_pid" || true
sim_exit="$(cat "$ROOT/.cache/stage6-sim.exit" 2>/dev/null || echo unknown)"
tail -12 "$LOG" || true
printf 'Simulator exit: %s (non-zero is expected during the injected fault)\n' "$sim_exit"
sleep 10

cat >"$INCIDENT_OUT" <<JSON
{"start":"$fault_start","end":"$fault_end","symptom":"Catalog returns 500","scope":"gateway->catalog"}
JSON
curl -fsS -X POST "$API/api/incidents" \
  -H 'Content-Type: application/json' \
  -d @"$INCIDENT_OUT" \
  >"$ROOT/.cache/stage6-incident-response.json"

python3 - "$ROOT/.cache/stage6-incident-response.json" <<'PY'
import json, sys
incident = json.load(open(sys.argv[1]))
evidence = incident.get("evidence", {})
suspects = evidence.get("suspect_edges", [])
print("incident", incident.get("id"))
print("suspect_edges", suspects)
print("suspect_nodes", evidence.get("suspect_nodes"))
catalog = any("gateway->workload:weave/catalog" in edge for edge in suspects)
cascade = any("frontend->workload:weave/gateway" in edge for edge in suspects)
outside = any("auth" in edge or "payment" in edge for edge in suspects)
routes = evidence.get("routes", [])
product_errors = [route for route in routes if route.get("path") == "/api/products" and route.get("status_5xx", 0) > 0]
if not (catalog and cascade) or outside or not product_errors:
    raise SystemExit("acceptance failed: expected Catalog + Gateway->Catalog + Frontend cascade, Auth/Payment outside, and 5xx route evidence")
print("acceptance region ok")
PY
