import { pairQRURL, type Me } from '../../api'

interface Props {
  me: Me | null
  pairCode: string
  pairInput: string
  onPairInput: (v: string) => void
  onPairSubmit: (e: React.FormEvent) => void
}

export function PairPanel({ me, pairCode, pairInput, onPairInput, onPairSubmit }: Props) {
  return (
    <>
      <h2 className="section-title">Pair a device</h2>
      <div className="pair-box">
        {me && <img className="qr" src={pairQRURL()} alt="Pairing QR code for this device" width={132} height={132} />}
        <p className="muted">Scan this QR, or paste another device's pairing code:</p>
        <form onSubmit={onPairSubmit} className="pair-form">
          <input
            value={pairInput}
            onChange={(e) => onPairInput(e.target.value)}
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
    </>
  )
}
