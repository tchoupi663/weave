import type { Role, Snapshot } from '../types'

const ORDER: Role[] = ['source', 'hub', 'service', 'external', 'data']

const LABELS: Record<Role, string> = {
  source: 'Source',
  hub: 'Hub',
  service: 'Service',
  external: 'External',
  data: 'Data',
}

export function Legend({ snapshot }: { snapshot: Snapshot }) {
  const present = ORDER.filter((role) => snapshot.nodes.some((node) => node.role === role))
  if (present.length === 0) return null

  return (
    <div className="legend" aria-label="Node roles">
      {present.map((role) => (
        <span key={role} className="legend-item">
          <span className={`legend-dot role-${role}`} aria-hidden="true" />
          {LABELS[role]}
        </span>
      ))}
      <span className="legend-note">L4 observations</span>
    </div>
  )
}
