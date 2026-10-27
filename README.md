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

```text
backend/    Go node: bundle routing, discovery, tunnel
frontend/   React UI (to be scaffolded)
docs/       Architecture, roadmap, hackathon notes
```

## Status

Phase 1 (LAN chat) and Phase 2 (carried bundles / mule mode) work. Start with [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) and [docs/ROADMAP.md](docs/ROADMAP.md).

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

For mule mode, use **Carry (bundle)** in the UI, or queue from the API:

```powershell
Invoke-RestMethod -Method Post -Uri http://localhost:8080/api/bundles -ContentType 'application/json' -Body '{"to":"<device-id>","region":"MY-Johor","text":"hello Malaysia","copies":8,"ttl_hours":48}'
```

Persist a node's queue and identity across restarts:

```powershell
go run ./cmd/node -name Alice -port 8080 -data alice-bundles.json
```

## Hackathon submission checklist

- [ ] Problem, solution, and demo clearly described in this README
- [ ] Demo video or live demo link added here
- [ ] Screenshots added here
- [ ] Team members credited here
- [ ] License present (MIT)
- [ ] Code builds and runs from a clean clone

## License

MIT — see [LICENSE](LICENSE).
