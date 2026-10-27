import type { NodeEvent } from './types'

const params = new URLSearchParams(window.location.search)

// Base URL of the local Go node this UI is attached to ('' = same origin via Vite proxy).
export const nodeBase = params.get('node') ?? ''

export async function getJSON<T>(path: string): Promise<T> {
  const r = await fetch(`${nodeBase}${path}`)
  if (!r.ok) throw new Error('node unreachable')
  return r.json() as Promise<T>
}

export async function postJSON(path: string, body: unknown): Promise<void> {
  const r = await fetch(`${nodeBase}${path}`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(body),
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
