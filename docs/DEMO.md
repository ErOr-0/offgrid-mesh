# Demo script (2–3 minutes)

Setup: three terminals for the nodes, two browser tabs for the UIs.

```powershell
cd backend
go build -o bin/node.exe ./cmd/node
.\bin\node.exe -name Hub   -port 8090 -id hub-demo
.\bin\node.exe -name Alice -port 8080 -id alice-demo -region ID-Batam -hub http://127.0.0.1:8090
.\bin\node.exe -name Bob   -port 8081 -id bob-demo   -region MY-Johor -peer 127.0.0.1:8080 -hub http://127.0.0.1:8090 -host site=../hosted
cd ../frontend; npm run dev
```

Tabs: http://localhost:5173 (Alice) and http://localhost:5173/?node=http://localhost:8081 (Bob)

## Scenes

1. **Live chat (20s).** Select Bob in Alice's tab, send a message. It arrives marked end-to-end encrypted. Point at the pairing code and the **Pair** button.
2. **Go offline, queue a bundle (40s).** In Alice's tab switch to **Carry (bundle)**, destination Bob, region `MY-Johor`, send "ferry message to Malaysia". Show the bundle in the queue: 8 copies, hops, expiry. Explain: no internet exists here; the bundle waits.
3. **Carrier delivers (40s).** The nodes exchange automatically. Bob receives the message later than sent, and Alice's copy flips to **delivered** when the receipt travels back. Show **Best carriers**: encounter scores rose from meeting.
4. **Host without internet (30s).** Bob hosts the sample site. Open `http://localhost:8080/tunnel/bob-demo/site/` — a page served by Bob's device, reached through Alice's node. No web server on the internet involved.
5. **Hub bridge (30s).** Explain the server story: the Hub terminal is the same binary you run on your own server. Any device with brief internet pushes and pulls bundles; everyone else keeps working offline.
6. **Close (10s).** Honest limits: delivery is delayed, density matters, live tunnel needs a live path. Next: BLE/Wi-Fi Direct transport, production hub with TLS.
