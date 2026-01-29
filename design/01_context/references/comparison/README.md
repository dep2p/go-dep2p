# 跨产品对比分析索引

> **分析产品**: iroh、go-libp2p、torrent  
> **分析日期**: 2026-01-11

---

## 1. 概述

本目录包含对三个 P2P 产品的横向对比分析，为 DeP2P 的设计决策提供参考。

---

## 2. 目录结构

```
comparison/
├── README.md                           # 本文件 - 对比分析索引
├── architecture/                       # 架构层对比
│   ├── README.md                       # 架构层索引
│   └── 01-overall.md                   # 整体架构对比
├── protocol/                           # 协议层对比
│   ├── README.md                       # 协议层索引
│   └── 01-protocol-design.md           # 协议设计对比
├── transport/                          # 传输层对比
│   ├── README.md                       # 传输层索引
│   ├── 01-transport-protocols.md       # 传输协议对比
│   ├── 02-connection-management.md     # 连接管理对比
│   └── 03-relay.md                     # Relay 设计对比
├── network/                            # 网络层对比
│   ├── README.md                       # 网络层索引
│   ├── 01-discovery.md                 # 节点发现对比
│   └── 02-nat-traversal.md             # NAT 穿透对比
├── security/                           # 安全层对比
│   ├── README.md                       # 安全层索引
│   ├── 01-identity.md                  # 身份认证对比
│   └── 02-encryption.md                # 加密与安全传输对比
└── interface/                          # 接口层对比
    ├── README.md                       # 接口层索引
    └── 01-api-design.md                # API 设计对比
```

---

## 2. 对比文档汇总

### 2.1 架构层对比

| 文档 | 说明 | 关键内容 |
|------|------|----------|
| [01-overall.md](architecture/01-overall.md) | 整体架构对比 | 分层模型、模块组织、设计模式 |

### 2.2 协议层对比

| 文档 | 说明 | 关键内容 |
|------|------|----------|
| [01-protocol-design.md](protocol/01-protocol-design.md) | 协议设计对比 | 协议标识、协商、消息格式、扩展 |

### 2.3 传输层对比

| 文档 | 说明 | 关键内容 |
|------|------|----------|
| [01-transport-protocols.md](transport/01-transport-protocols.md) | 传输协议对比 | TCP、QUIC、WebSocket、QUIC 特性 |
| [02-connection-management.md](transport/02-connection-management.md) | 连接管理对比 | 连接池、生命周期、保活、复用 |
| [03-relay.md](transport/03-relay.md) | Relay 设计对比 | 架构、协议、特性 |

### 2.4 网络层对比

| 文档 | 说明 | 关键内容 |
|------|------|----------|
| [01-discovery.md](network/01-discovery.md) | 节点发现对比 | DHT、DNS、mDNS、Tracker |
| [02-nat-traversal.md](network/02-nat-traversal.md) | NAT 穿透对比 | NAT 类型、打洞、STUN |

### 2.5 安全层对比

| 文档 | 说明 | 关键内容 |
|------|------|----------|
| [01-identity.md](security/01-identity.md) | 身份认证对比 | 身份模型、密钥、验证 |
| [02-encryption.md](security/02-encryption.md) | 加密传输对比 | TLS、Noise、密钥交换、会话安全 |

### 2.6 接口层对比

| 文档 | 说明 | 关键内容 |
|------|------|----------|
| [01-api-design.md](interface/01-api-design.md) | API 设计对比 | 入口、配置、连接、事件、错误 |

---

## 3. 产品概览

| 产品 | 语言 | 定位 | 核心特性 |
|------|------|------|----------|
| **iroh** | Rust | 现代 P2P 网络库 | QUIC、Relay、Home Relay |
| **go-libp2p** | Go | 模块化 P2P 协议栈 | 协议抽象、多传输、DHT |
| **torrent** | Go | BitTorrent 库 | DHT、Tracker、流式传输 |

---

## 4. 关键对比结论

### 4.1 Relay 设计

| 产品 | 特点 | DeP2P 启示 |
|------|------|------------|
| **iroh** | Home Relay 概念 | 采用 Home Relay |
| **go-libp2p** | 协议化、去中心化 | 参考协议设计 |

**DeP2P 决策**: 统一 Relay（v2.0 缓存加速层）

### 4.2 NAT 穿透

| 产品 | 特点 | DeP2P 启示 |
|------|------|------------|
| **iroh** | QAD 探测 | 现代化检测 |
| **go-libp2p** | AutoNAT、DCUtR | 协调机制 |

**DeP2P 决策**: NAT 检测 + 自动升级

### 4.3 节点发现

| 产品 | 特点 | DeP2P 启示 |
|------|------|------------|
| **iroh** | DNS + 多机制 | 组合使用 |
| **go-libp2p** | DHT + 命名空间 | 服务发现 |
| **torrent** | Tracker + DHT | 多机制并行 |

**DeP2P 决策**: Rendezvous + DHT + mDNS

### 4.4 身份认证

| 产品 | 特点 | DeP2P 启示 |
|------|------|------------|
| **iroh** | 公钥即身份 | 简洁设计 |
| **go-libp2p** | Multihash、Noise | 安全协议 |

**DeP2P 决策**: NodeID = SHA256(PublicKey) + Noise XX + PSK

### 4.5 架构模式

| 项目 | 架构模式 | 核心理念 |
|------|----------|----------|
| **go-libp2p** | 技术层次栈 | 按网络协议栈分层，可插拔组件 |
| **iroh** | 功能聚焦 | 极简核心 + 独立协议库组合 |
| **torrent** | 扁平模块 | 围绕 BitTorrent 协议的功能模块 |

**DeP2P 启示**: 
- 参考 go-libp2p 的技术层次分层
- 借鉴 iroh 的 MagicSock 传输抽象
- Realm 作为协议层模块而非独立"业务域"

详见：[整体架构对比](architecture/01-overall.md)

---

## 5. 与业界对标能力矩阵

### 5.1 核心能力对比

| 能力 | libp2p | iroh | DeP2P | 说明 |
|------|:------:|:----:|:-----:|------|
| 模块化传输 | ✅ | ⚠️ | ✅ | libp2p 高度模块化 |
| QUIC 支持 | ✅ | ✅ | ✅ | 现代传输协议 |
| NAT 穿透 | ✅ | ✅ | ✅ | 核心能力 |
| 中继服务 | ✅ | ✅ | ✅ | Relay 保底 |
| DHT 发现 | ✅ | ✅ | ✅ | Kademlia |
| PubSub | ✅ | ✅ | ✅ | GossipSub |
| 资源管理 | ✅ | ❌ | ✅ | ResourceManager |
| **业务隔离 (Realm)** | ⚠️ | ❌ | ✅✅ | **DeP2P 核心创新** |
| **跨域网关** | ❌ | ❌ | ✅ | **DeP2P 独有** |

> **图例**: ✅ 完整支持 | ⚠️ 部分支持 | ❌ 不支持

### 5.2 顶级 P2P 项目共同特征

| 特征 | libp2p | iroh | DeP2P |
|------|--------|------|-------|
| **核心理念** | 模块化协议栈 | 极简连接 | **隔离化 P2P 网络** |
| **架构风格** | 可插拔组件 | 功能聚焦 | **协议驱动 + 隔离域** |
| **扩展方式** | 接口注入 | 组合协议 | **协议注册 + 域扩展** |
| **独特价值** | 通用性 | 穿透性能 | **Realm 隔离** |

详细对比请参阅：[整体架构对比](architecture/01-overall.md#11-与业界对标能力矩阵)

---

## 6. DeP2P 创新点

基于竞品分析，DeP2P 的创新设计：

| 方面 | 竞品做法 | DeP2P 创新 |
|------|----------|------------|
| **Relay 架构** | 单层 | 统一 Relay（v2.0 缓存加速层） |
| **域隔离** | 无 | Realm + PSK |
| **DHT 权威** | 无 | DHT 作为权威目录 |
| **成员管理** | 无 | Realm 成员验证 |

---

## 7. 相关文档

- [个体产品分析](../individual/README.md)
- [ADR-0003: Relay 优先连接](../../decisions/ADR-0003-relay-first-connect.md)
- [ADR-0010: Relay 明确配置](../../decisions/ADR-0010-relay-explicit-config.md)

---

**最后更新**：2026-01-13
