import { useEffect, useMemo, useRef, useState, type PointerEvent as ReactPointerEvent } from 'react'
import { formatCompact } from '../format'
import { GRAPH_HEIGHT, GRAPH_WIDTH, threadGeometry, type GraphLayout } from '../layout'
import type { Selection, Snapshot, StreamState, TopologyEdge } from '../types'
import { FitIcon, MinusIcon, PlusIcon } from '../icons'
import { useForceGraph } from '../useForceGraph'

interface ViewBox {
  x: number
  y: number
  w: number
  h: number
}

interface Props {
  snapshot: Snapshot | null
  layout: GraphLayout
  paused: boolean
  streamState: StreamState
  loading: boolean
  error: string | null
  selected: Selection | null
  hovered: Selection | null
  hotEdges: Set<string>
  onSelect: (selection: Selection | null) => void
  onHover: (selection: Selection | null) => void
  onRetry: () => void
}

interface NodeDrag {
  pointerId: number
  nodeId: string
  startX: number
  startY: number
  moved: boolean
}

const FIT: ViewBox = { x: 0, y: 0, w: GRAPH_WIDTH, h: GRAPH_HEIGHT }
const MIN_WIDTH = 400
const MAX_WIDTH = GRAPH_WIDTH * 2 // 50%, a hard floor on zoom-out
const FAR_LABEL_ZOOM = 70
const EDGE_LABEL_ZOOM = 90

function errorRate(edge: TopologyEdge): number | null {
  if (!edge.l7 || edge.l7.requests <= 0) return null
  return (edge.l7.status['4xx'] + edge.l7.status['5xx']) / edge.l7.requests
}

function errorRateLabel(edge: TopologyEdge): string {
  const rate = errorRate(edge)
  if (rate === null) return ''
  const percent = rate === 0 ? '0' : (rate * 100).toFixed(rate >= 0.1 ? 0 : 1)
  return ` · ${percent}% err`
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(max, Math.max(min, value))
}

function zoomViewBox(current: ViewBox, factor: number, px: number, py: number): ViewBox {
  const nextW = clamp(current.w / factor, MIN_WIDTH, MAX_WIDTH)
  const nextH = nextW * (GRAPH_HEIGHT / GRAPH_WIDTH)
  if (nextW >= GRAPH_WIDTH) {
    // Beyond fit there is no content to anchor to; hold the topology centred.
    return {
      x: (GRAPH_WIDTH - nextW) / 2,
      y: (GRAPH_HEIGHT - nextH) / 2,
      w: nextW,
      h: nextH,
    }
  }
  const anchorX = current.w > GRAPH_WIDTH ? GRAPH_WIDTH / 2 : px
  const anchorY = current.w > GRAPH_WIDTH ? GRAPH_HEIGHT / 2 : py
  const ratioX = (anchorX - current.x) / current.w
  const ratioY = (anchorY - current.y) / current.h
  return {
    x: clamp(anchorX - ratioX * nextW, 0, GRAPH_WIDTH - nextW),
    y: clamp(anchorY - ratioY * nextH, 0, GRAPH_HEIGHT - nextH),
    w: nextW,
    h: nextH,
  }
}

export function Graph({
  snapshot,
  layout,
  paused,
  streamState,
  loading,
  error,
  selected,
  hovered,
  hotEdges,
  onSelect,
  onHover,
  onRetry,
}: Props) {
  const svgRef = useRef<SVGSVGElement>(null)
  const [viewBox, setViewBox] = useState<ViewBox>(FIT)
  const drag = useRef<{
    pointerId: number
    x: number
    y: number
    view: ViewBox
    moved: boolean
  } | null>(null)
  const nodeDrag = useRef<NodeDrag | null>(null)

  const { nodes: forceNodes, draggingID, beginDrag, moveDrag, endDrag: endNodeDrag } = useForceGraph(
    layout.nodes,
    layout.edges,
  )
  const forceByID = useMemo(() => new Map(forceNodes.map((node) => [node.id, node])), [forceNodes])
  const nodeMeta = useMemo(() => new Map(layout.nodes.map((node) => [node.id, node])), [layout.nodes])
  const viewNodes = useMemo(
    () => layout.nodes.map((node) => {
      const live = forceByID.get(node.id)
      return live ? { ...node, x: live.x, y: live.y, r: live.r } : node
    }),
    [layout.nodes, forceByID],
  )
  const viewEdges = useMemo(
    () => layout.edges.flatMap((edge) => {
      const source = forceByID.get(edge.source) ?? nodeMeta.get(edge.source)
      const target = forceByID.get(edge.target) ?? nodeMeta.get(edge.target)
      if (!source || !target) return []
      return [{ edge, ...threadGeometry(source, target) }]
    }),
    [layout.edges, forceByID, nodeMeta],
  )

  useEffect(() => {
    const element = svgRef.current
    if (!element) return
    const onWheel = (event: WheelEvent) => {
      event.preventDefault()
      const matrix = element.getScreenCTM()
      if (!matrix) return
      const point = element.createSVGPoint()
      point.x = event.clientX
      point.y = event.clientY
      const local = point.matrixTransform(matrix.inverse())
      const factor = Math.exp(-event.deltaY * 0.0015)
      setViewBox((current) => zoomViewBox(current, factor, local.x, local.y))
    }
    element.addEventListener('wheel', onWheel, { passive: false })
    return () => element.removeEventListener('wheel', onWheel)
  }, [])

  // Keyboard zoom, matching the visible controls: + / - / 0.
  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.defaultPrevented || event.metaKey || event.ctrlKey || event.altKey) return
      const target = event.target as HTMLElement | null
      if (target?.closest('input, textarea, select, [contenteditable="true"]')) return
      if (event.key === '+' || event.key === '=') {
        event.preventDefault()
        setViewBox((current) => zoomViewBox(current, 1.25, current.x + current.w / 2, current.y + current.h / 2))
      } else if (event.key === '-' || event.key === '_') {
        event.preventDefault()
        setViewBox((current) => zoomViewBox(current, 0.8, current.x + current.w / 2, current.y + current.h / 2))
      } else if (event.key === '0') {
        event.preventDefault()
        setViewBox(FIT)
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [])

  const active = hovered ?? selected
  const focusEdges = useMemo(() => {
    if (!active) return new Set<string>()
    if (active.kind === 'edge') return new Set([active.id])
    return new Set(layout.edges.filter((edge) => edge.source === active.id || edge.target === active.id).map((edge) => edge.id))
  }, [active, layout.edges])
  const focusNodes = useMemo(() => {
    if (!active) return new Set<string>()
    if (active.kind === 'edge') {
      const edge = layout.edges.find((candidate) => candidate.id === active.id)
      return edge ? new Set([edge.source, edge.target]) : new Set<string>()
    }
    // One hop: the hovered knot and everything it touches.
    const nodes = new Set([active.id])
    for (const edge of layout.edges) {
      if (edge.source === active.id) nodes.add(edge.target)
      else if (edge.target === active.id) nodes.add(edge.source)
    }
    return nodes
  }, [active, layout.edges])

  const zoomPercent = Math.round((GRAPH_WIDTH / viewBox.w) * 100)
  const farOut = zoomPercent < FAR_LABEL_ZOOM
  const showEdgeLabels = zoomPercent >= EDGE_LABEL_ZOOM
  const hasFocus = Boolean(active)
  const showState = !snapshot || layout.nodes.length === 0
  const unavailable = !snapshot && !loading && !error && streamState === 'error'
  const stateTitle = error
    ? 'Topology unavailable'
    : unavailable
      ? 'API unavailable'
      : loading
        ? 'Following Hubble'
        : 'No traffic observed'
  const stateCopy = error
    ? error
    : unavailable
      ? 'The topology stream is not reachable. Retry the connection.'
      : loading
        ? 'Waiting for the first observed flow in this namespace.'
        : paused
          ? 'Live updates are paused. Resume to follow new flows.'
          : 'Weave builds this graph from Hubble flows. Start a Simulator run; the first observed flow creates a node.'
  const stateDetail = snapshot
    ? `${snapshot.namespace} · ${snapshot.hubble.connected ? 'relay connected' : 'relay offline'}`
    : 'hubble relay · connecting'

  const zoomAtCenter = (factor: number) => {
    setViewBox((current) => zoomViewBox(current, factor, current.x + current.w / 2, current.y + current.h / 2))
  }

  const toGraphPoint = (clientX: number, clientY: number): { x: number; y: number } => {
    const svg = svgRef.current
    if (!svg) return { x: 0, y: 0 }
    const point = svg.createSVGPoint()
    point.x = clientX
    point.y = clientY
    const matrix = svg.getScreenCTM()
    if (!matrix) return { x: 0, y: 0 }
    const local = point.matrixTransform(matrix.inverse())
    return { x: local.x, y: local.y }
  }

  const onNodePointerDown = (event: ReactPointerEvent<SVGGElement>, nodeId: string) => {
    if (event.button !== 0) return
    event.stopPropagation()
    const point = toGraphPoint(event.clientX, event.clientY)
    nodeDrag.current = {
      pointerId: event.pointerId,
      nodeId,
      startX: event.clientX,
      startY: event.clientY,
      moved: false,
    }
    event.currentTarget.setPointerCapture(event.pointerId)
    beginDrag(nodeId, point.x, point.y)
    onHover({ kind: 'node', id: nodeId })
  }

  const onNodePointerMove = (event: ReactPointerEvent<SVGGElement>) => {
    const current = nodeDrag.current
    if (!current || current.pointerId !== event.pointerId) return
    event.stopPropagation()
    if (Math.hypot(event.clientX - current.startX, event.clientY - current.startY) > 4) {
      current.moved = true
    }
    const point = toGraphPoint(event.clientX, event.clientY)
    moveDrag(point.x, point.y)
  }

  const onNodePointerUp = (event: ReactPointerEvent<SVGGElement>) => {
    const current = nodeDrag.current
    if (!current || current.pointerId !== event.pointerId) return
    nodeDrag.current = null
    event.stopPropagation()
    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId)
    }
    endNodeDrag(current.moved)
    if (!current.moved) onSelect({ kind: 'node', id: current.nodeId })
  }

  const onNodePointerCancel = (event: ReactPointerEvent<SVGGElement>) => {
    const current = nodeDrag.current
    if (!current || current.pointerId !== event.pointerId) return
    nodeDrag.current = null
    event.stopPropagation()
    endNodeDrag(current.moved)
  }

  const onPointerDown = (event: ReactPointerEvent<SVGSVGElement>) => {
    if (event.button !== 0) return
    event.currentTarget.setPointerCapture(event.pointerId)
    drag.current = {
      pointerId: event.pointerId,
      x: event.clientX,
      y: event.clientY,
      view: viewBox,
      moved: false,
    }
  }

  const onPointerMove = (event: ReactPointerEvent<SVGSVGElement>) => {
    const current = drag.current
    if (!current || current.pointerId !== event.pointerId) return
    const rect = event.currentTarget.getBoundingClientRect()
    const dx = event.clientX - current.x
    const dy = event.clientY - current.y
    if (!current.moved && Math.hypot(dx, dy) > 3) current.moved = true
    if (!current.moved) return
    const nextX = current.view.w >= GRAPH_WIDTH
      ? (GRAPH_WIDTH - current.view.w) / 2
      : clamp(current.view.x - (dx / rect.width) * current.view.w, 0, GRAPH_WIDTH - current.view.w)
    const nextY = current.view.h >= GRAPH_HEIGHT
      ? (GRAPH_HEIGHT - current.view.h) / 2
      : clamp(current.view.y - (dy / rect.height) * current.view.h, 0, GRAPH_HEIGHT - current.view.h)
    setViewBox({ ...current.view, x: nextX, y: nextY })
  }

  const endDrag = (event: ReactPointerEvent<SVGSVGElement>) => {
    const current = drag.current
    if (!current || current.pointerId !== event.pointerId) return
    if (event.currentTarget.hasPointerCapture(event.pointerId)) {
      event.currentTarget.releasePointerCapture(event.pointerId)
    }
    drag.current = null
    if (!current.moved) onSelect(null)
  }

  return (
    <section className="graph-shell" aria-label="Observed topology">
      <div className={`graph${hasFocus ? ' has-focus' : ''}${farOut ? ' is-far' : ''}${showEdgeLabels ? ' shows-edge-labels' : ''}`}>
        <svg
          ref={svgRef}
          className="graph-svg"
          viewBox={`${viewBox.x} ${viewBox.y} ${viewBox.w} ${viewBox.h}`}
          role="group"
          aria-label={`Observed topology of ${snapshot?.namespace ?? 'the watched namespace'}`}
          onPointerDown={onPointerDown}
          onPointerMove={onPointerMove}
          onPointerUp={endDrag}
          onPointerCancel={endDrag}
        >
          <defs>
            <marker id="edge-arrow" viewBox="0 0 8 8" refX="7" refY="4" markerWidth="5" markerHeight="5" orient="auto">
              <path d="M0 0 L8 4 L0 8" className="edge-arrow-head" />
            </marker>
          </defs>
          <g className="edges">
            {viewEdges.map(({ edge, path, labelX, labelY }) => {
              const isSelected = selected?.kind === 'edge' && selected.id === edge.id
              const isHovered = hovered?.kind === 'edge' && hovered.id === edge.id
              const isFocus = focusEdges.has(edge.id)
              const isHot = hotEdges.has(edge.id)
              const source = nodeMeta.get(edge.source)
              const target = nodeMeta.get(edge.target)
              return (
                <g
                  key={edge.id}
                  className={[
                    'edge-group',
                    isSelected ? 'is-selected' : '',
                    isHovered ? 'is-hovered' : '',
                    isFocus ? 'is-focus' : '',
                  ].filter(Boolean).join(' ')}
                  onPointerDown={(event) => event.stopPropagation()}
                  onClick={(event) => {
                    event.stopPropagation()
                    onSelect({ kind: 'edge', id: edge.id })
                  }}
                  onPointerEnter={() => onHover({ kind: 'edge', id: edge.id })}
                  onPointerLeave={() => onHover(null)}
                >
                  <path d={path} className="edge" markerEnd="url(#edge-arrow)" />
                  {isHot ? (
                    <path
                      key={`${edge.id}-${edge.last_seen}`}
                      d={path}
                      className="edge-traffic"
                      pathLength={1}
                    />
                  ) : null}
                  <text x={labelX} y={labelY} className="edge-label" textAnchor="middle" aria-hidden="true">
                    {edge.protocol} · {edge.port}{errorRateLabel(edge)}
                  </text>
                  <path
                    d={path}
                    className="edge-hit"
                    aria-label={`${source?.name ?? 'Unknown'} to ${target?.name ?? 'Unknown'}, ${edge.protocol} port ${edge.port}${edge.l7 ? ', HTTP observed' : ''}`}
                    role="button"
                    tabIndex={0}
                    onKeyDown={(event) => {
                      if (event.key === 'Enter' || event.key === ' ') {
                        event.preventDefault()
                        onSelect({ kind: 'edge', id: edge.id })
                      }
                    }}
                  />
                </g>
              )
            })}
          </g>

          <g className="nodes">
            {viewNodes.map((node, index) => {
              const isSelected = selected?.kind === 'node' && selected.id === node.id
              const isFocus = focusNodes.has(node.id)
              const isDragging = draggingID === node.id
              return (
                <g
                  key={node.id}
                  className={[
                    'node-group',
                    `role-${node.role}`,
                    isSelected ? 'is-selected' : '',
                    isFocus ? 'is-focus' : '',
                    isDragging ? 'is-dragging' : '',
                  ].filter(Boolean).join(' ')}
                  transform={`translate(${node.x} ${node.y})`}
                  style={{ animationDelay: `${index * 18}ms` }}
                  role="button"
                  tabIndex={0}
                  aria-label={`${node.name}, ${node.kind}, ${node.connections} connections, ${node.events} ${node.events === 1 ? 'event' : 'events'}`}
                  onPointerDown={(event) => onNodePointerDown(event, node.id)}
                  onPointerMove={onNodePointerMove}
                  onPointerUp={onNodePointerUp}
                  onPointerCancel={onNodePointerCancel}
                  onKeyDown={(event) => {
                    if (event.key === 'Enter' || event.key === ' ') {
                      event.preventDefault()
                      onSelect({ kind: 'node', id: node.id })
                    }
                  }}
                  onPointerEnter={() => onHover({ kind: 'node', id: node.id })}
                  onPointerLeave={() => onHover(null)}
                >
                  <circle className="node-hit" r={node.r + 10} />
                  <circle className="node-focus" r={node.r + 7} />
                  {node.role === 'source' ? (
                    <circle className="node-source" r={node.r} />
                  ) : (
                    <circle className="node-dot" r={node.r} />
                  )}
                  <text className="node-label" y={node.r + 19} textAnchor="middle">
                    {node.name}
                  </text>
                  <text className="node-count" y={node.r + 32} textAnchor="middle">
                    {formatCompact(node.events)} {node.events === 1 ? 'event' : 'events'}
                  </text>
                </g>
              )
            })}
          </g>
        </svg>

        {showState ? (
          <div className="graph-state" role="status">
            <span className="state-mark" aria-hidden="true" />
            <h1>{stateTitle}</h1>
            <p>{stateCopy}</p>
            {error || unavailable ? (
              <button type="button" className="text-button" onClick={onRetry}>
                Retry connection
              </button>
            ) : null}
            <span className="state-detail">{stateDetail}</span>
          </div>
        ) : null}

        {showState ? null : (
          <div className="graph-hud">
            <div className="zoom-controls" role="group" aria-label="Graph zoom">
              <button
                type="button"
                className="zoom-button"
                onClick={() => zoomAtCenter(0.8)}
                aria-label="Zoom out"
                title="Zoom out (−)"
                disabled={zoomPercent <= 50}
              >
                <MinusIcon size={14} />
              </button>
              <span className="zoom-level" aria-live="polite" aria-label={`Zoom ${zoomPercent} percent`}>
                {zoomPercent}%
              </span>
              <button
                type="button"
                className="zoom-button"
                onClick={() => zoomAtCenter(1.25)}
                aria-label="Zoom in"
                title="Zoom in (+)"
                disabled={zoomPercent >= 300}
              >
                <PlusIcon size={14} />
              </button>
              <span className="zoom-separator" aria-hidden="true" />
              <button type="button" className="zoom-button" onClick={() => setViewBox(FIT)} aria-label="Fit graph" title="Fit graph (0)">
                <FitIcon size={14} />
              </button>
            </div>
          </div>
        )}
      </div>
    </section>
  )
}
