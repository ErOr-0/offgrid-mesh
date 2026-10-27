export interface Peer {
  id: string
  name: string
  addr: string
  last_seen: string
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
}

export interface BundleInfo {
  id: string
  kind: 'message' | 'receipt'
  from: string
  from_name?: string
  to: string
  region?: string
  ref?: string
  created_at: string
  expires_at: string
  hop_count: number
  copies: number
  visited?: string[]
}

export interface Me {
  id: string
  name: string
  addr: string
}

export type NodeEvent =
  | { type: 'message'; message: ChatMessage }
  | { type: 'peers'; peers: Peer[] }
  | { type: 'peer_joined'; peer: Peer }
  | { type: 'bundles'; bundles: BundleInfo[] }
  | { type: 'delivery'; message: ChatMessage }

const params = new URLSearchParams(window.location.search)
export const nodeBase = params.get('node') ?? ''

export async function fetchMe(): Promise<Me> {
  const r = await fetch(`${nodeBase}/api/me`)
  if (!r.ok) throw new Error('node unreachable')
  return r.json()
}

export async function fetchPeers(): Promise<Peer[]> {
  const r = await fetch(`${nodeBase}/api/peers`)
  if (!r.ok) throw new Error('node unreachable')
  return r.json()
}

export async function fetchMessages(): Promise<ChatMessage[]> {
  const r = await fetch(`${nodeBase}/api/messages`)
  if (!r.ok) throw new Error('node unreachable')
  return r.json()
}

export async function fetchBundles(): Promise<BundleInfo[]> {
  const r = await fetch(`${nodeBase}/api/bundles`)
  if (!r.ok) throw new Error('node unreachable')
  return r.json()
}

export async function queueBundle(to: string, region: string, text: string): Promise<void> {
  const r = await fetch(`${nodeBase}/api/bundles`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ to, region, text, copies: 8, ttl_hours: 48 }),
  })
  if (!r.ok) throw new Error(await r.text())
}

export async function sendMessage(to: string, text: string): Promise<void> {
  const r = await fetch(`${nodeBase}/api/send`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ to, text }),
  })
  if (!r.ok) throw new Error(await r.text())
}

export function connectEvents(onEvent: (ev: NodeEvent) => void): WebSocket {
  const wsBase = nodeBase
    ? nodeBase.replace(/^http/, 'ws')
    : `${window.location.protocol === 'https:' ? 'wss' : 'ws'}://${window.location.host}`
  const ws = new WebSocket(`${wsBase}/ws`)
  ws.onmessage = (e) => {
    try {
      onEvent(JSON.parse(e.data) as NodeEvent)
    } catch {
      // ignore malformed events
    }
  }
  return ws
}
