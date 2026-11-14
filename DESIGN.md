# Signal Console

A portable interface direction for graph-and-telemetry tools. It is written as
a system, not a product: swap the vocabulary, roles, and data, keep the paper,
type, shapes, graph grammar, and motion.

Reference direction: [relatedb.ai](https://relatedb.ai/). The light paper field,
near-black ink, single blue signal, tight neutral-grotesque headings, mono
metadata, and quiet density are adapted from it. The reference uses proprietary
Relate Sans and Relate Mono; this system uses the closest open equivalents,
Inter Variable and Fragment Mono.

## Principles

1. **One field.** The graph is the page. Chrome sits at the edges and never
   competes with the data.
2. **Paper, not dashboard.** Light ground, ink text, hairline structure, one
   accent. No dark panels, glass, glows, or decorative gradients.
3. **Shape carries hierarchy.** Circles mean data points. Squircles mean
   surfaces and controls. Never mix the two jobs.
4. **Telemetry in mono.** Numbers, identifiers, timestamps, edge labels, and
   section eyebrows use the mono face. Prose uses the grotesque.
5. **Motion answers input.** Nothing loops. Entrances, focus, drag settle, and
   end-of-scroll acknowledgement are short and physical.
6. **Density with air.** Dense rows and large type can coexist when the grid,
   hairlines, and three-color text hierarchy do the work.
7. **One accent.** Brand blue marks the hub, selection, links, and focus. Rust
   is reserved for errors; green for healthy status; indigo for a second data
   role. Never tint a whole surface with the accent.
8. **Bento, not dashboard.** Sections are distinct squircle panels on a sunken
   paper ground, joined by one consistent gutter. The panels hold the work; the
   grid is not a wall of KPI cards.

## Color

Light-first. Every surface is paper; depth comes from hairlines and one step of
surface tone, not from shadows.

| Token | Value | Role |
| --- | --- | --- |
| `paper` | `#fafafa` | Page, graph field, top bar |
| `raised` | `#ffffff` | Floating panels, inspector, inputs |
| `sunken` | `#f2f2f2` | Rails, embedded regions, secondary surfaces |
| `pressed` | `#e8e8e8` | Hover/pressed step, empty marks |
| `line` | `#e4e7e8` | Hairlines, panel borders |
| `line-strong` | `#c9ced0` | Control borders, graph threads, chart baselines |
| `ink` | `#0a1114` | Primary text, primary button fill |
| `ink-muted` | `#59656a` | Secondary text, labels, counts |
| `ink-faint` | `#5f6a6e` | Tertiary metadata; same weight as muted |
| `brand` | `#0a6190` | Hub, selection, links, focus |
| `brand-deep` | `#014063` | Brand hover, active edge |
| `brand-soft` | `#e4f1f6` | Selected row, quiet accent fill |
| `brand-line` | `#a8d0e0` | Accent hairline |
| `indigo` | `#3b4fd0` | Data role, secondary signal |
| `success` | `#145439` | Healthy status, external role |
| `danger` | `#b02a00` | Error text |
| `highlight` | `#4c99a4` | Activity mid-ramp |
| `timeline-clear` | `#4a8066` | A sampled timeline bucket with no errors |
| `timeline-elevated` | `#b37c47` | An elevated error rate on the timeline |
| `timeline-fault` | `#8b261a` | A faulted timeline bucket |

Graph roles:

| Role | Value | Note |
| --- | --- | --- |
| source | `ink-muted` | Open ring, external entry point |
| hub | `brand` | The one emphasized node |
| service | `#6f7a7f` | Neutral filled circle |
| external | `success` | A third-party endpoint |
| data | `indigo` | Storage or derived data |
| thread | `line-strong` | Observed edge hairline |

Two ramps only:

**Error field** (timeline): no sample `#e8e8e8`, then clear `#4a8066` →
elevated `#b37c47` → fault `#8b261a` → severe `#641811`. Green is reserved for
a sampled bucket with no errors; a leading gap is neutral, not green.

**Activity heat** (bucketed activity strips): `#f2f2f2`, `#e4f1f6`, `#a8d0e0`,
`#4c99a4`, `#8a5a3a`, `#b02a00`.

Shadows:

- Floating surface: `0 1px 2px #0a11140f, 0 8px 24px #0a111414`
- Raised control: `0 1px 2px #0a111414, 0 1px 3px #0a11140d`

Shadows belong only to things that actually float above the page: a HUD pill,
an expanded activity card, a toast. Flat panels use a hairline, not a shadow.

On a dark adapter, invert the paper/ink roles, keep the same accent and role
hues, and keep the error ramp's semantic order. Do not invert the accent into a
pastel.

## Type

A neutral grotesque for interface and prose; a mono for telemetry.

- **Interface and prose:** Inter Variable (or an equivalent neutral grotesque).
- **Data, labels, identifiers:** Fragment Mono (or an equivalent mono).
- **Weights:** 400, 500, 600. No 700, italic, or display face.
- **Tracking:** negative on display and headings; zero on body; positive on
  mono eyebrows. Never track body text.
- **Case:** sentence case. Mono eyebrows are not uppercase; the tracking and
  the mono face do the labeling work.

| Role | Face | Size | Weight | Line height | Tracking |
| --- | --- | --- | --- | --- | --- |
| Display / workspace title | Grotesque | 32 px | 500 | 1.1 | −0.025em |
| Section heading | Grotesque | 22 px | 600 | 1.2 | −0.02em |
| List / panel title | Grotesque | 17 px | 600 | 1.25 | −0.02em |
| Body | Grotesque | 15 px | 400 | 1.5 | 0 |
| Small body | Grotesque | 13 px | 400 | 1.45 | 0 |
| Micro metadata | Grotesque | 12 px | 400 | 1.4 | 0 |
| Eyebrow / section label | Mono | 13 px | 500 | 1.2 | 0.04em |
| Data, timestamps, identifiers | Mono | 13 px | 400 | 1.4 | 0 |
| Metric | Mono | 26 px | 500 | 1.1 | −0.01em |
| Button | Grotesque | 14 px | 500 | 1 | 0 |
| Graph node label | Grotesque | 16 px | 500 | 1.2 | 0 |
| Graph edge title | Mono | 12.5 px | 400 | 1.2 | 0 |
| Status chip | Mono | 12 px | 500 | 1 | 0 |

Rules:

- 12 px is the floor for operator-visible text. Do not invent 9–10 px metadata.
- Use the mono metric size for one number per panel; everything else is data
  size. A screen with five giant numbers has no hierarchy.
- Numbers use tabular figures. Timestamps always use the same format in a
  view; list metadata may use a compact range, but the detail view shows the
  full range.
- Body copy is at most 66 characters per line. Do not justify.
- Section labels are mono and tracked; do not also uppercase them.

## Shape

Two shape jobs, never mixed:

1. **Graph nodes are circles.** A node is a point in a field, not a container.
   Its hit area and focus ring are circles too. This is the only circular shape
   in the system.
2. **Everything else is a squircle.** Buttons, inputs, cards, panels, rails,
   rows, chips, status marks, the zoom pill, the playhead thumb, the wordmark
   mark, and icon dots. No pill (`999px`), no `50%` UI, no plain sharp
   rectangle.

Squircle scale:

| Use | Radius |
| --- | --- |
| Small control, button, input, chip | 6 px |
| Panel, row, track | 10 px |
| Floating card, expanded HUD | 12 px |
| Dialog / large surface | 16 px |
| Status mark, legend swatch | 2–4 px |

A squircle is a superellipse. At the panel scale, a 14–20 px single-radius
corner reads as the same soft shape and is the practical implementation; at
control scale 6–10 px the radius is likewise exact enough. Reserve an SVG
superellipse mask for an oversized hero surface where the curve is the point.
Do not use a pill or a sharp rectangle as a substitute.

Graph nodes are the deliberate exception: circles only.

## Layout

The interface is a **bento of squircle panels on a sunken paper ground**. Every
major section is its own panel; the page gutter and the gutters between panels
are the same 12 px. One hero panel carries the primary reading; supporting
panels are smaller; nothing floats unanchored.

- Ground: `sunken` (`#f2f2f2`). Panels: `paper` (`#fafafa`). Floating
  instruments: `raised` (`#ffffff`).
- Outer padding 12 px; panel gap 12 px; phone 8 px.
- Panel radius 14–16 px; input/button radius 6 px; status mark 2–4 px.
- Every panel: 1 px `line` hairline, one faint shadow
  (`0 1px 2px #0a11140a`). Floating instruments use the stronger shadow.
- Top bar is a 58 px panel. Left rail is a 252 px panel. Evidence is a 336 px
  panel. Inspector is a 360 px floating panel. Content max width 1360 px.
- A hero panel (graph, timeline, affected region) may carry **instrument
  cards**: the flow card, legend, zoom, and inspector. A data field inside a
  hero (the graph canvas, the anomaly canvas) is a sunken inset, not another
  card. Do not nest content cards inside content cards.
- The primary workspace: top-bar panel; graph hero panel; flow card at its top
  right; legend card at its bottom left; zoom card at its bottom right;
  inspector side sheet when selected.
- The history workspace: rail panel; header panel; timeline hero; affected-
  region hero; onset and change panels paired; notes panel full width; evidence
  panel on the right.
- Breakpoints: 1180 (main column becomes one column), 1100 (evidence stacks
  below the heroes), 920 (rail becomes a horizontal panel above the stage),
  720 (phone: 8 px gutters, every panel one column, HUD at the bottom, graph
  edge titles hide).
- No KPI cards, no ribbon, no page-title block. The panels hold real
  instruments, not decorative statistics.
- The graph is full-bleed inside its hero panel. No grid, axis, or decorative
  background layer inside the panel.

## Components

**Top bar.** Brand at left, workspace switch next to it, scope/counts centered
in mono, status and actions at right. The switch is text buttons; the active
one is `raised` with the raised shadow, 6 px radius. Do not use tabs with
underlines, pills, or icons-only navigation for a two- or three-workspace
switch.

**Status.** A 5 px squircle mark plus a text label. Color never carries the
state alone: `Live`, `Paused`, `Offline`, `History` are always written.

**Buttons.** Primary is ink-filled with white text; secondary is raised paper
with a `line-strong` border and muted text; destructive is danger text/border.
Minimum height 32 px (28 px small), 6 px radius, 14 px/500. Icon buttons are
28–30 px, 6 px radius, muted until hover. One primary action per region.

**Inputs.** Raised paper, 1 px `line-strong`, 6 px radius, 14 px text. Labels
above in 13 px; hints below in 12 px muted. Focus is a 2 px brand ring with a
2 px offset. Native date/time controls are acceptable; keep `color-scheme:
light`.

**Panels and cards.** The panel is the layout unit: paper fill, 1 px hairline,
14–16 px radius, one faint shadow, 16–18 px padding. The top bar, graph,
timeline, affected region, evidence, notes, rail, and composer are panels.
Floating instrument cards (flow, legend, zoom, inspector) are white with the
stronger shadow. A sunken data field inside a hero is not a card. No glass, no
backdrop blur on data.

**Rails and lists.** A row is a 6 px squircle, 10–12 px vertical padding.
Selected: `brand-soft` fill, ink text, brand left marker or dot. Hover:
`sunken`. Meta is mono 12 px muted, truncated with a `title`; use compact time
ranges in lists.

**Detail panel.** Flush column with a hairline; eyebrow labels, a 22 px title,
a meta line, hairline sections, and mono values. No card inside the panel.

**Metrics.** Mono 26 px value, 12 px mono label, 12 px unit. Group at most four
per row. Align numbers on the baseline; tabular figures.

**Empty, loading, error.** A centered squircle mark, a short title, one
sentence, and at most one action. Use the mono detail line for scope. Loading
uses a skeleton with the paper/sunken steps, never a spinner-only state.

**HUD.** The zoom pill is a raised squircle with 28 px icon buttons and a mono
level. The legend is bottom-left, 12 px, with squircle swatches. No toolbar
chrome around the graph.

## Graph grammar

**Nodes.** Circles. Radius scales with volume using a square root, 5–15 graph
units. Fill by role; hub is brand; service is neutral gray; source is an open
ring; external is green; data is indigo. Each node carries a 16 px label and a
12 px mono count/meta below it. The label has a 3 px paper halo; edges and edge
titles never do.

**Edges.** A 1 px `line-strong` hairline with a 5 px arrow at the target.
Selected: brand, 1.4 px. Hover: ink at 75%. Focus: ink at 42%. Edges are
straight; no curves, no bundles, no decorative dashes.

**Edge titles.** Mono 12.5 px, `ink-muted`, set 13 px off the thread at 42% of
its length. Format `protocol · port`, plus a compact error percentage when
request-level evidence exists. **No background plate, halo, or box behind the
title.** The line must stay visible. Titles appear at 90% zoom and above; below
70% node labels fade; focus reduces labels to the selected neighborhood.

**Reading order.** Seed nodes on a left-to-right architecture axis. Keep a weak
x-rank bias, link springs, repulsion, collision, and a weak center gravity. The
field may move a node vertically to shorten threads; do not hard-lock x or y.
Dragging moves the node; releasing lets the field settle it back. Entrance is
280 ms with an 18 ms stagger. The graph is a real force field, not a static
layout.

**Interaction.** Wheel zooms at the pointer, 50% floor to 300%, fit resets.
Background drag pans. Hover lights the one-hop neighborhood; selection holds it
and opens the detail panel. Keyboard: `+`, `-`, `0` for zoom; Tab reaches every
node and edge; Enter/Space selects. A node drag must not turn into a click.

**One graph.** Live and history views use the same graph component and the same
forces, hover, drag, zoom, and edge-title behavior. History seeds the same field
from its own bounded node set; it does not get a separate static canvas.

## Event timeline

The timeline is a **field**, not a row of cells.

- Compute the error rate per sample (errors ÷ requests). Draw one horizontal
  CSS gradient across the playback context, from the incident baseline through
  the incident window.
- An empty sample is not calm. Inside a sampled run, hold the adjacent observed
  state; a leading gap with no observed state stays neutral and reads
  `No sample`.
- Ramp: clear green → elevated amber → fault rust → severe dark rust. The ramp
  is semantic; do not use a rainbow. Green is reserved for a sampled bucket
  with no errors.
- **Aggressive rises snap.** A rise of 6 percentage points or more, or a jump
  from below 8% to 20%+, gets a hard stop at the sample boundary: hold the
  previous color to `boundary − 0.5%`, then place the new color at the
  boundary. The color changes immediately at the sample.
- **Falls and gradual changes ramp.** Interpolate the rate across the bucket
  through three intermediate stops so the fall follows the same ramp.
- **Continuous readout.** The playhead, clock, rates, counts, latency, and
  related regional labels interpolate between samples. Counts round for
  display; rates and latency may be fractional.
- Playhead: 2 px ink line, 8 px squircle thumb with a 1 px ink ring.
- Keyboard: arrows move one sample; PageUp/PageDown move five; Home/End jump
  to the ends of the playback context. The range input carries `aria-valuetext`
  with the clock, the `Pre-incident` or `Incident` state, and the sampled error
  rate.
- Start the playback at the incident baseline. Mark the incident boundary and
  label the pre-incident and incident bands. Write the current moment as
  `Pre-incident` or `Incident` with its error rate; never rely on color alone.
- Label the field with the section eyebrow, the current state and rate, and the
  source line `time · Ns samples`. The rate is also written in the detail
  panel.

## Motion

- One curve: `cubic-bezier(0.2, 0.7, 0.2, 1)`.
- Hover/selection: 120–150 ms. Panel entrance: 200 ms. Node entrance: 280 ms,
  18 ms stagger. Scroll acknowledgement: 160 ms in, about 900 ms hold.
- Only one looping animation is allowed, and only when it encodes traffic: a
  short directional pulse on the busiest one or two edges, cleared after one
  second. Everything else is input-driven.
- **End-of-scroll.** A page must not look frozen. Containers that own their wheel
  show an inset wash and a hairline at the edge they ran out of; native-scroll
  containers show an inset brand ring. The signal is deferred until the scroll
  settles (~180 ms), so it never flickers mid-scroll and never animates at
  rest. Reduced motion removes it.
- No ambient breathing, floating, parallax, or cursor followers.

## Accessibility

- Text meets AA on paper: ink on paper, muted on paper, white on ink, white on
  brand. Do not use the danger color for body copy; reserve it for error
  values and status text.
- Focus is a 2 px brand ring with a 2 px offset. The graph's node ring and edge
  band use the same brand.
- Every node and edge is a real focusable control with an accessible name.
  Dragging is pointer-only; selecting is available by keyboard.
- The timeline is a native range input; the visual gradient is decorative. The
  current sample is available as text and in `aria-valuetext`.
- Status and error are never color-only: write the label and the number.
- Minimum target 32 px; 28 px for secondary icon buttons.
- Respect `prefers-reduced-motion`: no entrance, force response, pulse, or
  scroll acknowledgement.

## Adapting to another project

The portable layer is: paper + ink + one brand, neutral-grotesque + mono, circle
nodes, squircle chrome, hairline structure, one graph component, the gradient
event timeline, and the motion rules. Everything else is an adapter:

1. Replace the brand and role hues; keep paper, ink, muted, line, and the one
   accent. Keep success green, danger rust, and the error ramp order.
2. Map your domain roles onto source, hub, service, external, and data. If you
   need more than five, add a role token; do not add a new accent hue.
3. Map your records onto the list → detail → evidence pattern. Compact list
   metadata; full detail in the panel.
4. Keep the type scale. If the product is denser, reduce the base from 15 to 14
   and the display from 32 to 28; do not go below 12 for visible text.
5. Keep circles for graph nodes and squircles for every surface. Do not turn
   nodes into squircles or buttons into pills.
6. Keep the timeline a gradient field. If your samples are not error rates, map
   your severity metric onto the same ramp and keep the hard-rise rule for
   genuinely abrupt changes.
7. Keep one graph component for live and history. Reuse the forces, drag,
   hover, zoom, and titles.
8. Delete decorative chart libraries. The graph, the field, and the typography
   are the visual system.

Do not add: pill buttons, glows, gradient text, glass panels, dark floating
cards, tiny uppercase labels, five competing accent colors, content cards inside
content cards, full-bleed sections without a panel edge, or looping decorative
motion.

## Minimum contract

A screen is on-system when it has:

- a bento of squircle panels on a sunken paper ground, with 12 px gutters;
- paper panels, ink text, muted metadata, hairline structure, one faint shadow;
- one brand accent, with success/indigo/rust only in their semantic roles;
- a 32/22/17/15/13/12 type hierarchy with mono eyebrows and mono data;
- circular graph nodes and squircle everything else;
- edge titles that sit off the thread with no plate;
- a gradient event timeline that starts at the pre-incident baseline, marks the
  incident boundary, and keeps hard rises and continuous readout;
- one graph component shared by live and history;
- focus rings, keyboard paths, reduced motion, and no color-only status;
- no unnecessary chrome: no KPI cards, no ribbons, no decorative grid.
