# pkg/types - Go 内部数据结构

> DeP2P 的公共类型定义

---

## 概述

本包定义 DeP2P 的 **Go 内部数据结构**，用于模块间数据传递和 API 参数/返回值。

| 特征 | 说明 |
|-----|------|
| 目的 | 定义 **Go 类型**（内存结构） |
| 用途 | 模块间数据传递、API 参数/返回值 |
| 序列化 | 通常不需要（或用 JSON） |
| 稳定性 | API 稳定性 |
| 变更成本 | **中**（仅影响 Go 代码） |
| 依赖 | **零依赖**（最底层包） |

---

## 与 pkg/proto 的边界

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                              边界划分                                        │
├───────────────────────────────┬─────────────────────────────────────────────┤
│         pkg/proto             │              pkg/types                      │
│       ════════════════        │         ════════════════════════════        │
│                               │                                             │
│   【网络协议消息】              │   【Go 内部数据结构】                        │
│                               │                                             │
│   • wire format (Protobuf)    │   • 内存结构 (Go struct)                    │
│   • 跨语言序列化               │   • 模块间数据传递                           │
│   • 协议版本兼容               │   • API 参数/返回值                          │
│   • 变更成本高                 │   • 变更成本中                               │
│                               │                                             │
└───────────────────────────────┴─────────────────────────────────────────────┘
```

---

## 文件组织

### 基础类型

| 文件 | 说明 |
|-----|------|
| `ids.go` | 身份标识：PeerID, RealmID, RealmKey, PSK, StreamID |
| `enums.go` | 通用枚举：KeyType, Direction, NATType, Connectedness, Reachability, Priority |
| `protocol.go` | 协议标识：ProtocolID 及辅助函数、系统协议常量 |
| `multiaddr.go` | Multiaddr 多地址类型及辅助函数 |
| `base58.go` | Base58 编解码（零依赖实现） |
| `errors.go` | 公共错误定义 |

### 网络类型

| 文件 | 说明 |
|-----|------|
| `connection.go` | 连接：ConnInfo, ConnStat, ConnState, ConnScope, ConnectionStat |
| `stream.go` | 流：StreamInfo, StreamStat, StreamState, StreamScope |
| `discovery.go` | 发现：PeerInfo, AddrInfo |

### 业务类型

| 文件 | 说明 |
|-----|------|
| `realm.go` | Realm：RealmInfo, RealmConfig, RealmMember, RealmStats, RealmJoinOptions |

### 事件类型

| 文件 | 说明 |
|-----|------|
| `events.go` | 所有事件：EvtPeerConnected, EvtPeerDiscovered, EvtRealmJoined, EvtNATTypeDetected 等 |

---

## 类型分类

### ID 类型

| 类型 | 说明 | 示例 |
|-----|------|------|
| `PeerID` | 节点唯一标识（公钥派生） | `12D3KooW...` |
| `RealmID` | 业务域标识（PSK 派生） | Base58 编码 |
| `PSK` | 预共享密钥（32字节） | 高熵随机数 |
| `RealmKey` | Realm 密钥（PSK 别名） | 32字节数组 |
| `ProtocolID` | 协议标识 | `/dep2p/sys/ping/1.0.0` |
| `StreamID` | 流标识 | uint64 |

### 枚举类型

| 类型 | 值 |
|-----|------|
| `Direction` | DirInbound, DirOutbound |
| `Connectedness` | NotConnected, Connected, CanConnect, CannotConnect |
| `Reachability` | ReachabilityUnknown, ReachabilityPublic, ReachabilityPrivate |
| `NATType` | NATTypeNone, NATTypeFullCone, NATTypeSymmetric, ... |
| `KeyType` | KeyTypeEd25519, KeyTypeECDSA, KeyTypeRSA, KeyTypeSecp256k1, ... |
| `Priority` | PriorityLow, PriorityNormal, PriorityHigh, PriorityCritical |

### 事件类型

| 事件 | 说明 |
|-----|------|
| `EvtPeerConnected` | 节点连接事件 |
| `EvtPeerDisconnected` | 节点断开事件 |
| `EvtPeerDiscovered` | 节点发现事件 |
| `EvtRealmJoined` | 加入 Realm 事件 |
| `EvtRealmMemberJoined` | Realm 成员加入事件 |
| `EvtNATTypeDetected` | NAT 类型检测事件 |
| `EvtHolePunchAttempt` | 打洞尝试事件 |

---

## 使用示例

```go
import "github.com/dep2p/dep2p/pkg/types"

// 解析 PeerID
peerID, err := types.ParsePeerID("12D3KooW...")

// 创建 PeerInfo
peer := types.NewPeerInfo(peerID, addrs)

// 生成 PSK
psk := types.GeneratePSK()
realmID := psk.DeriveRealmID()

// 创建 Realm 配置
config := types.RealmConfig{
    PSK:      psk,
    AuthMode: types.AuthModePSK,
}

// 使用协议常量
proto := types.ProtocolPing // "/dep2p/sys/ping/1.0.0"

// Base58 编解码
encoded := types.Base58Encode(data)
decoded, err := types.Base58Decode(encoded)
```

---

## 设计原则

1. **不可变性**：类型创建后尽量不可修改，使用值类型
2. **可比较性**：实现 Equal 方法，支持作为 map key
3. **可序列化**：实现 TextMarshaler/Unmarshaler，支持 JSON
4. **安全性**：敏感类型（如 PSK）不实现 String，避免意外泄露
5. **零依赖**：不依赖任何其他 dep2p 内部包（最底层）

---

## 测试

```bash
go test ./pkg/types/...
```

测试覆盖：
- `base58_test.go` - Base58 编解码测试
- `ids_test.go` - ID 类型测试（PeerID, PSK, RealmKey）
- `enums_test.go` - 枚举类型测试

---

## 相关文档

| 文档 | 说明 |
|-----|------|
| [pkg/proto/](../proto/) | 网络协议消息 |
| [pkg/interfaces/](../interfaces/) | 公共接口定义 |
| [设计文档](../../design/03_architecture/L6_domains/pkg_types/) | 详细设计 |

---

**最后更新**：2026-01-13
