# GitOps (Argo CD)

Argo CD is installed by `deploy/cluster/bootstrap.sh`, after the CNI. It is
**not** self-managed and **not** responsible for the CNI: if Cilium breaks,
Argo CD would lose the network it needs to repair it.

| Path | Purpose |
|---|---|
| `argocd/` | values for the `argo-cd` Helm chart (not needed yet — bootstrap installs it with defaults) |
| `apps/target-app-local.yaml` | Application for the local environment |
| `apps/target-app-<env>.yaml` | add one per environment; same chart, different `$values/...` file |

Each Application uses two sources: the chart, plus a `ref: values` source that
lets `helm.valueFiles` point outside the chart directory
(`$values/deploy/envs/<env>/values.yaml`).

## One-time setup per cluster

```bash
# Repository credentials (the repo is private)
kubectl -n argocd create secret generic weave-repo \
  --from-literal=type=git \
  --from-literal=url=https://github.com/tchoupi663/weave.git \
  --from-literal=username=<github-user> \
  --from-literal=password=<fine-grained-pat> \
  --dry-run=client -o yaml | kubectl apply -f -
kubectl -n argocd label secret weave-repo argocd.argoproj.io/secret-type=repository --overwrite

kubectl apply -f deploy/gitops/apps/target-app-local.yaml
```

Order matters: the repository secret must exist before Argo CD tries to read
the repo, or the Application reports an authentication error.

## Operating it

```bash
kubectl -n argocd port-forward svc/argocd-server 8080:443
kubectl -n argocd get secret argocd-initial-admin-secret -o jsonpath='{.data.password}' | base64 -d

# or CLI
argocd app list
argocd app diff target-app-local
argocd app sync target-app-local
```

Start with manual sync (`syncPolicy: {}`), read the diff, then enable
`automated` + `selfHeal` once the chart is boring. Self-heal reverts manual
changes — relevant when you start fault injection.

## What Argo CD does not do

- Build images, or load them into kind. Both stay outside.
- Install itself, Cilium, or kind. That is `deploy/cluster/bootstrap.sh`.
- Manage the `weave` namespace if you delete it: it is created by
  `syncOptions: CreateNamespace=true`, but it is not a tracked resource.
