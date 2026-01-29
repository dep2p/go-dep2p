# 📚 DeP2P 文档导航

欢迎来到 DeP2P 文档中心！本文档将帮助你快速找到所需的信息。

---

## 🚀 快速路径

根据你的需求，选择以下路径：

| 路径 | 目标 | 入口 |
|------|------|------|
| **🆕 新手入门** | 5 分钟跑通第一个节点 | [Getting Started](getting-started/) |
| **📖 进阶学习** | 深入理解 DeP2P | [Concepts](concepts/) → [Tutorials](tutorials/) |
| **💎 商业价值** | 了解应用场景和价值 | [应用场景](concepts/use-cases.md) |
| **🔧 生产实践** | 部署和运维指南 | [How-To](how-to/) → [Reference](reference/) |
| **🤝 参与贡献** | 代码/文档贡献 | [Contributing](contributing/) |

---

## 📖 文档结构

### 第一层：用户入门（Getting Started）

**目标**：5 分钟跑通第一个节点

| 文档 | 说明 |
|------|------|
| [installation.md](getting-started/installation.md) | 安装（go get + 依赖说明） |
| [quickstart.md](getting-started/quickstart.md) | 5 分钟上手（最小可运行示例） |
| [first-node.md](getting-started/first-node.md) | 创建第一个节点 |
| [first-realm.md](getting-started/first-realm.md) | 加入第一个 Realm |
| [faq.md](getting-started/faq.md) | 常见问题（bootstrap/地址/JoinRealm） |

---

### 第二层：概念解释（Concepts）

**目标**：让用户"理解"而非"照做"

| 文档 | 说明 |
|------|------|
| [what-is-dep2p.md](concepts/what-is-dep2p.md) | DeP2P 是什么（愿景 + 定位） |
| [core-concepts.md](concepts/core-concepts.md) | 核心概念总纲（三层架构/Realm/身份第一性） |
| [architecture-overview.md](concepts/architecture-overview.md) | 架构概览（面向用户理解，非实现细节） |
| [relay-architecture.md](concepts/relay-architecture.md) | Relay 中继架构（v2.0/v2.1） |
| [use-cases.md](concepts/use-cases.md) | **应用场景与商业价值（区块链/存储/PCDN）** |
| [comparison.md](concepts/comparison.md) | 与 libp2p/iroh/torrent 对比 |

---

### 第三层：教程（Tutorials）

**目标**：端到端可跑通，"照着做"就能成功

| 文档 | 说明 |
|------|------|
| [01-hello-world.md](tutorials/01-hello-world.md) | Hello World - 使用 `known_peers` 两节点互连 |
| [02-local-chat.md](tutorials/02-local-chat.md) | 局域网聊天 - mDNS 自动发现 + Realm 成员管理 |
| [03-cloud-deploy.md](tutorials/03-cloud-deploy.md) | 云服务器部署 - `known_peers` + `trust_stun_addresses` |
| [04-realm-chat.md](tutorials/04-realm-chat.md) | Realm 群聊 - 成员管理与断开检测 |
| [05-troubleshooting-live.md](tutorials/05-troubleshooting-live.md) | 实战故障排查 - P2P 日志分析框架 |

---

### 第四层：操作指南（How-To）

**目标**：解决具体问题，"我要做 X"

| 文档 | 说明 |
|------|------|
| [bootstrap-network.md](how-to/bootstrap-network.md) | 如何 Bootstrap 网络 |
| [share-address.md](how-to/share-address.md) | 如何获取/分享可连接地址 |
| [use-relay.md](how-to/use-relay.md) | 如何使用中继 |
| [nat-traversal.md](how-to/nat-traversal.md) | NAT 穿透配置 |
| [peer-discovery.md](how-to/peer-discovery.md) | 节点发现 |
| [custom-protocols.md](how-to/custom-protocols.md) | 自定义协议 |
| [introspection.md](how-to/introspection.md) | 本地自省接口 |
| [observability.md](how-to/observability.md) | 可观测性（日志/指标） |
| [troubleshooting.md](how-to/troubleshooting.md) | 故障排查 |
| [relay-deployment-models.md](how-to/relay-deployment-models.md) | Relay 部署模型 |

---

### 第五层：API 参考（Reference）

**目标**：可查、稳定、结构化

#### API 文档

| 文档 | 说明 |
|------|------|
| [node.md](reference/api/node.md) | Node API |
| [endpoint.md](reference/api/endpoint.md) | Endpoint API |
| [realm.md](reference/api/realm.md) | Realm API |
| [messaging.md](reference/api/messaging.md) | Messaging API |
| [liveness.md](reference/api/liveness.md) | Liveness API |
| [pubsub.md](reference/api/pubsub.md) | PubSub API |
| [streams.md](reference/api/streams.md) | Streams API |

#### 配置与规范

| 文档 | 说明 |
|------|------|
| [configuration.md](reference/configuration.md) | 配置项参考 |
| [relay-api.md](reference/relay-api.md) | Relay API 参考（v2.0/v2.1） |
| [protocol-ids.md](reference/protocol-ids.md) | 协议 ID 命名规范 |
| [presets.md](reference/presets.md) | 预设配置（Desktop/Mobile/Server） |
| [api-defaults.md](reference/api-defaults.md) | API 默认值 |
| [error-codes.md](reference/error-codes.md) | 错误码参考 |
| [glossary.md](reference/glossary.md) | 术语表 |

---

### 贡献者入口（Contributing）

**目标**：引导开发者参与贡献

| 文档 | 说明 |
|------|------|
| [README.md](contributing/README.md) | 如何贡献（代码/文档/Issue） |
| [development-setup.md](contributing/development-setup.md) | 开发环境搭建 |
| [code-style.md](contributing/code-style.md) | 代码规范 |
| [design-docs.md](contributing/design-docs.md) | 设计文档（→ 链接到 [design/README.md](../design/README.md)） |

---

## 🎯 推荐学习路径

### 路径 1：快速上手（新手）

1. [安装](getting-started/installation.md)
2. [5 分钟上手](getting-started/quickstart.md)
3. [创建第一个节点](getting-started/first-node.md)
4. [加入第一个 Realm](getting-started/first-realm.md)
5. [Hello World 教程](tutorials/01-hello-world.md)

### 路径 2：深入理解（进阶）

1. [DeP2P 是什么](concepts/what-is-dep2p.md)
2. [核心概念总纲](concepts/core-concepts.md)
3. [架构概览](concepts/architecture-overview.md)
4. [局域网聊天](tutorials/02-local-chat.md) - mDNS + Realm
5. [云服务器部署](tutorials/03-cloud-deploy.md) - known_peers + trust_stun
6. [Realm 群聊应用](tutorials/04-realm-chat.md) - 成员管理与断开检测

### 路径 3：生产实践（专家）

1. [如何 Bootstrap 网络](how-to/bootstrap-network.md)
2. [如何使用中继](how-to/use-relay.md)
3. [NAT 穿透配置](how-to/nat-traversal.md)
4. [可观测性](how-to/observability.md)
5. [API 参考](reference/api/node.md)

---

## 🔗 相关资源

- **设计文档**：详见 [design/../../design/README.md](../../design/README.md) 目录
- **示例代码**：详见 [examples/../../examples/README.md](../../examples/README.md) 目录
- **项目概览**：详见 [_docs/00-overview/../../_docs/00-overview/README.md](../../_docs/00-overview/README.md)
- **English Documentation**: [English Version](../../docs/en/)

---

## 📝 文档反馈

如果你发现文档有误或有改进建议，欢迎：
- 提交 Issue
- 提交 Pull Request
- 参考 [贡献指南](contributing/README.md)
