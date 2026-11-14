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

The first incident fixture is
`fixtures/catalog-http-500.yaml`: an `HTTPChaos` that replaces Catalog
`GET /api/products` responses with HTTP 500 for two minutes. It must be applied
after the 15-minute healthy baseline and removed after the incident window. The
fixture is owned by the fault harness, not by Weave.

The full acceptance sequence is `scripts/incident-replay.sh`. It starts the
20-minute Simulator run, applies the fixture at the 15-minute mark, removes it
two minutes later, waits for the three-minute recovery, and creates the
incident. The recorded result is in [`docs/acceptance.md`](../../../docs/acceptance.md).
