import { useEffect, useMemo, useRef, useState, type FormEvent, type KeyboardEvent, type PointerEvent as ReactPointerEvent } from 'react'
import { formatClock, formatCount, formatDateTime } from '../format'
import type {
  ChangeEvent,
  CreateIncidentInput,
  EvidenceEdge,
  EvidenceRoute,
  Incident,
  IncidentTimeline,
  Snapshot,
  TimelineFrame,
} from '../types'

interface WorkspaceProps {
  snapshot: Snapshot | null
  incidents: Incident[]
  incident: Incident | null
  loading: boolean
  error: string | null
  changes: ChangeEvent[]
  changesLoading: boolean
  timeline: IncidentTimeline | null
  timelineLoading: boolean
  onSelect: (id: string) => void
  onCreate: (input: CreateIncidentInput) => Promise<unknown>
  onPatch: (patch: { status?: string; notes?: string }) => Promise<void>
  onRecompute: () => Promise<void>
  onViewTopology: () => void
  onRetry: () => void
}

function formatLatency(value: number): string {
  if (!value) return '—'
  return value >= 10 ? `${Math.round(value)} ms` : `${value.toFixed(1)} ms`
}

function formatRate(value: number): string {
  return `${(value * 100).toFixed(value >= 0.1 ? 0 : 1)}%`
}

function formatWindow(incident: Incident): string {
  return `${formatDateTime(incident.window_start)} → ${formatClock(incident.window_end)}`
}

function compactWindow(incident: Incident): string {
  return `${formatDateTime(incident.window_start)} – ${formatClock(incident.window_end)}`
}

function nodeName(snapshot: Snapshot | null, workload: string): string {
  const known = snapshot?.nodes.find((node) => node.workload === workload)
  if (known) return known.name
  return workload
    .split('-')
    .map((part) => part.charAt(0).toUpperCase() + part.slice(1))
    .join(' ')
}

function humanType(type: string): string {
  switch (type) {
    case 'state_changed':
      return 'state'
    case 'declared_added':
      return 'declared +'
    case 'declared_removed':
      return 'declared −'
    case 'observed_first_seen':
      return 'first seen'
    case 'live_missing':
      return 'live missing'
    default:
      return type.replaceAll('_', ' ')
  }
}

function eventDetail(event: ChangeEvent): string {
  const details = event.details ?? {}
  const from = typeof details.from === 'string' ? details.from : ''
  const to = typeof details.to === 'string' ? details.to : ''
  if (from || to) return `${from || '—'} → ${to || '—'}`
  return humanType(event.type)
}

function formatPreviewDate(value: string): string {
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return '—'
  return new Intl.DateTimeFormat('en-GB', {
    day: '2-digit',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
    hour12: false,
  }).format(date)
}

function localInput(date: Date): string {
  const pad = (value: number) => String(value).padStart(2, '0')
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`
}

function initialWindow(): { start: string; end: string } {
  const end = new Date()
  const start = new Date(end.getTime() - 5 * 60 * 1000)
  return { start: localInput(start), end: localInput(end) }
}

export function IncidentWorkspace({
  snapshot,
  incidents,
  incident,
  loading,
  error,
  changes,
  changesLoading,
  timeline,
  timelineLoading,
  onSelect,
  onCreate,
  onPatch,
  onRecompute,
  onViewTopology,
  onRetry,
}: WorkspaceProps) {
  const [composing, setComposing] = useState(false)

  const select = (id: string) => {
    setComposing(false)
    onSelect(id)
  }

  const newIncident = () => {
    setComposing(true)
  }

  return (
    <section className="incident-shell" aria-label="Incidents">
      <IncidentRail
        incidents={incidents}
        selectedID={incident?.id ?? null}
        loading={loading}
        error={error}
        onSelect={select}
        onNew={newIncident}
        onRetry={onRetry}
      />
      <div className="incident-stage">
        {composing ? (
          <IncidentComposer
            onCreate={async (input) => {
              await onCreate(input)
              setComposing(false)
            }}
            onCancel={() => setComposing(false)}
          />
        ) : incident ? (
          <IncidentDetail
            snapshot={snapshot}
            incident={incident}
            changes={changes}
            changesLoading={changesLoading}
            timeline={timeline}
            timelineLoading={timelineLoading}
            onPatch={onPatch}
            onRecompute={onRecompute}
            onViewTopology={onViewTopology}
          />
        ) : loading ? (
          <IncidentLoading />
        ) : error ? (
          <div className="incident-state">
            <h1>Incident history unavailable</h1>
            <p>{error}</p>
            <button type="button" className="button" onClick={onRetry}>Retry</button>
          </div>
        ) : (
          <IncidentEmpty onNew={newIncident} />
        )}
      </div>
    </section>
  )
}

function IncidentRail({
  incidents,
  selectedID,
  loading,
  error,
  onSelect,
  onNew,
  onRetry,
}: {
  incidents: Incident[]
  selectedID: string | null
  loading: boolean
  error: string | null
  onSelect: (id: string) => void
  onNew: () => void
  onRetry: () => void
}) {
  return (
    <aside className="incident-rail">
      <div className="incident-rail-head">
        <div>
          <h2>Incidents</h2>
          <span className="incident-rail-count">{loading && incidents.length === 0 ? 'loading' : `${incidents.length} saved`}</span>
        </div>
        <button type="button" className="button button-small button-primary" onClick={onNew}>New</button>
      </div>

      {loading && incidents.length === 0 ? (
        <div className="incident-rail-skeleton" aria-label="Loading incidents">
          <span /><span /><span />
        </div>
      ) : incidents.length === 0 ? (
        <div className="incident-rail-empty">
          <p>No investigations yet.</p>
          <button type="button" className="button button-small" onClick={onNew}>Open the first incident</button>
        </div>
      ) : (
        <div className="incident-list">
          {incidents.map((item) => (
            <button
              type="button"
              key={item.id}
              className={`incident-item${item.id === selectedID ? ' is-selected' : ''}`}
              onClick={() => onSelect(item.id)}
            >
              <span className={`incident-status-dot status-${item.status}`} aria-hidden="true" />
              <span className="incident-item-main">
                <span className="incident-item-title">{item.symptom || 'Untitled incident'}</span>
                <span className="incident-item-meta">{compactWindow(item)}</span>
              </span>
              <span className="incident-item-status">{item.status === 'open' ? 'Open' : 'Closed'}</span>
            </button>
          ))}
        </div>
      )}

      {error && incidents.length > 0 ? (
        <div className="incident-rail-error">
          <span>{error}</span>
          <button type="button" className="text-button" onClick={onRetry}>Retry</button>
        </div>
      ) : null}

      <p className="incident-rail-foot">History is read-only. Live traffic keeps recording.</p>
    </aside>
  )
}

function IncidentEmpty({ onNew }: { onNew: () => void }) {
  return (
    <div className="incident-state incident-empty">
      <span className="incident-empty-mark" aria-hidden="true" />
      <h1>Open a time window</h1>
      <p>Create an incident from a recorded window. Weave will rank the earliest connected anomalies and keep the full graph one action away.</p>
      <button type="button" className="button button-primary" onClick={onNew}>New incident</button>
    </div>
  )
}

function IncidentLoading() {
  return (
    <div className="incident-state" role="status">
      <span className="incident-empty-mark is-loading" aria-hidden="true" />
      <h1>Loading incident</h1>
      <p>Reading the evidence snapshot.</p>
    </div>
  )
}

function IncidentComposer({
  onCreate,
  onCancel,
}: {
  onCreate: (input: CreateIncidentInput) => Promise<void>
  onCancel: () => void
}) {
  const window = useMemo(initialWindow, [])
  const [start, setStart] = useState(window.start)
  const [end, setEnd] = useState(window.end)
  const [symptom, setSymptom] = useState('')
  const [scope, setScope] = useState('')
  const [error, setError] = useState<string | null>(null)
  const [busy, setBusy] = useState(false)
  const startDate = new Date(start)
  const endDate = new Date(end)
  const validWindow = !Number.isNaN(startDate.getTime()) && !Number.isNaN(endDate.getTime())
  const baselineStart = validWindow ? new Date(startDate.getTime() - 15 * 60 * 1000) : null
  const windowMinutes = validWindow ? Math.max(0, Math.round((endDate.getTime() - startDate.getTime()) / 60000)) : null

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault()
    setError(null)
    const startDate = new Date(start)
    const endDate = new Date(end)
    if (Number.isNaN(startDate.getTime()) || Number.isNaN(endDate.getTime())) {
      setError('Choose a valid start and end time.')
      return
    }
    if (endDate <= startDate) {
      setError('End must be after start.')
      return
    }
    setBusy(true)
    try {
      await onCreate({
        start: startDate.toISOString(),
        end: endDate.toISOString(),
        symptom: symptom.trim(),
        scope: scope.trim(),
      })
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'incident creation failed')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="incident-compose">
      <div className="incident-compose-head">
        <h1>Open an incident window</h1>
        <button type="button" className="button button-small" onClick={onCancel} disabled={busy}>Cancel</button>
      </div>
      <p className="incident-compose-copy">Use a recorded window. The 15 minutes before the start become the baseline; the suspect region is the earliest connected anomaly, not a root cause.</p>
      <div className="incident-compose-grid">
        <form onSubmit={(event) => void submit(event)}>
          <div className="incident-form-grid">
          <label className="field">
            <span>Start</span>
            <input type="datetime-local" value={start} onChange={(event) => setStart(event.target.value)} required />
          </label>
            <label className="field">
              <span>End</span>
              <input type="datetime-local" value={end} onChange={(event) => setEnd(event.target.value)} required />
            </label>
          </div>
          <label className="field">
            <span>Symptom</span>
            <input
              type="text"
              value={symptom}
              onChange={(event) => setSymptom(event.target.value)}
              placeholder="Payment returns 400s"
              maxLength={240}
            />
          </label>
          <label className="field">
            <span>Scope <span className="field-optional">optional</span></span>
            <input
              type="text"
              value={scope}
              onChange={(event) => setScope(event.target.value)}
              placeholder="gateway->catalog"
              maxLength={240}
            />
          </label>
          {error ? <p className="form-error" role="alert">{error}</p> : null}
          <div className="incident-form-actions">
            <button type="submit" className="button button-primary" disabled={busy}>
              {busy ? 'Building evidence…' : 'Open incident'}
            </button>
            <span className="field-hint">Symptom is context; evidence still searches the graph.</span>
          </div>
        </form>
        <aside className="compose-aside" aria-label="Evidence window preview">
          <div className="compose-fact">
            <span>Baseline starts</span>
            <strong>{baselineStart ? formatPreviewDate(baselineStart.toISOString()) : '—'}</strong>
          </div>
          <div className="compose-fact">
            <span>Window</span>
            <strong>{windowMinutes !== null ? `${windowMinutes} min` : '—'}</strong>
          </div>
          <div className="compose-fact">
            <span>Evidence rule</span>
            <strong>2× + 5 pp</strong>
            <p>The earliest connected anomaly becomes the suspect region. It is not a root-cause claim.</p>
          </div>
        </aside>
      </div>
    </div>
  )
}

function IncidentDetail({
  snapshot,
  incident,
  changes,
  changesLoading,
  timeline,
  timelineLoading,
  onPatch,
  onRecompute,
  onViewTopology,
}: {
  snapshot: Snapshot | null
  incident: Incident
  changes: ChangeEvent[]
  changesLoading: boolean
  timeline: IncidentTimeline | null
  timelineLoading: boolean
  onPatch: (patch: { status?: string; notes?: string }) => Promise<void>
  onRecompute: () => Promise<void>
  onViewTopology: () => void
}) {
  const evidence = incident.evidence
  const [selectedEdge, setSelectedEdge] = useState<string | null>(evidence.suspect_edges[0] ?? null)
  const [frameIndex, setFrameIndex] = useState(-1)
  const [busy, setBusy] = useState(false)
  const [actionError, setActionError] = useState<string | null>(null)

  useEffect(() => {
    setSelectedEdge(evidence.suspect_edges[0] ?? null)
  }, [incident.id, evidence.suspect_edges])

  useEffect(() => {
    if (!timeline || timeline.frames.length === 0) {
      setFrameIndex(-1)
      return
    }
    const onsets = evidence.edges
      .filter((edge) => edge.anomaly && edge.onset)
      .map((edge) => new Date(edge.onset ?? 0).getTime())
      .sort((left, right) => left - right)
    const target = onsets[0]
    if (target === undefined) {
      setFrameIndex(timeline.frames.length - 1)
      return
    }
    const index = timeline.frames.findIndex((frame) => new Date(frame.at).getTime() >= target)
    setFrameIndex(index >= 0 ? index : timeline.frames.length - 1)
  }, [incident.id, timeline, evidence.edges])

  const activeFrame = frameIndex >= 0 && timeline ? timeline.frames[frameIndex] ?? null : null

  const patch = async (next: { status?: string; notes?: string }) => {
    setBusy(true)
    setActionError(null)
    try {
      await onPatch(next)
    } catch (cause) {
      setActionError(cause instanceof Error ? cause.message : 'incident update failed')
    } finally {
      setBusy(false)
    }
  }

  const recompute = async () => {
    setBusy(true)
    setActionError(null)
    try {
      await onRecompute()
    } catch (cause) {
      setActionError(cause instanceof Error ? cause.message : 'recompute failed')
    } finally {
      setBusy(false)
    }
  }

  return (
    <div className="incident-detail">
      <header className="incident-head">
        <div className="incident-title-block">
          <div className="incident-title-meta">
            <span className={`status-pill status-${incident.status}`}>{incident.status === 'open' ? 'Open' : 'Closed'}</span>
            <span className="incident-window">{formatWindow(incident)}</span>
          </div>
          <h1>{incident.symptom || 'Untitled incident'}</h1>
          <p className="incident-sub">
            {evidence.suspect_edges.length} suspect {evidence.suspect_edges.length === 1 ? 'edge' : 'edges'} · baseline {evidence.thresholds.baseline_minutes} min · {evidence.algorithm_version}
          </p>
        </div>
        <div className="incident-actions">
          <button type="button" className="button button-small" onClick={() => void recompute()} disabled={busy}>Recompute</button>
          <button type="button" className="button button-small" onClick={() => void patch({ status: incident.status === 'open' ? 'closed' : 'open' })} disabled={busy}>
            {incident.status === 'open' ? 'Close incident' : 'Reopen'}
          </button>
          <button type="button" className="button button-small" onClick={onViewTopology}>Live topology</button>
        </div>
      </header>

      {actionError ? <p className="form-error incident-action-error" role="alert">{actionError}</p> : null}

      <div className="incident-grid">
        <div className="incident-main-column">
          <TimelineScrubber
            timeline={timeline}
            loading={timelineLoading}
            frameIndex={frameIndex}
            onFrameChange={setFrameIndex}
          />
          <SuspectCanvas
            snapshot={snapshot}
            evidence={evidence}
            frame={activeFrame}
            selectedEdge={selectedEdge}
            onSelectEdge={setSelectedEdge}
          />
          <OnsetTimeline evidence={evidence} snapshot={snapshot} activeAt={activeFrame?.at} />
          <ChangeStrip changes={changes} loading={changesLoading} until={activeFrame?.at} />
          <NotesEditor incident={incident} busy={busy} onSave={(notes) => void patch({ notes })} />
        </div>
        <EvidencePanel
          evidence={evidence}
          snapshot={snapshot}
          selectedEdge={selectedEdge}
          frame={activeFrame}
          onSelectEdge={setSelectedEdge}
        />
      </div>
    </div>
  )
}

function TimelineScrubber({
  timeline,
  loading,
  frameIndex,
  onFrameChange,
}: {
  timeline: IncidentTimeline | null
  loading: boolean
  frameIndex: number
  onFrameChange: (index: number) => void
}) {
  if (loading && !timeline) {
    return (
      <section className="incident-panel scrubber-panel" aria-busy="true">
        <div className="section-heading"><h3>Timeline</h3><span className="mono muted-copy">reading samples</span></div>
        <div className="scrubber-skeleton" />
      </section>
    )
  }
  if (!timeline || timeline.frames.length === 0) {
    return (
      <section className="incident-panel scrubber-panel">
        <div className="section-heading"><h3>Timeline</h3><span className="mono muted-copy">no samples</span></div>
        <p className="scrubber-empty">No 5-second samples in this window. Recompute after traffic has been recorded.</p>
      </section>
    )
  }
  const safeIndex = Math.max(0, Math.min(frameIndex, timeline.frames.length - 1))
  const frame = timeline.frames[safeIndex]
  const severity = timeline.frames.map((item) => {
    const requests = item.edges.reduce((total, edge) => total + edge.requests, 0)
    const errors = item.edges.reduce((total, edge) => total + edge.status_4xx + edge.status_5xx, 0)
    return requests > 0 ? errors / requests : 0
  })
  const max = Math.max(0.01, ...severity)
  const progress = timeline.frames.length <= 1 ? 0 : (safeIndex / (timeline.frames.length - 1)) * 100
  return (
    <section className="incident-panel scrubber-panel">
      <div className="section-heading">
        <h3>Timeline</h3>
        <span className="mono muted-copy">{formatDateTime(frame.at)} · {timeline.bucket_seconds}s samples</span>
      </div>
      <div className="scrubber-track">
        <div className="scrubber-heat" aria-hidden="true">
          {timeline.frames.map((item, index) => {
            const level = severity[index] === 0 ? 0 : Math.max(1, Math.min(5, Math.ceil((severity[index] / max) * 5)))
            return <span key={item.at} className={`heat-${level}`} />
          })}
        </div>
        <span className="scrubber-playhead" style={{ left: `${progress}%` }} aria-hidden="true" />
        <input
          type="range"
          min={0}
          max={timeline.frames.length - 1}
          value={safeIndex}
          step={1}
          aria-label="Incident timeline"
          aria-valuetext={`${formatClock(frame.at)}, ${timeline.bucket_seconds} second samples`}
          onChange={(event) => onFrameChange(Number(event.target.value))}
        />
      </div>
      <div className="scrubber-labels">
        <span>{formatClock(timeline.window.start)}</span>
        <strong>{formatClock(frame.at)}</strong>
        <span>{formatClock(timeline.window.end)}</span>
      </div>
    </section>
  )
}

interface Point {
  x: number
  y: number
  layer: number
}

function suspectLayout(edges: EvidenceEdge[]): Map<string, Point> {
  const names = new Set<string>()
  const incoming = new Map<string, number>()
  const outgoing = new Map<string, Set<string>>()
  for (const edge of edges) {
    names.add(edge.source)
    names.add(edge.target)
    incoming.set(edge.target, (incoming.get(edge.target) ?? 0) + 1)
    if (!outgoing.has(edge.source)) outgoing.set(edge.source, new Set())
    outgoing.get(edge.source)?.add(edge.target)
  }
  const ordered = [...names].sort()
  const roots = ordered.filter((name) => (incoming.get(name) ?? 0) === 0)
  if (roots.length === 0) roots.push(ordered[0])
  const layers = new Map<string, number>()
  const queue = roots.map((name) => ({ name, layer: 0 }))
  for (const root of roots) layers.set(root, 0)
  while (queue.length > 0) {
    const current = queue.shift()
    if (!current) break
    for (const next of outgoing.get(current.name) ?? []) {
      if (layers.has(next)) continue
      layers.set(next, current.layer + 1)
      queue.push({ name: next, layer: current.layer + 1 })
    }
  }
  const maxLayer = Math.max(0, ...layers.values())
  for (const name of ordered) {
    if (!layers.has(name)) layers.set(name, maxLayer + 1)
  }
  const result = new Map<string, Point>()
  const width = 760
  const height = 280
  const left = 92
  const right = width - 92
  const top = 68
  const bottom = height - 64
  const maxUsedLayer = Math.max(0, ...layers.values())
  const byLayer = new Map<number, string[]>()
  for (const name of ordered) {
    const layer = layers.get(name) ?? 0
    if (!byLayer.has(layer)) byLayer.set(layer, [])
    byLayer.get(layer)?.push(name)
  }
  for (const [layer, namesInLayer] of byLayer) {
    namesInLayer.sort()
    namesInLayer.forEach((name, index) => {
      const count = namesInLayer.length
      const y = count === 1 ? (top + bottom) / 2 : top + ((bottom - top) * index) / (count - 1)
      const x = maxUsedLayer === 0 ? width / 2 : left + ((right - left) * layer) / maxUsedLayer
      result.set(name, { x, y, layer })
    })
  }
  return result
}

function suspectMetric(edge: EvidenceEdge, frame: TimelineFrame | null) {
  if (!frame) {
    return {
      rate: edge.incident.error_rate,
      errors: edge.incident.status_4xx + edge.incident.status_5xx,
      requests: edge.incident.requests,
      p95: edge.incident.p95_ms,
      sampled: true,
    }
  }
  const current = frame.edges.find((candidate) => candidate.edge_id === edge.edge_id)
  if (!current) {
    return { rate: 0, errors: 0, requests: 0, p95: 0, sampled: false }
  }
  return {
    rate: current.error_rate,
    errors: current.status_4xx + current.status_5xx,
    requests: current.requests,
    p95: current.p95_ms,
    sampled: true,
  }
}

function formatErrorRate(value: number): string {
  if (value === 0) return '0%'
  return `${(value * 100).toFixed(value >= 0.1 ? 0 : 1)}%`
}

type GraphHover = { kind: 'node' | 'edge'; id: string } | null

function SuspectCanvas({
  snapshot,
  evidence,
  frame,
  selectedEdge,
  onSelectEdge,
}: {
  snapshot: Snapshot | null
  evidence: Incident['evidence']
  frame: TimelineFrame | null
  selectedEdge: string | null
  onSelectEdge: (id: string) => void
}) {
  const suspects = useMemo(
    () => evidence.edges.filter((edge) => evidence.suspect_edges.includes(edge.edge_id)),
    [evidence.edges, evidence.suspect_edges],
  )
  const layout = useMemo(() => suspectLayout(suspects), [suspects])
  const [positions, setPositions] = useState<Map<string, Point>>(layout)
  const [hover, setHover] = useState<GraphHover>(null)
  const svgRef = useRef<SVGSVGElement | null>(null)
  const drag = useRef<{ name: string; pointerId: number; offsetX: number; offsetY: number } | null>(null)
  const layoutKey = suspects.map((edge) => edge.edge_id).join('|')

  useEffect(() => {
    setPositions(layout)
    setHover(null)
  }, [layout, layoutKey])

  const focusEdges = useMemo(() => {
    if (!hover) return new Set<string>()
    if (hover.kind === 'edge') return new Set([hover.id])
    return new Set(suspects.filter((edge) => edge.source === hover.id || edge.target === hover.id).map((edge) => edge.edge_id))
  }, [hover, suspects])
  const focusNodes = useMemo(() => {
    if (!hover) return new Set<string>()
    if (hover.kind === 'node') return new Set([hover.id])
    const edge = suspects.find((candidate) => candidate.edge_id === hover.id)
    return edge ? new Set([edge.source, edge.target]) : new Set<string>()
  }, [hover, suspects])

  const toGraphPoint = (clientX: number, clientY: number) => {
    const svg = svgRef.current
    if (!svg) return { x: 0, y: 0 }
    const point = svg.createSVGPoint()
    point.x = clientX
    point.y = clientY
    const matrix = svg.getScreenCTM()
    if (!matrix) return { x: 0, y: 0 }
    return point.matrixTransform(matrix.inverse())
  }

  const startNodeDrag = (event: ReactPointerEvent<SVGGElement>, name: string) => {
    if (event.button !== 0) return
    event.preventDefault()
    event.stopPropagation()
    const point = toGraphPoint(event.clientX, event.clientY)
    const current = positions.get(name) ?? layout.get(name)
    if (!current) return
    event.currentTarget.setPointerCapture(event.pointerId)
    drag.current = {
      name,
      pointerId: event.pointerId,
      offsetX: current.x - point.x,
      offsetY: current.y - point.y,
    }
  }

  const moveNodeDrag = (event: ReactPointerEvent<SVGSVGElement>) => {
    const active = drag.current
    if (!active || active.pointerId !== event.pointerId) return
    const point = toGraphPoint(event.clientX, event.clientY)
    setPositions((current) => {
      const next = new Map(current)
      next.set(active.name, {
        x: Math.min(700, Math.max(60, point.x + active.offsetX)),
        y: Math.min(222, Math.max(36, point.y + active.offsetY)),
        layer: current.get(active.name)?.layer ?? 0,
      })
      return next
    })
  }

  const endNodeDrag = (event: ReactPointerEvent<SVGSVGElement>) => {
    if (!drag.current || drag.current.pointerId !== event.pointerId) return
    drag.current = null
  }

  if (suspects.length === 0) {
    return (
      <section className="incident-panel suspect-panel">
        <div className="section-heading">
          <h3>Suspect region</h3>
          <span className="mono muted-copy">no connected anomaly</span>
        </div>
        <div className="suspect-empty">
          <p>No connected anomaly crosses the threshold in this window.</p>
          <p className="muted-copy">Widen the window or check that the baseline has enough traffic.</p>
        </div>
      </section>
    )
  }

  return (
    <section className="incident-panel suspect-panel">
      <div className="section-heading">
        <h3>Suspect region</h3>
        <span className="mono muted-copy">{frame ? 'as sampled' : 'window aggregate'} · drag nodes, select a thread</span>
      </div>
      <div className="suspect-canvas">
        <svg
          ref={svgRef}
          viewBox="0 0 760 280"
          role="group"
          aria-label="Suspect region graph"
          className={hover ? 'has-focus' : ''}
          onPointerMove={moveNodeDrag}
          onPointerUp={endNodeDrag}
          onPointerCancel={endNodeDrag}
        >
          <defs>
            <marker id="suspect-arrow" viewBox="0 0 8 8" refX="7" refY="4" markerWidth="5" markerHeight="5" orient="auto">
              <path d="M0 0 L8 4 L0 8" className="suspect-arrow-head" />
            </marker>
          </defs>
          <g className="suspect-edges">
            {suspects.map((edge) => {
              const from = positions.get(edge.source) ?? layout.get(edge.source)
              const to = positions.get(edge.target) ?? layout.get(edge.target)
              if (!from || !to) return null
              const selected = selectedEdge === edge.edge_id
              const focused = focusEdges.has(edge.edge_id)
              const dimmed = focusEdges.size > 0 && !focused
              const metric = suspectMetric(edge, frame)
              const labelX = (from.x + to.x) / 2
              const labelY = (from.y + to.y) / 2 - 18
              return (
                <g
                  key={edge.edge_id}
                  className={`suspect-edge-group${selected ? ' is-selected' : ''}${focused ? ' is-focused' : ''}${dimmed ? ' is-dimmed' : ''}`}
                  role="button"
                  tabIndex={0}
                  aria-label={`${edge.source} to ${edge.target}, ${formatErrorRate(metric.rate)} errors, ${formatCount(metric.errors)} errors of ${formatCount(metric.requests)} requests`}
                  onPointerEnter={() => setHover({ kind: 'edge', id: edge.edge_id })}
                  onPointerLeave={() => setHover(null)}
                  onClick={() => onSelectEdge(edge.edge_id)}
                  onKeyDown={(event: KeyboardEvent<SVGGElement>) => {
                    if (event.key === 'Enter' || event.key === ' ') {
                      event.preventDefault()
                      onSelectEdge(edge.edge_id)
                    }
                  }}
                >
                  <line x1={from.x} y1={from.y} x2={to.x} y2={to.y} className="suspect-thread" markerEnd="url(#suspect-arrow)" />
                  <line x1={from.x} y1={from.y} x2={to.x} y2={to.y} className="suspect-thread-flow" pathLength={1} />
                  <rect x={labelX - 76} y={labelY - 19} width={152} height={38} className="suspect-label-bg" />
                  <text x={labelX} y={labelY - 3} textAnchor="middle" className="suspect-edge-label">
                    <tspan x={labelX}>{metric.sampled ? `${formatErrorRate(metric.rate)} errors` : 'no sample'}</tspan>
                    <tspan x={labelX} dy={14}>{metric.sampled ? `${formatCount(metric.errors)} of ${formatCount(metric.requests)} req` : 'at this moment'}</tspan>
                  </text>
                </g>
              )
            })}
          </g>
          <g className="suspect-nodes">
            {[...positions.entries()].map(([name, point]) => {
              const known = snapshot?.nodes.find((node) => node.workload === name)
              const label = nodeName(snapshot, name)
              const focused = focusNodes.has(name)
              const dimmed = focusNodes.size > 0 && !focused
              return (
                <g
                  key={name}
                  transform={`translate(${point.x} ${point.y})`}
                  className={`suspect-node role-${known?.role ?? 'service'}${focused ? ' is-focused' : ''}${dimmed ? ' is-dimmed' : ''}`}
                  onPointerDown={(event) => startNodeDrag(event, name)}
                  onPointerEnter={() => setHover({ kind: 'node', id: name })}
                  onPointerLeave={() => {
                    if (!drag.current) setHover(null)
                  }}
                >
                  <circle r={13} className="suspect-node-hit" />
                  <circle r={10} className="suspect-node-dot" />
                  <text y={27} textAnchor="middle" className="suspect-node-label">{label}</text>
                  <text y={42} textAnchor="middle" className="suspect-node-meta">{known?.kind ?? 'workload'}</text>
                </g>
              )
            })}
          </g>
        </svg>
      </div>
      <p className="incident-panel-note">Drag a node to arrange the region. Select a thread to read its evidence. This is a bounded suspect set, not a root cause.</p>
    </section>
  )
}

function OnsetTimeline({
  evidence,
  snapshot,
  activeAt,
}: {
  evidence: Incident['evidence']
  snapshot: Snapshot | null
  activeAt?: string
}) {
  const edges = evidence.edges
    .filter((edge) => edge.anomaly && edge.onset)
    .sort((left, right) => new Date(left.onset ?? 0).getTime() - new Date(right.onset ?? 0).getTime())
  if (edges.length === 0) return null
  const start = new Date(evidence.window.start).getTime()
  const end = new Date(evidence.window.end).getTime()
  const span = Math.max(1, end - start)
  const currentLeft = activeAt
    ? Math.min(100, Math.max(0, ((new Date(activeAt).getTime() - start) / span) * 100))
    : null
  return (
    <section className="incident-panel onset-panel">
      <div className="section-heading">
        <h3>Onset</h3>
        <span className="mono muted-copy">{formatDateTime(evidence.window.start)} → {formatClock(evidence.window.end)}</span>
      </div>
      <div className="onset-list">
        {edges.map((edge, index) => {
          const at = new Date(edge.onset ?? 0).getTime()
          const left = Math.min(100, Math.max(0, ((at - start) / span) * 100))
          return (
            <div className="onset-row" key={edge.edge_id}>
              <span className="onset-label">{nodeName(snapshot, edge.source)} → {nodeName(snapshot, edge.target)}</span>
              <div className="onset-track">
                <span className={`onset-mark${index === 0 ? ' is-first' : ''}`} style={{ left: `${left}%` }} />
                {currentLeft !== null ? <span className="onset-current" style={{ left: `${currentLeft}%` }} /> : null}
              </div>
              <span className="onset-time">{formatClock(edge.onset)}</span>
            </div>
          )
        })}
      </div>
    </section>
  )
}

function EvidencePanel({
  evidence,
  snapshot,
  selectedEdge,
  frame,
  onSelectEdge,
}: {
  evidence: Incident['evidence']
  snapshot: Snapshot | null
  selectedEdge: string | null
  frame: TimelineFrame | null
  onSelectEdge: (id: string) => void
}) {
  const suspectEdges = evidence.edges.filter((edge) => evidence.suspect_edges.includes(edge.edge_id))
  const edge = evidence.edges.find((candidate) => candidate.edge_id === selectedEdge) ?? suspectEdges[0]
  const routes = edge ? evidence.routes.filter((route) => route.edge_id === edge.edge_id) : []
  const current = edge ? suspectMetric(edge, frame) : null
  if (!edge || !current) {
    return (
      <aside className="evidence-panel">
        <div className="section-heading"><h3>Evidence</h3></div>
        <p className="muted-copy">No suspect edge to inspect.</p>
      </aside>
    )
  }
  return (
    <aside className="evidence-panel">
      <div className="section-heading">
        <h3>Evidence</h3>
        <span className="mono muted-copy">{edge.protocol} · {edge.port}</span>
      </div>
      <div className="evidence-edge-list">
        {suspectEdges.map((candidate) => (
          <button
            type="button"
            key={candidate.edge_id}
            className={`evidence-edge-button${candidate.edge_id === edge.edge_id ? ' is-selected' : ''}`}
            onClick={() => onSelectEdge(candidate.edge_id)}
          >
            <span>{nodeName(snapshot, candidate.source)} → {nodeName(snapshot, candidate.target)}</span>
            <span className="mono">{formatErrorRate(suspectMetric(candidate, frame).rate)} err · {formatCount(suspectMetric(candidate, frame).errors)}</span>
          </button>
        ))}
      </div>

      <div className="evidence-title">
        <span className="mono">{edge.source} → {edge.target}</span>
        <span className="evidence-onset">
          {frame ? `sample ${formatClock(frame.at)}` : 'window total'}
          {edge.onset ? ` · onset ${formatClock(edge.onset)}` : ''}
        </span>
      </div>
      <div className="evidence-reasons" aria-label="Anomaly reasons">
        {(edge.reasons ?? []).map((reason) => <span key={reason} className="reason-chip">{reason}</span>)}
      </div>

      <div className="compare-grid">
        <div>
          <span className="compare-label">Baseline</span>
          <strong>{formatCount(edge.baseline.requests)} <small>req</small></strong>
          <span className="compare-line">{formatRate(edge.baseline.error_rate)} errors · {formatLatency(edge.baseline.p95_ms)} p95</span>
        </div>
        <div>
          <span className="compare-label">{frame ? `At ${formatClock(frame.at)}` : 'Incident'}</span>
          <strong>{current.sampled ? formatCount(current.requests) : '—'} <small>req</small></strong>
          <span className="compare-line">
            {current.sampled ? `${formatRate(current.rate)} errors · ${formatLatency(current.p95)} p95` : 'no sample in this bucket'}
          </span>
        </div>
      </div>

      <div className="evidence-status-grid">
        <div><span>Errors</span><strong>{formatCount(current.errors)}</strong></div>
        <div><span>Requests</span><strong>{formatCount(current.requests)}</strong></div>
        <div><span>p95</span><strong>{formatLatency(current.p95)}</strong></div>
      </div>

      <div className="evidence-routes">
        <div className="section-heading"><h3>Routes</h3><span className="mono muted-copy">window top 5</span></div>
        {routes.length === 0 ? (
          <p className="muted-copy">No route detail for this edge.</p>
        ) : (
          <div className="route-list">
            {routes.map((route: EvidenceRoute) => (
              <div className="route-row" key={`${route.edge_id}-${route.method}-${route.path}`}>
                <span className="route-method">{route.method}</span>
                <span className="route-path">{route.path}</span>
                <span className="mono route-count">{formatCount(route.requests)}</span>
              </div>
            ))}
          </div>
        )}
      </div>

      <p className="incident-panel-note">
        Anomaly threshold: {evidence.thresholds.error_multiplier}× baseline and +{Math.round(evidence.thresholds.error_delta * 100)} pp; latency also requires +{evidence.thresholds.latency_delta_ms} ms.
      </p>
    </aside>
  )
}

function ChangeStrip({ changes, loading, until }: { changes: ChangeEvent[]; loading: boolean; until?: string }) {
  const visible = until
    ? changes.filter((event) => new Date(event.occurred_at).getTime() <= new Date(until).getTime())
    : changes
  return (
    <section className="change-strip" aria-label="Lifecycle changes">
      <div className="change-strip-head">
        <h3>Changes</h3>
        <span className="mono muted-copy">{loading ? 'reading…' : `${visible.length} up to now`}</span>
      </div>
      {visible.length === 0 ? (
        <p className="change-empty">{loading ? 'Loading lifecycle changes…' : 'No lifecycle changes in this window.'}</p>
      ) : (
        <div className="change-list">
          {visible.slice(0, 6).map((event) => (
            <div className="change-row" key={event.id}>
              <span className="change-time">{formatClock(event.occurred_at)}</span>
              <span className="change-workload">{event.workload}</span>
              <span className="change-detail">{eventDetail(event)}</span>
              <span className="change-source">{event.source}</span>
            </div>
          ))}
        </div>
      )}
    </section>
  )
}

function NotesEditor({ incident, busy, onSave }: { incident: Incident; busy: boolean; onSave: (notes: string) => void }) {
  const [notes, setNotes] = useState(incident.notes)
  const [dirty, setDirty] = useState(false)
  useEffect(() => {
    setNotes(incident.notes)
    setDirty(false)
  }, [incident.id, incident.notes])

  return (
    <section className="incident-panel notes-panel">
      <div className="section-heading">
        <h3>Notes</h3>
        <span className="mono muted-copy">{incident.status === 'open' ? 'open' : 'closed'}</span>
      </div>
      <textarea
        aria-label="Incident notes"
        value={notes}
        onChange={(event) => {
          setNotes(event.target.value)
          setDirty(true)
        }}
        placeholder="Record what you checked, what you ruled out, and what to hand off."
        rows={3}
        maxLength={4000}
      />
      <div className="notes-actions">
        <button type="button" className="button button-small" disabled={!dirty || busy} onClick={() => onSave(notes)}>
          {busy ? 'Saving…' : 'Save note'}
        </button>
        <span className="field-hint">Notes stay with this incident and its evidence.</span>
      </div>
    </section>
  )
}
