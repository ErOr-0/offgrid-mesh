import type { Peer } from '../../api'

interface Props {
  peers: Peer[]
  selected: string
  onSelect: (id: string) => void
  onAllow: (id: string) => void
}

export function PeerList({ peers, selected, onSelect, onAllow }: Props) {
  return (
    <>
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
              onClick={() => onSelect(p.id)}
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
    </>
  )
}
