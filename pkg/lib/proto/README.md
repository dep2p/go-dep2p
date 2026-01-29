# pkg/proto - 协议消息定义

> DeP2P 的网络协议消息（wire format）

---

## 概述

本包定义 DeP2P 的 **Protobuf 协议消息**，用于跨网络传输的数据序列化。

| 特征 | 说明 |
|-----|------|
| 目的 | 定义 **wire format**（网络字节格式） |
| 用途 | 异构设备通信、跨语言序列化 |
| 序列化 | Protobuf 二进制编码 |
| 稳定性 | 需要版本兼容（向后/向前兼容） |
| 变更成本 | **高**（影响网络协议） |

---

## 与 pkg/types 的边界

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

## 子包

| 子包 | 说明 | 主要消息 |
|-----|------|---------|
| [gossipsub/](gossipsub/) | GossipSub 发布订阅协议 | RPC, Message, ControlMessage |
| [rendezvous/](rendezvous/) | Rendezvous 命名空间发现协议 | Register, Discover, Registration |
| [realm/](realm/) | Realm 成员认证协议 | RealmAuthRequest, RealmAuthResponse |
| [peer/](peer/) | 节点记录（DHT 存储） | PeerRecord, SignedPeerRecord, AddressInfo |
| [key/](key/) | 密钥序列化格式 | PublicKey, PrivateKey, SignedData |

---

## 使用方式

### 生成 Go 代码

```bash
# 生成单个 proto 文件
protoc --go_out=. --go_opt=paths=source_relative \
  pkg/proto/gossipsub/gossipsub.proto

# 生成所有 proto 文件
find pkg/proto -name "*.proto" -exec \
  protoc --go_out=. --go_opt=paths=source_relative {} \;
```

### 在代码中使用

```go
import (
    "github.com/dep2p/go-dep2p/pkg/proto/gossipsub"
    "github.com/dep2p/go-dep2p/pkg/proto/peer"
)

// 创建 GossipSub 消息
msg := &gossipsub.Message{
    From:  nodeID.Bytes(),
    Topic: "my-topic",
    Data:  []byte("hello"),
}

// 序列化
data, err := proto.Marshal(msg)

// 反序列化
var received gossipsub.Message
err = proto.Unmarshal(data, &received)
```

---

## 设计原则

1. **向后兼容**：新版本必须能解析旧版本消息
2. **向前兼容**：旧版本应忽略未知字段
3. **最小化**：只包含网络传输必需的字段
4. **明确语义**：每个字段有清晰的含义和用途

---

## 相关文档

| 文档 | 说明 |
|-----|------|
| [pkg/types/](../types/) | Go 内部数据结构 |
| [pkg/interfaces/](../interfaces/) | 公共接口定义 |

---

**最后更新**：2026-01-12
