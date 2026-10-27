import type { RoutingEntry } from '../../api'

export function CarrierList({ routing }: { routing: RoutingEntry[] }) {
  return (
    <>
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
    </>
  )
}
