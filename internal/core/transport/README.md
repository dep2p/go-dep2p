# Transport 传输层模块

## 概述

**层级**: Tier 2  
**职责**: 提供 QUIC 传输能力，实现节点间原始数据传输。

## 设计引用

> **重要**: 实现前请详细阅读以下设计规范

| 设计文档 | 说明 |
|----------|------|
| [传输协议规范](../../../docs/01-design/protocols/transport/01-transport.md) | QUIC 传输选型与设计 |
| [安全协议规范](../../../docs/01-design/protocols/transport/02-security.md) | TLS 1.3 集成 |

## 能力清单

### 核心能力 (必须实现)

| 能力 | 状态 | 说明 |
|------|------|------|
| QUIC 连接 | ✅ 已实现 | 基于 quic-go 实现 |
| 1-RTT 握手 | ✅ 已实现 | 首次连接 1-RTT |
| 0-RTT 重连 | ✅ 已实现 | Session resumption (session_store.go) |
| 原生多路复用 | ✅ 已实现 | QUIC 流级别复用 |
| 连接迁移 | ✅ 已实现 | IP 变化不断连 (migration.go) |
| TLS 1.3 集成 | ✅ 已实现 | 内置安全层 |

### 地址解析能力

| 能力 | 状态 | 说明 |
|------|------|------|
| IPv4 地址 | ✅ 已实现 | `ip4/host/udp/port/quic-v1` |
| IPv6 地址 | ✅ 已实现 | `ip6/host/udp/port/quic-v1` |
| DNS 解析 | ✅ 已实现 | `dns4/dns6` 地址解析 |

## 依赖关系

### 接口依赖

```
pkg/types/              → NodeID, Address
pkg/interfaces/core/    → Connection, Listener, Address
pkg/interfaces/transport/ → Transport, TransportConfig
```

### 模块依赖

```
无（Tier 2 基础传输模块）
```

### 第三方依赖

```
github.com/quic-go/quic-go → QUIC 协议实现
```

## 目录结构

```
transport/
├── README.md            # 本文件
├── module.go            # fx 模块定义
└── quic/                # QUIC 实现
    ├── README.md        # QUIC 子模块说明
    ├── transport.go     # QUIC 传输实现
    ├── conn.go          # 连接包装
    ├── config.go        # QUIC 配置
    ├── session_store.go # 0-RTT 会话存储
    └── migration.go     # 连接迁移支持
```

## 公共接口

实现 `pkg/interfaces/transport/` 中的接口：

```go
// Transport 传输接口
type Transport interface {
    // CanDial 检查是否能拨号
    CanDial(addr core.Address) bool
    
    // Dial 建立连接
    Dial(ctx context.Context, remoteAddr core.Address, remoteID types.NodeID) (core.Connection, error)
    
    // Listen 监听连接
    Listen(ctx context.Context, listenAddr core.Address) (core.Listener, error)
    
    // Close 关闭传输
    Close() error
}

// Listener 监听器接口
type Listener interface {
    // Accept 接受连接
    Accept() (core.Connection, error)
    
    // Addr 返回监听地址
    Addr() core.Address
    
    // Close 关闭监听器
    Close() error
}
```

## 关键配置

### QUIC 配置 (来自设计文档)

```go
type QUICConfig struct {
    // 连接配置
    MaxIdleTimeout        time.Duration  // 默认 30s
    HandshakeTimeout      time.Duration  // 默认 10s
    KeepAlivePeriod       time.Duration  // 默认 15s
    
    // 流配置
    MaxIncomingStreams    int64          // 默认 1000
    MaxIncomingUniStreams int64          // 默认 100
    
    // 缓冲配置
    MaxReceiveStreamFlowControlWindow     uint64  // 默认 6MB
    MaxReceiveConnectionFlowControlWindow uint64  // 默认 15MB
    
    // TLS 配置
    TLSConfig            *tls.Config     // TLS 1.3 配置
    
    // 0-RTT 配置
    Enable0RTT           bool            // 启用 0-RTT
    SessionCacheSize     int             // 会话缓存大小
    SessionTTL           time.Duration   // 会话有效期
    
    // 连接迁移配置
    EnableMigration      bool            // 启用连接迁移
    MigrationTimeout     time.Duration   // 迁移超时
}
```

### 默认 TLS 配置

```go
func defaultTLSConfig(identity Identity) *tls.Config {
    return &tls.Config{
        MinVersion:   tls.VersionTLS13,
        Certificates: []tls.Certificate{identity.Certificate()},
        NextProtos:   []string{"dep2p"},
        ClientAuth:   tls.RequireAnyClientCert,
    }
}
```

## QUIC 优势 (来自设计文档)

```
1. 快速连接建立
   TCP + TLS: 3 RTT (TCP握手1 + TLS握手2)
   QUIC:      1 RTT (首次) / 0 RTT (重连)

2. 原生多路复用
   TCP: 单流，头部阻塞 - 一个包丢失，阻塞所有数据
   QUIC: 多流独立 - 一个流丢包，不影响其他流

3. 内置 TLS 1.3
   加密与传输一体化

4. NAT 友好
   基于 UDP，更容易穿透 NAT

5. 连接迁移
   IP 变化时连接可以无缝迁移
```

## fx 模块

```go
type ModuleInput struct {
    fx.In
    Config *transportif.Config `optional:"true"`
}

type ModuleOutput struct {
    fx.Out
    Transport transportif.Transport `name:"transport"`
}

func Module() fx.Option {
    return fx.Module("transport",
        fx.Provide(ProvideServices),
        fx.Invoke(registerLifecycle),
    )
}
```

## 相关文档

- [传输协议规范](../../../docs/01-design/protocols/transport/01-transport.md)
- [安全协议规范](../../../docs/01-design/protocols/transport/02-security.md)
- [pkg/interfaces/transport](../../../pkg/interfaces/transport/)
