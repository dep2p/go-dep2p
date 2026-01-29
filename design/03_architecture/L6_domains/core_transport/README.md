# Core Transport 模块

> **版本**: v1.1.0  
> **更新日期**: 2026-01-13  
> **定位**: 传输层（Core Layer）

---

## 模块概述

core_transport 提供网络传输层实现，支持 QUIC、TCP 和 WebSocket 等多种传输协议。

| 属性 | 值 |
|------|-----|
| **架构层** | Core Layer |
| **代码位置** | `internal/core/transport/` |
| **Fx 模块** | `fx.Module("transport")` |
| **状态** | ✅ 已实现 |
| **依赖** | core_identity |

---

## 核心职责

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        core_transport 职责                                   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. 连接管理                                                                │
│     • 监听入站连接                                                          │
│     • 发起出站连接                                                          │
│     • 连接生命周期管理                                                      │
│                                                                             │
│  2. QUIC 传输（默认）                                                       │
│     • 0-RTT 快速握手                                                        │
│     • 多路复用（内置）                                                      │
│     • 拥塞控制                                                              │
│     • 连接迁移                                                              │
│                                                                             │
│  3. TCP 传输                                                                │
│     • 兼容传统网络                                                          │
│     • 需配合 yamux 多路复用                                                 │
│                                                                             │
│  4. WebSocket 传输                                                          │
│     • 浏览器兼容                                                            │
│     • 防火墙穿透                                                            │
│                                                                             │
│  5. 地址处理                                                                │
│     • Multiaddr 解析                                                        │
│     • 协议适配                                                              │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 支持的传输协议

| 传输 | Multiaddr 格式 | 默认 | 说明 |
|------|---------------|------|------|
| QUIC | `/ip4/.../udp/.../quic-v1` | ✅ | 推荐，内置加密和多路复用 |
| TCP | `/ip4/.../tcp/...` | | 传统协议，需 Upgrader |
| WebSocket | `/ip4/.../tcp/.../ws` | | 浏览器兼容 |

---

## 公共接口

```go
// pkg/interfaces/transport.go

// Transport 传输接口
type Transport interface {
    // Dial 拨号连接到远程节点
    Dial(ctx context.Context, raddr types.Multiaddr, p types.PeerID) (Connection, error)
    
    // Listen 在指定地址监听
    Listen(laddr types.Multiaddr) (Listener, error)
    
    // Protocols 返回支持的协议 ID
    Protocols() []types.ProtocolID
    
    // Close 关闭传输层
    Close() error
}

// Listener 监听器接口
type Listener interface {
    Accept() (Connection, error)
    Addr() net.Addr
    Multiaddr() types.Multiaddr
    Close() error
}
```

> **TransportManager**：统一管理 QUIC + TCP 传输，支持 Rebind（网络切换）。
> QUIC 自带 TLS 加密和多路复用；TCP 需要 Upgrader 进行安全握手和 yamux 多路复用。

---

## 目录结构

```
internal/core/transport/
├── doc.go              # 包文档
├── module.go           # TransportManager + Fx 模块定义
├── errors.go           # 错误定义
├── quic/               # QUIC 实现
│   ├── transport.go    # QUIC 传输
│   ├── listener.go     # QUIC 监听器
│   ├── conn.go         # QUIC 连接
│   ├── stream.go       # QUIC 流
│   ├── tls.go          # TLS 配置（从 Identity 生成）
│   └── rebind.go       # 网络切换支持
├── tcp/                # TCP 实现
│   ├── transport.go    # TCP 传输
│   ├── listener.go     # TCP 监听器
│   └── conn.go         # TCP 连接（需 Upgrader）
└── *_test.go           # 单元测试
```

---

## 配置参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `EnableQUIC` | true | 启用 QUIC |
| `EnableTCP` | true | 启用 TCP |
| `EnableWebSocket` | false | 启用 WebSocket |
| `QUICConfig.MaxIdleTimeout` | 30s | QUIC 空闲超时 |

---

## 相关文档

| 文档 | 说明 |
|------|------|
| [core_upgrader](../core_upgrader/) | 连接升级（TCP 需要） |
| [core_security](../core_security/) | 安全层 |
| [core_swarm](../core_swarm/) | 连接群管理 |
| [L2: 五层架构](../../L2_structural/layer_model.md) | 架构定义 |

---

**最后更新**：2026-01-25
