import { useCallback, useEffect, useMemo, useState } from 'react'
import { FlowPanel } from './components/FlowPanel'
import { Graph } from './components/Graph'
import { IncidentWorkspace } from './components/Incidents'
import { Inspector } from './components/Inspector'
import { Legend } from './components/Legend'
import { TopBar } from './components/TopBar'
import { layoutTopology } from './layout'
import type { Selection, ViewMode } from './types'
import { useHotEdges, useNow, useTopology } from './useTopology'
import { useIncidents } from './useIncidents'

export default function App() {
  const [mode, setMode] = useState<ViewMode>(() =>
    new URLSearchParams(window.location.search).get('view') === 'incidents' ? 'incidents' : 'topology',
  )
  const [paused, setPaused] = useState(false)
  const [selected, setSelected] = useState<Selection | null>(null)
  const [hovered, setHovered] = useState<Selection | null>(null)
  const [resetting, setResetting] = useState(false)

  const { snapshot, streamState, error, loading, refresh, reset } = useTopology(paused)
  const incidents = useIncidents(mode === 'incidents')
  const now = useNow()
  const hotEdges = useHotEdges(snapshot?.edges)
  const layout = useMemo(
    () => layoutTopology(snapshot?.nodes ?? [], snapshot?.edges ?? []),
    [snapshot?.edges, snapshot?.nodes],
  )

  useEffect(() => {
    if (!selected || !snapshot) return
    const exists = selected.kind === 'node'
      ? snapshot.nodes.some((node) => node.id === selected.id)
      : snapshot.edges.some((edge) => edge.id === selected.id)
    if (!exists) setSelected(null)
  }, [selected, snapshot])

  useEffect(() => {
    const onKeyDown = (event: KeyboardEvent) => {
      if (event.key === 'Escape') {
        setSelected(null)
        return
      }
      if (mode !== 'topology' || event.defaultPrevented) return
      const target = event.target as HTMLElement | null
      if (target?.closest('button, a, input, textarea, select, [role="button"], [tabindex]')) return
      if (event.key === ' ' && !event.repeat) {
        event.preventDefault()
        setPaused((value) => !value)
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [mode])

  const handleReset = useCallback(async () => {
    setResetting(true)
    try {
      setSelected(null)
      await reset()
    } finally {
      setResetting(false)
    }
  }, [reset])

  const changeMode = useCallback((next: ViewMode) => {
    setMode(next)
    const url = new URL(window.location.href)
    if (next === 'incidents') {
      setSelected(null)
      url.searchParams.set('view', 'incidents')
    } else {
      url.searchParams.delete('view')
    }
    window.history.replaceState(null, '', url)
  }, [])

  return (
    <div className={`shell${mode === 'incidents' ? ' is-incidents' : ''}`}>
      <TopBar
        snapshot={snapshot}
        paused={paused}
        streamState={streamState}
        resetting={resetting}
        mode={mode}
        incidentCount={incidents.incidents.length}
        onModeChange={changeMode}
        onTogglePause={() => setPaused((value) => !value)}
        onReset={() => void handleReset()}
      />

      {mode === 'topology' ? (
        <>
          <main className={`stage${selected ? ' has-inspector' : ''}`}>
            <Graph
              snapshot={snapshot}
              layout={layout}
              paused={paused}
              streamState={streamState}
              loading={loading}
              error={error}
              selected={selected}
              hovered={hovered}
              hotEdges={hotEdges}
              onSelect={setSelected}
              onHover={setHovered}
              onRetry={() => void refresh()}
            />

            <FlowPanel
              activity={snapshot?.activity ?? []}
              events={snapshot?.totals.events ?? 0}
              windowSeconds={snapshot?.window_seconds ?? 300}
              paused={paused}
            />

            {snapshot ? <Legend snapshot={snapshot} /> : null}

            {snapshot && selected ? (
              <Inspector
                snapshot={snapshot}
                selected={selected}
                now={now}
                onClose={() => setSelected(null)}
                onSelect={setSelected}
              />
            ) : null}
          </main>
        </>
      ) : (
        <IncidentWorkspace
          snapshot={snapshot}
          incidents={incidents.incidents}
          incident={incidents.incident}
          loading={incidents.loading}
          error={incidents.error}
          changes={incidents.changes}
          changesLoading={incidents.changesLoading}
          timeline={incidents.timeline}
          timelineLoading={incidents.timelineLoading}
          onSelect={(id) => void incidents.select(id)}
          onCreate={incidents.create}
          onPatch={incidents.update}
          onRecompute={incidents.recompute}
          onViewTopology={() => changeMode('topology')}
          onRetry={() => void incidents.refresh()}
        />
      )}
    </div>
  )
}
