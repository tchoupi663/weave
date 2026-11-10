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
