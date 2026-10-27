export interface Peer {
  id: string
  name: string
  addr: string
  key?: string
  region?: string
  last_seen: string
}

export interface FileInfo {
  id: string
  from_id: string
  from_name: string
  name: string
  size: number
  created_at: string
  mine: boolean
}

export interface ChatMessage {
  id: string
  from_id: string
  from_name: string
  to_id: string
  text: string
  created_at: string
  mine: boolean
  status: string
  encrypted?: boolean
}

export interface BundleInfo {
  id: string
  kind: 'message' | 'receipt'
  from: string
  from_name?: string
  to: string
  dest_ip?: string
  region?: string
  ref?: string
  created_at: string
  expires_at: string
  hop_count: number
  copies: number
  encrypted?: boolean
  visited?: string[]
}

export interface Me {
  id: string
  name: string
  addr: string
  hub?: string
  key?: string
  fingerprint?: string
}

export interface RoutingEntry {
  device_id: string
  score: number
  encounters: number
  last_seen: string
}

export type NodeEvent =
  | { type: 'message'; message: ChatMessage }
  | { type: 'peers'; peers: Peer[] }
  | { type: 'peer_joined'; peer: Peer }
  | { type: 'bundles'; bundles: BundleInfo[] }
  | { type: 'delivery'; message: ChatMessage }
