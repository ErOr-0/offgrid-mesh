# Architecture

## Modes

1. **Live mesh (LAN / hotspot):** devices discover each other with mDNS/UDP broadcast, chat over WebSocket, and one device can open a tunnel to host services for the others.
2. **Mule mode (DTN):** no live path exists. Encrypted bundles are stored, carried by nearby devices and travellers, and forwarded hop by hop toward the destination region.
3. **Sync when available:** any internet or satellite link syncs queued bundles automatically. Same React UI in all modes.

## Bundle

The unit of offline delivery. Carriers read metadata, never the payload.

| Field | Purpose |
|---|---|
| ID | Unique bundle ID |
| From / To | Sender / recipient device IDs (public-key hashes, not IPs) |
| TTL | Expiry, e.g. 48h — carriers delete expired bundles |
| HopCount | Number of carriers so far |
| Visited | Carrier hashes already seen — loop prevention (Bloom filter in production) |
| Region | Destination region hint, e.g. `MY-Johor` |
| Payload | Message, end-to-end encrypted |

## Routing

- **Epidemic:** copy to every peer met. Fast, wasteful. Good for urgent small messages.
- **Spray-and-Wait:** limited copies, then wait for direct delivery. Balanced default.
- **PRoPHET-style:** forward to carriers whose encounter history suggests they reach the destination region.

Each hop is signed by the carrier so the path cannot be forged. Delivery receipts travel back the same slow way, so delivery confirmations can be delayed.

## Tunnel (live mode only)

A tunnel needs a live two-way connection, so it cannot run over carried bundles. In live mode, a host node exposes local ports/services (WireGuard or an encrypted TCP tunnel) and other peers reach them through the host.

## Privacy rules

- Minimise metadata: no plain-text visited lists in production, use hashes/Bloom filters.
- Payloads are always end-to-end encrypted.
- Bundles are size-limited and expire, to prevent spam and storage abuse.
