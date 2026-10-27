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
  looksLikeIPPort,
  pairWithCode,
  queueBundle,
  queueBundleDestIP,
  sendFileWithProgress,
  sendMessage,
  type BundleInfo,
  type ChatMessage,
  type FileInfo,
  type Me,
  type Peer,
  type RoutingEntry,
} from '../api'
import { MeCard } from '../features/identity/MeCard'
import { PeerList } from '../features/peers/PeerList'
import { BundleList } from '../features/bundles/BundleList'
import { FileList } from '../features/files/FileList'
import { CarrierList } from '../features/routing/CarrierList'
import { PairPanel } from '../features/pairing/PairPanel'
import { ChatPane, type ChatMode } from '../features/chat/ChatPane'
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
  const [mode, setMode] = useState<ChatMode>('live')
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
  const canSend = Boolean(conversationID && draft.trim())

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
        const dest = destID.trim()
        if (dest && looksLikeIPPort(dest)) {
          await queueBundleDestIP(dest, region.trim(), draft.trim())
        } else {
          await queueBundle(conversationID, region.trim(), draft.trim())
        }
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
        <MeCard me={me} status={status} />
        <PeerList peers={peers} selected={selected} onSelect={setSelected} onAllow={onAllow} />
        <BundleList bundles={bundles} />
        <FileList files={files} />
        <CarrierList routing={routing} />
        <PairPanel
          me={me}
          pairCode={pairCode}
          pairInput={pairInput}
          onPairInput={setPairInput}
          onPairSubmit={onPairSubmit}
        />
        <p className="foot muted">
          Live mode needs a reachable peer. Carry mode stores a bundle and hands copies to peers you meet.
          A peer hosts a website with -host site=./hosted; open it here via /tunnel/&lt;device-id&gt;/site/ on your node.
        </p>
      </aside>
      <ChatPane
        activePeer={activePeer}
        status={status}
        messages={visible}
        conversationID={conversationID}
        bottomRef={bottomRef}
        error={error}
        mode={mode}
        onMode={setMode}
        destID={destID}
        onDestID={setDestID}
        region={region}
        onRegion={setRegion}
        draft={draft}
        onDraft={setDraft}
        canSend={canSend}
        onSend={onSend}
        selected={selected}
        fileProgress={fileProgress}
        onFile={onFile}
      />
    </div>
  )
}

export default App
