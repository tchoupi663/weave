# Weave Testbed

The dummy Target App under observation, the Simulator that drives it, and Weave — the app that turns the resulting Hubble flows into an observed topology.

## Language

**Weave**:
The topology builder that observes the Target App's network activity and infers its nodes and edges. The Weave app runs in `weave-system`, follows Hubble Relay, and serves the operator screen. It is the consumer of the Simulator's traffic, not part of the Target App.
_Avoid_: topology service, graph service.

**Target App**:
The dummy application under observation, comprising the App cluster: Frontend, API Gateway, Auth, Catalog, Orders, Payment, External Payment Provider, and Postgres.
_Avoid_: app, system under test, test app.

**Simulator**:
The service that generates real traffic by having simulated Users execute Journeys against the Target App. It runs on the operator's laptop, outside the cluster, and is not part of the Target App deployment.
_Avoid_: loadgen, bot, synthetic traffic generator, user simulator.

**Source IP**:
The external boundary of the observed topology. All non-cluster traffic entering the exposed Frontend collapses into one Source IP node; the Simulator's laptop is the intended source. When the Frontend is reached through a local port-forward, Hubble reports the kind host endpoint instead, and Weave shows that observed address in the inspector.
_Avoid_: ext users, External Users, external clients, simulator node.

**User**:
A simulated visitor that executes exactly one Journey and then leaves.
_Avoid_: session, visitor, account, persona.

**Account**:
A pre-seeded Auth identity that a User logs in with. Accounts may be reused when the pool is exhausted.
_Avoid_: user, persona, login.

**Journey**:
A complete sequence of steps a User performs in the Target App, selected from a configured set. Every User executes exactly one Journey.
_Avoid_: flow, scenario, script, user flow.

**Bounce**:
A Journey that opens the landing page and leaves without browsing or logging in.
_Avoid_: visitor, drop-off.

**Browse**:
A Journey that opens the landing page, the product list, and 2–4 product details without logging in.
_Avoid_: window shopping, product journey.

**Order**:
A Journey that logs in, browses, checks out, and pays.
_Avoid_: purchase, conversion.

**Journey Mix**:
The configured shares of Users assigned to each Journey. The default is 30% Bounce, 50% Browse, 20% Order; every User takes exactly one Journey.
_Avoid_: behavior distribution, percentages, probabilities.

**Catalog**:
The Target App service that serves product data to the Frontend. It is separate from Orders.
_Avoid_: product service, inventory, orders.

**Run**:
One execution of the Simulator, defined by a Journey Mix, a concurrency target, and a duration. Each Run starts from a fresh Target App state and is reproducible from its configuration and seed.
_Avoid_: test, benchmark, simulation session.

**Run Report**:
The JSON artifact a Run emits: configuration, journey and step metrics, failures, and the Expected Topology.
_Avoid_: results, output, log.

**Inference Benchmark**:
A Run used to compare Weave's inferred Topology and edge telemetry against the Simulator's Expected Topology and request timeline.
_Avoid_: load test, performance benchmark.

**Expected Topology**:
The node and edge set the Simulator declares for a Run. The benchmark tool compares Weave's inferred Topology against it.
_Avoid_: golden graph, ground truth graph, reference topology.

**External Payment Provider**:
The dummy downstream service that Payment calls. It is part of the Target App fixture, not the Simulator's source network.
_Avoid_: External, external service, external users.

**Topology**:
The graph of nodes and edges inferred by Weave from observed network activity. The Simulator is not a node; its traffic appears as the Source IP boundary.
_Avoid_: graph, map, diagram.

**Node**:
A logical Kubernetes workload (Deployment, StatefulSet, or DaemonSet) that appeared in at least one qualifying flow, or the single Source IP boundary node. Pods and IP addresses are evidence attached to a node, not nodes themselves.
_Avoid_: pod node, service node, host.

**Edge**:
One ordered workload pair plus destination port. Replies and handshake packets fold into the request-direction edge. A new workload or port creates a new edge.
_Avoid_: flow, connection, link.

**Connection**:
A counted non-reply TCP SYN observation on an edge. It is an L4 observation, not an HTTP request; long-lived connections can carry many requests with no new connection.
_Avoid_: request, transaction, session.

**Event**:
A unique non-reply L4 flow observation after deduplication. Events are the activity unit on the graph and ribbon; they are not bytes or requests.
_Avoid_: packet, request, message.

**Activity Window**:
The five-minute rolling set of 60 event buckets on each edge and on the whole graph. Cumulative node and edge totals are kept separately since Weave started or was reset.
_Avoid_: history, retention, time series.
