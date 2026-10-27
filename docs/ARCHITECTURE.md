# Architecture — modular monolith

OffGrid Mesh is a **modular monolith**: one Go binary and one React build, split into modules with explicit boundaries. It is deliberately not microservices — a node must run on a laptop or phone with no infrastructure.

## Backend modules (`backend/internal/`)

```text
cmd/node/                 composition root: flags -> app.NewWithID -> httpapi.New
internal/
  app/                    application core (facade): wires modules, owns node lifecycle
    node.go               Node struct, identity loading, discovery wiring, loops
    messaging.go          send, queue, ingest, deliver, receipts, exchange, dest-IP delivery
    sync.go               hub sync + manual peers (outbound only)
    pairing.go            pairing/allowlist delegates, key registry, sealing
    files.go              file send/receive delegates
    tunnel.go             tunnel delegates
  modules/
    bundles/              Bundle domain type, Spray-and-Wait, persistent Store
    identity/             X25519 identity, fingerprint, Encrypt/Decrypt envelopes
    discovery/            UDP broadcast + mDNS peer discovery
    routing/              PRoPHET-style encounter scores
    pairing/              allowlist + pairing codes (fingerprint-verified)
    files/                in-memory file store
    tunnel/               exposed HTTP services + static site hosting
  transport/
    httpapi/              HTTP + WebSocket adapter ONLY: every handler delegates to app
```

### Dependency rules

```text
cmd/node -> app, httpapi
httpapi  -> app, modules/* (types only)
app      -> modules/*
modules  -> (leaves; bundles has no internal imports, store lives with it)
```

- No module imports `app` or `httpapi`. Transport never contains domain logic.
- Wire contracts are frozen across the refactor: Bundle JSON fields, `copies=8` / TTL 48h defaults, empty `peers`/`bundles` encoding as `[]`, envelope v1 (`offgrid-mesh-bundle-v1`).
- Tests live beside modules; app behavior tests use the external `app_test` package against the exported app API, which keeps the httpapi import acyclic.

## Frontend modules (`frontend/src/`)

```text
main.tsx                  entry -> app/App
app/App.tsx               composition + node-session state only
app/App.css               global layout/chat styles
api/                      one file per backend resource; client.ts owns nodeBase + events
  types.ts client.ts peers.ts chat.ts bundles.ts files.ts pairing.ts routing.ts index.ts
features/                 presentational components, props in / handlers out
  identity/MeCard  peers/PeerList  bundles/BundleList  files/FileList
  routing/CarrierList  pairing/PairPanel  chat/ChatPane
```

Dependency rules: `app -> features -> api`; `api` never imports React components; features never import each other (selection state stays in `app/App`).

## Addressing: device ID and destination IP

A bundle normally targets a **device ID**. It can also target a **destination IP:port** (`dest_ip`):

- If that IP answers `/api/me` now, the bundle resolves to the real device ID and is sealed to its key.
- Otherwise it travels addressed to `ip:<addr>`; every carrier probes the IP on each exchange tick, and the first carrier that reaches it resolves the ID and delivers.
- A private IP only has meaning on its own local network, so final delivery happens when a carrier joins or reaches that network. There is no live IP route across Indonesia–Malaysia without a continuous path; the bundle is carried, not routed.

## Modes

1. **Live mesh (LAN / hotspot):** discovery, direct chat/files, tunnel — no internet.
2. **Mule mode (DTN):** sealed bundles stored, carried, Spray-and-Wait exchanged; SOS-style priority flooding is a bundle-kind policy on top.
3. **Hub sync:** `-hub` adds an always-on node as a peer; sync is outbound-only (NAT-safe). The hub is the same binary.

## Privacy rules

- Minimise metadata; payloads sealed to the destination whenever its key is known (peer beacons and the hub key directory distribute keys).
- Pairing is fingerprint-verified; allowlists are enforced in the app module for receive, exchange, and files alike.
