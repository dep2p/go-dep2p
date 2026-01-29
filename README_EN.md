# DeP2P —— Make P2P Connections as Simple as Function Calls

<div align="center">

<pre>
██████╗   ███████╗  ██████╗    ██████╗   ██████╗ 
██╔══██╗  ██╔════╝  ██╔══██╗  ██╔═══██╗  ██╔══██╗
██║  ██║  █████╗    ██████╔╝     ███╔╝   ██████╔╝
██║  ██║  ██╔══╝    ██╔═══╝    ███╔╝     ██╔═══╝ 
██████╔╝  ███████╗  ██║       ███████╗   ██║     
╚═════╝   ╚══════╝  ╚═╝       ╚══════╝   ╚═╝     
</pre>

**Simple, Reliable, Secure P2P Networking Foundation (QUIC-first)**  
**NodeID Direct Connect + Realm Isolation + NAT Traversal/Relay Fallback, Ready to Use**  
**Make decentralization as natural as the internet, make connections as elegant as function calls**

📖 **[English](README_EN.md) | [中文](README.md)**

[![Go Version](https://img.shields.io/badge/Go-1.21+-blue.svg)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20macOS%20%7C%20Windows-lightgrey.svg)]()
[![Status](https://img.shields.io/badge/Status-Active-green.svg)]()

<sub>📊 Codebase: 161K lines of Go code (250K total, incl. comments/blank lines)</sub>

</div>

---

## 📑 Table of Contents

- [Core Vision](#-core-vision)
- [The Big Picture](#-the-big-picture)
- [Why Choose DeP2P?](#-why-choose-dep2p)
- [Core Features](#-core-features)
- [Quick Start](#-quick-start)
- [Technical Architecture](#-technical-architecture)
- [Business Value](#-business-value-web3-infrastructure-network-layer)
- [Use Cases](#-use-cases)
- [Documentation](#-documentation)
- [Contributing & Community](#-contributing--community)
- [License](#-license)

---

## 🌌 Core Vision

```
┌─────────────────────────────────────────────────────────────────────┐
│                                                                     │
│     Make P2P connections as simple as function calls:               │
│     Give a NodeID, send a message                                    │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

> **NodeID** = Public key identity (Base58 encoded). Goal is "connect by identity", not "connect by IP/domain".  
> **Realm** = Business boundary (multi-tenant/multi-app isolation). Nodes in different Realms are invisible to each other, preventing network pollution.

DeP2P's vision is not "rebuilding a P2P protocol collection", but converging production-ready connectivity into **actionable engineering goals**:

- **3 lines of code to connect and communicate**: Start node → Join Realm → Send/receive messages (→ [5-minute Quickstart](docs/en/getting-started/quickstart.md) / [Join Your First Realm](docs/en/getting-started/first-realm.md))
- **Automatic connection fallback**: Direct connect → Hole punch → Relay (no business configuration needed) (→ [NAT Traversal](docs/en/how-to/nat-traversal.md) / [Using Relay](docs/en/how-to/use-relay.md))
- **Observable and explainable**: One diagnostic report answers "why can't connect/why slow/why unstable" (→ [Local Introspection](docs/en/how-to/introspection.md) / [Troubleshooting](docs/en/how-to/troubleshooting.md) / [Observability](docs/en/how-to/observability.md))

---

## 🌠 The Big Picture

DeP2P aims to become the **network foundation connecting Web3 and the real world**. We want to eliminate connection boundaries, enabling systems to span cloud, edge, devices, blockchain, and global networks:

- **Decentralized Applications**: Enable every application to have its own private network with global reachability
- **AI and Agent Networks**: Enable intelligent agents to interconnect and collaborate like function calls
- **Edge and IoT**: Maintain "reachable, controllable, explainable" in unstable networks
- **Cross-Regional Collaboration**: Stable interconnection across multiple countries, networks, and carriers
- **Open Infrastructure**: Make every node both a user and part of the network

We believe the future network is not "more servers", but "more nodes". What DeP2P does is make every node part of the world.

---

## 🆚 Why Choose DeP2P?

### 5 Major Pain Points of Traditional P2P Libraries

| Pain Point | Traditional Solution | DeP2P Solution |
|------------|---------------------|----------------|
| **Complex API** | Configure Host, Transport, Muxer, Security... | `realm.Messaging().Send(ctx, nodeID, data)` 3-step flow |
| **Network Pollution** | Routing table filled with unrelated nodes | Realm isolation, only discover nodes in same business |
| **Cold Start Difficulty** | Need to build all infrastructure | DHT authoritative directory + known_peers direct connect |
| **Unclear Status** | Don't know if node is offline/crashed/unstable | Multi-layer disconnect detection + witness mechanism + reconnect grace |
| **Resource Out of Control** | Connection count explodes, resources exhausted | Watermark control + important connection protection |

### Comparison with Other P2P Libraries

| Dimension | libp2p | iroh | **DeP2P** |
|-----------|--------|------|-----------|
| **API Simplicity** | ⚠️ Complex configuration | ⚠️ Many concepts | **✅ Minimal API** |
| **Business Isolation** | ❌ No native support | ⚠️ Manual implementation | **✅ Realm Isolation** |
| **Connection Reliability** | ⚠️ Manual configuration | ⚠️ Manual configuration | **✅ Automatic Fallback** |
| **Disconnect Detection** | ⚠️ Self-implementation | ⚠️ Self-implementation | **✅ Multi-layer + Witness** |
| **Zero-Config Startup** | ❌ Requires configuration | ⚠️ Requires configuration | **✅ Ready to Use** |

---

## ✨ Core Features

| Feature | Description |
|---------|-------------|
| **Minimal API** | One line of code to send messages, no complex component configuration |
| **Identity-First** | Connection target is NodeID (public key), not IP address |
| **Realm Isolation** | Independent business networks, preventing node pollution |
| **Smart Connection** | Automatic NAT traversal, address discovery, transparent relay fallback |
| **Multi-layer Disconnect Detection** | QUIC heartbeat + reconnect grace + witness mechanism + flapping suppression |
| **DHT Authoritative Model** | DHT stores signed PeerRecord, Relay as cache acceleration |
| **Connection Management** | Watermark control + important connection protection + automatic pruning |
| **QUIC-First** | Modern transport protocol with built-in encryption and multiplexing |
| **Zero-Config Startup** | Sensible defaults, ready to use |

---

## 🚀 Quick Start

### System Requirements

- **Go**: 1.21+
- **Git**: For version control

### Installation

```bash
go get github.com/dep2p/go-dep2p
```

### 30-Second Quickstart: 3-Step Flow

```go
package main

import (
    "context"
    "fmt"
    "log"
    
    "github.com/dep2p/go-dep2p"
    "github.com/dep2p/go-dep2p/pkg/types"
)

func main() {
    ctx := context.Background()
    
    // Step 1: Create and start node (system layer auto-ready)
    node, err := dep2p.New(ctx, dep2p.WithPreset(dep2p.PresetDesktop))
    if err != nil {
        log.Fatalf("Failed to create node: %v", err)
    }
    if err := node.Start(ctx); err != nil {
        log.Fatalf("Failed to start node: %v", err)
    }
    defer node.Close()
    
    fmt.Printf("Node ID: %s\n", node.ID())
    
    // Step 2: Join business network (required!)
    realm, err := node.Realm("my-first-realm")
    if err != nil {
        log.Fatalf("Failed to get Realm: %v", err)
    }
    if err := realm.Join(ctx); err != nil {
        log.Fatalf("Failed to join Realm: %v", err)
    }
    
    // Step 3: Use business API
    messaging := realm.Messaging()
    // messaging.Send(ctx, peerID, "/my/protocol/1.0", []byte("Hello!"))
    
    fmt.Println("Node ready, can start communicating!")
}
```

**This is DeP2P's simplicity**:
- ✅ **3 lines of code to establish connection**: Start node → Join Realm → Send message
- ✅ **Automatically handles complex details**: NAT traversal, address discovery, relay fallback
- ✅ **Identity-first**: Only need NodeID, no need to care about IP address

### Cloud Server Deployment

Recommended configuration for cloud servers:

```go
// Cloud server: Use known_peers direct connect + trust STUN addresses
node, err := dep2p.New(ctx,
    dep2p.WithPreset(dep2p.PresetServer),
    dep2p.WithKnownPeers([]dep2p.KnownPeer{
        {PeerID: "12D3KooW...", Addrs: []string{"/ip4/1.2.3.4/udp/4001/quic-v1"}},
    }),
    dep2p.WithTrustSTUNAddresses(true),  // Skip inbound verification, accelerate startup
)
if err != nil {
    log.Fatalf("Failed to create node: %v", err)
}
if err := node.Start(ctx); err != nil {
    log.Fatalf("Failed to start node: %v", err)
}
```

> 📖 **Detailed Configuration**: [Configuration Guide](docs/configuration.md)

### More Examples

| Example | Difficulty | Description |
|---------|-----------|-------------|
| [Basic Example](examples/basic/) | ⭐ | Simplest node creation |
| [Echo Example](examples/echo/) | ⭐⭐ | Learn stream communication |
| [Chat Example](examples/chat/) | ⭐⭐⭐ | LAN chat application |
| [Chat Public](examples/chat_public/) | ⭐⭐⭐⭐ | Public network three-node chat |
| [Relay Example](examples/relay/) | ⭐⭐⭐⭐ | NAT traversal and relay |

---

## 🏗️ Technical Architecture

### Three-Layer Architecture

DeP2P adopts a three-layer architecture design, clearly separating system foundation, business isolation, and application protocols:

```
┌─────────────────────────────────────────────────────────────────────┐
│  Layer 3: Application Protocol Layer                                │
│  ─────────────────────────────────────────────────────────────────  │
│  • Messaging / PubSub / Discovery / Streams                          │
│  • Protocol prefix: /dep2p/app/*                                    │
│  • [!] Must join Realm before use                                   │
├─────────────────────────────────────────────────────────────────────┤
│  Layer 2: Realm Layer (Business Isolation)                          │
│  ─────────────────────────────────────────────────────────────────  │
│  • Business isolation, member management, access control (PSK auth) │
│  • Protocol prefix: /dep2p/realm/*                                  │
│  • [*] User explicitly joins, strict single Realm                   │
├─────────────────────────────────────────────────────────────────────┤
│  Layer 1: System Foundation Layer                                  │
│  ─────────────────────────────────────────────────────────────────  │
│  • Transport / Security / DHT / Relay / NAT / Bootstrap            │
│  • Protocol prefix: /dep2p/sys/*                                    │
│  • [~] Node startup auto-ready, user unaware                        │
└─────────────────────────────────────────────────────────────────────┘
```

| Layer | Responsibility | Characteristics |
|-------|---------------|-----------------|
| **Layer 3** | Provides business communication capabilities | Must join Realm first |
| **Layer 2** | Business isolation and member management | User explicitly joins, PSK authentication |
| **Layer 1** | P2P network infrastructure | Node startup auto-ready, user unaware |

### Design Goals

| Priority | Goal | Acceptance Criteria |
|----------|------|---------------------|
| **P0 Core** | Simplicity | 3 lines of code to establish connection |
| **P0 Core** | Reliability | 95%+ connection success rate (direct→punch→relay) |
| **P1 Important** | Security | End-to-end encryption, identity unforgeable |
| **P1 Important** | Modularity | Each module independently testable and replaceable |

> 📖 **Detailed Architecture**: [Architecture Overview](design/03_architecture/) | [Design Decisions](design/01_context/decisions/)

---

## 💎 Business Value: Web3 Infrastructure Network Layer

DeP2P is not just a P2P library, but the **core network layer of Web3 infrastructure**.

### Three Core Scenarios

```
┌─────────────────────────────────────────────────────────────────────┐
│                    DeP2P Business Value Positioning                 │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  🔗 Blockchain Networks                                             │
│  ──────────────                                                      │
│  • Transaction broadcast (PubSub + Gossip)                          │
│  • Block sync (multi-source parallel + resume)                     │
│  • Consensus communication (low-latency direct + relay fallback)  │
│  • Network isolation (mainnet/testnet Realm separation)            │
│                                                                      │
│  💾 Decentralized Storage                                           │
│  ──────────────                                                      │
│  • File chunking → Content addressing (DHT)                        │
│  • Multi-source download → Resume                                  │
│  • Merkle Proof integrity verification                              │
│                                                                      │
│  📡 PCDN Content Distribution                                        │
│  ──────────────                                                      │
│  • Software download (extremely high P2P ROI)                       │
│  • Static sites (Web3 DApp frontend)                                │
│  • Video on-demand (HLS/DASH chunk acceleration)                    │
│  • Live streaming (PubSub + tree topology)                          │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

### Four PCDN Forms

| Form | Characteristics | P2P ROI | DeP2P Solution |
|------|----------------|---------|----------------|
| **Software Download** | Large files, strong consistency | ⭐⭐⭐ Very High | Chunk exchange + multi-source parallel |
| **Static Sites** | Versioned, first-screen sensitive | ⭐⭐ Medium | Manifest + Merkle |
| **Video On-Demand** | Chunked, hotspot aggregation | ⭐⭐⭐ High | Chunk index + preload |
| **Live Streaming** | Ultra-low latency, real-time fanout | ⭐⭐⭐ High | PubSub + tree topology |

### Business Model Support

| Business Model | Capabilities Provided by DeP2P |
|----------------|-------------------------------|
| **Bandwidth Incentive** | Upload/download byte counting, provides data foundation for Token incentives |
| **Storage Incentive** | Content indexing protocol, proves "what data I stored" |
| **CDN Cost Optimization** | P2P offloading, reduces Origin/CDN bandwidth costs |
| **Decentralized Hosting** | Static site P2P distribution, no centralized server needed |

### Recommended Implementation Path

| Phase | Goal | Description |
|-------|------|-------------|
| **Phase 1** | Software Download | Easiest to verify P2P ROI, similar to BitTorrent |
| **Phase 2** | Static Sites | Manifest + Chunk, integrate with Web3 site protocol |
| **Phase 3** | Video On-Demand | Add chunk popularity, preload strategy |
| **Phase 4** | Live Streaming | PubSub + tree topology + strict latency control |

---

## 🌟 Use Cases

### Recommended Scenarios

| Scenario | DeP2P Advantages |
|----------|-----------------|
| **Blockchain / DeFi** | Realm isolation + node discovery + transaction broadcast |
| **Chain Games / GameFi** | Low latency + business isolation + state sync |
| **Decentralized Storage** | Multi-source download + content addressing + resume |
| **Instant Messaging** | Simple API + reliable transport + end-to-end encryption |
| **Collaborative Editing** | Real-time sync + conflict resolution + offline support |

### Suitability Assessment

| Assessment | Scenario | Description |
|------------|----------|-------------|
| ✅ **Highly Suitable** | Blockchain, distributed storage, instant messaging, collaborative editing | DeP2P core design goals |
| ⚠️ **Partially Suitable** | Video on-demand, IoT | Acceptable latency, need to evaluate resource usage |
| ❌ **Not Suitable** | Ultra-low latency streaming (<100ms), real-time video conferencing, cloud gaming | Requires unreliable transport (WebRTC) |

---

## 📋 Documentation

| Resource | Description |
|----------|-------------|
| 📖 [**Documentation Center**](docs/en/README.md) | Complete English documentation with tutorials, concepts, API reference |
| 🎯 [5-Minute Quickstart](docs/en/getting-started/quickstart.md) | Quick start tutorial from scratch |
| 💡 [Example Code](examples/) | Complete examples from simple to complex |
| ⚙️ [Configuration Guide](docs/configuration.md) | Preset configurations, connectivity optimization, deployment tips |
| 🏗️ [Design Documents](design/README.md) | Architecture decisions, protocol specifications (for contributors) |

---

## 🤝 Contributing & Community

We welcome community contributions!

### Quick Start Contributing

```bash
# 1. Fork and clone repository
git clone https://github.com/your-username/go-dep2p.git

# 2. Set up development environment
cd go-dep2p
go mod tidy

# 3. Run tests
go test ./...

# 4. Submit changes
git commit -S -m "feat: your contribution"
git push origin your-branch
```

### Get Help

| Channel | Purpose |
|---------|---------|
| [GitHub Issues](https://github.com/dep2p/go-dep2p/issues) | Bug reports, feature requests |
| [GitHub Discussions](https://github.com/dep2p/go-dep2p/discussions) | Questions, usage help |
| [Contributing Guide](docs/en/contributing/README.md) | How to contribute |

---

## 🔧 FAQ

<details>
<summary><b>Node startup failed</b></summary>

**Common cause**: Port already in use

```bash
# Check port usage
netstat -tulpn | grep :4001

# Solution: Use auto-assigned port
node, _ := dep2p.New(ctx, dep2p.WithPreset(dep2p.PresetDesktop))
_ = node.Start(ctx)
```
</details>

<details>
<summary><b>ErrNotMember error</b></summary>

**Cause**: Calling business API without joining Realm

```go
// ❌ Wrong: Calling business API without joining Realm
// err == ErrNotMember

// ✅ Correct: Get Realm and join first
realm, _ := node.Realm("my-realm")
_ = realm.Join(ctx)
err := realm.Messaging().Send(ctx, peerID, "/my/protocol/1.0", data)
```
</details>

<details>
<summary><b>Connection timeout</b></summary>

**Possible causes**:
1. Firewall blocking connection
2. NAT traversal failed
3. Incorrect address format

**Solutions**:
- Check network and firewall settings
- Enable Relay service
- Use `ShareableAddrs()` to get complete address

> 📖 **Detailed Troubleshooting**: [Troubleshooting](docs/en/how-to/troubleshooting.md) | [Error Codes](docs/en/reference/error-codes.md)
</details>

---

## 📄 License

This project is open source under the MIT License - see the [LICENSE](LICENSE) file for details.

---

<div align="center">

### Make P2P Connections as Simple as Function Calls

[Quick Start](#-quick-start) • [Documentation Center](docs/en/README.md) • [Design Docs](design/README.md)

Made with ❤️ by the DeP2P Team

</div>

