# 📚 DeP2P Documentation

Welcome to DeP2P documentation center! This documentation will help you quickly find the information you need.

---

## 🚀 Quick Paths

Choose a path based on your needs:

| Path | Goal | Entry |
|------|------|-------|
| **🆕 Getting Started** | Get your first node running in 5 minutes | [Getting Started](getting-started/) |
| **📖 Advanced Learning** | Deep dive into DeP2P | [Concepts](concepts/) → [Tutorials](tutorials/) |
| **💎 Business Value** | Understand use cases and value | [Use Cases](concepts/use-cases.md) |
| **🔧 Production** | Deployment and operations | [How-To](how-to/) → [Reference](reference/) |
| **🤝 Contributing** | Code/documentation contributions | [Contributing](contributing/) |

---

## 📖 Documentation Structure

### Layer 1: Getting Started

**Goal**: Get your first node running in 5 minutes

| Document | Description |
|----------|-------------|
| [installation.md](getting-started/installation.md) | Installation (go get + dependencies) |
| [quickstart.md](getting-started/quickstart.md) | 5-minute quickstart (minimal runnable example) |
| [first-node.md](getting-started/first-node.md) | Create your first node |
| [first-realm.md](getting-started/first-realm.md) | Join your first Realm |
| [faq.md](getting-started/faq.md) | FAQ (bootstrap/address/JoinRealm) |

---

### Layer 2: Concepts

**Goal**: Help users "understand" rather than just "follow along"

| Document | Description |
|----------|-------------|
| [what-is-dep2p.md](concepts/what-is-dep2p.md) | What is DeP2P (vision + positioning) |
| [core-concepts.md](concepts/core-concepts.md) | Core concepts overview (three-layer architecture/Realm/identity-first) |
| [architecture-overview.md](concepts/architecture-overview.md) | Architecture overview (user-oriented, not implementation details) |
| [use-cases.md](concepts/use-cases.md) | **Use cases & business value (Blockchain/Storage/PCDN)** |
| [comparison.md](concepts/comparison.md) | Comparison with libp2p/iroh |

---

### Layer 3: Tutorials

**Goal**: End-to-end runnable, "follow along" to succeed

| Document | Description |
|----------|-------------|
| [01-hello-world.md](tutorials/01-hello-world.md) | Hello World (two-node connection) |
| [02-secure-chat.md](tutorials/02-secure-chat.md) | Secure chat application |
| [03-cross-nat-connect.md](tutorials/03-cross-nat-connect.md) | Cross-NAT connection (Relay-first) |
| [04-realm-application.md](tutorials/04-realm-application.md) | Building Realm applications |

---

### Layer 4: How-To Guides

**Goal**: Solve specific problems, "I want to do X"

| Document | Description |
|----------|-------------|
| [bootstrap-network.md](how-to/bootstrap-network.md) | How to bootstrap a network |
| [share-address.md](how-to/share-address.md) | How to get/share connectable addresses |
| [use-relay.md](how-to/use-relay.md) | How to use relay |
| [nat-traversal.md](how-to/nat-traversal.md) | NAT traversal configuration |
| [peer-discovery.md](how-to/peer-discovery.md) | Peer discovery |
| [custom-protocols.md](how-to/custom-protocols.md) | Custom protocols |
| [observability.md](how-to/observability.md) | Observability (logs/metrics) |
| [troubleshooting.md](how-to/troubleshooting.md) | Troubleshooting |

---

### Layer 5: API Reference

**Goal**: Searchable, stable, structured

#### API Documentation

| Document | Description |
|----------|-------------|
| [node.md](reference/api/node.md) | Node API |
| [endpoint.md](reference/api/endpoint.md) | Endpoint API |
| [realm.md](reference/api/realm.md) | Realm API |
| [messaging.md](reference/api/messaging.md) | Messaging API |

#### Configuration & Specifications

| Document | Description |
|----------|-------------|
| [configuration.md](reference/configuration.md) | Configuration reference |
| [protocol-ids.md](reference/protocol-ids.md) | Protocol ID naming conventions |
| [presets.md](reference/presets.md) | Preset configurations (Desktop/Mobile/Server) |
| [glossary.md](reference/glossary.md) | Glossary |

---

### Contributing

**Goal**: Guide developers to contribute

| Document | Description |
|----------|-------------|
| [README.md](contributing/README.md) | How to contribute (code/documentation/Issue) |
| [development-setup.md](contributing/development-setup.md) | Development environment setup |
| [code-style.md](contributing/code-style.md) | Code style guide |
| [design-docs.md](contributing/design-docs.md) | Design documents (→ link to [design/README.md](../../design/README.md)) |

---

## 🎯 Recommended Learning Paths

### Path 1: Quick Start (Beginner)

1. [Installation](getting-started/installation.md)
2. [5-minute Quickstart](getting-started/quickstart.md)
3. [Create Your First Node](getting-started/first-node.md)
4. [Join Your First Realm](getting-started/first-realm.md)
5. [Hello World Tutorial](tutorials/01-hello-world.md)

### Path 2: Deep Understanding (Advanced)

1. [What is DeP2P](concepts/what-is-dep2p.md)
2. [Core Concepts Overview](concepts/core-concepts.md)
3. [Architecture Overview](concepts/architecture-overview.md)
4. [Secure Chat Application](tutorials/02-secure-chat.md)
5. [Cross-NAT Connection](tutorials/03-cross-nat-connect.md)
6. [Building Realm Applications](tutorials/04-realm-application.md)

### Path 3: Production Practice (Expert)

1. [How to Bootstrap a Network](how-to/bootstrap-network.md)
2. [How to Use Relay](how-to/use-relay.md)
3. [NAT Traversal Configuration](how-to/nat-traversal.md)
4. [Observability](how-to/observability.md)
5. [API Reference](reference/api/node.md)

---

## 🔗 Related Resources

- **Design Documents**: See [design/](../../design/README.md) directory
- **Examples**: See [examples/](../../examples/README.md) directory
- **Project Overview**: See [_docs/00-overview/](../../_docs/00-overview/README.md)
- **中文文档**: [Chinese Version](../zh/)

---

## 📝 Documentation Feedback

If you find errors or have suggestions for improvement, please:
- Submit an Issue
- Submit a Pull Request
- Refer to [Contributing Guide](contributing/README.md)

