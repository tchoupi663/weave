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
