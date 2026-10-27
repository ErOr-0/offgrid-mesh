# Hackathon submission

## Project

**OffGrid Mesh** — offline-first communication without internet. Encrypted messages carry routing metadata and hop device to device — carried by people nearby, on ferries, buses, and flights — until they reach their destination. A self-hosted hub bridges regions whenever any device gets internet.

## What to submit

- [x] Public GitHub repository with MIT license
- [x] README: problem, solution, architecture, run instructions
- [ ] Team members and roles (add below and in README)
- [ ] Demo video (follow [DEMO.md](DEMO.md), 2–3 minutes)
- [ ] Screenshots: peer list + chat, carried bundle queue, QR pairing, hosted site via tunnel
- [ ] Official hackathon form: project name, repo URL, video URL, team, track/category
- [ ] Check the official Meta hackathon rules once the link is known: eligibility, team size, submission deadline, judging criteria, IP/code-of-conduct terms

## Team

- _Name — role (backend / frontend / demo)_
- _Name — role_

## Judging points to show

1. **Working, not slides:** two laptops or one laptop with two nodes exchanging live, then one node going offline while a bundle is carried by a third node.
2. **Real constraint solved:** Indonesia–Malaysia with no infrastructure: ferry/airport carriers, TTL expiry, sealed payloads a carrier cannot read.
3. **Production thinking:** persistent queues and stable identities across restarts, allowlist pairing with fingerprints, delivery receipts that travel back, hub that never dials into homes.
4. **Honest limits:** delayed (not real-time), needs user density, live tunnel only with a live path.
