import { formatCount } from '../format'

interface Props {
  activity: number[]
  events: number
  windowSeconds: number
  paused: boolean
}

const WIDTH = 600
const HEIGHT = 26

function windowLabel(seconds: number): string {
  if (seconds >= 60 && seconds % 60 === 0) return `Last ${seconds / 60} minutes`
  return `Last ${seconds} seconds`
}

function activityTrace(values: number[]): { line: string; area: string; lastX: number; lastY: number } {
  const max = Math.max(1, ...values)
  const step = values.length > 1 ? WIDTH / (values.length - 1) : WIDTH
  const baseline = HEIGHT - 1
  const points = values.map((value, index) => ({
    x: index * step,
    y: baseline - (value / max) * (HEIGHT - 4),
  }))
  if (points.length === 0) {
    return { line: '', area: '', lastX: WIDTH, lastY: baseline }
  }

  // Sample and hold: each step is one five-second bucket, so a burst reads as
  // a burst rather than a decorative curve.
  let line = `M ${points[0].x} ${points[0].y}`
  for (let i = 1; i < points.length; i++) {
    line += ` H ${points[i].x} V ${points[i].y}`
  }
  const last = points[points.length - 1]
  const area = `${line} L ${last.x} ${baseline} L ${points[0].x} ${baseline} Z`
  return { line, area, lastX: last.x, lastY: last.y }
}

export function ActivityRibbon({ activity, events, windowSeconds, paused }: Props) {
  const values = activity.length > 0 ? activity : Array.from({ length: 60 }, () => 0)
  const { line, area, lastX, lastY } = activityTrace(values)

  return (
    <footer className={`ribbon${paused ? ' is-paused' : ''}`}>
      <div className="ribbon-copy">
        <span className="ribbon-title">Flow events</span>
        <span className="ribbon-sub">{windowLabel(windowSeconds)}</span>
      </div>
      <div className="ribbon-chart">
        <svg
          className="activity-spark"
          viewBox={`0 0 ${WIDTH} ${HEIGHT}`}
          preserveAspectRatio="none"
          role="img"
          aria-label={`${formatCount(events)} observed flow ${events === 1 ? 'event' : 'events'}, ${windowLabel(windowSeconds).toLowerCase()}`}
        >
          <path className="activity-area" d={area} />
          <path className="activity-line" d={line} vectorEffect="non-scaling-stroke" />
          <path
            className={`activity-head${values[values.length - 1] === 0 ? ' is-idle' : ''}`}
            d={`M ${lastX - 0.4} ${lastY} L ${lastX} ${lastY}`}
            vectorEffect="non-scaling-stroke"
          />
        </svg>
      </div>
      <div className="ribbon-total">
        <span className="ribbon-total-value">{formatCount(events)}</span>
        <span className="ribbon-total-label">{events === 1 ? 'event' : 'events'}</span>
      </div>
    </footer>
  )
}
