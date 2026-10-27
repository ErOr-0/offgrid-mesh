import type { ChatMessage, Peer } from '../../api'

export type ChatMode = 'live' | 'carry'

interface Props {
  activePeer?: Peer
  status: string
  messages: ChatMessage[]
  conversationID: string
  bottomRef: React.RefObject<HTMLDivElement | null>
  error: string
  mode: ChatMode
  onMode: (m: ChatMode) => void
  destID: string
  onDestID: (v: string) => void
  region: string
  onRegion: (v: string) => void
  draft: string
  onDraft: (v: string) => void
  canSend: boolean
  onSend: (e: React.FormEvent) => void
  selected: string
  fileProgress: number | null
  onFile: (e: React.ChangeEvent<HTMLInputElement>) => void
}

export function ChatPane({ activePeer, status, messages, conversationID, bottomRef, error, mode, onMode, destID, onDestID, region, onRegion, draft, onDraft, canSend, onSend, selected, fileProgress, onFile }: Props) {
  return (
    <main className="chat">
      <header className="chat-head">
        <div>
          <strong>{activePeer ? activePeer.name : 'Select a peer'}</strong>
          <span className="mono dim">{activePeer?.addr}</span>
        </div>
        <span className="status">{status}</span>
      </header>

      <div className="messages">
        {!conversationID && (
          <p className="muted center">Choose a peer on the left, or use Carry mode with a device ID or IP:port.</p>
        )}
        {conversationID && messages.length === 0 && <p className="muted center">No messages yet. Say hello.</p>}
        {messages.map((m) => (
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
          <select value={mode} onChange={(e) => onMode(e.target.value as ChatMode)}>
            <option value="live">Live (direct)</option>
            <option value="carry">Carry (bundle)</option>
          </select>
        </label>
        {mode === 'carry' && (
          <>
            <input
              value={destID}
              onChange={(e) => onDestID(e.target.value)}
              placeholder="Destination device ID or IP:port, e.g. 203.0.113.7:8080"
              aria-label="Destination device ID or IP"
            />
            <input
              value={region}
              onChange={(e) => onRegion(e.target.value)}
              placeholder="Region, e.g. MY-Johor"
              aria-label="Destination region"
            />
          </>
        )}
      </div>
      <form className="composer" onSubmit={onSend}>
        <input
          value={draft}
          onChange={(e) => onDraft(e.target.value)}
          placeholder={activePeer ? `Message ${activePeer.name}...` : 'Select a peer or enter a destination'}
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
  )
}
