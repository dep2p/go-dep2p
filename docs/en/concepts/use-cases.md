# Use Cases and Business Value

This document details DeP2P's use cases and business value, helping you understand DeP2P's position as Web3 infrastructure.

---

## Core Positioning

DeP2P is not just a P2P library, but the **core network layer of Web3 infrastructure**.

```
┌─────────────────────────────────────────────────────────────────────┐
│                    DeP2P Infrastructure Positioning                  │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │                    Application Layer                         │    │
│  │   Blockchain │ Decentralized Storage │ PCDN │ IM │ Collab    │    │
│  └─────────────────────────────────────────────────────────────┘    │
│                              ▲                                       │
│                              │                                       │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │                    DeP2P Network Layer                        │    │
│  │   Identity First │ Realm Isolation │ Smart Connect │ NAT     │    │
│  └─────────────────────────────────────────────────────────────┘    │
│                              ▲                                       │
│                              │                                       │
│  ┌─────────────────────────────────────────────────────────────┐    │
│  │                    Transport Layer                            │    │
│  │   QUIC │ TLS 1.3 │ Multiplexing │ Congestion Control         │    │
│  └─────────────────────────────────────────────────────────────┘    │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

---

## Three Core Scenarios

### 1. 🔗 Blockchain Networks

As the P2P network layer for blockchain projects, DeP2P provides:

| Capability | Blockchain Scenario | DeP2P Solution |
|------------|---------------------|----------------|
| **Transaction Broadcast** | Propagate transactions to the network | PubSub + Gossip Protocol |
| **Block Sync** | Fast sync for new nodes | Multi-source parallel + Resumable |
| **Consensus Communication** | Message passing between validators | Low-latency direct + Relay fallback |
| **Node Discovery** | Discover other nodes | DHT + Realm Isolation |
| **Network Isolation** | Mainnet/Testnet separation | Native Realm support |
| **Network Resilience** | Auto-recovery after network switch/device sleep | System-level network monitoring + auto-recovery ✅ |
| **Path Health** | Auto-selection of multiple paths (IPv4/IPv6/Relay) | PathHealth Manager ✅ |
| **Address Discovery** | Multi-source address fusion (PortMap/STUN/Local) | Reachability Coordinator ✅ |

#### Network Resilience Capabilities (Implemented)

DeP2P has implemented the following capabilities by learning from iroh's network resilience design:

| Capability | Description | Status |
|------------|-------------|--------|
| **System-level Network Monitoring** | Proactive detection of network changes (Wi-Fi switch, device sleep) | ✅ Implemented |
| **Single-flight Address Discovery** | Avoid concurrency races, ensure stable address discovery | ✅ Implemented |
| **Enhanced NetReport** | NAT type detection, Relay latency measurement | ✅ Implemented |
| **Multi-source Address Fusion** | Unified management of PortMap/STUN/Local/Configured | ✅ Implemented |
| **PortMapper Watcher** | Event-driven external address changes | ✅ Implemented |
| **Path Health Management** | Multi-path health monitoring and auto-switching | ✅ Implemented |

> **Detailed Analysis**: See [iroh Design Analysis](../../design/discussions/DISC-IROH-DESIGN-ANALYSIS.md)

DeP2P is designed to serve as the underlying network layer for blockchain projects, providing transaction broadcast, block sync, and consensus communication capabilities.

#### Blockchain Enhancement Capabilities (Planned)

DeP2P is enhancing the following capabilities to better support blockchain scenarios:

| Capability | Description | Priority |
|------------|-------------|----------|
| **Message Rate Tracking** | Dynamically adjust request size based on peer capacity to prevent overload | P0 |
| **Inbound/Outbound Ratio Control** | Ensure sufficient outbound connections for active discovery | P0 |
| **IP Limiting Mechanism** | Prevent nodes from the same IP segment from consuming too many connection resources (Sybil attack protection) | P0 |
| **Dial Scheduler** | Intelligently manage dial tasks, distinguish static/dynamic dials | P0 |
| **Static Node Auto-Reconnect** | Ensure stable connections with critical nodes (Bootstrap/Validator) | P0 |
| **Node Database Persistence** | Retain known node information after restart for faster startup | P1 |
| **Capabilities Negotiation** | Negotiate supported protocol versions during connection establishment | P1 |

> **Detailed Analysis**: See [Blockchain Enhancement Requirements](../../design/implementation/BLOCKCHAIN_ENHANCEMENT_REQUIREMENTS.md)

---

### 2. 💾 Decentralized Storage

Provides file transfer and indexing capabilities for decentralized storage:

```
┌─────────────────────────────────────────────────────────────────────┐
│                    Decentralized Storage + DeP2P                     │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  File Chunking  →  Calculate hash per chunk, generate Merkle Tree   │
│  Content Address →  Find storage nodes via DHT                      │
│  Multi-source   →  Download same chunk from multiple nodes          │
│  Resumable      →  Resume from any node after interruption          │
│  Integrity      →  Merkle Proof verification                        │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

---

### 3. 📡 PCDN Content Delivery

DeP2P can serve as the network layer for P2P CDN, supporting four content delivery modes:

| Mode | Characteristics | P2P Benefit | DeP2P Solution |
|------|-----------------|-------------|----------------|
| **Software Download** | Large files, strong consistency | ⭐⭐⭐ Very High | Block exchange + Multi-source |
| **Static Sites** | Versioned, first-screen sensitive | ⭐⭐ Medium | Manifest + Merkle |
| **Video on Demand** | Segmented, hotspot aggregation | ⭐⭐⭐ High | Segment index + Preload |
| **Live Streaming** | Ultra-low latency, real-time fanout | ⭐⭐⭐ High | PubSub + Tree topology |

---

## Business Model Support

DeP2P provides technical support for these business models:

| Business Model | DeP2P Capability |
|----------------|------------------|
| **Bandwidth Incentives** | Upload/download byte counting for Token incentives |
| **Storage Incentives** | Content indexing protocol, prove "what data I store" |
| **CDN Cost Optimization** | P2P offloading, reduce Origin/CDN bandwidth costs |
| **Decentralized Hosting** | Static site P2P distribution, no centralized servers |

---

## Recommended Implementation Path

| Phase | Target | Difficulty | Description |
|-------|--------|------------|-------------|
| **Phase 1** | Software Download | ⭐⭐ | Easiest to verify P2P benefits, similar to BitTorrent |
| **Phase 2** | Static Sites | ⭐⭐⭐ | Manifest + Chunk, integrate with Web3 site protocol |
| **Phase 3** | Video on Demand | ⭐⭐⭐ | Add segment hotness, preload strategies |
| **Phase 4** | Live Streaming | ⭐⭐⭐⭐ | PubSub + Tree topology + Strict latency control |

---

## Suitability Assessment

### Recommended Scenarios

| Scenario | DeP2P Advantage | Rating |
|----------|-----------------|--------|
| **Blockchain / DeFi** | Realm isolation + Node discovery + Transaction broadcast | ⭐⭐⭐⭐⭐ |
| **Decentralized Storage** | Multi-source download + Content addressing + Resumable | ⭐⭐⭐⭐⭐ |
| **GameFi** | Low latency + Business isolation + State sync | ⭐⭐⭐⭐ |
| **Instant Messaging** | Simple API + Reliable transport + E2E encryption | ⭐⭐⭐⭐ |
| **Software Distribution** | Large files + Multi-source parallel + Peak offload | ⭐⭐⭐⭐⭐ |

### Assessment Matrix

| Assessment | Scenarios | Notes |
|------------|-----------|-------|
| ✅ **Very Suitable** | Blockchain, Storage, IM, Collaboration | Core DeP2P design goals |
| ⚠️ **Partially Suitable** | Video on Demand, IoT | Acceptable latency, evaluate resource usage |
| ❌ **Not Suitable** | Ultra-low latency streaming (<100ms), Video conferencing, Cloud gaming | Requires unreliable transport (WebRTC) |

---

## Next Steps

- [What is DeP2P](what-is-dep2p.md) - Core vision and design goals
- [Core Concepts](core-concepts.md) - Identity first, three-layer architecture, Realm
- [Comparison](comparison.md) - Understand DeP2P's differentiation
- [Quickstart](../getting-started/quickstart.md) - Hands-on practice

