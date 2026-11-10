import { formatClock, formatCount, formatDateTime, formatRelative, roleLabel, shortKind } from '../format'
import type { L7Stats, Selection, Snapshot, TopologyEdge, TopologyNode } from '../types'
import { CloseIcon } from '../icons'

interface Props {
  snapshot: Snapshot
  selected: Selection
  now: number
  onClose: () => void
  onSelect: (selection: Selection) => void
}

function MiniActivity({ values, label = 'Observed flow events in the last five minutes' }: { values: number[]; label?: string }) {
  const max = Math.max(1, ...values)
  return (
    <div className="mini-activity" role="img" aria-label={label}>
      {values.map((value, index) => (
        <span
          key={index}
          className={index === values.length - 1 ? 'mini-bar is-current' : 'mini-bar'}
          style={{ height: `${Math.max(2, (value / max) * 100)}%` }}
        />
      ))}
    </div>
  )
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="stat">
      <dt>{label}</dt>
      <dd>{value}</dd>
    </div>
  )
}

function NodeInspector({
  snapshot,
  node,
  now,
  onSelect,
}: {
  snapshot: Snapshot
  node: TopologyNode
  now: number
  onSelect: (selection: Selection) => void
}) {
  const edges = snapshot.edges.filter((edge) => edge.source === node.id || edge.target === node.id)
  const byID = new Map(snapshot.nodes.map((candidate) => [candidate.id, candidate]))

  return (
    <>
      <div className="inspector-heading">
        <span className="inspector-type">Node</span>
        <span className={`role-tag role-${node.role}`}>
          <span className="role-dot" aria-hidden="true" />
          {roleLabel(node.role)}
        </span>
      </div>
      <h2 id="inspector-title">{node.name}</h2>
      <p className="inspector-sub">
        {shortKind(node.kind)}
        {node.namespace ? ` · ${node.namespace}` : ' · ingress'}
      </p>

      <dl className="stat-grid">
        <Stat label="Connections" value={formatCount(node.connections)} />
        <Stat label="Events" value={formatCount(node.events)} />
        <Stat label="Last seen" value={formatRelative(node.last_seen, now)} />
      </dl>

      {node.role === 'source' ? (
        <p className="inspector-note">
          Entered through the local port-forward. Hubble reports the host endpoint as the source.
        </p>
      ) : null}

      <section className="inspector-section">
        <h3>Observed as</h3>
        {node.pods.length > 0 ? (
          <div className="observation-block">
            <span className="observation-label">Pod{node.pods.length > 1 ? 's' : ''}</span>
            <ul className="value-list mono">
              {node.pods.map((pod) => <li key={pod}>{pod}</li>)}
            </ul>
          </div>
        ) : (
          <p className="muted-copy">No pod metadata; this is an external endpoint.</p>
        )}
        {node.ips.length > 0 ? (
          <div className="observation-block">
            <span className="observation-label">Address{node.ips.length > 1 ? 'es' : ''}</span>
            <ul className="value-list mono">
              {node.ips.map((ip) => <li key={ip}>{ip}</li>)}
            </ul>
          </div>
        ) : null}
        <div className="observation-block">
          <span className="observation-label">{node.namespace ? 'First observed' : 'Observed'}</span>
          <span className="mono value-line">
            {node.namespace ? formatDateTime(node.first_seen) : formatDateTime(node.last_seen)}
          </span>
        </div>
      </section>

      <section className="inspector-section">
        <h3>Edges</h3>
        {edges.length === 0 ? (
          <p className="muted-copy">No observed edges.</p>
        ) : (
          <div className="edge-list">
            {edges.map((edge) => {
              const outgoing = edge.source === node.id
              const other = byID.get(outgoing ? edge.target : edge.source)
              return (
                <button
                  key={edge.id}
                  type="button"
                  className="edge-row"
                  onClick={() => onSelect({ kind: 'edge', id: edge.id })}
                >
                  <span className="edge-row-body">
                    <span className="edge-row-title">
                      {outgoing ? 'To' : 'From'} {other?.name ?? 'Unknown'}
                    </span>
                    <span className="edge-row-meta mono">
                      {edge.protocol} · {edge.port} · {formatCount(edge.events)} {edge.events === 1 ? 'event' : 'events'}
                    </span>
                  </span>
                </button>
              )
            })}
          </div>
        )}
      </section>

      <p className="inspector-footnote">
        L4 observation. Connections count TCP opens; events count observed flows.
      </p>
    </>
  )
}

function formatLatency(value: number): string {
  if (!value) return '—'
  return value >= 10 ? `${Math.round(value)} ms` : `${value.toFixed(1)} ms`
}

function latencySourceLabel(source: L7Stats['latency_source']): string {
  if (source === 'server') return 'server latency'
  if (source === 'mixed') return 'mixed latency'
  return 'client latency'
}

function L7Section({ stats }: { stats: L7Stats }) {
  const latencySource = stats.latency_source ?? 'client'
  const statuses: Array<[string, number]> = [
    ['2xx', stats.status['2xx']],
    ['3xx', stats.status['3xx']],
    ['4xx', stats.status['4xx']],
    ['5xx', stats.status['5xx']],
  ]
  return (
    <section className="inspector-section l7-section">
      <div className="section-heading">
        <h3>HTTP</h3>
        <span className="mono muted-copy">{formatCount(stats.requests)} requests · {latencySourceLabel(latencySource)}</span>
      </div>

      <div className="l7-summary">
        <div>
          <span>Requests</span>
          <strong>{formatCount(stats.requests)}</strong>
        </div>
        <div>
          <span>Responses</span>
          <strong>{formatCount(stats.responses)}</strong>
        </div>
        <div>
          <span>p50</span>
          <strong>{formatLatency(stats.latency_ms.p50)}</strong>
        </div>
        <div>
          <span>p95</span>
          <strong>{formatLatency(stats.latency_ms.p95)}</strong>
        </div>
      </div>

      <div className="l7-status" aria-label="HTTP status classes">
        {statuses.map(([label, value]) => (
          <div className="l7-status-item" key={label}>
            <span className={`l7-status-dot status-${label}`} aria-hidden="true" />
            <span className="l7-status-label">{label}</span>
            <span className="mono l7-status-count">{formatCount(value)}</span>
          </div>
        ))}
      </div>

      <div className="section-heading l7-window-heading">
        <h3>Last five minutes</h3>
        <span className="mono muted-copy">{formatCount(stats.window.requests)} requests</span>
      </div>
      <MiniActivity values={stats.window.activity} label="HTTP requests in the last five minutes" />

      <h3 className="l7-routes-heading">Routes</h3>
      <div className="route-list">
        {stats.routes.map((route) => (
          <div className="route-row" key={`${route.method} ${route.path}`}>
            <span className="route-method">{route.method}</span>
            <span className="route-path">{route.path}</span>
            <span className="mono route-count">{formatCount(route.requests)}</span>
          </div>
        ))}
      </div>

      <p className="l7-note">
        {latencySource === 'server' ? 'Server-observed' : latencySource === 'mixed' ? 'Mixed client/server' : 'Client-observed'} HTTP via eBPF. Latency is {latencySourceLabel(latencySource)}; headers, query strings, and bodies are discarded.
      </p>
    </section>
  )
}

function EdgeInspector({
  snapshot,
  edge,
  now,
}: {
  snapshot: Snapshot
  edge: TopologyEdge
  now: number
}) {
  const byID = new Map(snapshot.nodes.map((node) => [node.id, node]))
  const source = byID.get(edge.source)
  const target = byID.get(edge.target)

  return (
    <>
      <div className="inspector-heading">
        <span className="inspector-type">Edge</span>
        <span className="edge-protocol">{edge.protocol}</span>
      </div>
      <h2 id="inspector-title" className="edge-heading">
        {source?.name ?? 'Unknown'} <span aria-hidden="true">→</span> {target?.name ?? 'Unknown'}
      </h2>
      <p className="inspector-sub">Port {edge.port} · {formatCount(edge.events)} observed {edge.events === 1 ? 'flow' : 'flows'}</p>

      <dl className="stat-grid">
        <Stat label="Connections" value={formatCount(edge.connections)} />
        <Stat label="Events" value={formatCount(edge.events)} />
        <Stat label="Last seen" value={formatRelative(edge.last_seen, now)} />
      </dl>

      <section className="inspector-section">
        <div className="section-heading">
          <h3>Last five minutes</h3>
          <span className="mono muted-copy">{formatCount(edge.activity.reduce((sum, value) => sum + value, 0))} {edge.activity.reduce((sum, value) => sum + value, 0) === 1 ? 'event' : 'events'}</span>
        </div>
        <MiniActivity values={edge.activity} />
      </section>

      {edge.l7 ? <L7Section stats={edge.l7} /> : null}

      <section className="inspector-section">
        <h3>Observation</h3>
        <div className="detail-rows">
          <div className="detail-row">
            <span>First observed</span>
            <span className="mono">{formatDateTime(edge.first_seen)}</span>
          </div>
          <div className="detail-row">
            <span>Last observed</span>
            <span className="mono">{formatClock(edge.last_seen)}</span>
          </div>
          <div className="detail-row">
            <span>Direction</span>
            <span className="mono">request</span>
          </div>
        </div>
      </section>

      <p className="inspector-footnote">
        {edge.l7
          ? 'L4 and HTTP. Connections and events are L4 observations; the HTTP block names its latency source and excludes query strings, headers, and bodies.'
          : 'L4 only. No HTTP client or server span was observed for this edge; HTTPS and Postgres are not L7-visible.'}
      </p>
    </>
  )
}

export function Inspector({ snapshot, selected, now, onClose, onSelect }: Props) {
  const node = selected.kind === 'node'
    ? snapshot.nodes.find((candidate) => candidate.id === selected.id)
    : undefined
  const edge = selected.kind === 'edge'
    ? snapshot.edges.find((candidate) => candidate.id === selected.id)
    : undefined
  if (!node && !edge) return null

  return (
    <aside className="inspector" aria-labelledby="inspector-title" aria-live="polite">
      <button type="button" className="icon-button inspector-close" onClick={onClose} aria-label="Close inspector" title="Close inspector">
        <CloseIcon size={16} />
      </button>
      {node ? <NodeInspector snapshot={snapshot} node={node} now={now} onSelect={onSelect} /> : null}
      {edge ? <EdgeInspector snapshot={snapshot} edge={edge} now={now} /> : null}
    </aside>
  )
}
