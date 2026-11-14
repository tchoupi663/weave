export type Role = 'source' | 'hub' | 'service' | 'external' | 'data'

export interface TopologyNode {
  id: string
  workload: string
  name: string
  kind: string
  namespace: string
  role: Role
  pods: string[]
  ips: string[]
  connections: number
  events: number
  first_seen: string
  last_seen: string
}

export interface L7StatusCounts {
  '2xx': number
  '3xx': number
  '4xx': number
  '5xx': number
  other: number
}

export interface LatencyPercentiles {
  p50: number
  p95: number
}

export interface L7Window {
  requests: number
  responses: number
  status: L7StatusCounts
  latency_ms: LatencyPercentiles
  activity: number[]
}

export interface L7Route {
  method: string
  path: string
  requests: number
  responses: number
  status: L7StatusCounts
}

export interface L7Stats {
  protocol: string
  latency_source: 'client' | 'server' | 'mixed'
  requests: number
  responses: number
  status: L7StatusCounts
  latency_ms: LatencyPercentiles
  window: L7Window
  routes: L7Route[]
  first_seen: string
  last_seen: string
}

export interface TopologyEdge {
  id: string
  source: string
  target: string
  protocol: string
  port: number
  connections: number
  events: number
  first_seen: string
  last_seen: string
  activity: number[]
  l7?: L7Stats
}

export interface HubbleStatus {
  connected: boolean
  last_flow_at?: string
  last_error?: string
  observed_flows: number
  observed_l7_observations: number
}

export interface Snapshot {
  generated_at: string
  started_at: string
  namespace: string
  window_seconds: number
  nodes: TopologyNode[]
  edges: TopologyEdge[]
  activity: number[]
  totals: {
    nodes: number
    edges: number
    connections: number
    events: number
    l7_edges: number
    l7_requests: number
    l7_responses: number
  }
  hubble: HubbleStatus
}

export type Selection =
  | { kind: 'node'; id: string }
  | { kind: 'edge'; id: string }

export type StreamState = 'connecting' | 'open' | 'error' | 'paused'

export interface IncidentTimeRange {
  start: string
  end: string
}

export interface EvidenceThresholds {
  baseline_minutes: number
  error_multiplier: number
  error_delta: number
  latency_multiplier: number
  latency_delta_ms: number
}

export interface EvidenceMetric {
  requests: number
  responses: number
  status_2xx: number
  status_3xx: number
  status_4xx: number
  status_5xx: number
  status_other: number
  error_rate_4xx: number
  error_rate_5xx: number
  error_rate: number
  p50_ms: number
  p95_ms: number
  latency_source: string
}

export interface EvidenceEdge {
  edge_id: string
  source: string
  target: string
  protocol: string
  port: number
  baseline: EvidenceMetric
  incident: EvidenceMetric
  anomaly: boolean
  onset?: string
  reasons?: string[]
}

export interface EvidenceRoute {
  edge_id: string
  method: string
  path: string
  requests: number
  responses: number
  status_4xx: number
  status_5xx: number
}

export interface IncidentEvidence {
  algorithm_version: string
  window: IncidentTimeRange
  baseline: IncidentTimeRange
  thresholds: EvidenceThresholds
  edges: EvidenceEdge[]
  suspect_edges: string[]
  suspect_nodes: string[]
  routes: EvidenceRoute[]
}

export interface Incident {
  id: string
  created_at: string
  updated_at: string
  window_start: string
  window_end: string
  baseline_start: string
  baseline_end: string
  symptom: string
  scope?: string
  status: 'open' | 'closed'
  notes: string
  evidence: IncidentEvidence
}

export interface CreateIncidentInput {
  start: string
  end: string
  symptom: string
  scope?: string
}

export interface ChangeEvent {
  id: number
  occurred_at: string
  source: 'declared' | 'live' | 'observed' | string
  type: string
  workload: string
  kind: string
  details?: Record<string, unknown>
}

export type ViewMode = 'topology' | 'incidents'

export interface TimelineEdge {
  edge_id: string
  requests: number
  status_4xx: number
  status_5xx: number
  error_rate: number
  p95_ms: number
  anomaly: boolean
}

export interface TimelineFrame {
  at: string
  edges: TimelineEdge[]
}

export interface IncidentTimeline {
  // The incident's own window.
  window: IncidentTimeRange
  // The playback span: the incident baseline through the incident window end.
  // Optional for compatibility with an older API response.
  context?: IncidentTimeRange
  bucket_seconds: number
  suspect_edges: string[]
  frames: TimelineFrame[]
}
