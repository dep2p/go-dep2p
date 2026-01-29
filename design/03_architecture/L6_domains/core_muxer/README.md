# Core Muxer 模块

> **版本**: v1.1.0  
> **更新日期**: 2026-01-13  
> **定位**: 流多路复用（Core Layer）

---

## 模块概述

core_muxer 提供流多路复用能力，在单个连接上支持多个并发流。

| 属性 | 值 |
|------|-----|
| **架构层** | Core Layer |
| **代码位置** | `internal/core/muxer/` |
| **Fx 模块** | `fx.Module("muxer")` |
| **状态** | ✅ 已实现 |
| **依赖** | 无 |

---

## 核心职责

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        core_muxer 职责                                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. 流多路复用                                                              │
│     • 单连接多流                                                            │
│     • 流 ID 分配                                                            │
│     • 流生命周期管理                                                        │
│                                                                             │
│  2. 流控制                                                                  │
│     • 流量控制                                                              │
│     • 背压处理                                                              │
│     • 窗口管理                                                              │
│                                                                             │
│  注：QUIC 内置多路复用，此模块主要用于 TCP 连接                              │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 支持的多路复用器

| 复用器 | 协议 ID | 默认 | 说明 |
|--------|---------|------|------|
| yamux | `/yamux/1.0.0` | ✅ | 推荐，go-libp2p 默认 |
| mplex | `/mplex/6.7.0` | | 旧版兼容 |

---

## 公共接口

```go
// pkg/interfaces/muxer.go

// StreamMuxer 流多路复用器接口
type StreamMuxer interface {
    // NewConn 创建多路复用连接
    NewConn(c net.Conn, isServer bool, scope PeerScope) (MuxedConn, error)
    
    // ID 返回协议 ID
    ID() protocol.ID
}

// MuxedConn 多路复用连接接口
type MuxedConn interface {
    // OpenStream 打开新流
    OpenStream(ctx context.Context) (MuxedStream, error)
    
    // AcceptStream 接受入站流
    AcceptStream() (MuxedStream, error)
    
    // IsClosed 检查是否关闭
    IsClosed() bool
    
    // Close 关闭连接
    Close() error
}

// MuxedStream 多路复用流接口
type MuxedStream interface {
    io.ReadWriteCloser
    
    // Reset 重置流
    Reset() error
    
    // SetDeadline 设置超时
    SetDeadline(time.Time) error
}
```

---

## yamux 配置

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `AcceptBacklog` | 256 | 接受队列大小 |
| `EnableKeepAlive` | true | 启用心跳 |
| `KeepAliveInterval` | 30s | 心跳间隔 |
| `ConnectionWriteTimeout` | 10s | 写超时 |
| `MaxStreamWindowSize` | 16MB | 最大窗口大小 |

---

## 目录结构

```
internal/core/muxer/
├── doc.go
├── module.go           # Fx 模块
├── muxer.go            # 多路复用器抽象
├── yamux/              # yamux 实现
│   ├── muxer.go
│   └── stream.go
└── interfaces/
    └── muxer.go
```

---

## 相关文档

| 文档 | 说明 |
|------|------|
| [core_upgrader](../core_upgrader/) | 连接升级（协商复用器） |
| [core_transport](../core_transport/) | 传输层 |
| [L2: 五层架构](../../L2_structural/layer_model.md) | 架构定义 |

---

**最后更新**：2026-01-13
