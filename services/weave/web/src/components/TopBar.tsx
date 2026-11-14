import { formatCount } from '../format'
import type { Snapshot, StreamState, ViewMode } from '../types'
import { ClearIcon, PauseIcon, PlayIcon, WeaveMark } from '../icons'

interface Props {
  snapshot: Snapshot | null
  paused: boolean
  streamState: StreamState
  resetting: boolean
  mode: ViewMode
  incidentCount: number
  onModeChange: (mode: ViewMode) => void
  onTogglePause: () => void
  onReset: () => void
}

type Tone = 'live' | 'waiting' | 'offline' | 'paused'

function statusFor(
  snapshot: Snapshot | null,
  paused: boolean,
  stream: StreamState,
  mode: ViewMode,
): { label: string; tone: Tone; detail: string } {
  if (mode === 'incidents') {
    return {
      label: 'History',
      tone: 'paused',
      detail: 'Incident windows are read-only; the live topology keeps recording in the background',
    }
  }
  if (paused) {
    return { label: 'Paused', tone: 'paused', detail: 'Live updates are paused' }
  }
  if (stream === 'error') {
    return { label: 'Reconnecting', tone: 'waiting', detail: 'The event stream is reconnecting' }
  }
  if (!snapshot) {
    return { label: 'Connecting', tone: 'waiting', detail: 'Waiting for the first topology snapshot' }
  }
  if (!snapshot.hubble.connected) {
    return {
      label: 'Relay offline',
      tone: 'offline',
      detail: snapshot.hubble.last_error || 'Hubble Relay is not connected',
    }
  }
  return { label: 'Live', tone: 'live', detail: 'Following Hubble Relay' }
}

export function TopBar({
  snapshot,
  paused,
  streamState,
  resetting,
  mode,
  incidentCount,
  onModeChange,
  onTogglePause,
  onReset,
}: Props) {
  const status = statusFor(snapshot, paused, streamState, mode)
  const counts = snapshot?.totals

  return (
    <header className="topbar">
      <div className="brand">
        <WeaveMark size={22} />
        <span className="wordmark">Weave</span>
        <nav className="view-nav" aria-label="Workspace">
          <button
            type="button"
            className={mode === 'topology' ? 'is-active' : ''}
            aria-current={mode === 'topology' ? 'page' : undefined}
            onClick={() => onModeChange('topology')}
          >
            Topology
          </button>
          <button
            type="button"
            className={mode === 'incidents' ? 'is-active' : ''}
            aria-current={mode === 'incidents' ? 'page' : undefined}
            onClick={() => onModeChange('incidents')}
          >
            Incidents
          </button>
        </nav>
      </div>

      <div className="topbar-center">
        <span className="namespace">{snapshot?.namespace ?? 'weave'}</span>
        <span className="counts">
          {mode === 'incidents'
            ? `${formatCount(incidentCount)} ${incidentCount === 1 ? 'incident' : 'incidents'}`
            : counts
              ? `${formatCount(counts.nodes)} ${counts.nodes === 1 ? 'node' : 'nodes'} · ${formatCount(counts.edges)} ${counts.edges === 1 ? 'edge' : 'edges'}`
              : 'No snapshot'}
        </span>
      </div>

      <div className="topbar-right">
        <div className={`status status-${status.tone}`} title={status.detail} aria-live="polite">
          <span className="status-dot" aria-hidden="true" />
          <span>{status.label}</span>
        </div>
        {mode === 'topology' ? (
          <div className="topbar-actions">
            <button
              type="button"
              className="icon-button"
              onClick={onTogglePause}
              aria-label={paused ? 'Resume live updates' : 'Pause live updates'}
              title={paused ? 'Resume live updates' : 'Pause live updates'}
              aria-pressed={paused}
            >
              {paused ? <PlayIcon size={16} /> : <PauseIcon size={16} />}
            </button>
            <button
              type="button"
              className="icon-button"
              onClick={onReset}
              aria-label="Clear observed graph"
              title="Clear observed graph"
              disabled={resetting}
            >
              <ClearIcon size={16} />
            </button>
          </div>
        ) : null}
      </div>
    </header>
  )
}
