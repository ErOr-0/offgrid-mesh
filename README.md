# OffGrid Mesh

Offline-first communication without internet. Messages carry routing metadata and hop from device to device — carried by people nearby, on ferries, buses, and flights — until they slowly reach their destination.

Built for a Meta hackathon.

## The problem

When internet, mobile networks, or power fail — disasters, remote areas, sea crossings, crowded events — people still need to communicate. Between Indonesia and Malaysia, for example, there is no direct offline path over long distance unless nearby devices relay messages for each other.

## The idea

Every device is a carrier. An encrypted message is wrapped in a **Bundle** with routing metadata: destination device ID, region hint, TTL, hop count, and visited carriers. When two devices meet over Bluetooth, Wi-Fi Direct, or LAN, they exchange bundles. Each carrier decides whether it is moving toward the destination region or often meets the recipient, and forwards copies accordingly. This is Delay-Tolerant Networking (DTN), also called store-carry-forward.

```text
Sender (Batam) -> nearby phone -> ferry passenger -> taxi driver (Johor) -> Recipient
     Hop 0           Hop 1             Hop 2              Hop 3            Delivered
```

Live mode (LAN / hotspot mesh) also supports a device tunnel, so one device can host services — a local site, files, an API — for others connected to it.

## Tech stack

- **Backend:** Go — concurrency, networking, cryptography, DTN bundle routing
- **Frontend:** React + TypeScript — peer list, chat, transfer progress, tunnel controls
- **Transports:** LAN/mDNS, Wi-Fi Direct, BLE, internet/satellite sync when available

## Repository structure

Modular monolith — one Go binary, one React build, explicit module boundaries (see [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)):

```text
backend/cmd/node            composition root (flags, wiring)
backend/internal/app        application core: messaging, sync, pairing, files, tunnel delegates
backend/internal/modules    bundles, identity, discovery, routing, pairing, files, tunnel
backend/internal/transport  httpapi (HTTP + WebSocket adapter, no domain logic)
frontend/src/app            App composition + session state
frontend/src/api            one client module per backend resource
frontend/src/features       identity, peers, bundles, files, routing, pairing, chat components
docs/                       Architecture, roadmap, demo and submission notes
hosted/                     sample static site served through the tunnel
```

## Status

Phases 1–3 work: LAN chat, carried bundles (mule mode), hub sync, end-to-end encryption, pairing/allowlist, file transfer, and the HTTP tunnel. Start with [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) and [docs/ROADMAP.md](docs/ROADMAP.md).

Phase 3 in short: every node has an X25519 identity (check the pairing fingerprint in the UI or startup log, compare it out-of-band, then press **Pair**). With `-allow <device-id>` a node rejects unpaired senders. Files send directly to a selected peer from the composer. A host exposes a local website with `-expose web=127.0.0.1:3000`, and peers open it at `http://<their-node>/tunnel/<host-device-id>/web/`.

In mule mode, a message becomes a bundle with 8 Spray-and-Wait copies and a 48h TTL. Nodes exchange bundles automatically whenever they meet. A carrier that is not the destination keeps a split of the copies and carries the bundle onward. The destination delivers the message and issues a receipt bundle, which travels back the same way until the sender's message is marked delivered. Bundle queues persist with `-data <file>`, and device IDs stay stable with `-id` or next to the data file.

## Run it (Phase 1 demo)

Start two Go nodes (two terminals, or two devices on one LAN):

```powershell
cd backend
go run ./cmd/node -name Alice -port 8080
go run ./cmd/node -name Bob -port 8081 -peer <Alice-IP>:8080
```

Nodes on the same LAN find each other by UDP broadcast automatically. Use `-peer` when broadcast is blocked, or for two nodes on one machine.

Start the React UI:

```powershell
cd frontend
npm install
npm run dev
```

Open http://localhost:5173 in your browser (proxies to the node on :8080). For the second node's UI, open http://localhost:5173/?node=http://localhost:8081

Carry mode also accepts a **destination IP:port** (for example a node in Malaysia) instead of a device ID. The bundle travels addressed to that IP, and the first carrier that can reach it resolves the node's real device ID and delivers. There is no live IP route between countries without a continuous path — the bundle is carried, not routed.

For mule mode, use **Carry (bundle)** in the UI, or queue from the API:

```powershell
Invoke-RestMethod -Method Post -Uri http://localhost:8080/api/bundles -ContentType 'application/json' -Body '{"to":"<device-id>","region":"MY-Johor","text":"hello Malaysia","copies":8,"ttl_hours":48}'
# or addressed to an IP:port instead of a device ID:
Invoke-RestMethod -Method Post -Uri http://localhost:8080/api/bundles -ContentType 'application/json' -Body '{"dest_ip":"203.0.113.7:8080","region":"MY-Johor","text":"hello Malaysia","copies":8,"ttl_hours":48}'
```

Persist a node's queue and identity across restarts:

```powershell
go run ./cmd/node -name Alice -port 8080 -data alice-bundles.json
```

## Hosting on your own server (hub / super peer)

The hub is the same binary. It does not replace local nodes — it bridges them when they have internet.

On your server (public IP, and a domain + TLS in production):

```powershell
go run ./cmd/node -name Hub -port 8090 -data hub-bundles.json
```

On each device, add the hub once. Sync is outbound-only, so NAT and firewalls do not block it:

```powershell
go run ./cmd/node -name Alice -port 8080 -data alice-bundles.json -hub https://hub.example.com
```

Behaviour:

- No internet: LAN chat, discovery, and carried bundles keep working locally. Nothing is lost.
- Internet appears on any device: it pushes bundles it carries and pulls bundles waiting for it (every 5s while reachable, with retries at startup).
- The hub stores bundles until the destination syncs, then issues and forwards delivery receipts the same way.
- Bundle payloads are end-to-end encrypted to the destination's X25519 key whenever the sender knows that key (learned from nearby peers or the hub's key directory). Carriers and the hub see only ciphertext plus routing metadata. If a destination key is unknown, the bundle is queued unsealed and marked as such in the UI — pair with the device or sync with the hub first to seal it.

## Hackathon submission checklist

- [ ] Problem, solution, and demo clearly described in this README
- [ ] Demo video or live demo link added here
- [ ] Screenshots added here
- [ ] Team members credited here
- [ ] License present (MIT)
- [ ] Code builds and runs from a clean clone

## License

MIT — see [LICENSE](LICENSE).
