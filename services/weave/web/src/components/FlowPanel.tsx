import { useMemo, useState } from 'react'
import { formatCount } from '../format'

interface Props {
  activity: number[]
  events: number
  windowSeconds: number
  paused: boolean
}

function windowLabel(seconds: number): string {
  if (seconds >= 60 && seconds % 60 === 0) return `last ${seconds / 60} min`
  return `last ${seconds} sec`
}

function heatLevel(value: number, max: number): number {
  if (value <= 0) return 0
  return Math.max(1, Math.min(5, Math.ceil((value / Math.max(1, max)) * 5)))
}

export function FlowPanel({ activity, events, windowSeconds, paused }: Props) {
  const [expanded, setExpanded] = useState(false)
  const values = useMemo(() => (activity.length > 0 ? activity : Array.from({ length: 60 }, () => 0)), [activity])
  const max = Math.max(1, ...values)
  const windowEvents = values.reduce((total, value) => total + value, 0)
  const peak = Math.max(0, ...values)
  const average = windowEvents / values.length
  const active = values.filter((value) => value > 0).length

  return (
    <section className={`flow-panel${expanded ? ' is-expanded' : ''}`} aria-label="Flow events">
      <button
        type="button"
        className="flow-toggle"
        aria-expanded={expanded}
        aria-controls="flow-detail"
        onClick={() => setExpanded((value) => !value)}
      >
        <span className="flow-toggle-head">
          <span className="flow-title">Flow events</span>
          <span className="flow-total">{formatCount(events)}</span>
          <span className="flow-expand" aria-hidden="true">{expanded ? '−' : '+'}</span>
        </span>
        <span className="flow-toggle-sub">
          {paused ? 'paused' : windowLabel(windowSeconds)}
          <span aria-hidden="true"> · </span>
          {formatCount(windowEvents)} in window
        </span>
        <span className="flow-mini" aria-hidden="true">
          {values.map((value, index) => (
            <span
              key={index}
              className={`flow-cell heat-${heatLevel(value, max)}`}
              style={{ height: `${Math.max(8, (value / max) * 100)}%` }}
            />
          ))}
        </span>
      </button>

      {expanded ? (
        <div className="flow-detail" id="flow-detail">
          <div className="flow-detail-grid">
            <div>
              <span>Window</span>
              <strong>{formatCount(windowEvents)}</strong>
              <small>events {windowLabel(windowSeconds)}</small>
            </div>
            <div>
              <span>Peak</span>
              <strong>{formatCount(peak)}</strong>
              <small>per {windowSeconds >= 60 && windowSeconds % 60 === 0 ? 'bucket' : '5 s'}</small>
            </div>
            <div>
              <span>Average</span>
              <strong>{average.toFixed(1)}</strong>
              <small>per bucket</small>
            </div>
            <div>
              <span>Active</span>
              <strong>{active}</strong>
              <small>of {values.length} buckets</small>
            </div>
          </div>
          <div className="flow-heat-legend">
            <span>Low</span>
            <span className="flow-legend-cells" aria-hidden="true">
              {[1, 2, 3, 4, 5].map((level) => <i key={level} className={`heat-${level}`} />)}
            </span>
            <span>High</span>
          </div>
          <p className="flow-note">Each cell is one five-second bucket. The heat ramp is relative to the busiest bucket in this window.</p>
        </div>
      ) : null}
    </section>
  )
}
