const countFormatter = new Intl.NumberFormat('en-US')

export function formatCount(value: number): string {
  return countFormatter.format(value)
}

export function formatCompact(value: number): string {
  if (value >= 1_000_000) return `${(value / 1_000_000).toFixed(1).replace('.0', '')}m`
  if (value >= 1000) return `${(value / 1000).toFixed(1).replace('.0', '')}k`
  return String(value)
}

export function formatRelative(iso: string | undefined, now: number): string {
  if (!iso) return 'not yet'
  const then = new Date(iso).getTime()
  if (Number.isNaN(then)) return 'unknown'
  const seconds = Math.max(0, Math.round((now - then) / 1000))
  if (seconds < 5) return 'just now'
  if (seconds < 60) return `${seconds}s ago`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  return `${hours}h ago`
}

export function formatClock(iso: string | undefined): string {
  if (!iso) return '—'
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return '—'
  return new Intl.DateTimeFormat('en-GB', {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  }).format(date)
}

export function formatDateTime(iso: string | undefined): string {
  if (!iso) return '—'
  const date = new Date(iso)
  if (Number.isNaN(date.getTime())) return '—'
  return new Intl.DateTimeFormat('en-GB', {
    day: '2-digit',
    month: 'short',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
    hour12: false,
  }).format(date)
}

export function roleLabel(role: string): string {
  switch (role) {
    case 'source':
      return 'Source'
    case 'hub':
      return 'Hub'
    case 'external':
      return 'External'
    case 'data':
      return 'Data'
    default:
      return 'Service'
  }
}

export function shortKind(kind: string): string {
  return kind === 'External Client' ? 'External client' : kind
}
