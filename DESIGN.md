# Weave design

Weave is an operator screen for one question: what did the network actually do?
The design is a working instrument first and an atmosphere second. L4 topology
and L7 HTTP evidence share one surface; they are read together, never added
together.

## World

An operator console at night, with the loom still underneath. The graph is a
warp of observed connections: a signal-blue hub, a fan of steel service
threads, and two quiet tails to data and the external payment provider. The
ground is cool near-black, never a warm cream or a blue-purple dashboard.
Material comes from hairlines, surface steps, and the topology itself, not from
decorative imagery.

The Control AI case study is the composition reference: a full-bleed network
field, sparse chrome, a clear node taxonomy, a detail inspector, and a temporal
strip. The content and cosmic regulatory surface are not copied.

## Tokens

| Token | Value | Use |
| --- | --- | --- |
| `--bg` | `#0b0d10` | Graph ground |
| `--bg-deep` | `#080a0c` | Top bar and activity ribbon |
| `--surface` | `#101419` | Inspector |
| `--surface-2` | `#171c23` | Hover, raised rows |
| `--surface-3` | `#1f262e` | Pressed states |
| `--line` | `rgba(203, 214, 226, 0.09)` | Hairlines |
| `--line-strong` | `rgba(203, 214, 226, 0.17)` | Selected hairlines, chart baseline |
| `--thread` | `rgba(203, 214, 226, 0.24)` | Observed edge |
| `--text` | `#dbe1e8` | Primary text, hovered labels |
| `--text-2` | `#98a2ae` | Node labels, secondary text |
| `--text-3` | `#7b8592` | Counts, meta, labels |
| `--accent` | `#6f9fd8` | Hub, selection, shuttle |
| `--accent-bright` | `#8cb6e8` | Selected edge, traffic shuttle |
| `--node` | `#66717f` | Service dot |
| `--source` | `#93a0ad` | Source IP ring |
| `--external` | `#6f9f98` | External role |
| `--data` | `#8a7f9c` | Data role |
| `--ok` | `#6fa58e` | Live state only |
| `--danger` | `#c06a6a` | Relay/API failure only |

The graph ground is flat. There is no warp grid, no radial wash, and no halo.
Depth comes from the two ground steps (`--bg` inside, `--bg-deep` for the
chrome), hairlines, and the dot/thread hierarchy.

## Type

- Interface: **Instrument Sans Variable**, weights 400 and 500.
- Observation figures and metadata: **Fragment Mono**, weight 400, tabular
  figures.
- No uppercase tracked labels, no italic, no display face in controls.
- Interface body is 13px. Node labels are 13px in graph units (14px with the
  inspector open); node counts and edge labels are 10px (11px with the
  inspector open) and scale with the graph. At tablet widths with the panel
  open, names grow and the counts move to the inspector. Inspector headings are 16–20px;
  meta and observation labels are 10–11px.
- Counts use one format; compact counts on the graph (`8.4k`), full figures in
  the inspector (`8,432`).

Both faces are self-hosted through Fontsource and shipped in the image. The
interface does not load a font from a third-party host.

## Composition

- 52px top bar: wordmark, `Topology`, namespace and counts (including the
  number of L7-bearing edges), live state, pause, clear.
- Full-bleed flat graph stage. No grid, no column rule, no decorative layer.
- Plain legend at bottom left: role dots and the observation note. When any
  edge has L7, the note reads `L4 + L7 observations`; otherwise `L4
  observations`.
- Zoom control at bottom right: a compact graphite pill with −, the live
  percentage, +, and fit. Zoom runs from a hard 50% floor (twice the fitted
  extent) to 300%; fit is 100%. The wheel zooms at the pointer; `+`, `-`, and
  `0` work from the keyboard. Below 70% the node labels fade, and below 90% the
  resting edge titles hide; hover or selection brings the neighbourhood's labels
  back, so the wide view stays clean.
- 64px activity ribbon: a stepped hairline trace of the five-minute window
  (one step per five-second bucket), a low-contrast fill under it, the current
  observation as a single head that is signal blue when the bucket has flow
  and neutral when it is idle, and the cumulative count.
- A 320px inspector shares the stage: when a node or edge is selected the graph
  reflows into the remaining width instead of hiding behind the panel. The
  inspector has a hairline left border, no radius, no shadow, and no glass.
- No navigation sidebar, no KPI cards, no page title block.

The first viewport at 1440×900 shows the whole nine-node graph, the legend, and
the ribbon without scrolling. At 1024px the graph still fits; node counts keep a
compensating size until 720px, where they drop and the inspector takes the full
stage.

## Graph grammar

**Nodes** are single dots, not illustrations. Radius scales with observed event
volume (square root, 5–15 graph units). Services are dark steel, the API Gateway
is signal blue, the Source IP is an open ring (it is not a workload), the
external provider is muted teal, and data is muted violet. Each dot carries its
name and a quiet compact count underneath; no rings, orbits, gradients, or
glints. A hover or selection adds one thin ring around the dot.

**Edges** are straight 1px hairlines between dot rims, with no arrowheads. At
fit and above, every thread carries a quiet `protocol · port` title at its
midpoint in the muted text role. An edge with HTTP evidence appends `· L7` in
the signal-blue accent. Focusing a knot narrows the visible titles to
its own threads, and below 90% the resting titles hide so a wide view stays
clean. Hovering or selecting an edge brings its title to full contrast.

**Reading order.** Five architecture ranks seed the field: ingress, entry
service, hub, downstream services, terminal data or third party. A weak x-rank
force keeps that broad external-to-internal direction; it is not a lock.
Repulsion, per-thread springs and collision are free to move a knot sideways
and vertically when that keeps its threads short. This is what untangles the
graph: a data knot settles near its parent instead of being forced into a far
column, and the ranks still keep the overall flow left to right. The field is
centred and lifted until dot and label bounds share the stage.

**Focus.** Hovering a node lights the node, its one-hop neighbours, and their
edges, keeps their titles visible, and dims everything else. Hovering an edge
lights the edge, its two endpoints, and its title. Selection holds the same relationship map and
opens the inspector; hover previews it. The active edge is signal blue; selected node
rings are signal blue.

**Forces and dragging.** A knot can be dragged with the pointer. While it moves,
the threads stay attached and the one-hop neighbours respond under a real force
field: pairwise repulsion, per-thread springs, collision, a weak rank bias and a
weak centre pull. Releasing lets the knot settle under the same field, carrying
momentum from a flick; the rank bias and its threads draw it back toward its
architecture rank and the field resolves any tangle. There is no pin and no
reset control: the graph is a simulation that always returns to a readable
state. Dragging the background still pans; the wheel zooms at the pointer. The
field is still at rest: there is no ambient loop.

**Observation.** A node or edge exists only after a Hubble flow created it.
Kubelet probes from host to non-Frontend workloads are dropped. All non-cluster
sources collapse into one Source IP node. The graph never reads the expected
topology.

## Inspector

The inspector is a flat graphite column with a hairline, not a card. It shows:

- node: role, display name, kind, namespace, connections, events, last seen,
  observed pods, addresses, first observed, connected edges;
- edge: endpoints, protocol, port, connections, events, last seen, five-minute
  L4 activity, first observed, last observed, request direction;
- edge with L7: an HTTP block with requests, responses, cumulative status
  classes, cumulative p50/p95, five-minute request sparkline, and up to five
  normalized routes. The block names the latency source (`client` or `server`)
  and says that headers, query strings, and bodies are discarded;
- edge without L7: footer copy says `L4 only` and names HTTPS/Postgres as not
  L7-visible.

The connected-edge list is a clean text list with a protocol line, no chevrons
or arrows. There are no invented requests, latency, bytes, or status figures.
L7 figures are rendered only when the snapshot contains them.

## States

- **Loading:** “Following Hubble”, with a quiet detail line.
- **Empty:** “No traffic observed”, the namespace, and a relay status. A small
  ring-and-dot mark uses the same grammar as a knot; the zoom controls are
  hidden because there is nothing to frame.
- **Paused:** top bar says Paused; the graph stays readable; the activity trace's
  current head turns neutral.
- **Relay offline:** top bar says Relay offline; the last snapshot remains.
- **API/stream unavailable:** “Topology unavailable” with one retry action.
- **Selected:** related nodes/edges stay at full strength; the rest dims; the
  inspector opens with a short rise and the graph reflows.
- **Hover:** the same relationship highlight, without opening the inspector.

## Motion

One easing curve, `cubic-bezier(0.16, 1, 0.3, 1)`. Hover and selection run at
120–150ms; the inspector enters over 200ms; newly observed nodes fade in over
280ms, staggered by 18ms; one signal shuttle travels the busiest thread over
950ms. At most one shuttle runs at a time, and no edge repeats within two
seconds. A released knot settles back into the field with a short spring and
the simulation then stops. Reduced-motion users get the same states without the
shuttle, entrance, or force response; a dragged knot still moves, but on release
it snaps straight back to its seed position and its neighbours stay put.

## Accessibility

- Every node is a focusable button; edges are focusable and also reachable
  through the node inspector’s edge list. Repositioning is a pointer affordance
  and is transient: releasing settles the knot back into the field, so no layout
  state needs restoring.
- Escape closes the inspector; Space pauses/resumes when focus is not in a
  control.
- Focus is a signal-blue ring on nodes and a signal-blue band on edges; selected
  and related state is not carried by colour alone (dimming plus the inspector
  text).
- Text meets AA on the graphite surfaces; node counts stay above the small-text
  contrast floor.
- The graph has an accessible label; the activity trace has a text equivalent;
  compact labels keep a full aria-label with counts.

## Known limits

- Desktop-first. The working layout is designed for 1024px and wider. The graph
  remains zoomable on a phone but is not a supported operator layout in v1.
- L7 covers only the five HTTP edges observed through Frontend and Gateway
  client spans. Source IP → Frontend, Orders → Postgres, and Payment → External
  Payment Provider stay L4-only.
- Four L7 edges are client-observed; Frontend → Gateway is server-observed.
  The inspector labels the latency source per edge. Neither is Cilium proxy
  latency.
- The source address seen through the local port-forward is the host endpoint,
  not the Simulator laptop. The inspector says so.

## Asset provenance

`docs/images/weave-topology.png` is a screenshot captured from the in-cluster
Weave app at 1440×900 during a 20-second Simulator smoke run (38 Bounce, 73
Browse, and 23 Order journeys, zero failures). `docs/images/weave-l7-inspector.png`
is the same surface with the HTTP block selected. They are not generated images.
The favicon and the wordmark glyph are authored SVG, no external raster.
