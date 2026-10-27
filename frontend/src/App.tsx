import { useEffect, useRef, useState } from 'react'
import {
  connectEvents,
  fetchMe,
  fetchMessages,
  fetchPeers,
  sendMessage,
  type ChatMessage,
  type Me,
  type Peer,
} from './api'
import './App.css'

function App() {
  const [me, setMe] = useState<Me | null>(null)
  const [peers, setPeers] = useState<Peer[]>([])
  const [selected, setSelected] = useState<string>('')
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [draft, setDraft] = useState('')
  const [status, setStatus] = useState('Connecting to local node...')
  const [error, setError] = useState('')
  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    let ws: WebSocket | undefined
    let cancelled = false
    ;(async () => {
      try {
        const [meData, peerData, msgData] = await Promise.all([fetchMe(), fetchPeers(), fetchMessages()])
        if (cancelled) return
        setMe(meData)
        setPeers(peerData)
        setMessages(msgData)
        setStatus('Connected')
        ws = connectEvents((ev) => {
          if (ev.type === 'message') {
            setMessages((prev) => (prev.some((m) => m.id === ev.message.id) ? prev : [...prev, ev.message]))
          } else if (ev.type === 'peers') {
            setPeers(ev.peers)
          } else if (ev.type === 'peer_joined') {
            setPeers((prev) => (prev.some((p) => p.id === ev.peer.id) ? prev : [...prev, ev.peer]))
          }
        })
        ws.onclose = () => setStatus('Disconnected from node')
      } catch {
        setStatus('Node unreachable — start the Go node first')
      }
    })()
    return () => {
      cancelled = true
      ws?.close()
    }
  }, [])

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [messages, selected])

  const visible = messages.filter(
    (m) => selected && (m.to_id === selected || m.from_id === selected),
  )
  const activePeer = peers.find((p) => p.id === selected)

  async function onSend(e: React.FormEvent) {
    e.preventDefault()
    if (!selected || !draft.trim()) return
    setError('')
    try {
      await sendMessage(selected, draft.trim())
      setDraft('')
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Send failed')
    }
  }

  return (
    <div className="layout">
      <aside className="sidebar">
        <div className="brand">
          <h1>OffGrid Mesh</h1>
          <p className="muted">Offline-first messaging</p>
        </div>

        <div className="me-card">
          <span className="label">This device</span>
          <strong>{me ? me.name : '...'}</strong>
          <span className="mono">{me ? me.addr : status}</span>
          <span className="mono dim">{me?.id}</span>
        </div>

        <h2 className="section-title">Nearby peers ({peers.length})</h2>
        {peers.length === 0 && (
          <p className="muted pad">
            No peers yet. Start another node on this network, or add one with <code>-peer host:port</code>.
          </p>
        )}
        <ul className="peer-list">
          {peers.map((p) => (
            <li key={p.id}>
              <button
                type="button"
                className={p.id === selected ? 'peer active' : 'peer'}
                onClick={() => setSelected(p.id)}
              >
                <span className="dot" aria-hidden="true" />
                <span className="peer-main">
                  <strong>{p.name}</strong>
                  <span className="mono">{p.addr}</span>
                </span>
              </button>
            </li>
          ))}
        </ul>

        <p className="foot muted">
          Live mode: LAN discovery. Mule mode (carried bundles) is on the roadmap.
        </p>
      </aside>

      <main className="chat">
        <header className="chat-head">
          <div>
            <strong>{activePeer ? activePeer.name : 'Select a peer'}</strong>
            <span className="mono dim">{activePeer?.addr}</span>
          </div>
          <span className="status">{status}</span>
        </header>

        <div className="messages">
          {!activePeer && <p className="muted center">Choose a peer on the left to start chatting.</p>}
          {activePeer && visible.length === 0 && <p className="muted center">No messages yet. Say hello.</p>}
          {visible.map((m) => (
            <div key={m.id} className={m.mine ? 'msg mine' : 'msg'}>
              <span className="msg-name">{m.mine ? 'You' : m.from_name}</span>
              <p>{m.text}</p>
              <time>{new Date(m.created_at).toLocaleTimeString()}</time>
            </div>
          ))}
          <div ref={bottomRef} />
        </div>

        {error && <p className="error">{error}</p>}
        <form className="composer" onSubmit={onSend}>
          <input
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            placeholder={activePeer ? `Message ${activePeer.name}...` : 'Select a peer first'}
            disabled={!activePeer}
            aria-label="Message"
          />
          <button type="submit" disabled={!activePeer || !draft.trim()}>
            Send
          </button>
        </form>
      </main>
    </div>
  )
}

export default App
