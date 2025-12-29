# Relay 中继服务模块

## 概述

**层级**: Tier 3  
**职责**: 提供中继连接能力，帮助 NAT 后的节点互相通信，作为直连和打洞失败的保底方案。

## 架构更新：Relay Transport Integration (2025-12-26)

Relay 已重构为 Transport 层实现（参考 libp2p Circuit Relay v2），实现透明的中继回退：

```
┌─────────────────────────────────────────────────────────────┐
│                     Endpoint.Connect()                       │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                   TransportRegistry                          │
│   ┌────────────┐ ┌────────────┐ ┌────────────────────┐      │
│   │ QUIC       │ │ TCP        │ │ Relay Transport    │      │
│   │ Transport  │ │ Transport  │ │ (p2p-circuit)      │      │
│   │ Proxy:false│ │ Proxy:false│ │ Proxy:true         │      │
│   └────────────┘ └────────────┘ └────────────────────┘      │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
              ┌───────────────────────────────┐
              │  Address Ranking:             │
              │  1. Direct addresses (first)  │
              │  2. Relay addresses (fallback)│
              └───────────────────────────────┘
```

### 关键变化

1. **RelayTransport**: 实现 `Transport` 接口，支持 `p2p-circuit` 协议
2. **TransportRegistry**: 管理多传输，根据地址类型选择合适的传输
3. **Address Ranking**: 直连地址优先，中继地址作为回退
4. **透明回退**: `Endpoint.Connect()` 自动尝试直连，失败后使用中继

### 迁移说明

旧方式（独立 API）：
```go
// 需要显式调用 RelayClient.Connect()
conn, err := relayClient.Connect(ctx, relayID, destID)
```

新方式（透明传输）：
```go
// Endpoint.Connect() 自动处理直连和中继回退
conn, err := endpoint.Connect(ctx, destID)
```

## 设计引用

> **重要**: 实现前请详细阅读以下设计规范

| 设计文档 | 说明 |
|----------|------|
| [中继协议规范](../../../docs/01-design/protocols/network/03-relay.md) | 中继架构与协议 |
| [NAT 穿透协议](../../../docs/01-design/protocols/network/02-nat.md) | NAT 穿透策略 |

## 能力清单

### 中继客户端能力 (必须实现)

| 能力 | 状态 | 说明 |
|------|------|------|
| 预留资源 | ✅ 已实现 | 在中继服务器预留资源 |
| 中继连接 | ✅ 已实现 | 通过中继连接目标节点 |
| 中继发现 | ✅ 已实现 | 发现可用中继服务器 |
| 自动选择 | ✅ 已实现 | AutoRelay 自动选择 |

### 内置协议 (必须实现)

| 协议 | 状态 | 说明 |
|------|------|------|
| `/dep2p/sys/relay/1.0.0` | ✅ 已实现 | 中继协议（RESERVE/CONNECT/转发） |

### 中继服务器能力 (可选实现)

| 能力 | 状态 | 说明 |
|------|------|------|
| 资源预留 | ✅ 已实现 | 接受预留请求 |
| 流量转发 | ✅ 已实现 | 转发节点间流量 |
| 资源限制 | ✅ 已实现 | 限制连接数和流量 |
| 统计信息 | ✅ 已实现 | 提供使用统计 |

### 连接升级能力

| 能力 | 状态 | 说明 |
|------|------|------|
| 直连升级 | ✅ 已实现 | ConnectionUpgrader 尝试升级 |
| 打洞协调 | ✅ 已实现 | 协助打洞 |

## 依赖关系

### 接口依赖

```
pkg/types/           → NodeID, RelayStats, ReservationInfo
pkg/interfaces/core/ → Address, Connection
pkg/interfaces/relay/ → RelayClient, RelayServer, Reservation
pkg/interfaces/transport/ → Conn
```

### 模块依赖

```
transport → 底层连接
protocol  → 协议处理
discovery → 发现中继服务器
```

## 目录结构

```
relay/
├── README.md            # 本文件
├── module.go            # fx 模块定义
├── client/              # 中继客户端
│   ├── README.md        # 客户端子模块说明
│   └── client.go        # 客户端实现
└── server/              # 中继服务器
    ├── README.md        # 服务器子模块说明
    └── server.go        # 服务器实现
```

## 公共接口

实现 `pkg/interfaces/relay/` 中的接口：

```go
// RelayClient 中继客户端接口
type RelayClient interface {
    // Reserve 在中继服务器预留资源
    Reserve(ctx context.Context, relay types.NodeID) (Reservation, error)
    
    // Connect 通过中继连接到目标节点
    Connect(ctx context.Context, relay types.NodeID, dest types.NodeID) (transport.Conn, error)
    
    // Relays 返回已知的中继服务器
    Relays() []types.NodeID
    
    // FindRelays 发现中继服务器
    FindRelays(ctx context.Context) ([]types.NodeID, error)
    
    // Close 关闭客户端
    Close() error
}

// RelayServer 中继服务器接口
type RelayServer interface {
    // Start 启动中继服务
    Start(ctx context.Context) error
    
    // Stop 停止中继服务
    Stop() error
    
    // Stats 返回统计信息
    Stats() types.RelayStats
    
    // Reservations 返回当前所有预留
    Reservations() []types.ReservationInfo
}

// Reservation 预留句柄接口
type Reservation interface {
    // RelayAddr 返回中继地址
    RelayAddr() core.Address
    
    // Expiry 返回过期时间
    Expiry() time.Time
    
    // Refresh 刷新预留
    Refresh(ctx context.Context) error
    
    // Close 释放预留
    Close() error
}
```

## 关键算法

### 中继类型 (来自设计文档)

```
1. 公共中继节点
   ├── dep2p 网络提供的公共中继
   ├── 任何人可以使用
   └── 有流量限制

2. 私有中继节点
   ├── 用户自己部署的中继
   ├── 只为特定 Realm 服务
   └── 无流量限制

3. 动态中继
   ├── 普通节点也可以作为中继
   ├── 需要有公网 IP 或已映射端口
   └── 自愿提供服务
```

### 中继连接流程 (来自设计文档)

```
节点 A                     中继服务器                    节点 B
  │                           │                           │
  │  1. RESERVE               │                           │
  │  预留中继资源              │                           │
  │─────────────────────────►│                           │
  │                           │                           │
  │  2. RESERVE_ACK           │                           │
  │  分配中继地址              │      3. RESERVE          │
  │◄─────────────────────────│◄──────────────────────────│
  │                           │                           │
  │  4. CONNECT               │      4. CONNECT          │
  │  请求连接到 B              │      已有预留             │
  │─────────────────────────►│◄──────────────────────────│
  │                           │                           │
  │  5. 建立电路               │                           │
  │◄═════════════════════════╪═══════════════════════════╪►
  │        A ◄──────────────► Relay ◄──────────────► B   │
  │                      端到端加密                       │
```

### 资源限制

```go
type ServerConfig struct {
    // 连接限制
    MaxReservations    int           // 最大预留数，默认 128
    MaxCircuits        int           // 最大同时连接数，默认 1024
    MaxCircuitsPerPeer int           // 每节点最大连接数，默认 8
    
    // 时间限制
    ReservationTTL     time.Duration // 预留有效期，默认 1h
    MaxDuration        time.Duration // 最大连接时长，默认 2min
    
    // 流量限制
    MaxDataRate        int64         // 最大数据速率，默认 64KB/s
    MaxData            int64         // 最大传输数据，默认 128MB
}
```

### 中继地址格式

```
/ip4/203.0.113.1/udp/4001/quic-v1/p2p/5Q2STWvBRelayNodeID.../p2p-circuit/p2p/5Q2STWvBTargetNodeID...

组成部分:
├── /ip4/203.0.113.1/udp/4001/quic-v1  → 中继服务器地址
├── /p2p/5Q2STWvBRelayNodeID...         → 中继服务器 NodeID
├── /p2p-circuit                        → 中继标记
└── /p2p/5Q2STWvBTargetNodeID...        → 目标节点 NodeID
```

### 安全性

```
端到端加密:
├── 中继只转发加密数据
├── 中继无法解密内容
├── 中继只知道通信双方身份
└── 与直连使用相同的 TLS 会话
```

## fx 模块

```go
type ModuleInput struct {
    fx.In
    Transport transportif.Transport `name:"transport"`
    Discovery discoveryif.DiscoveryService `name:"discovery" optional:"true"`
    Config    *relayif.Config       `optional:"true"`
}

type ModuleOutput struct {
    fx.Out
    RelayClient relayif.RelayClient `name:"relay"`
    RelayServer relayif.RelayServer `name:"relay_server" optional:"true"`
}

func Module() fx.Option {
    return fx.Module("relay",
        fx.Provide(ProvideServices),
        fx.Invoke(registerLifecycle),
    )
}
```

## 配置参数

```go
type Config struct {
    // 客户端配置
    EnableClient      bool
    PreferredRelays   []string       // 首选中继服务器
    MaxRelays         int            // 最大保存中继数
    RefreshInterval   time.Duration  // 预留刷新间隔
    
    // 服务器配置
    EnableServer      bool
    ServerConfig      ServerConfig   // 服务器资源限制
}
```

## 相关文档

- [中继协议规范](../../../docs/01-design/protocols/network/03-relay.md)
- [NAT 穿透协议](../../../docs/01-design/protocols/network/02-nat.md)
- [pkg/interfaces/relay](../../../pkg/interfaces/relay/)
