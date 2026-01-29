# Pkg Types 模块

> **版本**: v1.1.0  
> **更新日期**: 2026-01-13  
> **定位**: 公共类型定义

---

## 模块概述

pkg_types 定义 DeP2P 的公共类型，是所有模块共享的基础类型库。

| 属性 | 值 |
|------|-----|
| **类型** | 公共包（非架构层） |
| **代码位置** | `pkg/types/` |
| **Fx 模块** | 无（纯类型定义） |
| **状态** | ✅ 已实现 |
| **依赖** | 无（最底层） |

---

## 核心职责

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        pkg_types 职责                                        │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. 身份类型                                                                │
│     • PeerID                                                                │
│     • RealmID                                                               │
│     • PSK                                                                   │
│                                                                             │
│  2. 网络类型                                                                │
│     • Multiaddr（重导出）                                                   │
│     • PeerInfo                                                              │
│     • AddrInfo                                                              │
│                                                                             │
│  3. 协议类型                                                                │
│     • ProtocolID                                                            │
│     • MessageType                                                           │
│                                                                             │
│  4. 事件类型                                                                │
│     • 连接事件                                                              │
│     • 发现事件                                                              │
│     • 协议事件                                                              │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 主要类型

```go
// pkg/types/peerid.go
type PeerID string

// pkg/types/realm.go
type RealmID string
type PSK []byte

// pkg/types/multiaddr.go
type Multiaddr = multiaddr.Multiaddr

// pkg/types/protocol.go
type ProtocolID string

// pkg/types/connection.go
type Direction int
const (
    DirInbound Direction = iota
    DirOutbound
)

// pkg/types/stream.go
type Stream interface {
    io.ReadWriteCloser
    Protocol() ProtocolID
    Conn() Connection
}

// pkg/types/events.go
type EvtPeerConnected struct {
    Peer PeerID
}
```

---

## 目录结构

```
pkg/types/
├── doc.go
├── peerid.go           # PeerID 类型
├── multiaddr.go        # Multiaddr 重导出
├── protocol.go         # 协议 ID
├── stream.go           # 流类型
├── connection.go       # 连接类型
├── realm.go            # Realm 类型
└── events.go           # 事件类型
```

---

## 使用方式

```go
import "github.com/dep2p/dep2p/pkg/types"

var peerID types.PeerID = "12D3KooW..."
var addr types.Multiaddr = multiaddr.StringCast("/ip4/127.0.0.1/tcp/4001")
```

---

## 相关文档

| 文档 | 说明 |
|------|------|
| [pkg/interfaces](../../L4_interfaces/public_interfaces.md) | 公共接口 |
| [L2: 五层架构](../../L2_structural/layer_model.md) | 架构定义 |

---

**最后更新**：2026-01-13
