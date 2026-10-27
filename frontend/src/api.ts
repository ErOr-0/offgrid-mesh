export interface Peer {
  id: string
  name: string
  addr: string
  key?: string
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

export async function fetchFiles(): Promise<FileInfo[]> {
  const r = await fetch(`${nodeBase}/api/files`)
  if (!r.ok) throw new Error('node unreachable')
  return r.json()
}

export function fileDownloadURL(id: string): string {
  return `${nodeBase}/api/files/${id}/download`
}

export async function sendFile(to: string, file: File): Promise<void> {
  const buf = new Uint8Array(await file.arrayBuffer())
  let binary = ''
  buf.forEach((b) => (binary += String.fromCharCode(b)))
  const r = await fetch(`${nodeBase}/api/files/send`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ to, name: file.name, data: btoa(binary) }),
  })
  if (!r.ok) throw new Error(await r.text())
}

export async function allowDevice(id: string): Promise<void> {
  const r = await fetch(`${nodeBase}/api/allow`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ id }),
  })
  if (!r.ok) throw new Error(await r.text())
}

export interface RoutingEntry {
  device_id: string
  score: number
  encounters: number
  last_seen: string
}

export async function fetchRouting(): Promise<RoutingEntry[]> {
  const r = await fetch(`${nodeBase}/api/routing`)
  if (!r.ok) throw new Error('node unreachable')
  return r.json()
}

export function pairQRURL(): string {
  return `${nodeBase}/api/pair-qr`
}

export async function fetchPairCode(): Promise<string> {
  const r = await fetch(`${nodeBase}/api/pair-code`)
  if (!r.ok) throw new Error('node unreachable')
  const data = (await r.json()) as { code: string }
  return data.code
}

export async function pairWithCode(code: string): Promise<void> {
  const r = await fetch(`${nodeBase}/api/pair`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ code }),
  })
  if (!r.ok) throw new Error(await r.text())
}

export function sendFileWithProgress(
  to: string,
  file: File,
  onProgress: (pct: number) => void,
): Promise<void> {
  return file.arrayBuffer().then(
    (buf) =>
      new Promise<void>((resolve, reject) => {
        const bytes = new Uint8Array(buf)
        let binary = ''
        bytes.forEach((b) => (binary += String.fromCharCode(b)))
        const xhr = new XMLHttpRequest()
        xhr.open('POST', `${nodeBase}/api/files/send`)
        xhr.setRequestHeader('Content-Type', 'application/json')
        xhr.upload.onprogress = (e) => {
          if (e.lengthComputable) onProgress(Math.round((e.loaded / e.total) * 100))
        }
        xhr.onload = () =>
          xhr.status >= 200 && xhr.status < 300 ? resolve() : reject(new Error(xhr.responseText || 'send failed'))
        xhr.onerror = () => reject(new Error('send failed'))
        xhr.send(JSON.stringify({ to, name: file.name, data: btoa(binary) }))
      }),
  )
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
