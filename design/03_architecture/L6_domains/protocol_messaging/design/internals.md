# core_messaging 内部设计

> 内部结构与实现细节

---

## 内部结构

```
internal/core/messaging/
├── module.go           # Fx 模块
├── messaging.go        # 点对点消息
├── pubsub.go           # GossipSub
├── streams.go          # 流管理
├── validator.go        # 消息验证
└── messaging_test.go   # 测试
```

---

## 点对点消息实现

### 请求-响应

```
func (s *messagingService) Request(ctx context.Context, peer types.NodeID, proto string, data []byte) ([]byte, error) {
    // 1. 验证成员资格 (INV-002)
    if !s.realm.IsMember(peer) {
        return nil, ErrNotMember
    }
    
    // 2. 打开流
    stream, err := s.host.NewStream(ctx, peer, proto)
    if err != nil {
        return nil, err
    }
    defer stream.Close()
    
    // 3. 发送请求
    if err := writeMsg(stream, data); err != nil {
        return nil, err
    }
    
    // 4. 读取响应
    return readMsg(stream)
}
```

### 单向通知

```
func (s *messagingService) Send(ctx context.Context, peer types.NodeID, proto string, data []byte) error {
    if !s.realm.IsMember(peer) {
        return ErrNotMember
    }
    
    stream, err := s.host.NewStream(ctx, peer, proto)
    if err != nil {
        return err
    }
    defer stream.Close()
    
    return writeMsg(stream, data)
}
```

---

## GossipSub 实现

### 主题管理

```
type topicImpl struct {
    ps    *pubsub.PubSub
    topic *pubsub.Topic
    name  string
}

func (t *topicImpl) Publish(ctx context.Context, data []byte) error {
    return t.topic.Publish(ctx, data)
}

func (t *topicImpl) Subscribe() (Subscription, error) {
    sub, err := t.topic.Subscribe()
    if err != nil {
        return nil, err
    }
    return &subscription{sub: sub}, nil
}
```

### 消息验证

```
func (s *pubSubService) validator(ctx context.Context, peer peer.ID, msg *pubsub.Message) pubsub.ValidationResult {
    // 1. 验证发送者是 Realm 成员
    nodeID := types.NodeIDFromPeerID(peer)
    if !s.realm.IsMember(nodeID) {
        return pubsub.ValidationReject
    }
    
    // 2. 验证消息格式
    if len(msg.Data) > maxMessageSize {
        return pubsub.ValidationReject
    }
    
    return pubsub.ValidationAccept
}
```

---

## 流管理

```
type streamManager struct {
    host     host.Host
    handlers map[string]StreamHandler
    mu       sync.RWMutex
}

func (m *streamManager) SetHandler(proto string, handler StreamHandler) {
    m.mu.Lock()
    defer m.mu.Unlock()
    
    m.handlers[proto] = handler
    m.host.SetStreamHandler(proto, func(s network.Stream) {
        handler(&streamWrapper{Stream: s})
    })
}

func (m *streamManager) Open(ctx context.Context, peer types.NodeID, proto string) (Stream, error) {
    s, err := m.host.NewStream(ctx, peer, proto)
    if err != nil {
        return nil, err
    }
    return &streamWrapper{Stream: s}, nil
}
```

---

## 错误处理

```
var (
    ErrNotMember       = errors.New("messaging: peer is not realm member")
    ErrTopicNotFound   = errors.New("messaging: topic not found")
    ErrMessageTooLarge = errors.New("messaging: message too large")
    ErrStreamClosed    = errors.New("messaging: stream closed")
)
```

---

**最后更新**：2026-01-11
