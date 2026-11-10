# Topology

Mode: Operate
Primary target: `services/weave/web` at `/`

## Scope

The one live topology screen for Weave v1, plus its node/edge inspector and
states. No navigation shell other than the top bar, no landing page, no runs or
benchmarks screens.

## Audience and job

A platform or network engineer watches Hubble-fed traffic and needs to verify
which workloads talked to which, over which port, and how recently. The task is
inspection, not authoring. Success is a graph that reads at a glance and an
inspector that proves the observation.

## Constraints

- Data is real; there is no demo dataset and no synthetic topology.
- L4 only: connections and events, never requests, latency, bytes, or status.
- Dark-only.
- Desktop-first, usable down to 1024; mobile is not a target.
- The graph has nine nodes in the healthy run and must not fake density.

## Direction contract

**THESIS.** A live network loom: observed traffic is the only thing allowed to
draw the graph. It refuses the dashboard default of KPI cards around a
decorative diagram; the topology itself is the working surface.

**OWN-WORLD.** Cool near-black graphite ground, steel text, signal blue for the
hub and selection, and muted teal and violet only where node role needs to be
distinguished. Instrument Sans for language, Fragment Mono for observation
figures. One dot per knot, one straight hairline per observed thread with a
quiet protocol/port title at fit, labels below the dots; no grid, halo, glass, glow, or gradient chrome, and no warm
cream or brass.

**STORY.** The visitor sees the entry thread arrive at Source IP, pass through
Frontend and API Gateway, fan out to the four services, and terminate at
Postgres and the External Payment Provider. Selecting a knot or thread proves
the claim with its observed pods, addresses, ports, connections, events, and
timestamps.

**FIRST VIEWPORT.** A 52px top bar with the Weave wordmark, namespace, live
state, and controls; a full-bleed graph canvas; a 64px five-minute activity
ribbon along the bottom; a compact borderless legend at bottom left; a compact
zoom control with a live percentage at bottom right, from a 50% floor to 300%; a 320px inspector that
shares the right edge and reflows the graph on selection. Knots can be dragged;
threads follow, and the force field settles them back into the architecture
ranks so the external-to-internal reading order holds. The graph occupies the
optical centre and the empty state reports the namespace and the absence of
traffic.

**FORM.** Code-led. Operate mode. The composition comes from the Control AI
reference: full-bleed network field, sparse chrome, node taxonomy, inspector,
and temporal strip. The material is derived from Weave's own name: a loom at
night, not a cosmic regulatory constellation.

**FINISH.** Unreviewed and undocumented is unfinished; this build ends with the
finish review, the verdict, DESIGN.md, and every shipping raster carrying its
provenance.
