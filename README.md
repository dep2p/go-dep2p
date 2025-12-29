# DeP2P —— 让 P2P 连接像调用函数一样简单

<div align="center">

<pre>
████████╗   ███████╗  ██████╗    ██████╗   ██████╗ 
██╔═══██╗  ██╔════╝  ██╔══██╗  ██╔═══██╗  ██╔══██╗
██║   ██║  █████╗    ██████╔╝     ███╔╝   ██████╔╝
██║   ██║  ██╔══╝    ██╔═══╝    ███╔╝     ██╔═══╝ 
███████╔╝  ███████╗  ██║       ███████╗   ██║     
╚══════╝   ╚══════╝  ╚═╝       ╚══════╝   ╚═╝     
</pre>

**简洁、可靠、安全的 P2P 网络基础库（QUIC 优先）**  
**NodeID 直连 + Realm 隔离 + NAT 穿透/中继回退，开箱即用**

📖 **[English](README_EN.md) | 中文**

[![Go Version](https://img.shields.io/badge/Go-1.21+-blue.svg)](https://golang.org/)
[![License](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)
[![Platform](https://img.shields.io/badge/Platform-Linux%20%7C%20macOS%20%7C%20Windows-lightgrey.svg)]()
[![Status](https://img.shields.io/badge/Status-Active-green.svg)]()

<sub>📊 代码规模：14.8 万行 Go 代码</sub>

</div>

---

## 📑 目录

- [核心愿景](#-核心愿景)
- [为什么选择 DeP2P？](#-为什么选择-dep2p)
- [核心特性](#-核心特性)
- [快速开始](#-快速开始)
- [技术架构](#-技术架构)
- [商业价值](#-商业价值web3-基础设施的网络层)
- [适用场景](#-适用场景)
- [文档导航](#-文档导航)
- [贡献与社区](#-贡献与社区)
- [许可证](#-许可证)

---

## 🌌 核心愿景

```
┌─────────────────────────────────────────────────────────────────────┐
│                                                                     │
│         让 P2P 连接像调用函数一样简单：给一个 NodeID，发个消息             │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

> **NodeID** = 公钥身份（Base58 编码）。目标是“按身份连接”，而不是“按 IP/域名连接”。  
> **Realm** = 业务边界（多租户/多应用隔离）。不同 Realm 的节点互不可见，避免网络污染。

DeP2P 的愿景不是“再造一个 P2P 协议集合”，而是把生产可用的连接能力收敛成**可执行的工程目标**：

- **3 行代码入网可通信**：启动节点 → 加入 Realm → 发送/收消息（→ [5 分钟上手](docs/zh/getting-started/quickstart.md) / [加入第一个 Realm](docs/zh/getting-started/first-realm.md)）
- **连接链路自动回退**：直连 → 打洞 → 中继（无需业务介入配置）（→ [NAT 穿透](docs/zh/how-to/nat-traversal.md) / [使用中继](docs/zh/how-to/use-relay.md)）
- **可观测可解释**：用一份诊断报告回答“为什么连不上/为什么慢/为什么不稳定”（→ [本地自省接口](docs/zh/how-to/introspection.md) / [故障排查](docs/zh/how-to/troubleshooting.md) / [可观测性](docs/zh/how-to/observability.md)）

---

## 🆚 为什么选择 DeP2P？

### 传统 P2P 库的 5 大痛点

| 痛点 | 传统方案 | DeP2P 方案 |
|------|----------|-----------|
| **API 复杂** | 配置 Host, Transport, Muxer, Security... | `realm.Messaging().Send(ctx, nodeID, data)` 三步走 |
| **网络污染** | 路由表充满不相关节点 | Realm 隔离，只发现同业务节点 |
| **冷启动难** | 需自建所有基础设施 | 共享 DHT/中继，按 Realm 隔离 |
| **状态不明** | 不知道节点是离线/崩溃/不稳定 | 三态模型 + 优雅下线 + 心跳检测 |
| **资源失控** | 连接数暴涨，资源耗尽 | 水位线裁剪 + 重要连接保护 |

### 与其他 P2P 库对比

| 维度 | libp2p | iroh | **DeP2P** |
|------|--------|------|-----------|
| **API 简洁性** | ⚠️ 配置复杂 | ⚠️ 概念较多 | **✅ 极简 API** |
| **业务隔离** | ❌ 无原生支持 | ⚠️ 需手动实现 | **✅ Realm 隔离** |
| **连接可靠性** | ⚠️ 需手动配置 | ⚠️ 需手动配置 | **✅ 自动回退** |
| **节点状态感知** | ⚠️ 需自行实现 | ⚠️ 需自行实现 | **✅ 三态模型** |
| **零配置启动** | ❌ 需要配置 | ⚠️ 需要配置 | **✅ 开箱即用** |

---

## ✨ 核心特性

| 特性 | 说明 |
|------|------|
| **极简 API** | 一行代码发消息，无需配置复杂组件 |
| **身份优先** | 连接目标是 NodeID（公钥），而非 IP 地址 |
| **Realm 隔离** | 业务网络独立，避免节点污染 |
| **智能连接** | 自动 NAT 穿透、地址发现、透明中继回退 |
| **节点状态感知** | 三态模型 + 心跳检测，网络状态透明 |
| **连接管理** | 水位线控制 + 重要连接保护 + 自动裁剪 |
| **QUIC 优先** | 现代传输协议，内置加密和多路复用 |
| **零配置启动** | 合理默认值，开箱即用 |

---

## 🚀 快速开始

### 系统要求

- **Go**: 1.21+
- **Git**: 用于版本控制

### 安装

```bash
go get github.com/dep2p/go-dep2p
```

### 30 秒上手：三步走流程

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
    
    // Step 1: 启动节点（系统层自动就绪）
    node, err := dep2p.StartNode(ctx, dep2p.WithPreset(dep2p.PresetDesktop))
    if err != nil {
        log.Fatalf("启动节点失败: %v", err)
    }
    defer node.Close()
    
    fmt.Printf("节点 ID: %s\n", node.ID())
    
    // Step 2: 加入业务网络（必须！）
    realmKey := types.GenerateRealmKey()
    realm, err := node.JoinRealmWithKey(ctx, "my-first-realm", realmKey)
    if err != nil {
        log.Fatalf("加入 Realm 失败: %v", err)
    }
    
    // Step 3: 使用业务 API
    messaging := realm.Messaging()
    // messaging.Send(ctx, peerID, "/my/protocol/1.0", []byte("Hello!"))
    
    fmt.Println("节点已就绪，可以开始通信！")
}
```

**这就是 DeP2P 的简洁性**：
- ✅ **3 行代码建立连接**：启动节点 → 加入 Realm → 发送消息
- ✅ **自动处理复杂细节**：NAT 穿透、地址发现、中继回退
- ✅ **身份优先**：只需 NodeID，无需关心 IP 地址

### 更多示例

| 示例 | 难度 | 说明 |
|------|------|------|
| [基础示例](examples/basic/) | ⭐ | 最简单的节点创建 |
| [Echo 示例](examples/echo/) | ⭐⭐ | 学习流通信 |
| [Chat 示例](examples/chat/) | ⭐⭐⭐ | 局域网聊天应用 |
| [Chat Public](examples/chat_public/) | ⭐⭐⭐⭐ | 公网三节点聊天 |
| [Relay 示例](examples/relay/) | ⭐⭐⭐⭐ | NAT 穿透与中继 |

---

## 🏗️ 技术架构

### 三层架构

DeP2P 采用三层架构设计，清晰分离系统基础、业务隔离和应用协议：

```
┌─────────────────────────────────────────────────────────────────────┐
│  Layer 3: 应用协议层                                                  │
│  ─────────────────────────────────────────────────────────────────  │
│  • Messaging / PubSub / Discovery / Streams                         │
│  • 协议前缀: /dep2p/app/*                                            │
│  • [!] 必须加入 Realm 后才能使用                                       │
├─────────────────────────────────────────────────────────────────────┤
│  Layer 2: Realm 层（业务隔离）                                        │
│  ─────────────────────────────────────────────────────────────────  │
│  • 业务隔离、成员管理、准入控制（PSK 认证）                               │
│  • 协议前缀: /dep2p/realm/*                                          │
│  • [*] 用户显式加入，严格单 Realm                                      │
├─────────────────────────────────────────────────────────────────────┤
│  Layer 1: 系统基础层                                                 │
│  ─────────────────────────────────────────────────────────────────  │
│  • Transport / Security / DHT / Relay / NAT / Bootstrap             │
│  • 协议前缀: /dep2p/sys/*                                            │
│  • [~] 节点启动自动就绪，用户无感知                                      │
└─────────────────────────────────────────────────────────────────────┘
```

| 层 | 职责 | 特点 |
|---|------|------|
| **Layer 3** | 提供业务通信能力 | 必须先加入 Realm 才能使用 |
| **Layer 2** | 业务隔离和成员管理 | 用户显式加入，PSK 认证 |
| **Layer 1** | P2P 网络基础设施 | 节点启动自动就绪，用户无感知 |

### 设计目标

| 优先级 | 目标 | 验收标准 |
|--------|------|----------|
| **P0 核心** | 简洁性 | 3 行代码建立连接 |
| **P0 核心** | 可靠性 | 95%+ 连接成功率（直连→打洞→中继） |
| **P1 重要** | 安全性 | 端到端加密，身份不可伪造 |
| **P1 重要** | 模块化 | 各模块可独立测试和替换 |

> 📖 **详细架构**：[架构总览](design/architecture/overview.md) | [三层架构](design/architecture/layers.md)

---

## 💎 商业价值：Web3 基础设施的网络层

DeP2P 不仅是一个 P2P 库，更是 **Web3 基础设施的核心网络层**。

### 三大核心场景

```
┌─────────────────────────────────────────────────────────────────────┐
│                    DeP2P 商业价值定位                                 │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  🔗 区块链网络                                                       │
│  ──────────────                                                     │
│  • 交易广播（PubSub + Gossip）                                        │
│  • 区块同步（多源并行 + 断点续传）                                      │
│  • 共识通信（低延迟直连 + 中继回退）                                     │
│  • 网络隔离（主网/测试网 Realm 分离）                                   │
│                                                                     │
│  💾 去中心化存储                                                      │
│  ──────────────                                                     │
│  • 文件分块 → 内容寻址（DHT）                                          │
│  • 多源下载 → 断点续传                                                │
│  • Merkle Proof 完整性校验                                           │
│                                                                    │
│  📡 PCDN 内容分发                                                   │
│  ──────────────                                                    │
│  • 软件下载（P2P 收益极高）                                           │
│  • 静态站点（Web3 DApp 前端）                                        │
│  • 视频点播（HLS/DASH 分片加速）                                      │
│  • 直播（PubSub + 树状拓扑）                                          │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

### PCDN 四大形态

| 形态 | 特点 | P2P 收益 | DeP2P 方案 |
|------|------|----------|-----------|
| **软件下载** | 大文件、强一致性 | ⭐⭐⭐ 极高 | 块交换 + 多源并行 |
| **静态站点** | 版本化、首屏敏感 | ⭐⭐ 中等 | Manifest + Merkle |
| **视频点播** | 分片、热点聚集 | ⭐⭐⭐ 高 | 分片索引 + 预加载 |
| **直播** | 超低延迟、实时扇出 | ⭐⭐⭐ 高 | PubSub + 树状拓扑 |

### 商业模式支撑

| 商业模式 | DeP2P 提供的能力 |
|----------|-----------------|
| **带宽激励** | 上传/下载字节数计量，为 Token 激励提供数据基础 |
| **存储激励** | 内容索引协议，证明"我存储了哪些数据" |
| **CDN 成本优化** | P2P 分流，降低 Origin/CDN 带宽成本 |
| **去中心化托管** | 静态站点 P2P 分发，无需中心化服务器 |

### 推荐落地路径

| 阶段 | 目标 | 说明 |
|------|------|------|
| **Phase 1** | 软件下载 | 最易验证 P2P 收益，技术类似 BitTorrent |
| **Phase 2** | 静态站点 | Manifest + Chunk，对接 Web3 站点协议 |
| **Phase 3** | 视频点播 | 增加分片热度、预加载策略 |
| **Phase 4** | 直播 | PubSub + 树状拓扑 + 严格延迟控制 |

---

## 🌟 适用场景

### 推荐场景

| 场景 | DeP2P 优势 |
|------|-----------|
| **区块链 / DeFi** | Realm 隔离 + 节点发现 + 交易广播 |
| **链游 / GameFi** | 低延迟 + 业务隔离 + 状态同步 |
| **去中心化存储** | 多源下载 + 内容寻址 + 断点续传 |
| **即时通讯** | 简单 API + 可靠传输 + 端到端加密 |
| **协同编辑** | 实时同步 + 冲突解决 + 离线支持 |

### 适用性评估

| 评估 | 场景 | 说明 |
|------|------|------|
| ✅ **非常适合** | 区块链、分布式存储、即时通讯、协同编辑 | DeP2P 核心设计目标 |
| ⚠️ **部分适合** | 视频点播、物联网 | 延迟可接受，需评估资源占用 |
| ❌ **不适合** | 超低延迟直播(<100ms)、实时视频会议、云游戏 | 需要不可靠传输（WebRTC） |

---

## 📋 文档导航

### 按角色导航

| 角色 | 推荐路径 |
|------|----------|
| **用户/开发者** | [快速开始](#-快速开始) → [5 分钟上手](docs/zh/getting-started/quickstart.md) → [教程](docs/zh/tutorials/) |
| **架构师** | [架构总览](design/architecture/overview.md) → [协议规范](design/protocols/README.md) → [ADR](design/adr/) |
| **贡献者** | [开发环境](docs/zh/contributing/development-setup.md) → [代码风格](docs/zh/contributing/code-style.md) |

### 核心文档

| 文档 | 说明 |
|------|------|
| [DeP2P 是什么](docs/zh/concepts/what-is-dep2p.md) | 核心愿景、设计目标和适用场景 |
| [核心概念](docs/zh/concepts/core-concepts.md) | 身份优先、三层架构、Realm |
| [架构总览](design/architecture/overview.md) | 整体架构设计详解 |
| [设计文档导航](design/README.md) | 架构决策、协议规范、实现细节 |
| [API 参考](docs/zh/reference/api/node.md) | 完整的 API 文档 |
| [示例集合](examples/README.md) | 循序渐进的示例代码 |

### 文档结构

```
dep2p.git/
├── README.md              # 📍 本文件 - 项目总览
├── design/                # 设计文档（给架构师/贡献者）
│   ├── architecture/      # 架构设计
│   ├── protocols/         # 协议规范
│   ├── adr/               # 架构决策记录
│   └── invariants/        # 系统不变量
├── docs/                  # 用户文档（给开发者）
│   ├── zh/                # 中文文档
│   └── en/                # English documentation
└── examples/              # 示例代码
```

---

## 🤝 贡献与社区

我们欢迎社区贡献！

### 快速开始贡献

```bash
# 1. Fork 并克隆仓库
git clone https://github.com/your-username/go-dep2p.git

# 2. 设置开发环境
cd go-dep2p
go mod tidy

# 3. 运行测试
go test ./...

# 4. 提交变更
git commit -S -m "feat: your contribution"
git push origin your-branch
```

### 获取帮助

| 渠道 | 用途 |
|------|------|
| [GitHub Issues](https://github.com/dep2p/go-dep2p/issues) | Bug 报告、功能请求 |
| [GitHub Discussions](https://github.com/dep2p/go-dep2p/discussions) | 问题讨论、使用帮助 |
| [贡献指南](docs/zh/contributing/README.md) | 如何参与贡献 |

---

## 🔧 常见问题

<details>
<summary><b>节点启动失败</b></summary>

**常见原因**：端口被占用

```bash
# 检查端口占用
netstat -tulpn | grep :4001

# 解决方案：使用自动分配端口
node, _ := dep2p.StartNode(ctx, dep2p.WithPreset(dep2p.PresetDesktop))
```
</details>

<details>
<summary><b>ErrNotMember 错误</b></summary>

**原因**：未加入 Realm 就调用业务 API

```go
// ❌ 错误
err := node.Send(ctx, peerID, data) // err == ErrNotMember

// ✅ 正确：先加入 Realm
realm, _ := node.JoinRealmWithKey(ctx, "my-realm", realmKey)
err := realm.Messaging().Send(ctx, peerID, data)
```
</details>

<details>
<summary><b>连接超时</b></summary>

**可能原因**：
1. 防火墙阻止连接
2. NAT 穿透失败
3. 地址格式不正确

**解决方案**：
- 检查网络和防火墙设置
- 启用 Relay 服务
- 使用 `ShareableAddrs()` 获取完整地址

> 📖 **详细排障**：[故障排查](docs/zh/how-to/troubleshooting.md) | [错误码参考](docs/zh/reference/error-codes.md)
</details>

---

## 📄 许可证

本项目基于 MIT 许可证开源 - 详见 [LICENSE](LICENSE) 文件。

---

<div align="center">

### 让 P2P 连接像调用函数一样简单

[快速开始](#-快速开始) • [文档中心](docs/zh/README.md) • [设计文档](design/README.md)

Made with ❤️ by the DeP2P Team

</div>
