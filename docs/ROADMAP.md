# Roadmap

## Phase 1 — LAN chat (MVP)

- [x] Go node with LAN discovery (UDP broadcast beacons + manual `-peer`)
- [x] Chat between nodes (HTTP delivery, payload in a `Bundle`; WebSocket events to UI)
- [x] React UI: peer list + chat
- [ ] mDNS discovery (broadcast covers LAN; mDNS planned for wider subnet support)

## Phase 2 — Bundles and mule mode

- [x] `Bundle` type, persistent storage (`-data` file), TTL expiry
- [x] Peer bundle exchange over LAN/HTTP, automatic every 5s when peers meet
- [x] Spray-and-Wait forwarding (binary copy split), delivery receipts return to sender
- [x] Stable device IDs (`-id`, or persisted next to the `-data` file)
- [ ] BLE / Wi-Fi Direct transport (same exchange protocol, new transport)

## Server sync (Option 3: hub as super peer)

- [x] `-hub <url>` on any node: outbound bundle sync with an always-on hub node
- [x] Hub accepts `https://` base URLs, carries bundles between regions, never dials back (NAT-safe)
- [ ] Hub behind TLS with a public domain (deployment task, same binary)

## Phase 3 — Files, identity, tunnel

- [x] End-to-end encryption: X25519 identity per node, bundle payloads sealed to the destination (carriers/hub see ciphertext only)
- [x] File transfer between peers with download in the UI
- [x] Device pairing: fingerprint code per node, Pair button / `-allow` allowlist enforced on receive, exchange, and files
- [x] Tunnel: `-expose web=127.0.0.1:3000`, reachable by peers at `/tunnel/<device-id>/web/`
- [ ] File progress bar, QR pairing, file payloads carried inside bundles

## Phase 4 — Hosting and polish

- [ ] Host serves a static site / API gateway to peers
- [ ] Encounter-based routing (PRoPHET-style scoring)
- [ ] Demo video, screenshots, submission cleanup
