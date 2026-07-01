# QuakeMesh — Project Plan

## Project Names

| Binary | Language | Role |
|---|---|---|
| **QuakeMeshHub** | Go | Stable backbone service: registry, routing, NAT relay, Hub-to-Hub sync |
| **QuakeMesh** *(QuakeMesh Android Node)* | Kotlin + Go core | Android endpoint and repeater app |
| **QuakeMeshMonitor** | Go | Web-based admin and monitoring dashboard, runs alongside a Hub |

All three share `/core` (the Go mesh-core library) and `/proto` (protobuf wire schemas).

---

## Overview

QuakeMesh is a self-contained, infrastructure-independent private mesh network. It requires no cloud services, no central servers, and no pre-existing internet access to function. QuakeMeshHub nodes form a stable backbone; QuakeMesh Android nodes act as both endpoints and multi-hop repeaters; QuakeMeshMonitor provides a real-time browser-based view of the entire network for administrators.

The network auto-discovers nodes, auto-routes traffic across multiple hops, tracks physical locations, maintains per-node trust scores, and falls back to the internet only when explicitly permitted by the user. It is designed to work in the absence of any infrastructure and to degrade gracefully rather than fail when nodes disappear.

---

## Repository Layout (monorepo)

```
/core      Go module: identity, wire protocol (protobuf), routing engine,
           trust engine, DTN store-and-forward queue, transport interface,
           SQLite schema and migrations (modernc.org/sqlite — pure Go, no cgo)

/hub       QuakeMeshHub binary: imports /core; Hub-to-Hub gossip sync;
           internet rendezvous/relay server; loopback management API
           (127.0.0.1:8083); relay-hub discovery and propagation

/monitor   QuakeMeshMonitor binary: Go web server, default port 8082;
           all static assets embedded via go:embed (no external files);
           admin auth; reads from Hub loopback API and shared SQLite file

/android   Android Studio project: Kotlin UI + transport shims
           (BLE, Wi-Fi Direct/Meshrabiya, GPS) calling into /core via
           gomobile-generated .aar bindings;
           Kotlin Room DB for UI preferences only

/proto     Shared .proto schemas: frame headers, OGM/hello, trust records,
           DTN bundles, app envelopes, ban-list records, relay-hub records,
           management API events — codegen'd into both Go and Kotlin

/sdk       Mesh-as-a-transport SDK: Kotlin AAR + Go module wrapping the
           local IPC API for third-party apps

/docs      Protocol spec; SQLite schema reference; Monitor API spec
```

---

## Storage — SQLite Everywhere

Both QuakeMeshHub and QuakeMesh use `modernc.org/sqlite` (pure Go, no cgo — works with gomobile on Android). All schema definitions and migrations live in `/core` so Hub and Android share the same structure.

### QuakeMeshHub — `quakemeshhub.db`

| Table | Purpose |
|---|---|
| `node_registry` | All known nodes: pubkey, first/last seen, last GPS position, status |
| `routing_table` | Per-destination best next-hops: TQ score, latency ms, hop count |
| `proximity_events` | Observed direct radio contact: observer, observed, RSSI, transport, time |
| `trust_endorsements` | Signed explicit trust endorsements between nodes |
| `hub_registry` | Known hubs: pubkey, last IP/port, relay_capable flag |
| `relay_hubs` | Verified relay-capable hubs: ip, port, source (auto/manual), last_verified |
| `app_presence` | App id/name/version active per node, last reported time |
| `ban_list` | App ban proposals + per-hub agree/disagree verdicts |
| `dtq_queue` | DTN bundle store: payload blob, expiry timestamp, retry count |
| `historical_metrics` | Time-series snapshots for Monitor history views |
| `admin_users` | Monitor admin accounts: password_hash, salt, must_change_password flag |
| `config` | Key-value hub configuration store |

### QuakeMesh Android — `quakemesh.db` (Go core)

Subset: `node_registry` (cached), `routing_table` (local view), `proximity_events`, `trust_endorsements`, `dtq_queue`, `relay_hubs`, `app_presence`.

A separate Kotlin Room database (`quakemesh_ui.db`) stores user preferences, notification settings, and UI state — never shared with the Go core.

---

## Identity and Security

- Every node and hub generates an **Ed25519 keypair** on first run. `NodeID` = SHA-256 hash of the public key. No central CA, no registration — purely self-sovereign identity.
- **Per-hop encryption**: every radio link (Bluetooth, Wi-Fi Direct, LAN UDP) is wrapped in a **Noise Protocol** handshake (the same primitive used by WireGuard), so adjacent peers authenticate each other and encrypt every frame.
- **End-to-end encryption**: application payloads are additionally sealed to the destination node's public key. Intermediate repeater nodes forward opaque ciphertext and only read routing headers (hop count, destination ID, TTL). They cannot read content even though they carry it.
- **Internet-fallback path**: uses QUIC with TLS 1.3 built in — no separate Noise handshake needed on this path.

---

## Transport Layer

Transports are tried in priority order, direct-link first. The mesh-core sees only an abstract `Transport` interface (`Send`, `OnReceive`, `OnPeerUp`, `OnPeerDown`). Platform-specific implementations are plugged in by the host binary or Android Kotlin layer.

### 1. Bluetooth Classic / BLE
Universal fallback. Shortest range, lowest power. Always active for discovery beacons even when faster transports are available.

### 2. Wi-Fi Direct + Local Only Hotspot (Android primary)
The main high-bandwidth P2P transport between Android nodes. The implementation will study and closely follow the **Meshrabiya** framework (`github.com/UstadMobile/Meshrabiya`), which solves multi-hop on stock Android without root or custom firmware:

- Each device simultaneously runs a **Local Only Hotspot** (for incoming clients) and connects as a **station client** to an upstream node.
- Virtual link-local IPs assigned in the `169.254.x.x` range.
- BATMAN-inspired OGMs carrying virtual IP routing tables propagate across hops.

Meshrabiya should be studied before writing any transport shims — it may be directly reusable, or provide a validated blueprint to adapt.

### 3. Wi-Fi Aware (NAN)
Use where supported and where the privacy trade-off is acceptable. **Treat with caution**: the API exposes scoped IPv6 addresses that standard networking libraries often fail to resolve, and service advertisements are publicly broadcast, creating device-tracking and spoofing risks. Mitigation (rotating service identifiers, limiting advertised data) is required before deploying in privacy-sensitive contexts.

### 4. Local Wi-Fi / LAN
If a node and a hub (or two hubs) share a conventional network, use plain UDP/QUIC on that LAN. No AP or router involvement beyond Layer 2.

### 5. Internet Fallback (Wi-Fi-to-internet or mobile data)
Last resort. Gated by an explicit per-user opt-in permission toggle. Routed through internet-reachable hubs using the NAT traversal scheme below. The UI must make it visible when this path is active.

### Android Background Execution
Continuous mesh participation requires a persistent **foreground Service** with a visible notification. Android will kill background processes without this. This is a real UX cost — elevated battery drain and a permanent notification — that must be explained during onboarding, not buried in settings.

**iOS is out of scope.** `BGTaskScheduler` limits background execution to 1–2 hour opportunistic windows, making real-time off-grid mesh impractical on iOS without the app permanently in the foreground.

---

## NAT/CGNAT Traversal for the Internet Fallback Path

Direct Bluetooth/Wi-Fi/LAN links are local P2P and never touch NAT. The internet fallback path does. Most consumer connections and virtually all mobile-carrier connections sit behind NAT or CGNAT.

### Mechanism: UDP hole punch → QUIC tunnel

**UDP** is used to open the NAT binding. **QUIC** is used for the actual data stream. QUIC is UDP-based (works through the punched binding), has TLS 1.3 built in, handles multiplexed streams natively, and survives IP address changes — which matters when a phone switches between Wi-Fi and mobile data mid-session.

**Hubs act as the rendezvous/relay server** since they are already the stable, internet-reachable anchor of the network:

1. Node A and Node B each maintain a UDP session to a hub they both reach. The hub records each peer's NAT-translated public `(IP, port)` — exactly what a STUN server does.
2. The hub introduces them: sends each peer the other's public endpoint.
3. A and B simultaneously send UDP packets to each other's public endpoint. For full-cone or address-restricted-cone NATs (standard home routers), the outbound packet opens a NAT binding that admits the peer's reply — producing a direct path off the hub. Empirical DCUtR data: **~80% success on residential NAT, ~60% when a VPN is active**.
4. Both peers upgrade the hole-punched UDP path to a **QUIC connection** (`quic-go` on the Go side). This is what mesh traffic flows over.
5. **Symmetric NAT** (common on mobile carriers and CGNAT — external port differs per destination) defeats hole punching (~0% success). Fallback: **Hub relay** (TURN-style) — the hub forwards QUIC frames between A and B indefinitely. This is not an edge case; a meaningful fraction of mobile-data users will always need the relay path.
6. Once a QUIC connection is open it needs periodic keepalive packets (every ~15–25 s) or the NAT binding expires.

### DCUtR Timing (reference)
Mirror libp2p DCUtR's approach for the synchronized dial: measure RTT over the relayed control channel, wait exactly `0.5 × RTT`, then both peers dial simultaneously. The timing is critical — simultaneous outbound packets are what cause both NATs to record matching state table entries. Implement purpose-built rather than pulling in all of libp2p, but follow this protocol precisely.

### Relay Hub Discovery and Propagation

A hub is **relay-capable** if it has a publicly-reachable IP+port and has relaying explicitly enabled (opt-in). Three mechanisms keep the `relay_hubs` table current across the whole network:

1. **Manual addition** via QuakeMeshMonitor: an admin enters `ip:port` → the hub probes it (TCP connect + relay capability handshake) → stores with `source = "manual"` and `last_verified = now` if the probe succeeds.

2. **Automatic Hub-to-Hub discovery**: relay-capable hubs advertise their public endpoint in Hub-to-Hub gossip announcements. Any hub receiving the record adds it to its own `relay_hubs` table (as `source = "gossip"`) after a probe confirmation. Hubs also actively query newly-discovered or long-unseen hubs to refresh their relay status.

3. **Client-assisted propagation**: Android nodes receive the relay hub list from their home hub and carry it in their presence data. When a QuakeMesh node encounters a hub that is missing a relay it knows about, it reports that relay record to the hub — which then probes and adds it. This bridges gaps where two hubs cannot reach each other directly but share a common client in range of both.

The relay hub list is visible and editable in QuakeMeshMonitor's Configuration section: IP, port, source, last-verified time, probe status. Stale or unverifiable entries are flagged for admin review.

---

## Node/Hub Presence and Discovery

- On startup, a node/hub broadcasts a high-priority **"I'm up"** announcement on every available transport. This propagates hop-by-hop through the mesh and is pushed directly to any reachable hub.
- **Steady-state liveness**: lightweight pings to one-hop neighbors every few seconds detect dropped links fast, independent of the slower OGM-based routing refresh.
- **Re-discovery of stale nodes**: known-but-unreachable nodes are kept in the registry and probed periodically. When a lost node reappears, the network notices without requiring it to re-announce itself.

---

## Routing Protocol

Modeled on **BATMAN-adv** (Better Approach To Mobile Ad-hoc Networking Advanced). Unlike OLSR, BATMAN-adv never disseminates full topology, avoiding the topology/routing-table synchronisation problem that causes routing loops under churn. It is the right choice for a network where nodes constantly appear, disappear, and move.

### Originator Messages (OGMs)
Each node periodically broadcasts an OGM: `{NodeID, sequence_number, TTL}`. Neighbours decrement TTL and re-broadcast once, recording link quality using a sliding window of received-vs-missed OGMs.

### Bidirectional Link Quality — TQ = EQ / RQ
BATMAN-adv measures bidirectional link quality to handle asymmetric wireless links (A can hear B, but B can barely hear A — common in real wireless environments):

- **RQ (Receiving Link Quality)**: count of OGMs received directly from a neighbour.
- **EQ (Echo Link Quality)**: count of re-broadcasts of *this node's own OGMs* received back from that neighbour.
- **Transmit Quality**: `TQ = EQ / RQ`

This local TQ propagates through the network and degrades gracefully per hop, so multi-hop path quality accurately reflects cumulative link reliability rather than blindly counting hops.

### Routing Metric
Per known destination, the best next-hop is chosen by a weighted combination of: propagated `TQ`, link **latency**, and link **loss rate** — not pure hop count. The network can prefer a longer but faster or more reliable path, balancing hops against lag as required.

### MTU
BATMAN-adv encapsulation adds 32 bytes per frame. Physical interfaces must be configured at MTU **1532** (not 1500) to absorb this without forcing IP fragmentation.

### Route Failover
When a next-hop's quality drops or its OGMs time out, the node immediately fails over to the next-best alternate next-hop already in its routing table — no full re-discovery needed. The "I'm up" startup broadcast is a high-priority flooded OGM variant, so new nodes propagate quickly.

---

## Store-and-Forward (DTN Queue)

When a packet has no live route to its destination, it is not dropped. It is queued in the local **DTN bundle store** (`dtq_queue` in SQLite), following the store-carry-forward model (Bundle Protocol / RFC 9171), until:

- (a) A mesh route appears — destination reconnects, a carrying node moves into range, or routing recovers; or
- (b) The internet fallback is the only option and the user has permitted it.

Queued bundles carry a TTL/expiry timestamp and a retry count so they do not accumulate indefinitely. The queue depth is visible in QuakeMeshMonitor's Overview dashboard.

---

## Location and Proximity

- Nodes record GPS fixes (`lat`, `lon`, `alt`, `accuracy`, `timestamp`) when available via Android's `FusedLocationProviderClient`.
- When GPS is unavailable, nodes record **RSSI-based range estimates** to each directly-reachable neighbour over BT/Wi-Fi Direct and share their neighbour table as part of routing/presence data.
- Hubs and nodes combine GPS fixes with relative RSSI distances to estimate position for GPS-less nodes, and to compute an approximate **bearing and distance** from a node's last known position and its last known set of in-range neighbours — satisfying the "which direction and how far to move to reach an orphaned node" use case. This is a heuristic estimate, not exact triangulation, and is labelled with a confidence level and age indicator in the UI.

---

## Trust Register

A **0–100 score** per node, rendered as a colour band in the network map (red → amber → green → blue for verified-trusted). Combines three components:

### 1. Longevity
Time since first seen on the network, on a saturating (diminishing-returns) curve. A node cannot max out trust quickly just by staying up.

### 2. Physical Proximity History
Whether this node has ever been in direct radio range of a hub (strong signal) or of another already-trusted node (weaker signal). Recorded as proximity events observed by other devices — not self-reported, and therefore not forgeable by the node itself.

### 3. Explicit Endorsements
A user can explicitly trust another node. That endorsement applies first to *that user's own* view and decisions. In aggregate across the network, many independent endorsements nudge the node's *global* baseline trust upward, but with **diminishing returns per unique endorser** to resist Sybil attacks.

**Anti-rubber-stamping**: an endorsement can only be issued for a node whose `node_id` the endorsing user's device has itself recorded a direct proximity event with. You cannot endorse a node you have never physically been near.

---

## Hub-to-Hub Sync

Hubs gossip their view of the network over whatever channel they share (mesh, LAN, or internet). Gossiped records:

- Node registry (last-seen, last position, status, trust components)
- Relay hub list
- App presence data
- Ban-list proposals and verdicts

Records merge **last-writer-wins by timestamp**. Self-attested fields (e.g. a node's own GPS claim) carry the node's own signature. Hub-observed fields (e.g. "I personally saw this node in range") are attributed to the observing hub. No central authority — any two hubs that can reach each other converge their state.

---

## Application SDK and Transport-as-a-Service

The mesh is the **base layer**. Other applications use it purely as a transport — the way an app uses TCP/IP without caring about routers. Both QuakeMeshHub and QuakeMesh run the mesh-core as a standing **daemon**:

- **Android**: mesh-core runs inside a foreground `Service`. Other apps talk to it via a local IPC interface (AIDL-backed bound service or loopback gRPC/Unix domain socket).
- **Hub**: exposes the same local API on a Unix domain socket.

A thin **mesh-sdk** library (Kotlin AAR for Android, Go module for CLI/server integrations) wraps the IPC:

| Method | Description |
|---|---|
| `Register(appID, appName, appVersion, capabilities[])` | Opens a session, announces this app to the local node |
| `Send(session, destNodeID, payload)` | Point-to-point message, mesh-routed and store-and-forward |
| `Receive(session)` | Incoming message stream |
| `Publish(session, topic, payload)` | Broadcast to all subscribers of a topic |
| `Subscribe(session, topic)` | Receive topic broadcasts |
| `DiscoverPeers(appID, versionConstraint?)` | Find nodes running a compatible app version |

### App Identity, Stats, and Discovery

Every `Register()` call declares `{app_id, app_name, app_version}`. App IDs use reverse-DNS style (e.g. `net.quakemesh.privatechat`) to avoid collisions without a central registry.

A node's active `app_id@version` set rides alongside its OGM/presence announcements to hubs. Hubs aggregate this into network-wide stats and answer `DiscoverPeers` queries — the same data serves both the admin usage dashboard and app-to-app peer discovery.

---

## Software Ban List and Hub Governance

When an app or a specific version is found to be malicious or exploited:

- Hubs maintain a gossiped **ban list** propagated through the Hub-to-Hub sync channel: entries are `{app_id, version_range, reason, proposed_by (HubID, signed), proposed_at}`.
- A ban starts as a **proposal** from one hub. It is **never auto-enforced**.
- Every other hub's admin sees pending proposals in QuakeMeshMonitor's Ban List UI and explicitly chooses **Agree** or **Disagree**.
- **Agreeing** enforces the ban locally: the hub stops relaying traffic tagged with that `app_id`/version and stops forwarding its presence/discovery records.
- **Disagreeing** dismisses it locally. The original proposal remains in the gossiped record so other hubs can still decide independently.
- Hubs gossip their own verdicts (not just the original proposal), so the network has visibility into how many hubs have enforced a given ban. This data model is designed to support a future quorum-based auto-enforcement mode without implementing it yet — today, enforcement is purely per-hub-admin opt-in.

---

## Permissions for Non-Mesh Radios

Using Wi-Fi-to-internet or mobile data is a deliberate fallback, never silent. Both the hub config and the Android app require an explicit opt-in toggle before any traffic leaves over a non-mesh radio. The UI must make it clearly visible when the internet fallback path is actively in use.

---

## QuakeMeshMonitor

A standalone Go binary that runs on the same machine as a QuakeMeshHub, serving a browser-based admin and monitoring dashboard.

### Server

- Default bind: `0.0.0.0:8082`, configurable via config file or `QUAKEMESH_MONITOR_PORT` environment variable.
- All static assets (HTML, CSS, JavaScript, Leaflet.js, Chart.js, vis.js, map tiles) embedded in the binary via `//go:embed` — no external file dependencies at runtime, works fully offline.
- Connects to QuakeMeshHub's loopback management API (`127.0.0.1:8083`) for live event streams via WebSocket subscription.
- Reads QuakeMeshHub's SQLite file directly (read-only) for historical queries.
- Serves a `/ws` WebSocket endpoint for real-time browser push — no polling required.

### Authentication

- Session-based login (HTTP-only secure cookie, server-side session store in SQLite `admin_users` table).
- **Default credentials: username `Admin`, password `test1234`.**
- QuakeMeshMonitor **forces a password change on first successful login** with the default credentials. The default password cannot be used for normal operation — this requirement cannot be skipped or dismissed.
- Login attempts are rate-limited: 5 failures triggers a 60-second lockout.
- Optional HTTPS with auto-generated self-signed certificate on first run; admins should replace this with a proper certificate for production use.

### Dashboard Views

| View | Content |
|---|---|
| **Overview** | Node count (total/online/offline), hub count, active routes, DTN queue depth, internet-fallback status — all live via WebSocket |
| **Node Map** | Leaflet.js map with nodes at GPS coordinates or estimated positions; colour-coded by trust score; click a node for detail panel showing status, last seen, apps, routes |
| **Network Graph** | Force-directed topology (vis.js): nodes, hubs, active links, link TQ quality, hop counts — updates live as routing changes |
| **Routes** | Table of active routes: src → dst, next-hop chain, TQ per hop, round-trip latency; sortable and filterable |
| **Hop Timing** | Latency histogram and time-series chart (Chart.js) per node pair; shows historical trends |
| **App Stats** | Table of `app_id@version` active across the network: node count, first seen, last seen |
| **Trust Scores** | Per-node breakdown: longevity component, proximity event count, endorsement count, final score; filterable and sortable |
| **History** | Any live view can be switched to a historical time window — reads from `historical_metrics` SQLite table; time range picker |

### Configuration (admin-only)

| Setting | Detail |
|---|---|
| **Relay Hub List** | Table of relay-capable hubs: ip, port, source (manual/gossip/client), last-verified, probe status; add manually, trigger re-probe, remove; propagation status across the network visible |
| **Hub Settings** | OGM broadcast interval, liveness-ping frequency, DTN bundle TTL, Hub-to-Hub sync interval |
| **Internet Fallback** | Global toggle for fallback permission; per-node fallback status visible |
| **Ban List** | Pending proposals with reason and proposing hub; Agree/Disagree per proposal; network-wide agree/disagree tally per ban |
| **Admin Accounts** | Change password; add/remove additional admin users |
| **Monitor Settings** | Port, bind address, HTTPS certificate path, session timeout |

### Tech Stack

- Go `net/http` + `gorilla/websocket` for server and live push
- `quic-go` (already in dependency tree for internet-fallback transport)
- Frontend: vanilla HTML/JS — no build step, no Node.js required
- Leaflet.js (map), Chart.js (timeseries/histograms), vis.js (network graph) — all vendored and embedded so the binary is self-contained and works offline

---

## Wire Format

Protobuf schemas in `/proto`, code-generated for Go and Kotlin, covering:

- Frame header: `src_node_id`, `dst_node_id`, `hop_count`, `ttl`, `transport_hints`
- OGM / hello
- DTN bundle envelope
- Trust/proximity records
- Hub sync messages
- App presence envelope: `app_id`, `app_name`, `app_version`
- Ban-list proposal and verdict records
- Relay hub records
- Management API events (live stream from Hub to Monitor)

---

## Candidate Applications

Built on the SDK as both working software and proof that the base layer is usable:

| Priority | App | SDK pattern validated |
|---|---|---|
| 1 | **Private messaging** (1:1 and small group, E2E encrypted via node identity keys) | `Send`/`Receive`, store-and-forward across a partitioned mesh |
| 2 | **Public discussion boards / bulletin areas** (topic-based, eventually consistent via Hub gossip + DTN) | `Publish`/`Subscribe`, app discovery at scale |
| 3 | **Emergency/SOS beacon** (urgent broadcast with location, exploits bearing/distance-to-orphaned-node feature) | High-priority broadcast, location integration |
| 4 | **Trust-coloured network map** (visualise node status, trust, position) | Reads mesh-core data; mostly UI |
| 5 | **Opportunistic file/resource drop** (DTN-carried, pick up when in range) | Heavy DTN store-and-forward stress test |
| 6 | **Push-to-talk / low-bandwidth voice** (exercises latency-aware routing) | Latency-sensitive `Send`/`Receive` |
| 7 | **Sensor/telemetry relay** (weather, environmental sensors) | Machine-to-machine app identity/discovery |

---

## Phased Delivery Roadmap

| Phase | Deliverable |
|---|---|
| **1** | **Protocol & identity core** — Ed25519 identity, SQLite schema and migrations, protobuf schemas, Noise handshake, all in `/core`; virtual simulated-network harness for testing routing/trust without real radios |
| **2** | **QuakeMeshHub MVP** — Hub binary: SQLite-backed registry, OGM routing over UDP/LAN between multiple hub processes, loopback management API (127.0.0.1:8083) with event stream |
| **3** | **QuakeMeshMonitor MVP** — Web server on port 8082: admin login (forced password change from default Admin/test1234), Overview dashboard + Node Map reading from Hub API and SQLite, relay hub list UI with manual add and probe |
| **4** | **Android transports** — Study Meshrabiya source first; build/adapt Kotlin BLE + Wi-Fi Direct + Local Only Hotspot shims wired to gomobile-bound Go core; two physical phones exchanging frames across a two-hop Meshrabiya-style path |
| **5** | **Multi-hop routing and failover** — 3+ node topologies; automatic route-around on node disappearance; latency/hop-count metric balancing; Network Graph and Routes views added to QuakeMeshMonitor |
| **6** | **Store-and-forward + presence/re-discovery** — DTN bundle queue in SQLite; stale-node probing; "I'm up" propagation; DTN queue depth in Monitor Overview |
| **7** | **Trust register** — proximity events, longevity, endorsements, Sybil dampening; Trust Scores view in Monitor; colour-coded node map markers |
| **8** | **Location and proximity estimation** — GPS on Android, RSSI ranging, bearing/distance-to-orphaned-node heuristic; geo-pins and orphaned-node direction indicator in Monitor Node Map |
| **9** | **Hub-to-Hub sync + internet fallback + relay hub propagation** — gossip merge; UDP hole-punch + QUIC tunnel (DCUtR-style timing); Hub relay fallback for symmetric NAT/CGNAT; relay hub auto-discovery and client-assisted propagation; Hop Timing and History views in Monitor; relay hub list shows propagation status |
| **10** | **App SDK and local daemon API** — Android foreground-service IPC surface; Kotlin/Go SDK; app presence reporting; `DiscoverPeers`; App Stats view in Monitor |
| **11** | **Ban list and Hub governance** — gossip proposal/verdict records; Ban List management UI in Monitor (Agree/Disagree, agree-tally); local enforcement |
| **12** | **Reference apps** — private messaging and discussion-board app built on the SDK; end-to-end proof that the base layer is sufficient for third-party developers |

Each phase should be runnable and demoable before moving to the next.

---

## Verification Strategy

### `/core`
Go unit tests for: routing-metric selection (TQ/latency/loss weighting), trust score calculation, DTN queue expiry, SQLite schema migrations. Integration test spinning up N in-process virtual nodes over a simulated lossy/latent transport, asserting routing convergence and correct re-routing when a node is killed mid-test.

### `/hub` (QuakeMeshHub)
Integration test launching 3+ hub processes on localhost with different ports. Verify: OGM-based routing convergence, Hub-to-Hub gossip sync, relay hub propagation (manual add on hub A → probe → gossip to hub B → client C carries record to hub D that missed the gossip).

### `/monitor` (QuakeMeshMonitor)
Go `httptest` tests for all API endpoints and WebSocket event emission. Browser-level smoke tests (Playwright/headless Chromium) verifying:
- Login → forced password change screen appears → change accepted → dashboard loads
- Default Admin/test1234 rejected after password change
- 5 failed login attempts trigger lockout
- Overview shows live node count updating via WebSocket
- Node Map renders
- Relay hub can be manually added and triggers a probe

### `/android` (QuakeMesh)
Real-device testing with 2–3 physical phones — emulators cannot test BLE/Wi-Fi Direct/Local Only Hotspot. Validate: Meshrabiya-style discovery, direct connection, multi-hop relay, "node disappears → automatic reroute" end-to-end. Verify SQLite persistence survives app restart and foreground service re-bind.

### NAT traversal
Test UDP hole-punch + QUIC against:
- Real home-router NAT: expect ~80% punch success
- Real mobile-carrier CGNAT: expect ~0% punch success, relay must engage automatically

Verify QUIC stream survives an IP address change (Wi-Fi → mobile data switch mid-session).

### `/sdk`
Sample Go CLI app and sample Android app exercising all SDK methods (`Register`, `Send`, `Receive`, `Publish`, `Subscribe`, `DiscoverPeers`) against a local test hub. Confirm third-party apps never need to touch the wire protocol.

### Ban list
Multi-hub integration test: propose ban from hub A → confirm it gossips to hubs B and C as "pending" → Agree on B enforces locally → Disagree on C leaves C's behaviour unchanged → no-action on D leaves D's behaviour unchanged.

---

## Key Technology Decisions

| Decision | Choice | Reason |
|---|---|---|
| Hub language | Go | Single binary, strong concurrency, no runtime on server |
| Android mesh engine | Go core via gomobile | Code reuse, Go faster than Kotlin for crypto/compute; ~5.5 MB binary overhead is acceptable |
| Android transport | Meshrabiya pattern (Wi-Fi Direct + Local Only Hotspot) | Proven multi-hop on stock Android without root |
| NAT traversal | UDP hole punch + QUIC | QUIC is UDP-based (works through punch), TLS 1.3 built in, survives IP changes |
| Routing algorithm | BATMAN-adv-inspired OGM + TQ = EQ/RQ | No full topology flood, handles asymmetric links, self-healing |
| Storage | `modernc.org/sqlite` (pure Go) | Works on Android via gomobile without cgo |
| Wire format | Protocol Buffers | Type-safe, compact, codegen for both Go and Kotlin |
| Monitor frontend | Vanilla JS + Leaflet + Chart.js + vis.js, go:embed | No build step, offline-capable, single self-contained binary |
| iOS support | **Out of scope** | BGTaskScheduler prevents real-time background mesh |
