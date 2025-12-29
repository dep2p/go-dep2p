# Muxer 多路复用模块

## 概述

**层级**: Tier 2  
**职责**: 提供流多路复用能力，在单连接上支持多个独立流。

## 设计引用

> **重要**: 实现前请详细阅读以下设计规范

| 设计文档 | 说明 |
|----------|------|
| [传输协议规范](../../../docs/01-design/protocols/transport/01-transport.md) | QUIC 原生多路复用 |
| [多路复用协议](../../../docs/01-design/protocols/transport/03-muxer.md) | Yamux 备用方案 |

## 能力清单

### 核心能力 (必须实现)

| 能力 | 状态 | 说明 |
|------|------|------|
| 流创建 | ✅ 已实现 | 打开新流 |
| 流接受 | ✅ 已实现 | 接受入站流 |
| 流级别控制 | ✅ 已实现 | 每流独立流控 |
| 双向流 | ✅ 已实现 | 支持双向通信 |

### QUIC 原生多路复用

| 能力 | 状态 | 说明 |
|------|------|------|
| QUIC Stream | ✅ 已实现 | 使用 QUIC 原生流 |
| 无头部阻塞 | ✅ 已实现 | 流级别独立 |
| 0-RTT Stream | ✅ 已实现 | 通过 QUIC transport 支持 |

### Yamux 备用 (TCP 场景)

| 能力 | 状态 | 说明 |
|------|------|------|
| Yamux 多路复用 | ✅ 已实现 | TCP 上的多路复用 |
| 心跳保活 | ✅ 已实现 | Yamux HeartbeatMonitor |

## 依赖关系

### 接口依赖

```
pkg/interfaces/core/  → Stream 接口
pkg/interfaces/muxer/ → MuxerFactory, Muxer 接口
```

### 模块依赖

```
transport → 底层连接
security  → 安全连接
```

### 第三方依赖

```
github.com/hashicorp/yamux → Yamux 实现 (TCP 备用)
```

## 目录结构

```
muxer/
├── README.md            # 本文件
├── module.go            # fx 模块定义
└── yamux/               # Yamux 实现
    ├── README.md        # Yamux 子模块说明
    └── muxer.go         # Yamux 多路复用器
```

## 公共接口

实现 `pkg/interfaces/muxer/` 中的接口：

```go
// MuxerFactory 多路复用器工厂
type MuxerFactory interface {
    // NewMuxer 创建多路复用器
    // server: true 表示服务端，false 表示客户端
    NewMuxer(conn net.Conn, server bool) (Muxer, error)
}

// Muxer 多路复用器接口
type Muxer interface {
    // OpenStream 打开新流
    OpenStream(ctx context.Context) (core.Stream, error)
    
    // AcceptStream 接受入站流
    AcceptStream(ctx context.Context) (core.Stream, error)
    
    // IsClosed 检查是否已关闭
    IsClosed() bool
    
    // Close 关闭多路复用器
    Close() error
}

// Stream 流接口 (来自 core)
type Stream interface {
    io.ReadWriteCloser
    
    // Protocol 返回流协议
    Protocol() types.ProtocolID
    
    // SetProtocol 设置流协议
    SetProtocol(protocol types.ProtocolID)
    
    // Conn 返回所属连接
    Conn() Connection
    
    // SetDeadline 设置读写超时
    SetDeadline(t time.Time) error
    SetReadDeadline(t time.Time) error
    SetWriteDeadline(t time.Time) error
    
    // CloseWrite 关闭写入端
    CloseWrite() error
    
    // Reset 重置流（异常关闭）
    Reset() error
}
```

## QUIC vs Yamux

### QUIC 原生多路复用 (首选)

```
优势:
├── 原生支持，无额外开销
├── 流级别头部阻塞消除
├── 与传输层深度集成
└── 更好的拥塞控制

使用场景:
└── QUIC 传输（默认）
```

### Yamux 备用方案 (TCP)

```
优势:
├── TCP 兼容
├── 成熟稳定
└── 广泛使用

使用场景:
├── TCP 传输
├── WebSocket 传输
└── 某些受限网络
```

## Yamux 配置

```go
type YamuxConfig struct {
    // 连接保活
    EnableKeepAlive   bool          // 默认 true
    KeepAliveInterval time.Duration // 默认 30s
    
    // 流控制
    ConnectionWriteTimeout time.Duration // 默认 10s
    MaxStreamWindowSize    uint32        // 默认 256KB
    
    // 限制
    MaxIncomingStreams     int           // 默认 1000
}

func defaultYamuxConfig() *yamux.Config {
    return &yamux.Config{
        AcceptBacklog:          256,
        EnableKeepAlive:        true,
        KeepAliveInterval:      30 * time.Second,
        ConnectionWriteTimeout: 10 * time.Second,
        MaxStreamWindowSize:    256 * 1024,
    }
}
```

## fx 模块

```go
type ModuleInput struct {
    fx.In
    Config *muxerif.Config `optional:"true"`
}

type ModuleOutput struct {
    fx.Out
    MuxerFactory muxerif.MuxerFactory `name:"muxer"`
}

func Module() fx.Option {
    return fx.Module("muxer",
        fx.Provide(ProvideServices),
    )
}
```

## 相关文档

- [传输协议规范](../../../docs/01-design/protocols/transport/01-transport.md)
- [多路复用协议](../../../docs/01-design/protocols/transport/03-muxer.md)
- [pkg/interfaces/muxer](../../../pkg/interfaces/muxer/)
