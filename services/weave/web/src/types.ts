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
