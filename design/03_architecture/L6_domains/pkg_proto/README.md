# Pkg Proto 模块

> **版本**: v1.1.0  
> **更新日期**: 2026-01-13  
> **定位**: Protobuf 协议消息定义

---

## 模块概述

pkg_proto 定义 DeP2P 的网络协议消息（wire format），使用 Protobuf 序列化。

| 属性 | 值 |
|------|-----|
| **架构层** | Level 0（基础工具包） |
| **代码位置** | `pkg/proto/` |
| **Fx 模块** | 无（工具包，直接调用） |
| **状态** | ✅ 已实现 |
| **依赖** | google.golang.org/protobuf |

---

## 核心职责

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        pkg_proto 职责                                        │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. 协议消息定义                                                             │
│     • 定义 .proto 文件                                                      │
│     • 生成 .pb.go 代码                                                      │
│     • wire format 规范                                                      │
│                                                                             │
│  2. 网络序列化                                                               │
│     • Protobuf 二进制编码                                                   │
│     • 跨语言兼容                                                             │
│     • 版本兼容性                                                             │
│                                                                             │
│  3. 消息类型                                                                 │
│     • 系统协议（identify, autonat, relay, dht）                              │
│     • Realm 协议（realm 认证）                                               │
│     • 应用协议（messaging, gossipsub）                                       │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 消息目录

### 1. 公共消息（common/）

| 消息 | 说明 |
|------|------|
| Timestamp | 时间戳 |
| Error | 错误信息 |
| Result | 通用结果 |
| Version | 版本信息 |
| Metadata | 元数据 |

### 2. 密钥消息（key/）

| 消息 | 说明 |
|------|------|
| PublicKey | 公钥 |
| PrivateKey | 私钥 |
| KeyPair | 密钥对 |
| Signature | 签名 |

**支持的密钥类型**：RSA, Ed25519, Secp256k1, ECDSA

### 3. 节点消息（peer/）

| 消息 | 说明 |
|------|------|
| PeerID | 节点 ID |
| AddrInfo | 节点地址信息 |
| PeerRecord | 节点记录 |
| SignedPeerRecord | 签名的节点记录 |
| AddressInfo | 地址信息 |

### 4. 身份识别（identify/）

| 消息 | 说明 |
|------|------|
| Identify | 身份识别请求 |
| Push | 身份推送（地址更新） |

**协议**: `/dep2p/sys/identify/1.0.0`

### 5. AutoNAT（autonat/）

| 消息 | 说明 |
|------|------|
| Message | AutoNAT 消息 |
| Dial | 拨号请求 |
| DialResponse | 拨号响应 |
| PeerInfo | 节点信息 |

**协议**: `/dep2p/sys/autonat/1.0.0`

### 6. Hole Punch（holepunch/）

| 消息 | 说明 |
|------|------|
| HolePunch | 打洞消息（CONNECT/SYNC） |

**协议**: `/dep2p/sys/holepunch/1.0.0`

### 7. Relay（relay/）

| 消息 | 说明 |
|------|------|
| HopMessage | 中继跳消息 |
| StopMessage | 停止消息 |
| ReservationVoucher | 预留凭证 |
| Peer | 节点信息 |
| Reservation | 预留信息 |

**协议**: `/dep2p/relay/1.0.0/hop`, `/dep2p/relay/1.0.0/stop`

### 8. DHT（dht/）

| 消息 | 说明 |
|------|------|
| Message | DHT 消息（PUT/GET/FIND_NODE） |
| Peer | 节点信息 |
| Record | DHT 记录 |

**协议**: `/dep2p/sys/dht/1.0.0`

### 9. Rendezvous（rendezvous/）

| 消息 | 说明 |
|------|------|
| Message | Rendezvous 消息 |
| Register | 注册请求 |
| Discover | 发现请求 |
| Registration | 注册信息 |

**协议**: `/dep2p/sys/rendezvous/1.0.0`

### 10. Realm（realm/）

| 消息 | 说明 |
|------|------|
| AuthRequest | Realm 认证请求 |
| AuthResponse | Realm 认证响应 |
| JoinRequest | 加入 Realm 请求 |
| JoinResponse | 加入 Realm 响应 |
| MemberInfo | 成员信息 |
| MemberList | 成员列表 |

**协议**: `/dep2p/realm/<realmID>/auth/1.0.0`

### 11. Messaging（messaging/）

| 消息 | 说明 |
|------|------|
| Message | 消息（点对点/广播） |
| Ack | 确认消息 |
| Query | 消息查询 |
| QueryResponse | 查询响应 |

**协议**: `/dep2p/app/<realmID>/messaging/1.0.0`

### 12. GossipSub（gossipsub/）

| 消息 | 说明 |
|------|------|
| RPC | GossipSub RPC |
| Message | 发布消息 |
| ControlMessage | 控制消息 |
| SubOpts | 订阅选项 |

**协议**: `/dep2p/app/<realmID>/pubsub/1.0.0`

---

## 与 pkg/types 的边界

```
┌───────────────────────────────┬─────────────────────────────────────────────┐
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

## 使用方式

### 生成 Go 代码

```bash
cd pkg/proto
go generate ./...
```

### 在代码中使用

```go
import (
    "github.com/dep2p/go-dep2p/pkg/proto/identify"
    "github.com/dep2p/go-dep2p/pkg/proto/realm"
    "google.golang.org/protobuf/proto"
)

// 创建消息
id := &identify.Identify{
    ProtocolVersion: []byte("dep2p/1.0.0"),
    AgentVersion:    []byte("go-dep2p/1.0.0"),
    Protocols:       []string{"/dep2p/sys/ping/1.0.0"},
}

// 序列化
data, _ := proto.Marshal(id)

// 反序列化
var decoded identify.Identify
proto.Unmarshal(data, &decoded)
```

---

## 文件结构

```
pkg/proto/
├── doc.go                  # 包文档
├── generate.go             # Go generate 指令
├── README.md               # 本文件
├── proto_test.go           # 总体测试
│
├── common/                 # 公共消息
│   ├── common.proto
│   ├── common.pb.go
│   └── common_test.go
│
├── key/                    # 密钥消息
├── peer/                   # 节点消息
├── identify/               # 身份识别
├── autonat/                # AutoNAT
├── holepunch/              # 打洞
├── relay/                  # 中继
├── dht/                    # DHT
├── rendezvous/             # Rendezvous
├── realm/                  # Realm（DeP2P 特有）
├── messaging/              # 消息传递（DeP2P 特有）
└── gossipsub/              # GossipSub
```

---

## 测试覆盖率

```
总体覆盖率：~33%（Protobuf 生成代码）
```

| 子包 | 覆盖率 | 状态 |
|------|--------|------|
| common | 44.0% | ✅ |
| key | 51.6% | ✅ |
| peer | 50.5% | ✅ |
| identify | 42.2% | ✅ |
| holepunch | 44.7% | ✅ |
| dht | 27.8% | ✅ |
| autonat | 24.5% | ✅ |
| relay | 23.8% | ✅ |
| realm | 23.0% | ✅ |
| gossipsub | 29.3% | ✅ |
| rendezvous | 18.4% | ✅ |
| messaging | 17.0% | ✅ |

**说明**：Protobuf 生成的代码主要是数据结构和 getter/setter，覆盖率通常低于手写代码。重点是测试序列化/反序列化功能。

---

## 实施状态

| 步骤 | 状态 | 说明 |
|------|------|------|
| Step 1: 设计审查 | ✅ | 参考 go-libp2p proto |
| Step 2: 接口定义 | N/A | 工具包无需接口 |
| Step 3: 测试先行 | ✅ | 9 个测试文件 |
| Step 4: 核心实现 | ✅ | 13 个 proto 文件 |
| Step 5: 测试通过 | ✅ | 所有测试通过 |
| Step 6: 集成验证 | ✅ | go build 成功 |
| Step 7: 设计复盘 | ✅ | proto3 语法，版本兼容 |
| Step 8: 文档更新 | ✅ | L6_domains 完整 |

---

## 相关文档

| 文档 | 说明 |
|------|------|
| [requirements/requirements.md](requirements/requirements.md) | 需求说明 |
| [design/overview.md](design/overview.md) | 设计概述 |
| [design/message_catalog.md](design/message_catalog.md) | 消息目录 |
| [coding/guidelines.md](coding/guidelines.md) | 编码指南 |
| [testing/strategy.md](testing/strategy.md) | 测试策略 |
| [pkg/proto/README.md](../../../../pkg/proto/README.md) | 代码包说明 |

---

**最后更新**：2026-01-13
