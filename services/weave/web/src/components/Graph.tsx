import { useMemo } from 'react'
import { formatCompact } from '../format'
import type { GraphLayout } from '../layout'
import type { Selection, Snapshot, StreamState, TopologyEdge } from '../types'
import { NetworkGraph, type NetworkEdge, type NetworkNode } from './NetworkGraph'

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
  const nodes = useMemo<NetworkNode[]>(
    () => layout.nodes.map((node) => ({
      id: node.id,
      label: node.name,
      meta: `${formatCompact(node.events)} ${node.events === 1 ? 'event' : 'events'}`,
      role: node.role,
      x: node.x,
      y: node.y,
      r: node.r,
      ariaLabel: `${node.name}, ${node.kind}, ${node.connections} connections, ${node.events} ${node.events === 1 ? 'event' : 'events'}`,
    })),
    [layout.nodes],
  )
  const nodesByID = useMemo(() => new Map(layout.nodes.map((node) => [node.id, node])), [layout.nodes])
  const edges = useMemo<NetworkEdge[]>(
    () => layout.edges.map((edge) => ({
      id: edge.id,
      source: edge.source,
      target: edge.target,
      title: `${edge.protocol} · ${edge.port}${errorRateLabel(edge)}`,
      ariaLabel: `${nodesByID.get(edge.source)?.name ?? 'Unknown'} to ${nodesByID.get(edge.target)?.name ?? 'Unknown'}, ${edge.protocol} port ${edge.port}${edge.l7 ? ', HTTP observed' : ''}`,
      pulseKey: edge.last_seen,
    })),
    [layout.edges, nodesByID],
  )

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

  return (
    <section className="graph-shell" aria-label="Observed topology">
      <NetworkGraph
        nodes={nodes}
        edges={edges}
        selected={selected}
        hovered={hovered}
        activeEdges={hotEdges}
        onSelect={onSelect}
        onHover={onHover}
        ariaLabel={`Observed topology of ${snapshot?.namespace ?? 'the watched namespace'}`}
        showZoom={!showState}
      >
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
      </NetworkGraph>
    </section>
  )
}
