# 消息传递流程 (Messaging Flow)

> 消息传递、PubSub 的端到端流程

---

## 文档定位

本文档是 L3_behavioral 的**纵向流程文档**，专注于描述消息传递的完整行为。

### 与横切面的关系

消息传递发生在节点的 Phase C（稳态运行）阶段，详见 [lifecycle_overview.md](lifecycle_overview.md)：

| 生命周期阶段 | 消息相关内容 | 本文档章节 |
|-------------|-------------|-----------|
| Phase C: 稳态运行 | Request-Response 通信 | [请求-响应流程](#请求-响应流程) |
| Phase C: 稳态运行 | PubSub 广播 | [PubSub 流程](#pubsub-流程) |
| Phase C: 稳态运行 | MemberList Gossip 同步 | [GossipSub](#gossipsub-配置) |

---

## 流程概述

DeP2P 提供多种消息传递模式，满足不同场景需求。

```mermaid
flowchart LR
    subgraph P2P["点对点"]
        Request["请求-响应"]
        Notify["单向通知"]
        Stream["流式消息"]
    end
    
    subgraph Broadcast["广播"]
        PubSub["发布订阅"]
        Gossip["GossipSub"]
    end
```

### 消息模式

| 模式 | 适用场景 | 特点 |
|------|----------|------|
| **请求-响应** | RPC、数据请求 | 同步等待响应 |
| **单向通知** | 私聊、命令 | 无需响应 |
| **流式** | 文件传输、批量同步 | 双向流 |
| **PubSub** | 群聊、广播 | 一对多 |

### 参与组件

| 组件 | 目录 | 职责 |
|------|------|------|
| **MessagingService** | `core/messaging/service.go` | 消息服务入口 |
| **RequestHandler** | `core/messaging/request.go` | 请求响应 |
| **PubSub** | `core/messaging/pubsub.go` | 发布订阅 |
| **GossipSub** | `core/messaging/gossipsub/` | Gossip 路由 |
| **Delivery** | `core/messaging/delivery/` | 消息投递 |

---

## 请求-响应流程

```mermaid
sequenceDiagram
    participant App as 应用层
    participant Msg as MessagingService
    participant Host as Host
    participant Remote as 远程节点
    
    App->>Msg: Request(peer, protocol, data)
    
    Note over Msg,EP: 1. 验证成员资格
    Msg->>Msg: 检查 Realm 成员资格
    
    alt 非成员
        Msg-->>App: ErrNotMember
    end
    
    Note over Msg,EP: 2. 打开流
    Msg->>EP: OpenStream(peer, protocol)
    EP-->>Msg: Stream
    
    Note over Msg,Remote: 3. 发送请求
    Msg->>Msg: 封装消息 (Header + Payload)
    Msg->>Remote: REQUEST(req_id, data)
    
    Note over Msg,Remote: 4. 等待响应
    Remote->>Remote: 处理请求
    Remote-->>Msg: RESPONSE(req_id, result)
    
    Note over Msg,App: 5. 返回结果
    Msg->>Msg: 解析响应
    Msg-->>App: result
```

### 请求消息格式

```
请求消息：

  ┌──────────────────────────────────────────────────────┐
  │  Header (12 bytes)                                    │
  ├──────────────────────────────────────────────────────┤
  │  Version (1)  │  Type (1)  │  Flags (2)  │  ReqID (4)│
  │  Length (4)                                           │
  ├──────────────────────────────────────────────────────┤
  │  Payload (变长)                                        │
  └──────────────────────────────────────────────────────┘

  Type:
    REQUEST     = 0x01
    RESPONSE    = 0x02
    ERROR       = 0x06
```

---

## 单向通知流程

```mermaid
sequenceDiagram
    participant App as 应用层
    participant Msg as MessagingService
    participant Host as Host
    participant Remote as 远程节点
    
    App->>Msg: Send(peer, protocol, data)
    
    Msg->>Msg: 检查 Realm 成员资格
    
    Msg->>EP: OpenStream(peer, protocol)
    EP-->>Msg: Stream
    
    Msg->>Remote: NOTIFY(data)
    
    Msg->>Msg: 关闭流
    Msg-->>App: OK (不等待响应)
```

---

## 流式消息流程

### 服务端流

```mermaid
sequenceDiagram
    participant Client as 客户端
    participant Server as 服务端
    
    Client->>Server: REQUEST(query)
    
    Server->>Client: STREAM_DATA(item1)
    Server->>Client: STREAM_DATA(item2)
    Server->>Client: STREAM_DATA(item3)
    Server->>Client: STREAM_END
    
    Client->>Client: 处理所有数据
```

### 双向流

```mermaid
sequenceDiagram
    participant A as 节点 A
    participant B as 节点 B
    
    A->>B: OpenStream("sync")
    
    par 双向传输
        A->>B: STREAM_DATA(data1)
        B->>A: STREAM_DATA(reply1)
        A->>B: STREAM_DATA(data2)
        B->>A: STREAM_DATA(reply2)
    end
    
    A->>B: STREAM_END
    B->>A: STREAM_END
```

---

## PubSub 发布订阅流程

### 订阅流程

```mermaid
sequenceDiagram
    participant App as 应用层
    participant PS as PubSub
    participant GS as GossipSub
    participant Peers as 其他节点
    
    App->>PS: Join("topic")
    PS->>GS: 加入主题
    GS->>Peers: GRAFT(topic)
    Peers-->>GS: 建立 Mesh
    PS-->>App: Topic
    
    App->>PS: Subscribe()
    PS-->>App: Subscription
    
    Note over App,Peers: 开始接收消息
    loop 消息接收
        Peers->>GS: 消息
        GS->>PS: 投递
        PS->>App: 消息回调
    end
```

### 发布流程

```mermaid
sequenceDiagram
    participant App as 应用层
    participant PS as PubSub
    participant GS as GossipSub
    participant M1 as Mesh 节点 1
    participant M2 as Mesh 节点 2
    participant M3 as Mesh 节点 3
    
    App->>PS: Publish("topic", data)
    
    Note over PS,GS: 1. 消息签名
    PS->>PS: 签名消息
    
    Note over GS,M3: 2. 广播到 Mesh
    GS->>M1: 消息
    GS->>M2: 消息
    GS->>M3: 消息
    
    Note over M1,M3: 3. 继续传播
    M1->>M1: 验证签名
    M1->>M1: 转发给自己的 Mesh
```

---

## GossipSub 详细流程

### Mesh 管理

```mermaid
sequenceDiagram
    participant Node as 本节点
    participant Peers as 其他节点
    
    Note over Node,Peers: 心跳周期 (每 1 秒)
    
    loop 心跳
        Node->>Node: 检查 Mesh 连接度
        
        alt 连接度 < D_lo
            Node->>Peers: GRAFT(topic)
            Peers-->>Node: 加入 Mesh
        else 连接度 > D_hi
            Node->>Peers: PRUNE(topic)
            Note over Peers: 移出 Mesh
        end
        
        Node->>Peers: IHAVE(msg_ids)
        Peers->>Node: IWANT(msg_ids)
        Node->>Peers: 消息数据
    end
```

### 消息验证

```mermaid
flowchart TB
    Receive["收到消息"]
    
    Receive --> VerifySig{"签名验证"}
    VerifySig -->|失败| Reject["拒绝 + 惩罚"]
    
    VerifySig -->|成功| VerifyMember{"成员验证"}
    VerifyMember -->|失败| Reject
    
    VerifyMember -->|成功| CheckDup{"重复检查"}
    CheckDup -->|重复| Ignore["忽略"]
    
    CheckDup -->|新消息| Accept["接受 + 传播"]
```

### GossipSub 参数

```
GossipSub 核心参数：

  连接度:
    D = 6       (目标连接度)
    D_lo = 4    (最低连接度)
    D_hi = 12   (最高连接度)
    D_lazy = 6  (lazy push 度)
    
  时间:
    heartbeat_interval = 1s   (心跳间隔)
    fanout_ttl = 60s          (fanout 超时)
    
  缓存:
    mcache_len = 5            (消息缓存长度)
    mcache_gossip = 3         (gossip 窗口)
```

---

## 消息投递确认

对于需要可靠投递的场景：

```mermaid
sequenceDiagram
    participant Sender as 发送方
    participant Delivery as DeliveryService
    participant Receiver as 接收方
    
    Sender->>Delivery: Publish(msg, reliable=true)
    
    Delivery->>Delivery: 生成 MessageID
    Delivery->>Delivery: 加入待确认队列
    
    Delivery->>Receiver: 消息
    Receiver->>Receiver: 处理消息
    Receiver-->>Delivery: ACK(messageID)
    
    Delivery->>Delivery: 从队列移除
    Delivery-->>Sender: 投递成功
    
    alt 超时未确认
        Delivery->>Receiver: 重传消息
    end
```

---

## 消息路由

```mermaid
flowchart TB
    Message["收到消息"]
    
    Message --> CheckTopic{"主题匹配?"}
    
    CheckTopic -->|是| CheckSub{"已订阅?"}
    CheckSub -->|是| Deliver["投递到应用"]
    CheckSub -->|否| Forward["仅转发"]
    
    CheckTopic -->|否| Drop["丢弃"]
    
    Deliver --> Propagate["传播给 Mesh"]
    Forward --> Propagate
```

---

## 消息状态机

```mermaid
stateDiagram-v2
    [*] --> Created
    
    Created --> Sending: Send()
    Sending --> Delivered: 发送成功
    Sending --> Failed: 发送失败
    
    Failed --> Retrying: 重试
    Retrying --> Delivered: 成功
    Retrying --> Failed: 重试次数耗尽
    
    Delivered --> Acknowledged: 收到 ACK
    Delivered --> Expired: 超时无 ACK
    
    Acknowledged --> [*]
    Expired --> Retrying: 重试
    Failed --> [*]
```

---

## 错误处理

### 错误类型

| 错误 | 原因 | 处理 |
|------|------|------|
| **ErrNotMember** | 未加入 Realm | 返回错误 |
| **ErrPeerNotFound** | 目标节点不可达 | 重试或返回错误 |
| **ErrStreamClosed** | 流被关闭 | 重新打开流 |
| **ErrTimeout** | 请求超时 | 重试或返回错误 |
| **ErrTopicNotFound** | 主题不存在 | 自动创建或返回错误 |
| **ErrMessageTooLarge** | 消息过大 | 返回错误 |
| **ErrRateLimited** | 速率限制 | 等待后重试 |

### 重试策略

```
请求重试:
  - 指数退避
  - 初始间隔: 100ms
  - 最大间隔: 30s
  - 最大重试: 5 次

消息投递重试:
  - 固定间隔: 5s
  - 最大重试: 3 次
  - 超时后标记失败
```

---

## 流量控制

```
速率限制：

  发布速率: 100 消息/秒
  消息大小: 1 MB
  订阅数: 100 主题/节点

背压处理：

  队列满:
    - 丢弃旧消息 (可配置)
    - 或暂停接收
```

---

## 代码路径

| 阶段 | 代码路径 |
|------|----------|
| 消息服务 | `internal/protocol/messaging/` |
| 请求响应 | `internal/protocol/messaging/request.go` |
| PubSub | `internal/protocol/pubsub/` |
| GossipSub | `internal/protocol/pubsub/gossipsub/` |
| 消息投递 | `internal/protocol/messaging/delivery/` |
| 消息编解码 | `internal/protocol/messaging/codec.go` |

---

## ★ 可靠消息投递 (v1.3.0 计划中)

> 相关需求：[REQ-PROTO-004](../../01_context/requirements/functional/F6_protocol/REQ-PROTO-004.md)

对于关键消息，支持可靠投递机制：

### 架构设计

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          ReliablePublisher 架构                              │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐                     │
│  │ Application │───▶│  Publisher  │───▶│MessageQueue │                     │
│  └─────────────┘    └─────────────┘    └─────────────┘                     │
│                                               │                             │
│                                               ▼                             │
│                     ┌─────────────┐    ┌─────────────┐                     │
│                     │  ACK处理器  │◀───│   Sender    │                     │
│                     └─────────────┘    └─────────────┘                     │
│                            │                  │                             │
│                            ▼                  ▼                             │
│                     ┌─────────────┐    ┌─────────────┐                     │
│                     │ PendingAcks │    │   Network   │                     │
│                     └─────────────┘    └─────────────┘                     │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 投递流程

```mermaid
sequenceDiagram
    participant App as 应用层
    participant RP as ReliablePublisher
    participant Queue as MessageQueue
    participant Net as Network
    participant Peer as 对端节点
    
    App->>RP: Publish(msg, reliable=true)
    RP->>Queue: 入队
    Queue->>Net: 发送
    Net->>Peer: 消息
    
    alt 网络可达
        Peer->>Net: ACK
        Net->>RP: 收到 ACK
        RP->>Queue: 出队
        RP-->>App: 投递成功
    else 超时
        RP->>Queue: 重新发送
        Note over RP,Queue: 最多重试 3 次
    end
```

### 关键配置

| 参数 | 默认值 | 说明 |
|------|--------|------|
| QueueSize | 1000 | 消息队列大小 |
| MessageMaxAge | 5 分钟 | 消息最大保留时间 |
| AckTimeout | 5 秒 | ACK 等待超时 |
| MaxRetries | 3 | 最大重试次数 |
| CriticalPeers | [] | 关键节点列表（必须 ACK） |

---

## 相关文档

### L3 行为文档

| 文档 | 说明 |
|------|------|
| [lifecycle_overview.md](lifecycle_overview.md) | ★ 节点生命周期横切面（Phase C: 稳态运行） |
| [realm_flow.md](realm_flow.md) | Realm 加入流程 |
| [connection_flow.md](connection_flow.md) | 连接建立流程 |
| [state_machines.md](state_machines.md) | 状态机定义 |

### 协议规范与需求

| 文档 | 说明 |
|------|------|
| [Messaging 协议规范](../../02_constraints/protocol/L4_application/messaging.md) | 协议详情 |
| [PubSub 协议规范](../../02_constraints/protocol/L4_application/pubsub.md) | 协议详情 |
| [REQ-PROTO-004](../../01_context/requirements/functional/F6_protocol/REQ-PROTO-004.md) | 可靠消息投递需求 |

---

**最后更新**：2026-01-25（添加 lifecycle_overview.md 引用关系）
