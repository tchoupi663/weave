import { useCallback, useEffect, useRef, useState } from 'react'
import {
  createIncident,
  fetchChanges,
  fetchIncident,
  fetchIncidentTimeline,
  fetchIncidents,
  patchIncident,
  recomputeIncident,
} from './api'
import type { ChangeEvent, CreateIncidentInput, Incident, IncidentTimeline } from './types'

export function useIncidents(active: boolean) {
  const [incidents, setIncidents] = useState<Incident[]>([])
  const [incident, setIncident] = useState<Incident | null>(null)
  const [loading, setLoading] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const [changes, setChanges] = useState<ChangeEvent[]>([])
  const [changesLoading, setChangesLoading] = useState(false)
  const [timeline, setTimeline] = useState<IncidentTimeline | null>(null)
  const [timelineLoading, setTimelineLoading] = useState(false)
  const loaded = useRef(false)
  const selected = useRef<string | null>(null)

  const select = useCallback(async (id: string) => {
    selected.current = id
    setLoading(true)
    try {
      const next = await fetchIncident(id)
      setIncident(next)
      setError(null)
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'incident request failed')
    } finally {
      setLoading(false)
    }
  }, [])

  const refresh = useCallback(async () => {
    setLoading(true)
    try {
      const next = await fetchIncidents()
      setIncidents(next)
      setError(null)
      loaded.current = true
      if (next.length === 0) {
        selected.current = null
        setIncident(null)
      } else if (!selected.current || !next.some((candidate) => candidate.id === selected.current)) {
        await select(next[0].id)
      } else {
        const current = next.find((candidate) => candidate.id === selected.current)
        if (current) setIncident(current)
      }
    } catch (cause) {
      setError(cause instanceof Error ? cause.message : 'incident list failed')
    } finally {
      setLoading(false)
    }
  }, [select])

  useEffect(() => {
    if (active && !loaded.current) void refresh()
  }, [active, refresh])

  useEffect(() => {
    if (!incident) {
      setChanges([])
      return
    }
    let current = true
    setChangesLoading(true)
    void fetchChanges(incident.window_start, incident.window_end)
      .then((next) => {
        if (current) setChanges(next)
      })
      .catch(() => {
        if (current) setChanges([])
      })
      .finally(() => {
        if (current) setChangesLoading(false)
      })
    return () => {
      current = false
    }
  }, [incident])

  useEffect(() => {
    if (!incident) {
      setTimeline(null)
      return
    }
    let current = true
    setTimeline(null)
    setTimelineLoading(true)
    void fetchIncidentTimeline(incident.id)
      .then((next) => {
        if (current) setTimeline(next)
      })
      .catch(() => {
        if (current) setTimeline(null)
      })
      .finally(() => {
        if (current) setTimelineLoading(false)
      })
    return () => {
      current = false
    }
  }, [incident])

  const create = useCallback(async (input: CreateIncidentInput) => {
    const created = await createIncident(input)
    selected.current = created.id
    setIncident(created)
    setIncidents((current) => [created, ...current.filter((candidate) => candidate.id !== created.id)])
    setError(null)
    return created
  }, [])

  const update = useCallback(async (patch: { status?: string; notes?: string }) => {
    if (!incident) return
    const updated = await patchIncident(incident.id, patch)
    setIncident(updated)
    setIncidents((current) => current.map((candidate) => (candidate.id === updated.id ? updated : candidate)))
  }, [incident])

  const recompute = useCallback(async () => {
    if (!incident) return
    const updated = await recomputeIncident(incident.id)
    setIncident(updated)
    setIncidents((current) => current.map((candidate) => (candidate.id === updated.id ? updated : candidate)))
  }, [incident])

  return {
    incidents,
    incident,
    loading,
    error,
    changes,
    changesLoading,
    timeline,
    timelineLoading,
    select,
    refresh,
    create,
    update,
    recompute,
  }
}
