# Weave Testbed

The dummy Target App under observation and the Simulator that drives it. Weave consumes the resulting network activity; the service that displays the graph is out of scope here.

## Language

**Weave**:
The topology builder that observes the Target App's network activity and infers its nodes and edges. Weave is the consumer of the Simulator's traffic, not part of the Target App.
_Avoid_: topology service, graph service.

**Target App**:
The dummy application under observation, comprising the App cluster: Frontend, API Gateway, Auth, Catalog, Orders, Payment, External Payment Provider, and Postgres.
_Avoid_: app, system under test, test app.

**Simulator**:
The service that generates real traffic by having simulated Users execute Journeys against the Target App. It runs on the operator's laptop, outside the cluster, and is not part of the Target App deployment.
_Avoid_: loadgen, bot, synthetic traffic generator, user simulator.

**Source IP**:
The laptop's network address as observed by Weave. All Simulator traffic shares this external source; Weave displays the IP rather than a named user population.
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
The graph of nodes and edges inferred by Weave from observed network activity. The Simulator is not a node; its traffic appears as the laptop's Source IP.
_Avoid_: graph, map, diagram.
