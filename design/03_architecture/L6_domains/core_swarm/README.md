# Core Swarm 模块

> **版本**: v1.2.0  
> **更新日期**: 2026-01-23  
> **定位**: 连接群管理（Core Layer）

---

## 模块概述

core_swarm 是 DeP2P 网络层的核心组件，负责管理节点间的所有连接和流。它是 Host 的底层引擎，处理多路复用连接池、拨号调度、连接生命周期管理等关键功能。

| 属性 | 值 |
|------|-----|
| **架构层** | Core Layer |
| **代码位置** | `internal/core/swarm/` |
| **Fx 模块** | `fx.Module("swarm")` |
| **状态** | ✅ 已实现 |
| **依赖** | transport, upgrader, connmgr, peerstore, eventbus |

---

## 核心职责

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         core_swarm 职责                                      │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. 连接管理                                                                 │
│     • 维护到所有节点的连接池                                                │
│     • 连接复用（同一节点的多个流共享连接）                                  │
│     • 连接生命周期（创建、保持、关闭）                                      │
│     • 连接状态跟踪                                                          │
│                                                                             │
│  2. 拨号调度                                                                 │
│     • 智能地址排序（优先本地、优先 QUIC）                                   │
│     • 并发拨号（Dial Many）                                                │
│     • 拨号超时与重试                                                        │
│     • 黑洞检测                                                              │
│                                                                             │
│  3. 监听管理                                                                 │
│     • 多地址监听                                                            │
│     • 入站连接接受                                                          │
│     • 传输层适配                                                            │
│                                                                             │
│  4. 流管理                                                                   │
│     • 流创建与复用                                                          │
│     • 协议协商                                                              │
│     • 流超时控制                                                            │
│                                                                             │
│  5. 事件通知                                                                 │
│     • 连接建立/断开事件                                                     │
│     • 流打开/关闭事件                                                       │
│     • 通知器（Notifiee）机制                                                │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 架构设计

### 组件关系

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              Host                                            │
├─────────────────────────────────────────────────────────────────────────────┤
│                                 │                                            │
│                                 ▼                                            │
│  ┌──────────────────────────────────────────────────────────────────────┐   │
│  │                            Swarm                                      │   │
│  │  ┌────────────────┐  ┌─────────────────┐  ┌───────────────────────┐ │   │
│  │  │  Dial Manager  │  │  Listen Manager │  │  Connection Pool      │ │   │
│  │  └───────┬────────┘  └────────┬────────┘  └───────────┬───────────┘ │   │
│  │          │                    │                        │             │   │
│  │          ▼                    ▼                        ▼             │   │
│  │  ┌──────────────────────────────────────────────────────────────┐   │   │
│  │  │                         Upgrader                              │   │   │
│  │  │           Raw Conn → Security → Muxer → Upgraded Conn        │   │   │
│  │  └──────────────────────────────────────────────────────────────┘   │   │
│  │                               │                                      │   │
│  │                               ▼                                      │   │
│  │  ┌──────────────────────────────────────────────────────────────┐   │   │
│  │  │                       Transports                              │   │   │
│  │  │              QUIC  │  TCP  │  WebSocket                       │   │   │
│  │  └──────────────────────────────────────────────────────────────┘   │   │
│  └──────────────────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────────────────┘
```

### ★ 连接优先级策略（惰性中继）

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    连接优先级（惰性中继策略）                                 │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. 直连 ← 优先                                                             │
│     │     如果对方地址可直连，直接连接                                      │
│     ↓ 失败                                                                  │
│                                                                             │
│  2. 通过 Relay 连接 + 打洞升级                                              │
│     │     如果对方发布的是 Relay 地址，先通过 Relay 连接                    │
│     │     然后通过 Relay 信令通道尝试打洞升级                               │
│     ↓ 打洞失败                                                              │
│                                                                             │
│  3. 继续使用 Relay                                                          │
│          Symmetric NAT 或其他打洞失败情况                                   │
│          数据经过第三方（兜底）                                              │
│                                                                             │
│  ★ 打洞成功后保留 Relay 连接作为备份                                        │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 拨号流程（含打洞）

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              拨号流程                                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  DialPeer(peerID)                                                           │
│       │                                                                     │
│       ├─── 1. 检查是否已有连接 → 复用现有连接                               │
│       │                                                                     │
│       ├─── 2. 从 PeerStore 获取地址                                         │
│       │                                                                     │
│       ├─── 3. 地址分类：直连地址 / Relay 地址                               │
│       │                                                                     │
│       ├─── 4. 尝试直连（如有直连地址）                                      │
│       │         │                                                           │
│       │         ├─── 成功 → 返回直连                                        │
│       │         │                                                           │
│       │         └─── 失败 → 继续                                            │
│       │                                                                     │
│       ├─── 5. 通过 Relay 连接（如有 Relay 地址）                            │
│       │         │                                                           │
│       │         ├─── 检查 NAT 类型组合                                      │
│       │         │         │                                                 │
│       │         │         ├─── 双方 Symmetric → 跳过打洞                    │
│       │         │         │                                                 │
│       │         │         └─── 其他 → 通过信令通道尝试打洞                  │
│       │         │                     │                                     │
│       │         │                     ├─── 打洞成功 → 直连（保留 Relay）    │
│       │         │                     │                                     │
│       │         │                     └─── 打洞失败 → 使用 Relay            │
│       │         │                                                           │
│       │         └─── 返回 Relay 连接                                        │
│       │                                                                     │
│       ├─── 6. 通过 Upgrader 升级连接                                        │
│       │                                                                     │
│       └─── 7. 触发 Connected 事件                                           │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 公共接口

```go
// pkg/interfaces/swarm.go

// Swarm 连接群管理接口
type Swarm interface {
    // 身份
    LocalPeer() types.PeerID
    
    // 连接管理
    Peers() []types.PeerID
    Conns() []Connection
    ConnsToPeer(peer types.PeerID) []Connection
    
    // 拨号
    DialPeer(ctx context.Context, peer types.PeerID) (Connection, error)
    
    // 流
    NewStream(ctx context.Context, peer types.PeerID) (Stream, error)
    
    // 监听
    Listen(addrs ...types.Multiaddr) error
    ListenAddrs() []types.Multiaddr
    
    // 通知
    Notify(Notifiee)
    StopNotify(Notifiee)
    
    // 生命周期
    Close() error
}

// Connection 连接接口
type Connection interface {
    LocalPeer() types.PeerID
    RemotePeer() types.PeerID
    LocalMultiaddr() types.Multiaddr
    RemoteMultiaddr() types.Multiaddr
    NewStream(ctx context.Context) (Stream, error)
    GetStreams() []Stream
    Stat() ConnectionStat
    Close() error
}

// Stream 流接口
type Stream interface {
    io.ReadWriteCloser
    Protocol() types.ProtocolID
    SetProtocol(types.ProtocolID) error
    Conn() Connection
    Reset() error
    SetDeadline(time.Time) error
}

// Notifiee 通知接口
type Notifiee interface {
    Connected(Connection)
    Disconnected(Connection)
    OpenedStream(Stream)
    ClosedStream(Stream)
}
```

---

## 参考实现

### go-libp2p Swarm

```
github.com/libp2p/go-libp2p/p2p/net/swarm/
├── swarm.go              # 主结构
├── swarm_conn.go         # 连接实现
├── swarm_stream.go       # 流实现
├── swarm_dial.go         # 拨号逻辑
├── swarm_listen.go       # 监听逻辑
├── dial_worker.go        # 并发拨号工作器
├── black_hole_detector.go # 黑洞检测
└── metrics.go            # 指标收集

关键特性：
• 连接多路复用
• 智能拨号调度
• 黑洞检测（避免无效拨号）
• 资源限制集成
```

### iroh MagicSock

```
iroh/src/magicsock/
├── magicsock.rs          # 核心套接字
├── remote_map/           # 远程节点映射
├── transports/           # 传输实现
└── metrics.rs            # 指标

关键特性：
• QUIC 原生支持
• 自动 NAT 穿透
• 中继自动切换
```

---

## 配置参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `DialTimeout` | 15s | 拨号超时 |
| `DialTimeoutLocal` | 5s | 本地网络拨号超时 |
| `NewStreamTimeout` | 15s | 创建流超时 |
| `MaxParallelDials` | 100 | 最大并发拨号数 |
| `DialRanker` | DefaultRanker | 地址排序算法 |

---

## 目录导航

| 文档 | 说明 |
|------|------|
| [requirements/requirements.md](requirements/requirements.md) | 需求追溯 |
| [design/overview.md](design/overview.md) | 整体设计 |
| [design/internals.md](design/internals.md) | 内部设计 |
| [coding/guidelines.md](coding/guidelines.md) | 编码指南 |
| [testing/strategy.md](testing/strategy.md) | 测试策略 |

---

## 相关文档

| 文档 | 说明 |
|------|------|
| [core_transport](../core_transport/) | 传输层 |
| [core_upgrader](../core_upgrader/) | 连接升级 |
| [core_connmgr](../core_connmgr/) | 连接管理 |
| [L2: 五层架构](../../L2_structural/layer_model.md) | 架构定义 |
| [core_nat](../core_nat/) | NAT 类型检测、打洞 |
| [core_relay](../core_relay/) | Relay 连接、信令通道 |

---

**最后更新**：2026-01-23
