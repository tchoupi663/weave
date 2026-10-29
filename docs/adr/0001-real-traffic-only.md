# Real traffic only

The Simulator generates real requests against the Target App and lets Weave observe the resulting network activity. It does not POST synthetic topology or telemetry into Weave.

The alternative was the loadgen design in `docs/images/2.excalidraw`, which fabricates 5s telemetry bursts. That is easier to control and cheaper to run, but it makes the topology self-fulfilling: Weave would be tested against data the Simulator invented, not against traffic the Target App actually produced. The cost is that the dummy services must be real enough to make their own downstream calls.
