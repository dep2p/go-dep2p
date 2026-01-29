# 与其他库对比

本文档对比 DeP2P 与业界主流 P2P 库（libp2p、iroh、torrent），帮助你选择合适的库。

> **数据来源**：本文档中的代码规模数据基于 [scc](https://github.com/boyter/scc) 工具统计，统计日期 2026-01-29。

---

## 目录

- [项目概览](#项目概览)
- [代码规模对比](#代码规模对比)
- [DeP2P vs libp2p](#dep2p-vs-libp2p)
- [DeP2P vs iroh](#dep2p-vs-iroh)
- [DeP2P vs torrent](#dep2p-vs-torrent)
- [能力对照表](#能力对照表)
- [选择决策树](#选择决策树)
- [迁移指南](#迁移指南)
- [总结](#总结)

---

## 项目概览

| 项目 | 语言 | 定位 | 主要应用 | 特点 |
|------|------|------|---------|------|
| **DeP2P** | Go | 通用 P2P 基础库 | 区块链、分布式应用 | 极简 API、Realm 隔离、三层架构 |
| **libp2p** | Go/Rust/JS | 模块化 P2P 协议栈 | IPFS、Filecoin、Ethereum 2.0 | 高度模块化、协议丰富、生态成熟 |
| **iroh** | Rust | 轻量 P2P 连接库 | iroh-net、Dumbpipe | 极简 API、MagicSock、官方基础设施 |
| **torrent** | Go | BitTorrent 实现 | 文件分发、P2P 下载 | 专注 BT 协议、DHT 成熟 |

---

## 代码规模对比

### 统计方法

```bash
# 使用 scc 工具统计（https://github.com/boyter/scc）
# 排除目录：.git, .github, vendor, node_modules
# 统计日期：2026-01-29

scc --exclude-dir .git,.github,vendor,node_modules --include-ext go <project_path>
scc --exclude-dir .git,.github,vendor,node_modules --include-ext rs <project_path>
```

### 核心数据对比

| 项目 | 语言 | 文件数 | 代码行 | 总行数 | 复杂度 | 注释率 |
|------|------|-------:|-------:|-------:|-------:|-------:|
| **DeP2P** | Go | 961 | 160,645 | 250,535 | 29,191 | 18.9% |
| **go-libp2p** | Go | 582 | 92,063 | 117,940 | 17,168 | 8.1% |
| **torrent** | Go | 390 | 35,879 | 43,703 | 5,858 | 7.6% |
| **iroh** | Rust | 124 | 30,071 | 41,557 | 1,813 | 17.3% |

**指标说明**：
- **代码行（Code）**：可执行代码行数，不含注释和空行
- **总行数（Lines）**：包含代码、注释、空行的总行数
- **复杂度（Complexity）**：圈复杂度，反映代码分支和逻辑复杂程度
- **注释率**：注释行数 / 总行数，反映文档化程度

### 数据解读

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         代码规模可视化对比                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  代码行数（万行）                                                            │
│  ────────────────                                                           │
│  DeP2P      ████████████████████████████████  16.1 万行                     │
│  go-libp2p  ██████████████████████            9.2 万行                      │
│  torrent    ███████                           3.6 万行                      │
│  iroh       ██████                            3.0 万行                      │
│                                                                             │
│  注释率                                                                      │
│  ────────────────                                                           │
│  DeP2P      ████████████████████  18.9%  （文档化程度高）                    │
│  iroh       █████████████████     17.3%  （文档化程度高）                    │
│  go-libp2p  ████████              8.1%   （代码为主）                        │
│  torrent    ████████              7.6%   （代码为主）                        │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

**关键洞察**：

1. **规模差异的原因**
   - DeP2P 代码量较大，因为包含完整的三层架构实现、详细的测试用例和示例代码
   - go-libp2p 采用高度模块化设计，核心库精简，功能分散在子模块中
   - torrent 专注于 BitTorrent 协议，功能范围明确
   - iroh 采用 Rust 语言，代码密度更高，且专注于连接层

2. **注释率对比**
   - DeP2P 和 iroh 注释率较高（17-19%），文档化程度好
   - go-libp2p 和 torrent 注释率较低（7-8%），更依赖外部文档

3. **复杂度分析**
   - DeP2P 复杂度较高，反映其功能覆盖面广
   - iroh 复杂度最低，设计追求简洁

### 注意事项

> **代码行数 ≠ 项目质量**
> 
> 代码规模仅供参考，不应作为选择库的主要依据。选择时应综合考虑：
> - 功能是否满足需求
> - API 设计是否符合习惯
> - 社区活跃度和维护状态
> - 文档完整性和学习曲线

---

## DeP2P vs libp2p

### 核心对比

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        DeP2P vs libp2p                                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  DeP2P                                   libp2p                              │
│  ──────────────────                      ──────────────────                  │
│                                                                              │
│  ✅ 极简 API                             ⚠️ API 复杂                         │
│  realm.Messaging().Send(ctx, nodeID, data)  需要配置 Host, Transport,        │
│  3 步走：启动→加入→发送                    Muxer, Security, Discovery...      │
│                                                                              │
│  ✅ Realm 业务隔离                        ⚠️ PSK (弱隔离)                     │
│  不同业务网络完全隔离                     需要手动管理隔离                    │
│  共享基础设施                                                                │
│                                                                              │
│  ✅ 预设配置                             ⚠️ 需要详细配置                      │
│  WithPreset(PresetDesktop)               需要逐项配置各个组件                │
│                                                                              │
│  ✅ 学习曲线平缓                         ⚠️ 学习曲线陡峭                      │
│  概念简单，快速上手                       概念繁多，需要深入理解              │
│                                                                              │
│  ⚠️ 定制能力适中                         ✅ 高度可定制                        │
│  适合大多数场景                           完全可插拔，组件可替换              │
│                                                                              │
│  ⚠️ 生态较新                             ✅ 生态成熟                          │
│  发展中                                   大量生产案例                        │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 特性对比表

| 特性 | DeP2P | libp2p |
|------|-------|--------|
| **代码规模** | 16.1 万行 | 9.2 万行（核心库） |
| **传输层** | QUIC (主) | TCP, QUIC, WebSocket, WebTransport |
| **加密** | TLS 1.3 | TLS 1.3, Noise Protocol |
| **多路复用** | QUIC 原生 | yamux, mplex, QUIC |
| **节点发现** | DHT, mDNS | DHT, mDNS, Rendezvous, Bootstrap |
| **NAT 穿透** | STUN, 打洞, 中继 | AutoNAT, Relay, Hole Punching, UPnP |
| **消息模式** | Stream, Request, PubSub | Stream, Request-Response, GossipSub |
| **业务隔离** | Realm ✅ | PSK (弱) |
| **连接管理** | 水位线 + 保护 | ConnectionManager + ResourceManager |
| **API 复杂度** | 低 | 高 |
| **学习曲线** | 平缓 | 陡峭 |

### 代码对比

**DeP2P 启动节点：**

```go
// DeP2P: 3 行代码启动节点并加入网络
node, _ := dep2p.New(ctx, dep2p.WithPreset(dep2p.PresetDesktop))
_ = node.Start(ctx)
realm, _ := node.Realm("my-realm")
_ = realm.Join(ctx)
realm.Messaging().Send(ctx, peerID, "/my/protocol", data)
```

**libp2p 启动节点：**

```go
// libp2p: 需要配置多个组件
host, _ := libp2p.New(
    libp2p.ListenAddrStrings("/ip4/0.0.0.0/tcp/0"),
    libp2p.Transport(tcp.NewTCPTransport),
    libp2p.Transport(quic.NewTransport),
    libp2p.Security(noise.ID, noise.New),
    libp2p.Security(tls.ID, tls.New),
    libp2p.Muxer(yamux.ID, yamux.DefaultTransport),
    libp2p.NATPortMap(),
    libp2p.EnableRelay(),
    libp2p.EnableHolePunching(),
    // ... 更多配置
)
defer host.Close()

// 还需要手动配置 DHT、PubSub 等
dht, _ := dht.New(ctx, host)
ps, _ := pubsub.NewGossipSub(ctx, host)
```

### 选择建议

```mermaid
flowchart TD
    Start["选择 P2P 库"] --> Q1{"需要高度定制？"}
    Q1 -->|是| libp2p["选择 libp2p"]
    Q1 -->|否| Q2{"需要业务隔离？"}
    Q2 -->|是| DeP2P["选择 DeP2P"]
    Q2 -->|否| Q3{"优先简洁性？"}
    Q3 -->|是| DeP2P
    Q3 -->|否| Q4{"需要多语言？"}
    Q4 -->|是| libp2p
    Q4 -->|否| DeP2P
```

| 选择 DeP2P 如果... | 选择 libp2p 如果... |
|-------------------|---------------------|
| 需要快速上手 | 需要高度定制 |
| 需要业务网络隔离 | 需要多语言支持 |
| 优先简洁 API | 需要特定传输协议 |
| Go 项目 | 需要成熟生态 |

---

## DeP2P vs iroh

### 核心对比

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        DeP2P vs iroh                                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  DeP2P                                   iroh                                │
│  ──────────────────                      ──────────────────                  │
│                                                                              │
│  ✅ Go 原生                              ⚠️ Rust (需要 FFI)                  │
│  Go 项目直接使用                          Go 项目需要绑定                     │
│                                                                              │
│  ✅ Realm 业务隔离                        ❌ 无业务隔离                        │
│  多租户网络隔离                           需要自己实现                        │
│                                                                              │
│  ✅ 三层架构                             ✅ 极简 API                          │
│  Layer 1 透明                            Endpoint API                        │
│  Layer 2 显式                                                                │
│  Layer 3 业务                                                                │
│                                                                              │
│  ⚠️ 自建基础设施                         ✅ 官方中继网络                      │
│  需要自己部署 Bootstrap/Relay             可使用 n0 基础设施                  │
│                                                                              │
│  ⚠️ 标准 DHT                             ✅ DNS 发现 + pkarr                  │
│  Kademlia DHT                            更适合公网发现                      │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 特性对比表

| 特性 | DeP2P | iroh |
|------|-------|------|
| **代码规模** | 16.1 万行 (Go) | 3.0 万行 (Rust) |
| **语言** | Go | Rust |
| **传输层** | QUIC | QUIC |
| **连接策略** | 直连 → 打洞 → 中继 | MagicSock 智能路径 |
| **节点发现** | DHT, mDNS | DNS, DHT (可选), Local Swarm |
| **业务隔离** | Realm ✅ | 无 |
| **官方基础设施** | 无 | 中继网络 |
| **API 复杂度** | 低 | 低 |
| **注释率** | 18.9% | 17.3% |

### 代码对比

**DeP2P 启动节点：**

```go
// DeP2P (Go)
node, _ := dep2p.New(ctx, dep2p.WithPreset(dep2p.PresetDesktop))
_ = node.Start(ctx)
realm, _ := node.Realm("my-realm")
_ = realm.Join(ctx)
realm.Messaging().Send(ctx, peerID, "/my/protocol", data)
```

**iroh 启动节点：**

```rust
// iroh (Rust)
let endpoint = Endpoint::builder()
    .discovery(DnsDiscovery::n0_dns())
    .bind()
    .await?;

let conn = endpoint.connect(node_id, ALPN).await?;
let mut send = conn.open_uni().await?;
send.write_all(data).await?;
```

### 选择建议

| 选择 DeP2P 如果... | 选择 iroh 如果... |
|-------------------|-------------------|
| Go 项目 | Rust 项目 |
| 需要业务隔离 | 需要官方基础设施 |
| 需要三层架构 | 需要 MagicSock 智能路径 |
| 自建网络 | 使用公网节点 |

---

## DeP2P vs torrent

### 核心对比

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        DeP2P vs torrent                                      │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  DeP2P                                   torrent                             │
│  ──────────────────                      ──────────────────                  │
│                                                                              │
│  ✅ 通用 P2P 基础库                       ⚠️ 专注 BitTorrent                  │
│  适用于各种 P2P 场景                      专注于文件分发                      │
│                                                                              │
│  ✅ Realm 业务隔离                        ⚠️ Info Hash 隔离                   │
│  多租户网络隔离                           基于内容哈希隔离                    │
│                                                                              │
│  ✅ 多种消息模式                          ⚠️ 块交换为主                       │
│  Stream/Request/PubSub                   Piece 交换协议                      │
│                                                                              │
│  ⚠️ 无内置文件分发                        ✅ 成熟的文件分发                   │
│  需要自己实现                             BitTorrent 协议完整实现             │
│                                                                              │
│  ⚠️ DHT 通用实现                          ✅ BT DHT 优化                      │
│  标准 Kademlia                            针对 BT 场景优化                    │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 特性对比表

| 特性 | DeP2P | torrent |
|------|-------|---------|
| **代码规模** | 16.1 万行 | 3.6 万行 |
| **定位** | 通用 P2P 基础库 | BitTorrent 客户端库 |
| **传输层** | QUIC | TCP, uTP |
| **节点发现** | DHT, mDNS | BT DHT, Tracker, PEX |
| **消息模式** | Stream, Request, PubSub | Piece 交换 |
| **文件分发** | 需自行实现 | 原生支持 |
| **业务隔离** | Realm | Info Hash |
| **注释率** | 18.9% | 7.6% |

### 选择建议

| 选择 DeP2P 如果... | 选择 torrent 如果... |
|-------------------|---------------------|
| 通用 P2P 通信需求 | 专注文件分发 |
| 需要多种消息模式 | 需要 BitTorrent 兼容 |
| 需要业务隔离 | 基于内容寻址 |
| 区块链/即时通讯场景 | PCDN/下载器场景 |

---

## 能力对照表

### 完整对比

| 能力 | DeP2P | libp2p | iroh | torrent |
|------|-------|--------|------|---------|
| **语言** | Go | Go/Rust/JS | Rust | Go |
| **代码行** | 16.1 万 | 9.2 万 | 3.0 万 | 3.6 万 |
| **传输** | QUIC (主) | 多种 | QUIC | TCP/uTP |
| **加密** | TLS 1.3 | TLS/Noise | QUIC TLS | MSE |
| **多路复用** | QUIC 原生 | yamux/mplex | QUIC 原生 | 无 |
| **节点发现** | DHT/mDNS | DHT/mDNS/Rendezvous | DNS/DHT | DHT/Tracker/PEX |
| **NAT 穿透** | STUN/打洞/中继 | AutoNAT/Relay | MagicSock | uTP/Relay |
| **消息模式** | Stream/Req/PubSub | 同左 | Stream | Piece |
| **业务隔离** | Realm ✅ | PSK (弱) | 无 | Info Hash |
| **连接管理** | 水位线/保护 | 完整 | 简单 | 完整 |
| **API 复杂度** | 低 | 高 | 低 | 中 |
| **学习曲线** | 平缓 | 陡峭 | 平缓 | 中等 |
| **生态成熟度** | 发展中 | 成熟 | 发展中 | 成熟 |
| **注释率** | 18.9% | 8.1% | 17.3% | 7.6% |

### DeP2P 独有能力

| 能力 | 说明 |
|------|------|
| **Realm 业务隔离** | 多租户网络隔离，共享基础设施，业务数据不会跨 Realm 泄露 |
| **三层架构** | Layer 1 透明（系统层）、Layer 2 显式（Realm）、Layer 3 业务（应用协议） |
| **严格单 Realm** | 节点同一时间只能属于一个 Realm，状态清晰，资源可控 |
| **Preset 预设配置** | 开箱即用，适应不同场景（Mobile/Desktop/Server/Minimal） |
| **身份第一性** | 所有连接以 NodeID 为目标，IP/端口只是拨号路径 |

---

## 选择决策树

```mermaid
flowchart TD
    Start["需要 P2P 通信"] --> UseCase{"主要用途？"}
    
    UseCase -->|文件分发/下载| torrent["选择 torrent"]
    UseCase -->|通用 P2P| Lang{"主要开发语言？"}
    
    Lang -->|Go| GoPath["Go 项目"]
    Lang -->|Rust| RustPath["Rust 项目"]
    Lang -->|JS/多语言| libp2p["选择 libp2p"]
    
    GoPath --> Isolation{"需要业务隔离？"}
    Isolation -->|是| DeP2P["选择 DeP2P"]
    Isolation -->|否| GoSimple{"优先简洁性？"}
    GoSimple -->|是| DeP2P
    GoSimple -->|否| GoCustom{"需要高度定制？"}
    GoCustom -->|是| libp2p
    GoCustom -->|否| DeP2P
    
    RustPath --> RustInfra{"需要官方基础设施？"}
    RustInfra -->|是| iroh["选择 iroh"]
    RustInfra -->|否| RustCustom{"需要高度定制？"}
    RustCustom -->|是| libp2p
    RustCustom -->|否| iroh
```

---

## 迁移指南

### 从 libp2p 迁移到 DeP2P

1. **替换 Host 为 Node**
   - libp2p `Host` → DeP2P `Node`
   - 使用 `dep2p.WithPreset()` 替代详细配置

2. **添加 Realm**
   - 在使用业务 API 前调用 `JoinRealm()`
   - 这是 DeP2P 的核心差异

3. **调整消息发送**
   - libp2p `host.NewStream()` → DeP2P `realm.Messaging().Send()` 或 `realm.Messaging().Request()`

4. **简化 PubSub**
   - libp2p `pubsub.NewGossipSub()` → DeP2P `realm.PubSub().Subscribe()` / `realm.PubSub().Publish()`

```go
// Before (libp2p)
host, _ := libp2p.New(/* 大量配置 */)
dht, _ := dht.New(ctx, host)
ps, _ := pubsub.NewGossipSub(ctx, host)
topic, _ := ps.Join("my-topic")
topic.Publish(ctx, data)

// After (DeP2P)
node, _ := dep2p.New(ctx, dep2p.WithPreset(dep2p.PresetDesktop))
_ = node.Start(ctx)
realm, _ := node.Realm("my-realm")
_ = realm.Join(ctx)
realm.PubSub().Publish(ctx, "my-topic", data)
```

---

## 总结

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           选择建议总结                                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  选择 DeP2P 如果：                                                           │
│  ─────────────────                                                          │
│  ✅ Go 项目                                                                  │
│  ✅ 需要业务网络隔离（Realm）                                                │
│  ✅ 优先简洁易用的 API                                                       │
│  ✅ 需要快速上手                                                             │
│  ✅ 区块链、分布式应用场景                                                   │
│                                                                              │
│  选择 libp2p 如果：                                                          │
│  ─────────────────                                                          │
│  ✅ 需要高度定制                                                             │
│  ✅ 需要多语言支持（Go/Rust/JS）                                             │
│  ✅ 需要特定传输协议                                                         │
│  ✅ 需要成熟生态和生产案例                                                   │
│                                                                              │
│  选择 iroh 如果：                                                            │
│  ─────────────────                                                          │
│  ✅ Rust 项目                                                                │
│  ✅ 需要官方中继基础设施                                                     │
│  ✅ 需要 MagicSock 智能路径选择                                              │
│  ✅ 简单的点对点通信                                                         │
│                                                                              │
│  选择 torrent 如果：                                                         │
│  ─────────────────                                                          │
│  ✅ 专注文件分发场景                                                         │
│  ✅ 需要 BitTorrent 协议兼容                                                 │
│  ✅ PCDN、下载器场景                                                         │
│  ✅ 基于内容寻址                                                             │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 下一步

- [DeP2P 是什么](what-is-dep2p.md) - 了解 DeP2P 的愿景和定位
- [核心概念总纲](core-concepts.md) - 深入理解核心概念
- [架构概览](architecture-overview.md) - 了解系统架构
- [5 分钟上手](../getting-started/quickstart.md) - 动手实践
