# Observability platform

Weave reads HTTP L7 observations from Grafana Beyla. Beyla uses eBPF probes; it
does not insert a proxy into the Target App data path and does not modify the
Target App.

## Beyla

Beyla is an external Helm chart, not part of the Weave chart. Install it once per
cluster:

```sh
helm repo add grafana https://grafana.github.io/helm-charts
helm repo update grafana
helm upgrade --install beyla grafana/beyla \
  --namespace beyla-system --create-namespace \
  --version 1.16.17 \
  -f deploy/platform/observability/beyla-values.yaml
```

The values file configures three things:

- instrument only the `gateway` Deployment in `weave`; gateway client spans cover the four downstream HTTP edges, and gateway server spans cover `Frontend → Gateway`;
- export OTLP HTTP/protobuf traces to `weave.weave-system.svc.cluster.local:8080`;
- keep the DaemonSet privileged, as Beyla requires for eBPF.

Weave accepts OTLP traces at `POST /v1/traces` and keeps gateway client spans
plus the gateway server spans whose peer is a watched workload. It discards
headers, query strings, and bodies. The span's source
Deployment and destination service are attached to the matching L4 topology
edge; the L4 graph remains the source of truth for the edge's endpoints.

The Cilium L7 visibility policy was evaluated and rejected because its Envoy
proxy added a measurable latency regression. The gateway-only Beyla
configuration measured +0.3% weighted p95 with zero failures. See
`docs/adr/0004-l7-observability.md`.
