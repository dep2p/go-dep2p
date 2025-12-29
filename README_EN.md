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

📖 **[English](README_EN.md) | [中文](README.md)**

[![Go Version](https://img.shields.io/badge/Go-1.21+-blue.svg)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20macOS%20%7C%20Windows-lightgrey.svg)]()
[![Status](https://img.shields.io/badge/Status-Active-green.svg)]()

<sub>📊 Codebase: 148K lines of Go code</sub>

</div>

---

## 📑 Table of Contents

- [Core Vision](#-core-vision)
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

## 🆚 Why Choose DeP2P?

### 5 Major Pain Points of Traditional P2P Libraries

| Pain Point | Traditional Solution | DeP2P Solution |
|------------|---------------------|----------------|
| **Complex API** | Configure Host, Transport, Muxer, Security... | `realm.Messaging().Send(ctx, nodeID, data)` 3-step flow |
| **Network Pollution** | Routing table filled with unrelated nodes | Realm isolation, only discover nodes in same business |
| **Cold Start Difficulty** | Need to build all infrastructure | Shared DHT/Relay, isolated by Realm |
| **Unclear Status** | Don't know if node is offline/crashed/unstable | Three-state model + graceful shutdown + heartbeat |
| **Resource Out of Control** | Connection count explodes, resources exhausted | Watermark control + important connection protection |

### Comparison with Other P2P Libraries

| Dimension | libp2p | iroh | **DeP2P** |
|-----------|--------|------|-----------|
| **API Simplicity** | ⚠️ Complex configuration | ⚠️ Many concepts | **✅ Minimal API** |
| **Business Isolation** | ❌ No native support | ⚠️ Manual implementation | **✅ Realm Isolation** |
| **Connection Reliability** | ⚠️ Manual configuration | ⚠️ Manual configuration | **✅ Automatic Fallback** |
| **Node Status Awareness** | ⚠️ Self-implementation | ⚠️ Self-implementation | **✅ Three-State Model** |
| **Zero-Config Startup** | ❌ Requires configuration | ⚠️ Requires configuration | **✅ Ready to Use** |

---

## ✨ Core Features

| Feature | Description |
|---------|-------------|
| **Minimal API** | One line of code to send messages, no complex component configuration |
| **Identity-First** | Connection target is NodeID (public key), not IP address |
| **Realm Isolation** | Independent business networks, preventing node pollution |
| **Smart Connection** | Automatic NAT traversal, address discovery, transparent relay fallback |
| **Node Status Awareness** | Three-state model + heartbeat detection, transparent network status |
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
    
    // Step 1: Start node (system layer auto-ready)
    node, err := dep2p.StartNode(ctx, dep2p.WithPreset(dep2p.PresetDesktop))
    if err != nil {
        log.Fatalf("Failed to start node: %v", err)
    }
    defer node.Close()
    
    fmt.Printf("Node ID: %s\n", node.ID())
    
    // Step 2: Join business network (required!)
    realmKey := types.GenerateRealmKey()
    realm, err := node.JoinRealmWithKey(ctx, "my-first-realm", realmKey)
    if err != nil {
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

> 📖 **Detailed Architecture**: [Architecture Overview](design/architecture/overview.md) | [Three-Layer Architecture](design/architecture/layers.md)

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

### Navigation by Role

| Role | Recommended Path |
|------|------------------|
| **User/Developer** | [Quick Start](#-quick-start) → [5-Minute Quickstart](docs/en/getting-started/quickstart.md) → [Tutorials](docs/en/tutorials/) |
| **Architect** | [Architecture Overview](design/architecture/overview.md) → [Protocol Specifications](design/protocols/README.md) → [ADRs](design/adr/) |
| **Contributor** | [Development Setup](docs/en/contributing/development-setup.md) → [Code Style](docs/en/contributing/code-style.md) |

### Core Documents

| Document | Description |
|----------|-------------|
| [What is DeP2P](docs/en/concepts/what-is-dep2p.md) | Core vision, design goals, and use cases |
| [Core Concepts](docs/en/concepts/core-concepts.md) | Identity-first, three-layer architecture, Realm |
| [Architecture Overview](design/architecture/overview.md) | Detailed overall architecture design |
| [Design Documentation](design/README.md) | Architecture decisions, protocol specifications, implementation details |
| [API Reference](docs/en/reference/api/node.md) | Complete API documentation |
| [Example Collection](examples/README.md) | Progressive example code |

### Documentation Structure

```
dep2p.git/
├── README.md              # 📍 This file - Project overview
├── README_EN.md           # 📍 English version
├── design/                # Design docs (for architects/contributors)
│   ├── architecture/      # Architecture design
│   ├── protocols/        # Protocol specifications
│   ├── adr/               # Architecture Decision Records
│   └── invariants/       # System invariants
├── docs/                  # User docs (for developers)
│   ├── zh/                # Chinese documentation
│   └── en/                # English documentation
└── examples/              # Example code
```

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
node, _ := dep2p.StartNode(ctx, dep2p.WithPreset(dep2p.PresetDesktop))
```
</details>

<details>
<summary><b>ErrNotMember error</b></summary>

**Cause**: Calling business API without joining Realm

```go
// ❌ Wrong
err := node.Send(ctx, peerID, data) // err == ErrNotMember

// ✅ Correct: Join Realm first
realm, _ := node.JoinRealmWithKey(ctx, "my-realm", realmKey)
err := realm.Messaging().Send(ctx, peerID, data)
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

