import { useCallback, useEffect, useRef, useState } from 'react'
import { fetchSnapshot, openTopologyStream, resetGraph } from './api'
import type { Snapshot, StreamState, TopologyEdge } from './types'

export function useTopology(paused: boolean) {
  const [snapshot, setSnapshot] = useState<Snapshot | null>(null)
  const [streamState, setStreamState] = useState<StreamState>('connecting')
  const [error, setError] = useState<string | null>(null)
  const [loading, setLoading] = useState(true)

  const load = useCallback(async () => {
    try {
      const next = await fetchSnapshot()
      setSnapshot(next)
      setError(null)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'topology request failed')
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    if (paused) {
      setStreamState('paused')
      return
    }

    void load()
    setStreamState('connecting')
    const close = openTopologyStream({
      onSnapshot: (next) => {
        setSnapshot(next)
        setError(null)
        setLoading(false)
      },
      onState: (state) => setStreamState(state),
    })
    return close
  }, [load, paused])

  const reset = useCallback(async () => {
    try {
      await resetGraph()
      await load()
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'reset request failed')
    }
  }, [load])

  return { snapshot, streamState, error, loading, refresh: load, reset }
}

export function useHotEdges(edges: TopologyEdge[] | undefined): Set<string> {
  const [hot, setHot] = useState<Set<string>>(new Set())
  const previous = useRef<Map<string, number> | null>(null)
  const lastPulse = useRef<Map<string, number>>(new Map())
  const timer = useRef<number | null>(null)

  useEffect(() => {
    if (!edges) return
    const counts = new Map(edges.map((edge) => [edge.id, edge.events]))
    const old = previous.current
    previous.current = counts
    if (!old) return

    // Two quiet shuttles on the busiest threads, refreshed often enough to
    // read as traffic rather than a decorative pulse.
    const now = Date.now()
    const next = new Set<string>()
    const changes = edges
      .map((edge) => ({ edge, delta: edge.events - (old.get(edge.id) ?? 0) }))
      .filter((change) => change.delta > 0)
      .sort((a, b) => b.delta - a.delta)
      .slice(0, 2)
    for (const { edge } of changes) {
      const last = lastPulse.current.get(edge.id) ?? 0
      if (now - last < 500) continue
      lastPulse.current.set(edge.id, now)
      next.add(edge.id)
    }
    if (next.size === 0) return

    setHot(next)
    if (timer.current !== null) window.clearTimeout(timer.current)
    timer.current = window.setTimeout(() => {
      setHot(new Set())
      timer.current = null
    }, 1000)
  }, [edges])

  useEffect(() => () => {
    if (timer.current !== null) window.clearTimeout(timer.current)
  }, [])

  return hot
}

export function useNow(intervalMs = 1000): number {
  const [now, setNow] = useState(() => Date.now())
  useEffect(() => {
    const id = window.setInterval(() => setNow(Date.now()), intervalMs)
    return () => window.clearInterval(id)
  }, [intervalMs])
  return now
}
