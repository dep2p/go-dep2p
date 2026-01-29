# pkg_proto 设计概述

> **版本**: v1.1.0  
> **更新日期**: 2026-01-13

---

## 设计目标

pkg_proto 的设计目标是提供**标准化、版本兼容、跨语言**的协议消息定义。

### 核心原则

1. **协议对齐**：系统协议与 go-libp2p 兼容
2. **版本兼容**：向后/向前兼容
3. **最小化**：只包含网络传输必需的字段
4. **明确语义**：每个字段有清晰的含义

---

## 架构设计

### 消息分类

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        pkg_proto 消息分类                                    │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌────────────────────────────────────────────────────────────────────┐     │
│  │                        基础消息                                     │     │
│  │  common, key, peer                                                │     │
│  └────────────────────────────────────────────────────────────────────┘     │
│                                    │                                        │
│           ┌────────────────────────┼────────────────────────┐               │
│           ↓                        ↓                        ↓               │
│  ┌──────────────┐        ┌──────────────┐        ┌──────────────┐          │
│  │  系统协议     │        │ Realm 协议   │        │  应用协议     │          │
│  │  identify    │        │   realm      │        │  messaging   │          │
│  │  autonat     │        │              │        │  gossipsub   │          │
│  │  holepunch   │        │              │        │              │          │
│  │  relay       │        │              │        │              │          │
│  │  dht         │        │              │        │              │          │
│  │  rendezvous  │        │              │        │              │          │
│  └──────────────┘        └──────────────┘        └──────────────┘          │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 与 pkg/types 的关系

### 边界划分

| 维度 | pkg/proto | pkg/types |
|------|-----------|-----------|
| **用途** | 网络传输 | 内存数据 |
| **格式** | Protobuf 二进制 | Go struct |
| **变更成本** | 高（影响协议） | 中（内部重构） |
| **生命周期** | 网络通信期间 | 程序运行期间 |
| **跨语言** | 支持 | 不支持 |

### 转换策略

```go
// proto -> types（在接收端）
func ProtoToTypes(protoMsg *peer.PeerRecord) *types.PeerInfo {
    return &types.PeerInfo{
        ID: types.PeerID(protoMsg.PeerId),
        // ...
    }
}

// types -> proto（在发送端）
func TypesToProto(info *types.PeerInfo) *peer.PeerRecord {
    return &peer.PeerRecord{
        PeerId: []byte(info.ID),
        // ...
    }
}
```

**注意**：转换函数在**使用处**实现，不在 pkg/proto 中。

---

## 消息设计模式

### 1. 请求/响应模式

```protobuf
// 请求消息
message AuthRequest {
  bytes realm_id = 1;
  bytes peer_id = 2;
  bytes challenge_response = 3;
}

// 响应消息
message AuthResponse {
  enum Status {
    OK = 0;
    E_AUTH_FAILED = 100;
  }
  Status status = 1;
  string status_text = 2;
  bytes member_token = 3;
}
```

### 2. 枚举状态码

```protobuf
enum Status {
  UNUSED = 0;          // proto3 要求
  OK = 100;            // 成功（1xx）
  CLIENT_ERROR = 200;  // 客户端错误（2xx）
  SERVER_ERROR = 300;  // 服务端错误（3xx）
  INVALID = 400;       // 无效请求（4xx）
}
```

**规则**：
- 0 保留给默认值
- 1xx: 成功状态
- 2xx: 客户端错误
- 3xx: 服务端错误
- 4xx: 消息格式错误

### 3. 嵌套消息

```protobuf
message Message {
  enum MessageType {
    REGISTER = 0;
    DISCOVER = 1;
  }
  
  message Register {
    string ns = 1;
    uint64 ttl = 2;
  }
  
  MessageType type = 1;
  Register register = 2;
}
```

---

## 版本兼容性设计

### Proto3 兼容性规则

1. **字段只增不减**：新版本可添加字段，不可删除
2. **字段编号不变**：已分配的编号永不更改
3. **默认值处理**：未设置的字段使用类型默认值
4. **未知字段忽略**：旧版本忽略新字段

### 字段编号规划

```
1-15:     核心字段（1 字节 varint）
16-2047:  扩展字段（2 字节 varint）
2048+:    预留
```

**示例**：
```protobuf
message Message {
  bytes id = 1;       // 核心字段
  bytes from = 2;     // 核心字段
  string topic = 3;   // 核心字段
  
  uint64 timestamp = 16;  // 扩展字段
  map<string, bytes> metadata = 17;  // 扩展字段
}
```

---

## 协议消息映射

### 系统协议（go-libp2p 兼容）

| DeP2P proto | libp2p proto | 兼容性 |
|-------------|--------------|--------|
| identify/ | core/crypto, id/ | ✅ 兼容 |
| autonat/ | p2p/host/autonat/ | ✅ 兼容 |
| holepunch/ | p2p/protocol/holepunch/ | ✅ 兼容 |
| relay/ | p2p/protocol/circuitv2/ | ✅ 兼容 |
| dht/ | p2p/protocol/kad-dht/ | ✅ 部分兼容 |

### DeP2P 特有协议

| proto | 说明 | 用途 |
|-------|------|------|
| realm/ | Realm 认证 | PSK 认证、成员管理 |
| messaging/ | 消息传递 | 点对点/广播消息 |

---

## 数据流

### 发送端

```mermaid
graph LR
    TypesData[types.PeerInfo] --> Convert[转换函数]
    Convert --> ProtoMsg[proto.PeerRecord]
    ProtoMsg --> Marshal[proto.Marshal]
    Marshal --> Bytes[字节流]
    Bytes --> Network[网络传输]
```

### 接收端

```mermaid
graph LR
    Network[网络接收] --> Bytes[字节流]
    Bytes --> Unmarshal[proto.Unmarshal]
    Unmarshal --> ProtoMsg[proto.PeerRecord]
    ProtoMsg --> Convert[转换函数]
    Convert --> TypesData[types.PeerInfo]
```

---

## 关键设计决策

### 1. proto3 vs proto2

**决策**：使用 proto3 语法

**理由**：
- ✅ 简化语法（无 required/optional）
- ✅ 更好的向前兼容
- ✅ 官方推荐
- ✅ 工具支持更好

### 2. go_package 路径

**决策**：使用 `github.com/dep2p/dep2p/pkg/proto/<subpkg>`

**理由**：
- ✅ 符合 Go module 规范
- ✅ 便于导入
- ✅ 避免路径冲突

### 3. 消息嵌套 vs 平铺

**决策**：相关消息嵌套定义

**理由**：
- ✅ 命名空间隔离
- ✅ 减少顶层类型
- ✅ 语义更清晰

---

## 相关文档

- [message_catalog.md](message_catalog.md) - 消息目录
- [../requirements/requirements.md](../requirements/requirements.md) - 需求说明
- [../coding/guidelines.md](../coding/guidelines.md) - 编码指南

---

**最后更新**：2026-01-13
