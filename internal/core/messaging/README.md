# Messaging 消息服务模块

## 概述

**层级**: Tier 3  
**职责**: 提供高级通信模式，包括请求响应、发布订阅、查询，支持 Realm 感知。

## 设计引用

> **重要**: 实现前请详细阅读以下设计规范

| 设计文档 | 说明 |
|----------|------|
| [消息传递协议](../../../docs/01-design/protocols/application/03-messaging.md) | 三种核心通信模式 |
| [Realm 协议](../../../docs/01-design/protocols/application/04-realm.md) | Realm 感知的 Pub-Sub |

## 能力清单

### Request-Response 能力 (必须实现)

| 能力 | 状态 | 说明 |
|------|------|------|
| 同步请求 | ✅ 已实现 | Request() 等待响应 |
| 异步发送 | ✅ 已实现 | Send() 不等待响应 |
| 请求处理器 | ✅ 已实现 | SetRequestHandler() |
| 超时控制 | ✅ 已实现 | 请求超时机制 |
| 状态码 | ✅ 已实现 | 200/400/404/408/500 |

### Pub-Sub 能力 (必须实现)

| 能力 | 状态 | 说明 |
|------|------|------|
| 发布消息 | ✅ 已实现 | Publish() |
| 订阅主题 | ✅ 已实现 | Subscribe() |
| 消息去重 | ✅ 已实现 | 防止重复接收 |
| Gossip 传播 | ✅ 已实现 | GossipSub v1.1 |
| Realm 隔离 | ✅ 已实现 | 消息在 Realm 内传播 |

### Query 能力 (可选)

| 能力 | 状态 | 说明 |
|------|------|------|
| 单响应查询 | ✅ 已实现 | Query() 等待首个响应 |
| 多响应查询 | ✅ 已实现 | QueryAll() 收集多个响应 |

## 依赖关系

### 接口依赖

```
pkg/types/               → NodeID, ProtocolID, Request, Response, Message
pkg/interfaces/core/     → Stream, Connection
pkg/interfaces/messaging/ → MessagingService, Subscription
```

### 模块依赖

```
transport → 底层连接
protocol  → 协议管理
endpoint  → 连接管理
```

## 目录结构

```
messaging/
├── README.md            # 本文件
├── module.go            # fx 模块定义
├── reqres.go            # 请求响应实现
├── pubsub.go            # 发布订阅实现
└── query.go             # 查询实现
```

## 公共接口

实现 `pkg/interfaces/messaging/` 中的接口：

```go
// MessagingService 消息服务接口
type MessagingService interface {
    // 请求响应模式
    Request(ctx context.Context, nodeID types.NodeID, protocol types.ProtocolID, data []byte) ([]byte, error)
    Send(ctx context.Context, nodeID types.NodeID, protocol types.ProtocolID, data []byte) error
    SetRequestHandler(protocol types.ProtocolID, handler RequestHandler)
    
    // 发布订阅模式
    Publish(ctx context.Context, topic string, data []byte) error
    Subscribe(ctx context.Context, topic string) (Subscription, error)
    Unsubscribe(topic string) error
    
    // 查询模式
    Query(ctx context.Context, topic string, query []byte) ([]byte, error)
    QueryAll(ctx context.Context, topic string, query []byte, opts QueryOptions) ([]QueryResponse, error)
    
    // 生命周期
    Start(ctx context.Context) error
    Stop() error
}

// Subscription 订阅句柄接口
type Subscription interface {
    Topic() string
    Messages() <-chan *Message
    Cancel()
    IsActive() bool
}

// RequestHandler 请求处理器
type RequestHandler func(ctx context.Context, nodeID types.NodeID, data []byte) ([]byte, error)
```

## 关键算法

### 三种核心模式 (来自设计文档)

```
┌─────────────────────────────────────────────────────────────────────┐
│  模式 1: Stream（流）                                                │
│  • 点对点双向通信                                                    │
│  • 长连接，多次交互                                                  │
│  • 适合：文件传输、实时通信、复杂协议                                 │
│                                                                      │
│  A ◄════════════════════════════════════════════════════► B         │
└─────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────┐
│  模式 2: Request-Response（请求响应）                                │
│  • 一问一答                                                          │
│  • 同步语义，等待响应                                                │
│  • 适合：RPC、查询、命令                                             │
│                                                                      │
│  A ──── Request ────► B                                             │
│  A ◄─── Response ──── B                                             │
└─────────────────────────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────────────────────────┐
│  模式 3: Pub-Sub（发布订阅）                                         │
│  • 一对多通信                                                        │
│  • 解耦发送方和接收方                                                │
│  • 适合：事件通知、数据同步、广播                                    │
│                                                                      │
│  Publisher ──┬──► Subscriber1                                       │
│              ├──► Subscriber2                                       │
│              └──► Subscriber3                                       │
└─────────────────────────────────────────────────────────────────────┘
```

### Request-Response 协议

```go
// 协议 ID: /dep2p/reqres/1.0

type Request struct {
    RequestID uint64            // 请求ID，用于匹配响应
    Protocol  types.ProtocolID  // 业务协议
    Payload   []byte            // 请求数据
    Timeout   uint32            // 超时时间（毫秒）
    NeedReply bool              // 是否需要响应
}

type Response struct {
    RequestID uint64  // 对应的请求ID
    Status    uint16  // 状态码
    Payload   []byte  // 响应数据
    Error     string  // 错误信息
}

// 状态码
const (
    StatusOK              = 200  // 成功
    StatusBadRequest      = 400  // 请求格式错误
    StatusNotFound        = 404  // 未找到
    StatusTimeout         = 408  // 请求超时
    StatusPayloadTooLarge = 413  // 负载过大
    StatusInternalError   = 500  // 内部错误
    StatusUnavailable     = 503  // 服务不可用
)
```

### Realm 感知的 Pub-Sub (来自设计文档)

```go
// 主题命名空间:
// 内部实际主题 = RealmID + "/" + 用户主题

func (s *pubSubService) Publish(ctx context.Context, topic string, data []byte) error {
    // 获取当前 Realm
    realmID := s.currentRealm()
    
    // 构建内部主题
    internalTopic := fmt.Sprintf("%s/%s", realmID.String(), topic)
    
    // 创建消息
    msg := &Message{
        ID:        generateMsgID(),
        Topic:     internalTopic,
        Data:      data,
        Timestamp: time.Now(),
        RealmID:   realmID,
    }
    
    // 获取该 topic 的 mesh 邻居（同 Realm）
    peers := s.getMeshPeers(internalTopic)
    
    // 广播消息
    for peer := range peers {
        s.sendMessage(peer, msg)
    }
    
    return nil
}
```

### 消息去重

```go
type messageCache struct {
    seen map[string]time.Time
    ttl  time.Duration
    mu   sync.RWMutex
}

func (c *messageCache) IsSeen(msgID []byte) bool {
    c.mu.RLock()
    defer c.mu.RUnlock()
    _, seen := c.seen[string(msgID)]
    return seen
}

func (c *messageCache) Add(msgID []byte) {
    c.mu.Lock()
    defer c.mu.Unlock()
    c.seen[string(msgID)] = time.Now()
}

// 定期清理过期消息ID
func (c *messageCache) cleanup() {
    c.mu.Lock()
    defer c.mu.Unlock()
    
    now := time.Now()
    for id, timestamp := range c.seen {
        if now.Sub(timestamp) > c.ttl {
            delete(c.seen, id)
        }
    }
}
```

## fx 模块

```go
type ModuleInput struct {
    fx.In
    Endpoint endpointif.Endpoint `name:"endpoint"`
    Config   *messagingif.Config `optional:"true"`
}

type ModuleOutput struct {
    fx.Out
    MessagingService messagingif.MessagingService `name:"messaging"`
}

func Module() fx.Option {
    return fx.Module("messaging",
        fx.Provide(ProvideServices),
        fx.Invoke(registerLifecycle),
    )
}
```

## 配置参数

```go
type Config struct {
    // Request-Response 配置
    DefaultTimeout    time.Duration  // 默认超时 30s
    MaxPayloadSize    int            // 最大负载 1MB
    
    // Pub-Sub 配置
    MaxTopics         int            // 最大订阅主题数
    MessageCacheTTL   time.Duration  // 消息缓存时间 2min
    HeartbeatInterval time.Duration  // Gossip 心跳间隔 1s
    
    // Query 配置
    QueryTimeout      time.Duration  // 查询超时 10s
    MaxQueryResponses int            // 最大响应数 100
}
```

## 相关文档

- [消息传递协议](../../../docs/01-design/protocols/application/03-messaging.md)
- [Realm 协议](../../../docs/01-design/protocols/application/04-realm.md)
- [pkg/interfaces/messaging](../../../pkg/interfaces/messaging/)
