import type { Snapshot, StreamState } from './types'

export async function fetchSnapshot(signal?: AbortSignal): Promise<Snapshot> {
  const response = await fetch('/api/topology', {
    signal,
    cache: 'no-store',
    headers: { Accept: 'application/json' },
  })
  if (!response.ok) {
    throw new Error(`topology request failed: ${response.status}`)
  }
  return (await response.json()) as Snapshot
}

export async function resetGraph(): Promise<void> {
  const response = await fetch('/api/reset', {
    method: 'POST',
    headers: { Accept: 'application/json' },
  })
  if (!response.ok) {
    throw new Error(`reset request failed: ${response.status}`)
  }
}

export interface StreamHandlers {
  onSnapshot: (snapshot: Snapshot) => void
  onState: (state: StreamState) => void
}

export function openTopologyStream({ onSnapshot, onState }: StreamHandlers): () => void {
  const source = new EventSource('/api/stream')

  source.onopen = () => onState('open')
  source.onerror = () => onState('error')
  source.addEventListener('topology', (event) => {
    try {
      onSnapshot(JSON.parse((event as MessageEvent<string>).data) as Snapshot)
      onState('open')
    } catch {
      onState('error')
    }
  })

  return () => source.close()
}

async function requestJSON<T>(url: string, init?: RequestInit): Promise<T> {
  const response = await fetch(url, {
    ...init,
    cache: 'no-store',
    headers: {
      Accept: 'application/json',
      ...(init?.headers ?? {}),
    },
  })
  if (!response.ok) {
    let message = `${response.status} ${response.statusText}`
    try {
      const body = (await response.json()) as { error?: string }
      if (body.error) message = body.error
    } catch {
      // Keep the status line when the body is not JSON.
    }
    throw new Error(message)
  }
  return (await response.json()) as T
}

export async function fetchIncidents(limit = 50): Promise<import('./types').Incident[]> {
  return requestJSON(`/api/incidents?limit=${limit}`)
}

export async function fetchIncident(id: string): Promise<import('./types').Incident> {
  return requestJSON(`/api/incidents/${encodeURIComponent(id)}`)
}

export async function createIncident(input: import('./types').CreateIncidentInput): Promise<import('./types').Incident> {
  return requestJSON('/api/incidents', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(input),
  })
}

export async function patchIncident(
  id: string,
  patch: { status?: string; notes?: string },
): Promise<import('./types').Incident> {
  return requestJSON(`/api/incidents/${encodeURIComponent(id)}`, {
    method: 'PATCH',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(patch),
  })
}

export async function recomputeIncident(id: string): Promise<import('./types').Incident> {
  return requestJSON(`/api/incidents/${encodeURIComponent(id)}/recompute`, { method: 'POST' })
}

export async function fetchChanges(from: string, to: string): Promise<import('./types').ChangeEvent[]> {
  const query = new URLSearchParams({ from, to, limit: '200' })
  return requestJSON(`/api/changes?${query.toString()}`)
}

export async function fetchIncidentTimeline(id: string): Promise<import('./types').IncidentTimeline> {
  return requestJSON(`/api/incidents/${encodeURIComponent(id)}/timeline`)
}
