# Protocol PubSub - 发布订阅

> **版本**: v1.0.0  
> **状态**: ✅ 已完成  
> **架构层**: Protocol Layer

---

## 概述

`pubsub` 实现基于 GossipSub v1.1 的发布订阅协议，提供可靠的消息广播能力。

**协议标识**: `/dep2p/app/<realmID>/pubsub/1.0.0`

**核心功能**:
- 📢 消息发布 - 发布消息到主题
- 📥 消息订阅 - 订阅主题消息
- 🕸️ GossipSub - 基于 Mesh 的消息传播
- 💓 心跳机制 - 周期性维护 Mesh
- ✅ 消息验证 - Realm 成员验证 + 去重

---

## 快速开始

### 发布订阅

```go
import "github.com/dep2p/go-dep2p/internal/protocol/pubsub"

// 创建服务
svc, err := pubsub.New(host, realmMgr)
if err != nil {
    log.Fatal(err)
}

if err := svc.Start(ctx); err != nil {
    log.Fatal(err)
}
defer svc.Stop(ctx)

// 加入主题
topic, err := svc.Join("my-topic")
if err != nil {
    log.Fatal(err)
}

// 订阅主题
sub, err := topic.Subscribe()
if err != nil {
    log.Fatal(err)
}

// 发布消息
err = topic.Publish(ctx, []byte("hello"))

// 接收消息
msg, err := sub.Next(ctx)
fmt.Printf("Received: %s from %s\n", msg.Data, msg.From)
```

### 事件处理

```go
handler, err := topic.EventHandler()
if err != nil {
    log.Fatal(err)
}

for {
    event, err := handler.NextPeerEvent(ctx)
    if err != nil {
        break
    }
    
    switch event.Type {
    case interfaces.PeerJoin:
        fmt.Printf("Peer joined: %s\n", event.Peer)
    case interfaces.PeerLeave:
        fmt.Printf("Peer left: %s\n", event.Peer)
    }
}
```

---

## 子模块

| 子模块 | 说明 |
|--------|------|
| `delivery/` | 消息可靠投递（ACK、重传队列） |

---

## 配置

```go
svc, err := pubsub.New(
    host,
    realmMgr,
    pubsub.WithHeartbeatInterval(time.Second),  // 心跳间隔
    pubsub.WithMeshDegree(6, 4, 12),           // Mesh 度数 (D, D_lo, D_hi)
    pubsub.WithMaxMessageSize(1<<20),          // 最大消息 1MB
)
```

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `HeartbeatInterval` | `1s` | 心跳间隔 |
| `D` | `6` | Mesh 目标度数 |
| `D_lo` | `4` | Mesh 最小度数 |
| `D_hi` | `12` | Mesh 最大度数 |
| `MaxMessageSize` | `1MB` | 最大消息大小 |

---

## GossipSub 协议

遵循 GossipSub v1.1 规范：

| 消息类型 | 说明 |
|----------|------|
| `GRAFT` | 请求加入 Mesh |
| `PRUNE` | 请求离开 Mesh |
| `IHAVE` | 告知拥有的消息 |
| `IWANT` | 请求消息 |

### Mesh 维护

```
心跳周期 (1s):
├── 检查 Mesh 度数
├── 度数 < D_lo → 发送 GRAFT 请求
├── 度数 > D_hi → 发送 PRUNE 请求
├── 传播 IHAVE 消息
└── 清理过期消息缓存
```

---

## 错误处理

| 错误 | 说明 |
|------|------|
| `ErrNotStarted` | 服务未启动 |
| `ErrTopicNotFound` | 主题未找到 |
| `ErrTopicAlreadyJoined` | 主题已加入 |
| `ErrTopicClosed` | 主题已关闭 |
| `ErrSubscriptionCancelled` | 订阅已取消 |
| `ErrMessageTooLarge` | 消息过大 |
| `ErrDuplicateMessage` | 重复消息 |

---

## 性能特性

- **消息延迟**: < 200ms (局域网)
- **吞吐量**: > 1000 msg/s
- **Mesh 度数**: 6 (可配置)
- **消息去重**: LRU 缓存 + TTL
- **并发安全**: 所有方法并发安全

---

## 测试

```bash
go test -v ./internal/protocol/pubsub/...
go test -cover ./internal/protocol/pubsub/...
go test -bench=. ./internal/protocol/pubsub/...
```

---

## 相关文档

- [doc.go](doc.go) - 包文档
- [delivery/doc.go](delivery/doc.go) - 可靠投递子模块
- [pkg/interfaces/pubsub.go](../../../pkg/interfaces/pubsub.go) - 公共接口
- [pkg/proto/gossipsub/](../../../pkg/proto/gossipsub/) - Protobuf 定义

---

**最后更新**: 2026-01-20
