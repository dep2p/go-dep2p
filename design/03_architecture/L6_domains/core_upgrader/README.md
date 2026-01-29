# Core Upgrader 模块

> **版本**: v1.1.0  
> **更新日期**: 2026-01-13  
> **定位**: 连接升级（Core Layer）

---

## 模块概述

core_upgrader 负责将原始网络连接升级为安全、多路复用的 P2P 连接。它协调安全握手和流复用器的协商。

| 属性 | 值 |
|------|-----|
| **架构层** | Core Layer |
| **代码位置** | `internal/core/upgrader/` |
| **Fx 模块** | `fx.Module("upgrader")` |
| **状态** | ✅ 已实现 |
| **依赖** | security, muxer |

---

## 核心职责

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                       core_upgrader 职责                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. 连接升级流程                                                             │
│     • 原始连接 → 安全连接 → 多路复用连接                                    │
│                                                                             │
│  2. 安全层协商                                                               │
│     • 支持多种安全传输 (TLS 1.3, Noise)                                     │
│     • 安全协议 multistream 协商                                             │
│     • 双向身份验证                                                          │
│                                                                             │
│  3. 多路复用器协商                                                           │
│     • 支持多种复用器 (yamux, mplex)                                         │
│     • 复用器 multistream 协商                                               │
│                                                                             │
│  4. 早期数据处理                                                             │
│     • 处理升级过程中的早期数据                                              │
│     • 确保数据不丢失                                                        │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 升级流程

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          连接升级流程                                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│    Raw Connection (TCP/WebSocket)                                           │
│         │                                                                   │
│         ▼                                                                   │
│    ┌─────────────────────────────────────────┐                             │
│    │        Security Negotiation             │                             │
│    │  multistream-select: /tls/1.0.0         │                             │
│    │                      /noise             │                             │
│    └─────────────────────────────────────────┘                             │
│         │                                                                   │
│         ▼                                                                   │
│    ┌─────────────────────────────────────────┐                             │
│    │        Security Handshake               │                             │
│    │  • 密钥交换                              │                             │
│    │  • 身份验证 (PeerID 验证)               │                             │
│    │  • 加密通道建立                          │                             │
│    └─────────────────────────────────────────┘                             │
│         │                                                                   │
│         ▼                                                                   │
│    ┌─────────────────────────────────────────┐                             │
│    │        Muxer Negotiation                │                             │
│    │  multistream-select: /yamux/1.0.0       │                             │
│    │                      /mplex/6.7.0       │                             │
│    └─────────────────────────────────────────┘                             │
│         │                                                                   │
│         ▼                                                                   │
│    Upgraded Connection (Secure + Multiplexed)                               │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### QUIC 特殊处理

```
QUIC Connection (自带加密和多路复用)
     │
     ▼
跳过 Security 和 Muxer 协商
     │
     ▼
直接创建 Upgraded Connection

QUIC 优势：
• 减少 1-RTT 握手延迟
• 原生支持 0-RTT
• 更好的连接迁移
```

---

## 公共接口

```go
// pkg/interfaces/upgrader.go

// Upgrader 连接升级器接口
type Upgrader interface {
    // Upgrade 升级连接
    //
    // 参数：
    //  - ctx: 上下文（用于超时控制）
    //  - conn: 原始网络连接
    //  - dir: 连接方向（Inbound/Outbound）
    //  - remotePeer: 远程节点 ID（Outbound 必须提供）
    //
    // 返回：
    //  - UpgradedConn: 升级后的连接
    //  - error: 协商失败、握手失败等
    Upgrade(ctx context.Context, conn net.Conn, dir Direction, 
            remotePeer types.PeerID) (UpgradedConn, error)
}

// UpgradedConn 升级后的连接接口
type UpgradedConn interface {
    MuxedConn                          // 多路复用连接
    
    LocalPeer() types.PeerID           // 本地节点 ID
    RemotePeer() types.PeerID          // 远端节点 ID
    Security() types.ProtocolID        // 安全协议（如 "/tls/1.0.0"）
    Muxer() string                     // 多路复用器（如 "/yamux/1.0.0"）
}
```

---

## 协商协议

```
Multistream-Select 协商过程：

Client                           Server
  │                                 │
  │ ───── /multistream/1.0.0 ────▶ │  协议头
  │ ◀──── /multistream/1.0.0 ───── │
  │                                 │
  │ ───── /tls/1.0.0 ────────────▶ │  选择 TLS
  │ ◀──── /tls/1.0.0 ───────────── │  确认
  │                                 │
  │ ══════ TLS Handshake ═════════ │  安全握手
  │                                 │
  │ ───── /yamux/1.0.0 ──────────▶ │  选择 yamux
  │ ◀──── /yamux/1.0.0 ─────────── │  确认
  │                                 │
  │ ══════ Muxer Active ══════════ │  开始复用
  │                                 │
```

---

## 参考实现

### go-libp2p Upgrader

```
github.com/libp2p/go-libp2p/p2p/net/upgrader/
├── upgrader.go       # 主升级器
├── listener.go       # 升级监听器
└── metrics.go        # 指标

github.com/multiformats/go-multistream/
├── client.go         # 客户端协商
├── multistream.go    # 协议选择
└── lazyConn.go       # 延迟协商
```

---

## 相关文档

| 文档 | 说明 |
|------|------|
| [core_security](../core_security/) | 安全传输 |
| [core_muxer](../core_muxer/) | 流复用器 |
| [core_swarm](../core_swarm/) | 连接群管理 |
| [L2: 五层架构](../../L2_structural/layer_model.md) | 架构定义 |

---

**最后更新**：2026-01-25
