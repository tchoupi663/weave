#!/usr/bin/env bash
# Build, load, and install the four-service Weave stack, then expose the UI.
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

for command in docker kind kubectl helm; do
  command -v "$command" >/dev/null || { echo "missing $command" >&2; exit 1; }
done

echo "==> Build service images"
docker build --target live -t weave-live:dev -f services/weave/Dockerfile services/weave
docker build --target history -t weave-history:dev -f services/weave/Dockerfile services/weave
docker build --target inventory -t weave-inventory:dev -f services/weave/Dockerfile services/weave
docker build --target api -t weave-api:dev -f services/weave/Dockerfile services/weave

echo "==> Load images into kind"
kind load docker-image weave-live:dev --name kind
kind load docker-image weave-history:dev --name kind
kind load docker-image weave-inventory:dev --name kind
kind load docker-image weave-api:dev --name kind

if kubectl -n weave-system get pvc weave-data >/dev/null 2>&1; then
  echo "==> Preserve the history volume"
  kubectl -n weave-system annotate pvc weave-data helm.sh/resource-policy=keep --overwrite >/dev/null
fi

if kubectl -n weave-system get deployment weave >/dev/null 2>&1; then
  echo "==> Stop the legacy single-process deployment"
  kubectl -n weave-system scale deployment/weave --replicas=0 >/dev/null
  kubectl -n weave-system delete deployment/weave service/weave --ignore-not-found >/dev/null
fi

echo "==> Install the umbrella chart"
helm upgrade --install weave deploy/charts/weave -n weave-system --create-namespace \
  --set live.image.tag=dev \
  --set history.image.tag=dev \
  --set inventory.image.tag=dev \
  --set api.image.tag=dev

for deployment in weave-live weave-history weave-inventory weave-api; do
  echo "==> Restart and wait for $deployment"
  kubectl -n weave-system rollout restart "deployment/$deployment"
  kubectl -n weave-system rollout status "deployment/$deployment" --timeout=180s
done

echo "==> Open the UI at http://127.0.0.1:8090"
exec kubectl -n weave-system port-forward svc/weave-api 8090:8080
