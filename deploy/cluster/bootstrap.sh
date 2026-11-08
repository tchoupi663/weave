#!/usr/bin/env bash
# Day 0: create the cluster, install the CNI, install Argo CD.
#
# Run this by hand once per cluster (or from CI for ephemeral clusters).
# Argo CD cannot bootstrap this layer: its own Pods need a CNI to get
# network, and Cilium must already be running before anything else starts.
#
#   deploy/cluster/bootstrap.sh
#
# Idempotent: re-running upgrades Cilium/Argo CD in place.
set -euo pipefail

cd "$(dirname "$0")/../.."

CLUSTER=kind
CILIUM_CHART=oci://quay.io/cilium/charts/cilium
CILIUM_VERSION="${CILIUM_VERSION:-1.20.4}"
ARGO_CHART=oci://ghcr.io/argoproj/argo-helm/argo-cd
ARGO_VERSION="${ARGO_VERSION:-}"

if ! kind get clusters | grep -qx "$CLUSTER"; then
  echo "==> creating kind cluster ${CLUSTER}"
  kind create cluster --config deploy/cluster/kind.yaml
fi
kubectl config use-context "kind-${CLUSTER}" >/dev/null

echo "==> installing cilium ${CILIUM_VERSION} (this is the CNI)"
helm upgrade --install cilium "$CILIUM_CHART" \
  --version "$CILIUM_VERSION" \
  --namespace kube-system \
  --values deploy/cluster/cilium-values.yaml \
  --wait

if command -v cilium >/dev/null 2>&1; then
  echo "==> waiting for the cilium datapath"
  cilium status --wait
fi

echo "==> installing argo cd"
kubectl create namespace argocd --dry-run=client -o yaml | kubectl apply -f -
argo_args=()
if [[ -n "$ARGO_VERSION" ]]; then
  argo_args+=(--version "$ARGO_VERSION")
else
  echo "    note: ARGO_VERSION unset, installing the latest argo-helm chart"
fi
helm upgrade --install argocd "$ARGO_CHART" \
  "${argo_args[@]}" --namespace argocd --wait

cat <<'EON'

Cluster is up. Remaining one-time steps:

  1. Repository credentials (the repo is private):
       kubectl -n argocd create secret generic weave-repo \
         --from-literal=type=git \
         --from-literal=url=https://github.com/tchoupi663/weave.git \
         --from-literal=username=<github-user> \
         --from-literal=password=<fine-grained-pat> \
         --dry-run=client -o yaml | kubectl apply -f -
       kubectl -n argocd label secret weave-repo argocd.argoproj.io/secret-type=repository --overwrite

  2. Deploy the app through Argo CD:
       kubectl apply -f deploy/gitops/apps/target-app-local.yaml

  3. Load the locally built images into the node (Argo CD cannot do this):
       docker compose build
       kind load docker-image --name kind weave-target-app:dev weave-target-frontend:dev

  4. Argo CD UI:
       kubectl -n argocd port-forward svc/argocd-server 8080:443
       kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d
EON
