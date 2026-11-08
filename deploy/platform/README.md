# platform

Cluster-level addons that are neither the CNI nor the app. Empty on purpose —
each addon gets a directory with its own `values.yaml` when it is adopted.

Candidates, in the order they usually become necessary:

| Directory | Addon | Why |
|---|---|---|
| `ingress/` | ingress-nginx (or Gateway API) | replace the frontend NodePort with an L7 entry point |
| `observability/` | kube-prometheus-stack, Hubble UI exposure | metrics, dashboards, flow visibility |
| `secrets/` | Sealed Secrets or External Secrets | get `postgres-secret` out of the chart |

Addon install order relative to Argo CD: an addon required by the app can be
installed by `bootstrap.sh` (before Argo) or by its own Application (after
Argo), but an addon required *by Argo CD itself* (CNI, CoreDNS) must be in
`deploy/cluster/`.
