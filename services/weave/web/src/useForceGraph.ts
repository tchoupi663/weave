import { useCallback, useEffect, useRef, useState } from 'react'
import type { PositionedNode } from './layout'
import type { TopologyEdge } from './types'

export interface SimNode {
  id: string
  x: number
  y: number
  vx: number
  vy: number
  r: number
  seedX: number
  seedY: number
  rankX: number
  dragging: boolean
  fx: number
  fy: number
}

interface Link {
  source: string
  target: string
  rest: number
}

const MIN_X = 64
const MAX_X = 1112
const MIN_Y = 40
const MAX_Y = 566
const CENTER_X = 600
const CENTER_Y = 320

// Forces are per-frame accelerations at 60fps. This is a real force field, not
// a set of springs to fixed slots: the architecture rank is a bias, the links
// are the strongest structure, and the field is free to shift a knot sideways
// or vertically when that keeps the threads short and untangled.
const REPULSION = 4200
const COLLISION_PAD = 50
const LINK_STRENGTH = 0.012
const LINK_SLACK = 0.85
const RANK_X = 0.006
const GRAVITY = 0.00025
const DAMPING = 0.9
const MAX_SPEED = 8
const SLEEP_SPEED = 0.05

interface SyncResult {
  structural: boolean
  ranksMoved: boolean
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value))
}

function prefersReducedMotion(): boolean {
  return typeof window !== 'undefined' && window.matchMedia('(prefers-reduced-motion: reduce)').matches
}

class ForceSimulation {
  nodes: SimNode[] = []
  links: Link[] = []
  private byID = new Map<string, SimNode>()
  private dragging: SimNode | null = null
  private drag = {
    originX: 0,
    originY: 0,
    offsetX: 0,
    offsetY: 0,
    lastDX: 0,
    lastDY: 0,
    lastMoveAt: 0,
  }

  constructor(seeds: PositionedNode[], edges: TopologyEdge[]) {
    this.sync(seeds, edges)
  }

  sync(seeds: PositionedNode[], edges: TopologyEdge[]): SyncResult {
    const seedByID = new Map(seeds.map((seed) => [seed.id, seed]))
    const next = new Map<string, SimNode>()
    let structural = false
    let ranksMoved = false

    for (const seed of seeds) {
      const existing = this.byID.get(seed.id)
      if (existing) {
        existing.r = seed.r
        existing.seedX = seed.x
        existing.seedY = seed.y
        if (Math.abs(existing.rankX - seed.x) > 0.5) {
          existing.rankX = seed.x
          ranksMoved = true
        }
        next.set(seed.id, existing)
      } else {
        next.set(seed.id, {
          id: seed.id,
          x: seed.x,
          y: seed.y,
          vx: 0,
          vy: 0,
          r: seed.r,
          seedX: seed.x,
          seedY: seed.y,
          rankX: seed.x,
          dragging: false,
          fx: 0,
          fy: 0,
        })
        structural = true
      }
    }

    if (this.dragging && !next.has(this.dragging.id)) this.dragging = null
    if (next.size !== this.byID.size) structural = true
    this.nodes = Array.from(next.values())
    this.byID = next

    const previous = new Map(this.links.map((link) => [`${link.source}\u0000${link.target}`, link]))
    const nextLinks: Link[] = []
    for (const edge of edges) {
      if (!next.has(edge.source) || !next.has(edge.target)) continue
      const key = `${edge.source}\u0000${edge.target}`
      const existing = previous.get(key)
      if (existing) {
        nextLinks.push(existing)
        previous.delete(key)
      } else {
        const source = seedByID.get(edge.source)
        const target = seedByID.get(edge.target)
        const rest = source && target
          ? clamp(Math.hypot(target.x - source.x, target.y - source.y) * LINK_SLACK, 150, 260)
          : 200
        nextLinks.push({ source: edge.source, target: edge.target, rest })
        structural = true
      }
    }
    if (previous.size > 0) structural = true
    this.links = nextLinks

    return { structural, ranksMoved }
  }

  beginDrag(id: string, x: number, y: number): boolean {
    const node = this.byID.get(id)
    if (!node) return false
    this.dragging = node
    node.dragging = true
    node.vx = 0
    node.vy = 0
    this.drag = {
      originX: node.x,
      originY: node.y,
      offsetX: node.x - x,
      offsetY: node.y - y,
      lastDX: 0,
      lastDY: 0,
      lastMoveAt: performance.now(),
    }
    return true
  }

  moveDrag(x: number, y: number): boolean {
    const node = this.dragging
    if (!node) return false
    const nextX = clamp(x + this.drag.offsetX, MIN_X, MAX_X)
    const nextY = clamp(y + this.drag.offsetY, MIN_Y, MAX_Y)
    this.drag.lastDX = nextX - node.x
    this.drag.lastDY = nextY - node.y
    this.drag.lastMoveAt = performance.now()
    node.x = nextX
    node.y = nextY
    node.vx = 0
    node.vy = 0
    return true
  }

  endDrag(moved: boolean): boolean {
    const node = this.dragging
    if (!node) return false
    node.dragging = false
    if (prefersReducedMotion()) {
      // Without a simulation, return the knot to its architecture rank so the
      // reading order cannot be lost.
      node.x = node.seedX
      node.y = node.seedY
      node.vx = 0
      node.vy = 0
    } else {
      // A flick carries a little momentum into the settle instead of stopping
      // dead on release.
      const recent = moved && performance.now() - this.drag.lastMoveAt < 120
      node.vx = recent ? clamp(this.drag.lastDX * 0.45, -7, 7) : 0
      node.vy = recent ? clamp(this.drag.lastDY * 0.45, -7, 7) : 0
    }
    this.dragging = null
    return true
  }

  step(dt: number): boolean {
    if (this.nodes.length === 0) return false

    for (const node of this.nodes) {
      node.fx = 0
      node.fy = 0
    }

    for (let i = 0; i < this.nodes.length; i++) {
      const a = this.nodes[i]
      for (let j = i + 1; j < this.nodes.length; j++) {
        const b = this.nodes[j]
        let dx = b.x - a.x
        let dy = b.y - a.y
        let distance = Math.hypot(dx, dy)
        if (distance < 0.01) {
          dx = 0.5 + i * 0.1
          dy = 0.5 + j * 0.1
          distance = Math.hypot(dx, dy)
        }
        const ux = dx / distance
        const uy = dy / distance
        const minimum = a.r + b.r + COLLISION_PAD
        let force = distance < 520 ? REPULSION / Math.max(distance * distance, 900) : 0
        if (distance < minimum) force += (minimum - distance) * 0.35
        force = Math.min(3, force)
        a.fx -= ux * force
        a.fy -= uy * force
        b.fx += ux * force
        b.fy += uy * force
      }
    }

    for (const link of this.links) {
      const a = this.byID.get(link.source)
      const b = this.byID.get(link.target)
      if (!a || !b) continue
      const dx = b.x - a.x
      const dy = b.y - a.y
      const distance = Math.hypot(dx, dy)
      if (distance < 0.01) continue
      const ux = dx / distance
      const uy = dy / distance
      const force = clamp((distance - link.rest) * LINK_STRENGTH, -2.5, 2.5)
      a.fx += ux * force
      a.fy += uy * force
      b.fx -= ux * force
      b.fy -= uy * force
    }

    for (const node of this.nodes) {
      if (node.dragging) continue
      node.fx += (node.rankX - node.x) * RANK_X + (CENTER_X - node.x) * GRAVITY
      node.fy += (CENTER_Y - node.y) * GRAVITY
    }

    const damping = Math.pow(DAMPING, dt)
    let maxSpeed = 0
    for (const node of this.nodes) {
      if (node.dragging) {
        node.vx = 0
        node.vy = 0
        continue
      }
      node.vx = (node.vx + node.fx * dt) * damping
      node.vy = (node.vy + node.fy * dt) * damping
      const speed = Math.hypot(node.vx, node.vy)
      if (speed > MAX_SPEED) {
        node.vx = (node.vx / speed) * MAX_SPEED
        node.vy = (node.vy / speed) * MAX_SPEED
      }
      node.x += node.vx * dt
      node.y += node.vy * dt
      if (node.x < MIN_X || node.x > MAX_X) {
        node.x = clamp(node.x, MIN_X, MAX_X)
        node.vx = 0
      }
      if (node.y < MIN_Y || node.y > MAX_Y) {
        node.y = clamp(node.y, MIN_Y, MAX_Y)
        node.vy = 0
      }
      maxSpeed = Math.max(maxSpeed, Math.hypot(node.vx, node.vy))
    }

    return this.dragging !== null || maxSpeed > SLEEP_SPEED
  }

  snapshot(): SimNode[] {
    return this.nodes.map((node) => ({ ...node }))
  }
}

export function useForceGraph(seedNodes: PositionedNode[], edges: TopologyEdge[]) {
  const simRef = useRef<ForceSimulation | null>(null)
  if (simRef.current === null) {
    simRef.current = new ForceSimulation(seedNodes, edges)
  }
  const [nodes, setNodes] = useState<SimNode[]>(() => simRef.current!.snapshot())
  const raf = useRef<number | null>(null)
  const lastTime = useRef(0)

  const publish = useCallback(() => {
    setNodes(simRef.current!.snapshot())
  }, [])

  const frame = useCallback((time: number) => {
    const sim = simRef.current
    if (!sim) return
    const elapsed = lastTime.current === 0 ? 16.667 : time - lastTime.current
    lastTime.current = time
    const dt = clamp(elapsed / 16.667, 0.25, 2.5)
    const moving = sim.step(dt)
    publish()
    if (moving) {
      raf.current = requestAnimationFrame(frame)
    } else {
      raf.current = null
    }
  }, [publish])

  const wake = useCallback(() => {
    if (prefersReducedMotion()) return
    if (raf.current !== null) return
    lastTime.current = 0
    raf.current = requestAnimationFrame(frame)
  }, [frame])

  useEffect(() => {
    const sim = simRef.current!
    const result = sim.sync(seedNodes, edges)
    publish()
    if (result.structural || result.ranksMoved) wake()
  }, [seedNodes, edges, publish, wake])

  useEffect(() => () => {
    if (raf.current !== null) {
      cancelAnimationFrame(raf.current)
      raf.current = null
    }
  }, [])

  const beginDrag = useCallback((id: string, x: number, y: number) => {
    if (!simRef.current!.beginDrag(id, x, y)) return
    publish()
    wake()
  }, [publish, wake])

  const moveDrag = useCallback((x: number, y: number) => {
    if (!simRef.current!.moveDrag(x, y)) return
    publish()
  }, [publish])

  const endDrag = useCallback((moved: boolean) => {
    simRef.current!.endDrag(moved)
    publish()
    wake()
  }, [publish, wake])

  return {
    nodes,
    draggingID: nodes.find((node) => node.dragging)?.id ?? null,
    beginDrag,
    moveDrag,
    endDrag,
  }
}
