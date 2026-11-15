#!/usr/bin/env bash
# Detection acceptance: healthy window creates no incident, the catalog fault
# creates exactly one detected incident, and it closes after recovery.
set -euo pipefail

cd "$(dirname "$0")/.."
ROOT="$PWD"

WEAVE_URL="${WEAVE_URL:-http://localhost:8090}"
TARGET_URL="${TARGET_URL:-https://localhost:8443}"
HEALTHY_SECONDS="${HEALTHY_SECONDS:-600}"
FAULT_SECONDS="${FAULT_SECONDS:-120}"
RECOVERY_SECONDS="${RECOVERY_SECONDS:-120}"
CONCURRENCY="${CONCURRENCY:-50}"
SIM_BIN="${SIM_BIN:-$ROOT/.cache/weave-sim}"
TMP_DIR="$(mktemp -d)"
SIM_PID=""
CHAOS_APPLIED=0

cleanup() {
  if [[ "$CHAOS_APPLIED" == "1" ]]; then
    kubectl delete -f deploy/platform/chaos/fixtures/catalog-http-500.yaml --ignore-not-found >/dev/null 2>&1 || true
  fi
  if [[ -n "$SIM_PID" ]]; then
    kill "$SIM_PID" >/dev/null 2>&1 || true
    wait "$SIM_PID" >/dev/null 2>&1 || true
  fi
  rm -rf "$TMP_DIR"
}
trap cleanup EXIT

export GOCACHE="${GOCACHE:-$ROOT/.cache/go-build}"
export GOMODCACHE="${GOMODCACHE:-$ROOT/.cache/go-mod}"
export GOPATH="${GOPATH:-$ROOT/.cache/weave-go-mod}"
export GOTMPDIR="${GOTMPDIR:-$ROOT/.cache/gotmp}"
export GOSUMDB="${GOSUMDB:-off}"
mkdir -p "$GOCACHE" "$GOMODCACHE" "$GOTMPDIR"

for command in curl python3 kubectl; do
  command -v "$command" >/dev/null || { echo "missing $command" >&2; exit 1; }
done

json_get() {
  python3 -c "$1"
}

assert_health() {
  curl -sf -m 5 "$WEAVE_URL/api/health" | python3 -c '
import json, sys
payload = json.load(sys.stdin)
if not payload.get("detection_enabled"):
    raise SystemExit("detection is not enabled: " + json.dumps(payload))
if int(payload.get("rules_loaded") or 0) < 1:
    raise SystemExit("no SLA rules loaded: " + json.dumps(payload))
if payload.get("dependencies", {}).get("history") != "ok":
    raise SystemExit("history is not healthy: " + json.dumps(payload))
print("detection enabled with", payload.get("rules_loaded"), "rules, version", payload.get("rule_pack_version"))
'
}

# Snapshot the detection incident IDs that predate this run, then count only
# new ones. The acceptance script does not delete old incidents.
incident_snapshot() {
  curl -sf -m 5 "$WEAVE_URL/api/incidents?limit=200" > "$TMP_DIR/incidents.json"
  python3 - "$TMP_DIR/incidents.json" <<'PY'
import json, sys
items = json.load(open(sys.argv[1]))
for item in items:
    if item.get("origin") == "detection":
        print(item["id"])
PY
}

new_detected_count() {
  incident_snapshot > "$TMP_DIR/now-detected.txt"
  python3 - "$TMP_DIR/baseline-detected.txt" "$TMP_DIR/now-detected.txt" <<'PY'
import sys
baseline = set(open(sys.argv[1]).read().split())
now = open(sys.argv[2]).read().split()
print(len([item for item in now if item not in baseline]))
PY
}

new_detected_first() {
  incident_snapshot > "$TMP_DIR/now-detected.txt"
  python3 - "$TMP_DIR/baseline-detected.txt" "$TMP_DIR/now-detected.txt" <<'PY'
import sys
baseline = set(open(sys.argv[1]).read().split())
for item in open(sys.argv[2]).read().split():
    if item not in baseline:
        print(item)
        break
PY
}

assert_no_new_detected_incident() {
  local count
  count="$(new_detected_count)"
  if [[ "$count" != "0" ]]; then
    echo "unexpected new detection incidents: $(new_detected_first)" >&2
    exit 1
  fi
}

echo "==> Preconditions"
assert_health
curl -skf -m 5 "$TARGET_URL/health" >/dev/null
# The old, manually created incidents must still decode without detection fields.
curl -sf -m 5 "$WEAVE_URL/api/incidents?limit=5" | python3 -c '
import json, sys
items = json.load(sys.stdin)
for item in items:
    if item.get("origin") == "detection":
        continue
    if item.get("trigger") is not None:
        raise SystemExit("legacy incident unexpectedly has a trigger")
print("legacy incidents decode without trigger fields")
'
incident_snapshot > "$TMP_DIR/baseline-detected.txt"
open_detected="$(python3 - "$TMP_DIR/incidents.json" <<'PY'
import json, sys
items = json.load(open(sys.argv[1]))
print(sum(1 for item in items if item.get("origin") == "detection" and item.get("status") == "open"))
PY
)"
if [[ "$open_detected" != "0" ]]; then
  echo "an open detected incident already exists; close it before running acceptance" >&2
  exit 1
fi
assert_no_new_detected_incident

echo "==> Build simulator"
if [[ ! -x "$SIM_BIN" ]]; then
  (cd services/simulator && go build -o "$SIM_BIN" ./cmd/sim)
fi

DURATION="$((HEALTHY_SECONDS + FAULT_SECONDS + RECOVERY_SECONDS + 30))s"
cat > "$TMP_DIR/run.yaml" <<EOF
run:
  id: detection-acceptance
  seed: 42
  target: $TARGET_URL
  expected_topology: $ROOT/expected-topology.yaml
  duration: $DURATION
  concurrency: $CONCURRENCY
  ramp: 10s
  think_time: {min: 1s, max: 5s}
  mix: {bounce: 30, browse: 50, order: 20}
  accounts: {pool: 2000}
  reset: true
  tls: {insecure_skip_verify: true}
EOF

echo "==> Start traffic ($DURATION, concurrency $CONCURRENCY)"
"$SIM_BIN" run --config "$TMP_DIR/run.yaml" --report "$TMP_DIR/report.json" > "$TMP_DIR/sim.log" 2>&1 &
SIM_PID=$!
sleep 20

echo "==> Healthy window (${HEALTHY_SECONDS}s): expect zero detected incidents"
sleep "$HEALTHY_SECONDS"
assert_no_new_detected_incident

echo "==> Inject catalog HTTP 500 for ${FAULT_SECONDS}s"
FAULT_START="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
kubectl apply -f deploy/platform/chaos/fixtures/catalog-http-500.yaml >/dev/null
CHAOS_APPLIED=1

DETECTED=""
for _ in $(seq 1 24); do
  sleep 5
  DETECTED="$(new_detected_first)"
  [[ -n "$DETECTED" ]] && break
done
if [[ -z "$DETECTED" ]]; then
  echo "no detected incident appeared within 120s of the fault" >&2
  tail -30 "$TMP_DIR/sim.log" >&2 || true
  exit 1
fi
DETECTED_AT="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
count="$(new_detected_count)"
if [[ "$count" != "1" ]]; then
  echo "expected exactly one new detected incident, got $count: $(new_detected_first)" >&2
  exit 1
fi
echo "detected incident $DETECTED at $DETECTED_AT"

curl -sf -m 5 "$WEAVE_URL/api/incidents/$DETECTED" > "$TMP_DIR/incident.json"
python3 - "$TMP_DIR/incident.json" "$FAULT_START" <<'PY'
import json, sys, datetime
incident = json.load(open(sys.argv[1]))
fault_start = datetime.datetime.fromisoformat(sys.argv[2].replace("Z", "+00:00"))
window_start = datetime.datetime.fromisoformat(incident["window_start"].replace("Z", "+00:00"))
if incident.get("origin") != "detection":
    raise SystemExit("incident origin is not detection")
if incident.get("trigger", {}).get("rule_id") != "catalog-products-5xx":
    raise SystemExit("unexpected trigger: " + json.dumps(incident.get("trigger")))
if window_start > fault_start + datetime.timedelta(seconds=20):
    raise SystemExit(f"window start {window_start} is more than 20s after the fault start {fault_start}")
print("trigger and window cover the fault")
PY

echo "==> Recover for ${RECOVERY_SECONDS}s"
sleep "$FAULT_SECONDS"
kubectl delete -f deploy/platform/chaos/fixtures/catalog-http-500.yaml --ignore-not-found >/dev/null
CHAOS_APPLIED=0

CLOSED=0
for _ in $(seq 1 36); do
  sleep 5
  status="$(curl -sf -m 5 "$WEAVE_URL/api/incidents/$DETECTED" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("status",""))')"
  if [[ "$status" == "closed" ]]; then
    CLOSED=1
    break
  fi
done
if [[ "$CLOSED" != "1" ]]; then
  echo "incident did not close after recovery" >&2
  curl -sf -m 5 "$WEAVE_URL/api/incidents/$DETECTED" >&2 || true
  exit 1
fi
count="$(new_detected_count)"
if [[ "$count" != "1" ]]; then
  echo "duplicate new detection incidents after recovery: $count" >&2
  exit 1
fi

echo "==> Manual incident path regression"
MANUAL_START="$(python3 -c 'import datetime; print((datetime.datetime.now(datetime.timezone.utc)-datetime.timedelta(minutes=2)).strftime("%Y-%m-%dT%H:%M:%SZ"))')"
MANUAL_END="$(python3 -c 'import datetime; print((datetime.datetime.now(datetime.timezone.utc)-datetime.timedelta(minutes=1)).strftime("%Y-%m-%dT%H:%M:%SZ"))')"
MANUAL="$(curl -sf -m 5 -X POST "$WEAVE_URL/api/incidents" -H 'Content-Type: application/json' \
  -d "{\"start\":\"$MANUAL_START\",\"end\":\"$MANUAL_END\",\"symptom\":\"detection acceptance manual regression\"}")"
python3 - "$MANUAL" <<'PY'
import json, sys
incident = json.loads(sys.argv[1])
if incident.get("origin") or incident.get("trigger"):
    raise SystemExit("manual incident received detection fields")
if not incident.get("id"):
    raise SystemExit("manual incident has no id")
print("manual incident created:", incident["id"])
PY

echo "PASS: detection acceptance"
