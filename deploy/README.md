# deploy

Everything needed to run the Target App on a cluster, ordered by the layer it
belongs to. Layers are applied bottom-up; each one needs the previous one to
exist.

| Directory | Layer | Contents |
|---|---|---|
| `cluster/` | day 0 — before any GitOps | kind cluster definition, CNI (Cilium) values, `bootstrap.sh` |
| `gitops/` | day 1 — the deployer | Argo CD values (later), Application definitions |
| `platform/` | day 2+ — cluster addons | ingress, observability, secret management (empty for now) |
| `charts/` | the app itself | `target-app/` Helm chart, environment-neutral defaults |
| `envs/` | the app's environments | one values file per environment, deltas only |

## Order of operations

```
deploy/cluster/bootstrap.sh          # kind + cilium + argo cd (run by hand, once)
kubectl apply -f deploy/gitops/apps/target-app-local.yaml
```

1. **Cluster + CNI.** Cilium is the CNI, so it must exist before Argo CD: Argo's
   own Pods need pod networking. This is why the CNI is not managed by Argo.
2. **Argo CD.** Installed by the same bootstrap script, after Cilium is healthy.
3. **Applications.** Argo CD renders `charts/target-app` with
   `envs/<env>/values.yaml` and applies the result.

## Rendering without a cluster

```bash
helm template weave ./deploy/charts/target-app -f deploy/envs/local/values.yaml
helm lint ./deploy/charts/target-app
```

The chart must render on its own (`helm template weave ./deploy/charts/target-app`)
so a new environment starts from something that works. Environment files contain
deltas only: if `envs/local/values.yaml` grows close to `charts/target-app/values.yaml`
in length, defaults have leaked out of the chart.

## What lives where, concretely

- A new environment: add `envs/<env>/values.yaml` + `gitops/apps/target-app-<env>.yaml`.
  Same chart, same commit.
- A new cluster addon (ingress controller, metrics-server, sealed-secrets):
  `platform/<addon>/values.yaml`, installed either by `bootstrap.sh` or, once
  trusted, by its own Argo CD Application.
- A new app chart: `charts/<name>/`.

## Two rules

- **One owner per object.** Either Helm (`helm upgrade --install`) or Argo CD,
  never both for the same workload. Argo CD does not create Helm releases.
- **Argo CD does not build or load images.** `docker compose build` and
  `kind load docker-image --name kind weave-target-app:dev weave-target-frontend:dev`
  stay outside GitOps.
