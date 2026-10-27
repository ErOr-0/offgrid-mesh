import type { BundleInfo } from '../../api'

export function BundleList({ bundles }: { bundles: BundleInfo[] }) {
  return (
    <>
      <h2 className="section-title">Carried bundles ({bundles.length})</h2>
      {bundles.length === 0 && <p className="muted pad">No bundles in queue. Carry mode queues one here.</p>}
      <ul className="bundle-list">
        {bundles.map((b) => (
          <li key={b.id} className="bundle-item">
            <strong>
              {b.kind === 'receipt' ? 'Receipt' : 'Message'} → {(b.dest_ip ?? b.to).slice(0, 22)}
            </strong>
            <span className="mono">
              hops {b.hop_count} · copies {b.copies} · {b.visited?.length ?? 0} carriers
              {b.region ? ` · ${b.region}` : ''}
              {b.encrypted ? ' · sealed' : ''}
            </span>
            <span className="mono dim">expires {new Date(b.expires_at).toLocaleString()}</span>
          </li>
        ))}
      </ul>
    </>
  )
}
