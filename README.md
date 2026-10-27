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

Hackathon project — in active development. Start with [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) and [docs/ROADMAP.md](docs/ROADMAP.md).

## Hackathon submission checklist

- [ ] Problem, solution, and demo clearly described in this README
- [ ] Demo video or live demo link added here
- [ ] Screenshots added here
- [ ] Team members credited here
- [ ] License present (MIT)
- [ ] Code builds and runs from a clean clone

## License

MIT — see [LICENSE](LICENSE).
