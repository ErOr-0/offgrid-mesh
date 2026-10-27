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

## Phase 3 — Files, identity, tunnel

- [ ] Encrypted file transfer with progress
- [ ] Device pairing (QR / code), allowlist
- [ ] Tunnel: expose a local port through a host node

## Phase 4 — Hosting and polish

- [ ] Host serves a static site / API gateway to peers
- [ ] Encounter-based routing (PRoPHET-style scoring)
- [ ] Demo video, screenshots, submission cleanup
