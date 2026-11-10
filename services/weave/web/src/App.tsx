import { useCallback, useEffect, useMemo, useState } from 'react'
import { ActivityRibbon } from './components/ActivityRibbon'
import { Graph } from './components/Graph'
import { Inspector } from './components/Inspector'
import { Legend } from './components/Legend'
import { TopBar } from './components/TopBar'
import { layoutTopology } from './layout'
import type { Selection } from './types'
import { useHotEdges, useNow, useTopology } from './useTopology'

export default function App() {
  const [paused, setPaused] = useState(false)
  const [selected, setSelected] = useState<Selection | null>(null)
  const [hovered, setHovered] = useState<Selection | null>(null)
  const [resetting, setResetting] = useState(false)

  const { snapshot, streamState, error, loading, refresh, reset } = useTopology(paused)
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
      if (event.defaultPrevented) return
      const target = event.target as HTMLElement | null
      if (target?.closest('button, a, input, textarea, select, [role="button"], [tabindex]')) return
      if (event.key === ' ' && !event.repeat) {
        event.preventDefault()
        setPaused((value) => !value)
      }
    }
    window.addEventListener('keydown', onKeyDown)
    return () => window.removeEventListener('keydown', onKeyDown)
  }, [])

  const handleReset = useCallback(async () => {
    setResetting(true)
    try {
      setSelected(null)
      await reset()
    } finally {
      setResetting(false)
    }
  }, [reset])

  return (
    <div className="shell">
      <TopBar
        snapshot={snapshot}
        paused={paused}
        streamState={streamState}
        resetting={resetting}
        onTogglePause={() => setPaused((value) => !value)}
        onReset={() => void handleReset()}
      />

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

      <ActivityRibbon
        activity={snapshot?.activity ?? []}
        events={snapshot?.totals.events ?? 0}
        windowSeconds={snapshot?.window_seconds ?? 300}
        paused={paused}
      />
    </div>
  )
}
