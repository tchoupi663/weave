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
}

export interface HubbleStatus {
  connected: boolean
  last_flow_at?: string
  last_error?: string
  observed_flows: number
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
  }
  hubble: HubbleStatus
}

export type Selection =
  | { kind: 'node'; id: string }
  | { kind: 'edge'; id: string }

export type StreamState = 'connecting' | 'open' | 'error' | 'paused'
