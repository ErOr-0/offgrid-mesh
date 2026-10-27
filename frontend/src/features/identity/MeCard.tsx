import type { Me } from '../../api'

export function MeCard({ me, status }: { me: Me | null; status: string }) {
  return (
    <div className="me-card">
      <span className="label">This device</span>
      <strong>{me ? me.name : '...'}</strong>
      <span className="mono">{me ? me.addr : status}</span>
      <span className="mono dim">{me?.id}</span>
      <span className="mono dim">{me?.hub ? `Hub sync: ${me.hub}` : 'Hub sync: not configured'}</span>
      <span className="mono dim">Pairing code: {me?.fingerprint ?? '...'}</span>
    </div>
  )
}
