# Argo CD

Argo CD is the declared-state source for the Target App. It is a platform
release, not part of the Weave chart.

Install the minimal configuration:

```sh
helm repo add argo https://argoproj.github.io/argo-helm
helm repo update argo
helm upgrade --install argocd argo/argo-cd \
  --namespace argocd --create-namespace \
  --version 10.18.0 \
  -f deploy/platform/argocd/values.yaml
```

The values disable HA, Dex, notifications, and the dashboard. Argo still
provides the API and UI server; Weave reads the API, not the UI.

The repository is private. Create a repository secret with a token that has repository read access;
do not commit the token:

```sh
kubectl -n argocd create secret generic argocd-repo-weave \
  --from-literal=type=git \
  --from-literal=url=https://github.com/tchoupi663/weave.git \
  --from-literal=username="$GITHUB_USER" \
  --from-literal=password="$GITHUB_TOKEN"
kubectl -n argocd label secret argocd-repo-weave \
  argocd.argoproj.io/secret-type=repository
```

Register the Target App:

```sh
kubectl apply -f deploy/gitops/apps/target-app-local.yaml
```

The Application is left on manual sync. The repository intentionally does not
enable self-heal; fault-injection experiments must not be reverted by Argo.


## Weave's read-only API account

The Argo values create the local account `weave` with the `weave-reader`
project role. The role can only `get` applications.

Create an API token for that account with an admin session, then store it for
Weave:

```sh
# Port-forward the Argo API first: 18080 -> argocd-server:80
ADMIN_TOKEN=$(curl -sS -X POST http://127.0.0.1:18080/api/v1/session \
  -H 'Content-Type: application/json' \
  -d "{\"username\":\"admin\",\"password\":\"$ARGOCD_ADMIN_PASSWORD\"}" \
  | python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])')
WEAVE_TOKEN=$(curl -sS -X POST http://127.0.0.1:18080/api/v1/account/weave/token \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H 'Content-Type: application/json' \
  -d '{"id":"weave-history","name":"weave history"}' \
  | python3 -c 'import json,sys; print(json.load(sys.stdin)["token"])')
kubectl -n weave-system create secret generic weave-argocd-token \
  --from-literal=token="$WEAVE_TOKEN"
```

The secret is not committed. Weave reads `ARGOCD_URL`, `ARGOCD_APP`, and the
token from its chart values.
