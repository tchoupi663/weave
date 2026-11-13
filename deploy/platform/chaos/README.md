# Chaos Mesh

Chaos Mesh is the fault-injection platform for the incident replay. It is not
part of the Weave chart.

Install the minimal configuration:

```sh
helm repo add chaos-mesh https://charts.chaos-mesh.org
helm repo update chaos-mesh
helm upgrade --install chaos-mesh chaos-mesh/chaos-mesh \
  --namespace chaos-mesh --create-namespace \
  --version 2.8.8 \
  -f deploy/platform/chaos/values.yaml
```

The values use the kind containerd socket, one controller replica, and no
dashboard. Chaos DNS is installed by the chart but is not used by Weave.

The first incident fixture is an `HTTPChaos` that replaces Catalog
`GET /api/products` responses with HTTP 500. It must be applied after the
Target App is running and removed after the run. The fixture is owned by the
fault harness, not by Weave.
