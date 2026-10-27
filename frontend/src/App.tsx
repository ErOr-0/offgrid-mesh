import { useEffect, useRef, useState } from 'react'
import {
  allowDevice,
  connectEvents,
  fetchBundles,
  fetchFiles,
  fetchMe,
  fetchMessages,
  fetchPairCode,
  fetchPeers,
  fetchRouting,
  fileDownloadURL,
  pairQRURL,
  pairWithCode,
  queueBundle,
  sendFileWithProgress,
  sendMessage,
  type BundleInfo,
  type ChatMessage,
  type FileInfo,
  type Me,
  type Peer,
  type RoutingEntry,
} from './api'
import './App.css'

function App() {
  const [me, setMe] = useState<Me | null>(null)
  const [peers, setPeers] = useState<Peer[]>([])
  const [selected, setSelected] = useState<string>('')
  const [messages, setMessages] = useState<ChatMessage[]>([])
  const [draft, setDraft] = useState('')
  const [bundles, setBundles] = useState<BundleInfo[]>([])
  const [files, setFiles] = useState<FileInfo[]>([])
  const [routing, setRouting] = useState<RoutingEntry[]>([])
  const [pairCode, setPairCode] = useState('')
  const [pairInput, setPairInput] = useState('')
  const [fileProgress, setFileProgress] = useState<number | null>(null)
  const [mode, setMode] = useState<'live' | 'carry'>('live')
  const [destID, setDestID] = useState('')
  const [region, setRegion] = useState('')
  const [status, setStatus] = useState('Connecting to local node...')
  const [error, setError] = useState('')
  const bottomRef = useRef<HTMLDivElement>(null)

  useEffect(() => {
    let ws: WebSocket | undefined
    let cancelled = false
    ;(async () => {
      try {
        const [meData, peerData, msgData, bundleData, fileData, routingData, myPairCode] = await Promise.all([
          fetchMe(),
          fetchPeers(),
          fetchMessages(),
          fetchBundles(),
          fetchFiles(),
          fetchRouting(),
          fetchPairCode(),
        ])
        if (cancelled) return
        setMe(meData)
        setPeers(peerData ?? [])
        setMessages(msgData ?? [])
        setBundles(bundleData ?? [])
        setFiles(fileData ?? [])
        setRouting(routingData ?? [])
        setPairCode(myPairCode)
        setStatus('Connected')
        ws = connectEvents((ev) => {
          if (ev.type === 'message') {
            setMessages((prev) => (prev.some((m) => m.id === ev.message.id) ? prev : [...prev, ev.message]))
          } else if (ev.type === 'peers') {
            setPeers(ev.peers ?? [])
          } else if (ev.type === 'peer_joined') {
            setPeers((prev) => (prev.some((p) => p.id === ev.peer.id) ? prev : [...prev, ev.peer]))
          } else if (ev.type === 'bundles') {
            setBundles(ev.bundles ?? [])
            fetchRouting().then((r) => setRouting(r ?? [])).catch(() => {})
            fetchFiles().then((f) => setFiles(f ?? [])).catch(() => {})
          } else if (ev.type === 'delivery') {
            setMessages((prev) => prev.map((m) => (m.id === ev.message.id ? ev.message : m)))
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

  const conversationID = mode === 'carry' ? destID.trim() || selected : selected
  const visible = messages.filter(
    (m) => conversationID && (m.to_id === conversationID || m.from_id === conversationID),
  )
  const activePeer = peers.find((p) => p.id === selected)
  const canSend = mode === 'carry' ? Boolean(conversationID && draft.trim()) : Boolean(selected && draft.trim())

  async function onFile(e: React.ChangeEvent<HTMLInputElement>) {
    const file = e.target.files?.[0]
    e.target.value = ''
    if (!file || !selected) return
    setError('')
    setFileProgress(0)
    try {
      await sendFileWithProgress(selected, file, setFileProgress)
      setFiles((await fetchFiles()) ?? [])
    } catch (err) {
      setError(err instanceof Error ? err.message : 'File send failed')
    } finally {
      setFileProgress(null)
    }
  }

  async function onPairSubmit(e: React.FormEvent) {
    e.preventDefault()
    if (!pairInput.trim()) return
    setError('')
    try {
      await pairWithCode(pairInput.trim())
      setPairInput('')
      setRouting((await fetchRouting()) ?? [])
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Pairing failed')
    }
  }

  async function onAllow(id: string) {
    setError('')
    try {
      await allowDevice(id)
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Pairing failed')
    }
  }

  async function onSend(e: React.FormEvent) {
    e.preventDefault()
    if (!canSend) return
    setError('')
    try {
      if (mode === 'carry') {
        await queueBundle(conversationID, region.trim(), draft.trim())
        setBundles((await fetchBundles()) ?? [])
        setMessages((await fetchMessages()) ?? [])
      } else {
        await sendMessage(selected, draft.trim())
      }
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
          <span className="mono dim">{me?.hub ? `Hub sync: ${me.hub}` : 'Hub sync: not configured'}</span>
          <span className="mono dim">Pairing code: {me?.fingerprint ?? '...'}</span>
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
              <button type="button" className="pair-btn" onClick={() => onAllow(p.id)}>
                Pair
              </button>
            </li>
          ))}
        </ul>

        <h2 className="section-title">Carried bundles ({bundles.length})</h2>
        {bundles.length === 0 && <p className="muted pad">No bundles in queue. Carry mode queues one here.</p>}
        <ul className="bundle-list">
          {bundles.map((b) => (
            <li key={b.id} className="bundle-item">
              <strong>{b.kind === 'receipt' ? 'Receipt' : 'Message'} → {b.to.slice(0, 8)}…</strong>
              <span className="mono">
                hops {b.hop_count} · copies {b.copies} · {b.visited?.length ?? 0} carriers
                {b.region ? ` · ${b.region}` : ''}
                {b.encrypted ? ' · sealed' : ''}
              </span>
              <span className="mono dim">expires {new Date(b.expires_at).toLocaleString()}</span>
            </li>
          ))}
        </ul>

        <h2 className="section-title">Files ({files.length})</h2>
        {files.length === 0 && <p className="muted pad">No files yet. Select a peer, then attach a file below.</p>}
        <ul className="bundle-list">
          {files.map((f) => (
            <li key={f.id} className="bundle-item">
              <strong>{f.name}</strong>
              <span className="mono">
                {f.mine ? 'sent' : `from ${f.from_name}`} · {f.size} bytes
              </span>
              <a className="download" href={fileDownloadURL(f.id)}>
                Download
              </a>
            </li>
          ))}
        </ul>

        <h2 className="section-title">Best carriers</h2>
        {routing.length === 0 && <p className="muted pad">No encounters yet. Scores grow as devices meet.</p>}
        <ul className="bundle-list">
          {routing.map((r) => (
            <li key={r.device_id} className="bundle-item">
              <span className="mono">{r.device_id.slice(0, 8)}…</span>
              <span className="mono">
                score {r.score.toFixed(2)} · met {r.encounters}x
              </span>
            </li>
          ))}
        </ul>

        <h2 className="section-title">Pair a device</h2>
        <div className="pair-box">
          {me && <img className="qr" src={pairQRURL()} alt="Pairing QR code for this device" width={132} height={132} />}
          <p className="muted">Scan this QR, or paste another device's pairing code:</p>
          <form onSubmit={onPairSubmit} className="pair-form">
            <input
              value={pairInput}
              onChange={(e) => setPairInput(e.target.value)}
              placeholder='{"id":"...","key":"..."}'
              aria-label="Pairing code"
            />
            <button type="submit" disabled={!pairInput.trim()}>
              Pair
            </button>
          </form>
          {pairCode && (
            <details>
              <summary className="muted">My pairing code</summary>
              <p className="mono pair-code">{pairCode}</p>
            </details>
          )}
        </div>

        <p className="foot muted">
          Live mode needs a reachable peer. Carry mode stores a bundle and hands copies to peers you meet.
          A peer hosts a website with -host site=./hosted; open it here via /tunnel/&lt;device-id&gt;/site/ on your node.
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
          {!conversationID && <p className="muted center">Choose a peer on the left, or use Carry mode with a device ID.</p>}
          {conversationID && visible.length === 0 && <p className="muted center">No messages yet. Say hello.</p>}
          {visible.map((m) => (
            <div key={m.id} className={m.mine ? 'msg mine' : 'msg'}>
              <span className="msg-name">{m.mine ? 'You' : m.from_name}</span>
              <p>{m.text}</p>
              <time>
                {new Date(m.created_at).toLocaleTimeString()}
                {m.mine && m.status ? ` · ${m.status}` : ''}
                {m.encrypted ? ' · end-to-end encrypted' : ''}
              </time>
            </div>
          ))}
          <div ref={bottomRef} />
        </div>

        {error && <p className="error">{error}</p>}
        <div className="mode-row">
          <label>
            Mode
            <select value={mode} onChange={(e) => setMode(e.target.value as 'live' | 'carry')}>
              <option value="live">Live (direct)</option>
              <option value="carry">Carry (bundle)</option>
            </select>
          </label>
          {mode === 'carry' && (
            <>
              <input
                value={destID}
                onChange={(e) => setDestID(e.target.value)}
                placeholder="Destination device ID (blank = selected peer)"
                aria-label="Destination device ID"
              />
              <input
                value={region}
                onChange={(e) => setRegion(e.target.value)}
                placeholder="Region, e.g. MY-Johor"
                aria-label="Destination region"
              />
            </>
          )}
        </div>
        <form className="composer" onSubmit={onSend}>
          <input
            value={draft}
            onChange={(e) => setDraft(e.target.value)}
            placeholder={activePeer ? `Message ${activePeer.name}...` : 'Select a peer or enter a device ID'}
            disabled={!conversationID}
            aria-label="Message"
          />
          <button type="submit" disabled={!canSend}>
            {mode === 'carry' ? 'Queue bundle' : 'Send'}
          </button>
          <label className={selected ? 'file-btn' : 'file-btn disabled'}>
            {fileProgress !== null ? `Sending ${fileProgress}%` : 'Attach file'}
            <input type="file" hidden disabled={!selected} onChange={onFile} />
          </label>
        </form>
        {fileProgress !== null && (
          <progress className="file-progress" value={fileProgress} max={100}>
            {fileProgress}%
          </progress>
        )}
      </main>
    </div>
  )
}

export default App
