import type { Role, TopologyEdge, TopologyNode } from './types'

export const GRAPH_WIDTH = 1200
export const GRAPH_HEIGHT = 640

const LEFT = 120
const RIGHT = 1080
const CENTER_Y = 320
const MIN_GAP = 148
const TOP_MARGIN = 76
const BOTTOM_MARGIN = 84
const MIN_RADIUS = 5
const MAX_RADIUS = 15
const COLUMN_COUNT = 5

// The reading order of an architecture diagram: ingress, entry service, hub,
// downstream services, terminal data or third party. The floors keep that
// order even when a flow between two tiers has not been observed yet.
const ROLE_FLOOR: Record<Role, number> = {
  source: 0,
  service: 1,
  hub: 2,
  external: 4,
  data: 4,
}

export interface PositionedNode extends TopologyNode {
  x: number
  y: number
  r: number
  tier: number
}

export interface GraphLayout {
  nodes: PositionedNode[]
  edges: TopologyEdge[]
}

export interface ThreadGeometry {
  path: string
  labelX: number
  labelY: number
}

function radiusFor(node: TopologyNode, maxEvents: number): number {
  if (node.events <= 0) return MIN_RADIUS
  const share = Math.sqrt(node.events / Math.max(1, maxEvents))
  return MIN_RADIUS + (MAX_RADIUS - MIN_RADIUS) * Math.min(1, share)
}

// The seed layout is the operator's reading order: source, frontend, gateway,
// services, data. The force model starts here and gently relaxes around it.
export function layoutTopology(nodes: TopologyNode[], edges: TopologyEdge[]): GraphLayout {
  const known = new Set(nodes.map((node) => node.id))
  const usableEdges = edges.filter((edge) => known.has(edge.source) && known.has(edge.target))

  const outgoing = new Map<string, TopologyEdge[]>()
  const incoming = new Map<string, TopologyEdge[]>()
  const indegree = new Map<string, number>()
  for (const node of nodes) {
    outgoing.set(node.id, [])
    incoming.set(node.id, [])
    indegree.set(node.id, 0)
  }
  for (const edge of usableEdges) {
    outgoing.get(edge.source)?.push(edge)
    incoming.get(edge.target)?.push(edge)
    indegree.set(edge.target, (indegree.get(edge.target) ?? 0) + 1)
  }

  const nodeByID = new Map(nodes.map((node) => [node.id, node]))
  const roots = nodes.filter((node) => node.role === 'source' || (indegree.get(node.id) ?? 0) === 0)
  const tier = new Map<string, number>()
  const queue: string[] = []
  for (const root of roots) {
    tier.set(root.id, ROLE_FLOOR[root.role] ?? 1)
    queue.push(root.id)
  }
  if (queue.length === 0 && nodes.length > 0) {
    tier.set(nodes[0].id, ROLE_FLOOR[nodes[0].role] ?? 1)
    queue.push(nodes[0].id)
  }

  const maxTierLimit = Math.max(6, nodes.length)
  let guard = 0
  while (queue.length > 0 && guard < nodes.length * Math.max(1, nodes.length)) {
    guard++
    const id = queue.shift() as string
    const currentTier = tier.get(id) ?? 0
    for (const edge of outgoing.get(id) ?? []) {
      const target = nodeByID.get(edge.target)
      const nextTier = Math.max(currentTier + 1, ROLE_FLOOR[target?.role ?? 'service'] ?? 1)
      if (nextTier > (tier.get(edge.target) ?? -1) && nextTier <= maxTierLimit) {
        tier.set(edge.target, nextTier)
        queue.push(edge.target)
      }
    }
  }
  for (const node of nodes) {
    if (!tier.has(node.id)) tier.set(node.id, ROLE_FLOOR[node.role] ?? 1)
  }

  // The five architecture columns stay put even while the graph is partial,
  // so nodes arriving one by one do not rescale the whole diagram.
  const maxTier = Math.max(COLUMN_COUNT - 1, ...Array.from(tier.values()))
  const byTier = new Map<number, TopologyNode[]>()
  for (const node of nodes) {
    const t = tier.get(node.id) ?? 0
    const group = byTier.get(t) ?? []
    group.push(node)
    byTier.set(t, group)
  }

  const maxEvents = Math.max(1, ...nodes.map((node) => node.events))
  const positioned = new Map<string, PositionedNode>()
  const tierGap = maxTier > 0 ? (RIGHT - LEFT) / maxTier : 0

  for (let t = 0; t <= maxTier; t++) {
    const group = (byTier.get(t) ?? []).slice().sort((a, b) => a.name.localeCompare(b.name))
    if (group.length === 0) continue

    const desired = new Map<string, number>()
    for (const node of group) {
      const parents = (incoming.get(node.id) ?? [])
        .map((edge) => positioned.get(edge.source))
        .filter((parent): parent is PositionedNode => Boolean(parent))
      if (parents.length === 0) {
        desired.set(node.id, CENTER_Y)
      } else {
        desired.set(node.id, parents.reduce((sum, parent) => sum + parent.y, 0) / parents.length)
      }
    }

    const collapsed = group.length > 1 &&
      group.every((node) => Math.abs((desired.get(node.id) ?? CENTER_Y) - (desired.get(group[0].id) ?? CENTER_Y)) < 1)
    let order = group.slice().sort((a, b) => {
      const diff = (desired.get(a.id) ?? CENTER_Y) - (desired.get(b.id) ?? CENTER_Y)
      return diff !== 0 ? diff : a.name.localeCompare(b.name)
    })
    if (collapsed) {
      order = group.slice().sort((a, b) => a.name.localeCompare(b.name))
    }

    const gap = order.length > 1
      ? Math.min(MIN_GAP, (GRAPH_HEIGHT - TOP_MARGIN - BOTTOM_MARGIN) / (order.length - 1))
      : MIN_GAP
    const ys = order.map((node) => desired.get(node.id) ?? CENTER_Y)
    for (let i = 1; i < ys.length; i++) {
      ys[i] = Math.max(ys[i], ys[i - 1] + gap)
    }
    // A tier with no preferred position fans out around the centre. A tier
    // with parents keeps their alignment, so the threads stay horizontal
    // where the traffic actually flows parent to child.
    if (collapsed) {
      const blockCenter = (ys[0] + ys[ys.length - 1]) / 2
      const shift = CENTER_Y - blockCenter
      for (let i = 0; i < ys.length; i++) ys[i] += shift
    }
    if (ys[0] < TOP_MARGIN) {
      const lift = TOP_MARGIN - ys[0]
      for (let i = 0; i < ys.length; i++) ys[i] += lift
    }
    if (ys[ys.length - 1] > GRAPH_HEIGHT - BOTTOM_MARGIN) {
      const drop = ys[ys.length - 1] - (GRAPH_HEIGHT - BOTTOM_MARGIN)
      for (let i = 0; i < ys.length; i++) ys[i] -= drop
    }

    const x = maxTier > 0 ? LEFT + t * tierGap : GRAPH_WIDTH / 2
    order.forEach((node, i) => {
      positioned.set(node.id, {
        ...node,
        x,
        y: ys[i],
        r: radiusFor(node, maxEvents),
        tier: t,
      })
    })
  }

  // Labels hang below their knots, so the visual mass sits lower than the
  // node centres. Lift the whole field until dot and label bounds share the
  // stage evenly.
  if (positioned.size > 0) {
    let top = Number.POSITIVE_INFINITY
    let bottom = Number.NEGATIVE_INFINITY
    for (const node of positioned.values()) {
      top = Math.min(top, node.y - node.r - 4)
      bottom = Math.max(bottom, node.y + node.r + 40)
    }
    const wanted = CENTER_Y - (top + bottom) / 2
    const shift = Math.min(GRAPH_HEIGHT - 8 - bottom, Math.max(8 - top, wanted))
    for (const node of positioned.values()) node.y += shift
  }

  return {
    nodes: Array.from(positioned.values()),
    edges: usableEdges,
  }
}

// A thread touches the rim of each knot. The label floats just above the
// midpoint on the side away from the other nodes.
export function threadGeometry(
  source: { x: number; y: number; r: number },
  target: { x: number; y: number; r: number },
): ThreadGeometry {
  const dx = target.x - source.x
  const dy = target.y - source.y
  const length = Math.hypot(dx, dy) || 1
  const ux = dx / length
  const uy = dy / length

  const sx = source.x + ux * (source.r + 3)
  const sy = source.y + uy * (source.r + 3)
  const tx = target.x - ux * (target.r + 5)
  const ty = target.y - uy * (target.r + 5)

  const mx = (sx + tx) / 2
  const my = (sy + ty) / 2
  let nx = -uy
  let ny = ux
  if (ny > 0) {
    nx = -nx
    ny = -ny
  }

  return {
    path: `M ${sx} ${sy} L ${tx} ${ty}`,
    labelX: mx + nx * 13,
    labelY: my + ny * 13 + 3.5,
  }
}
