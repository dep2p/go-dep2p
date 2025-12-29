# GossipSub v1.1 协议实现

## 概述

本模块实现了 [GossipSub v1.1](https://github.com/libp2p/specs/blob/master/pubsub/gossipsub/gossipsub-v1.1.md) 发布订阅协议，提供高效的消息传播能力。

## 架构

```
┌─────────────────────────────────────────────────────────────────┐
│                      GossipRouter (router.go)                    │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────────────┐   │
│  │   Publish    │  │  Subscribe   │  │    HandleRPC         │   │
│  └──────────────┘  └──────────────┘  └──────────────────────┘   │
├─────────────────────────────────────────────────────────────────┤
│                                                                  │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐              │
│  │ MeshManager │  │ PeerScorer  │  │  Heartbeat  │              │
│  │  (mesh.go)  │  │(scoring.go) │  │(heartbeat.go)│              │
│  └──────┬──────┘  └──────┬──────┘  └──────┬──────┘              │
│         │                │                │                      │
│         ▼                ▼                ▼                      │
│  ┌─────────────────────────────────────────────────────────┐    │
│  │                    消息处理流程                          │    │
│  │  ┌────────┐  ┌────────┐  ┌────────┐  ┌────────┐        │    │
│  │  │ GRAFT  │  │ PRUNE  │  │ IHAVE  │  │ IWANT  │        │    │
│  │  └────────┘  └────────┘  └────────┘  └────────┘        │    │
│  └─────────────────────────────────────────────────────────┘    │
│                                                                  │
├─────────────────────────────────────────────────────────────────┤
│  ┌────────────────┐  ┌────────────────┐  ┌─────────────────┐    │
│  │ MessageCache   │  │  SeenCache     │  │ BackoffTracker  │    │
│  │  (cache.go)    │  │  (cache.go)    │  │   (cache.go)    │    │
│  └────────────────┘  └────────────────┘  └─────────────────┘    │
├─────────────────────────────────────────────────────────────────┤
│  ┌──────────────────────────────────────────────────────────┐   │
│  │                   RPCCodec (protocol.go)                  │   │
│  │            Protobuf 编解码 (pkg/proto/gossipsub/)         │   │
│  └──────────────────────────────────────────────────────────┘   │
└─────────────────────────────────────────────────────────────────┘
```

## 文件结构

```
gossipsub/
├── README.md          # 本文档
├── types.go           # 类型别名（使用 pkg/types/gossipsub.go）
├── config.go          # 配置（适配 messaging.Config）
├── protocol.go        # Protobuf 协议编解码
├── router.go          # GossipRouter 核心实现
├── mesh.go            # Mesh 网络管理
├── scoring.go         # Peer 评分系统
├── heartbeat.go       # 心跳维护机制
├── cache.go           # 消息缓存和去重
└── gossipsub_test.go  # 单元测试
```

## 核心概念

### Mesh 网络

每个主题维护一个 Mesh 网络，包含 D 个全双工连接的 peers：

```
           ┌─────┐
           │Peer1│
           └──┬──┘
              │
    ┌─────────┼─────────┐
    │         │         │
┌───▼───┐ ┌──▼──┐ ┌────▼────┐
│ Peer2 │◄┤Local├►│  Peer3  │
└───────┘ └──┬──┘ └─────────┘
             │
        ┌────▼────┐
        │  Peer4  │
        └─────────┘

D=4 (目标 mesh 大小)
```

### 消息传播

1. **发布者** → 发送消息给 mesh 中所有 peers
2. **Mesh peers** → 继续转发给各自的 mesh peers  
3. **Gossip** → 非 mesh peers 通过 IHAVE/IWANT 获取消息

```
时间线:
─────────────────────────────────────────────────────►

T0: 发布者发送消息
    ┌─────┐
    │ Pub │ ──► Mesh Peers (D个)
    └─────┘

T1: Mesh 转发
    Mesh Peers ──► 各自的 Mesh Peers

T2: Gossip 传播
    ┌─────────────────────────────────────┐
    │ IHAVE: "我有消息 X"                 │
    │ IWANT: "请发送消息 X"               │
    │ 消息传递: 发送完整消息              │
    └─────────────────────────────────────┘

传播复杂度: O(log N)
```

### Heartbeat 机制

每秒执行一次心跳，维护网络健康：

```
┌──────────────────────────────────────────────────┐
│                 Heartbeat 周期                    │
├──────────────────────────────────────────────────┤
│                                                   │
│  1. Mesh 维护                                     │
│     ├── mesh.size < D_low  → 发送 GRAFT          │
│     └── mesh.size > D_high → 发送 PRUNE          │
│                                                   │
│  2. Fanout 清理                                   │
│     └── 清理超时的 fanout 条目                    │
│                                                   │
│  3. Gossip 传播                                   │
│     └── 发送 IHAVE 给非 mesh peers               │
│                                                   │
│  4. 评分衰减                                      │
│     └── 定期衰减 peer 评分                        │
│                                                   │
└──────────────────────────────────────────────────┘
```

### Peer 评分

评分系统防止恶意节点攻击：

```
Score = P1 + P2 + P3 + P3b + P4 + P5 + P6 + P7

┌────────┬────────────────────────┬────────┐
│ 参数   │ 说明                   │ 方向   │
├────────┼────────────────────────┼────────┤
│ P1     │ Mesh 时间奖励          │   +    │
│ P2     │ 首次消息投递奖励       │   +    │
│ P3     │ Mesh 消息投递          │   ±    │
│ P3b    │ Mesh 失败惩罚          │   -    │
│ P4     │ 无效消息惩罚           │   -    │
│ P5     │ 应用层评分             │   ±    │
│ P6     │ IP 协同惩罚            │   -    │
│ P7     │ 行为惩罚               │   -    │
└────────┴────────────────────────┴────────┘

阈值:
├── GossipThreshold  = -500   (不发送 gossip)
├── PublishThreshold = -1000  (不发送消息)
└── GraylistThreshold = -2500 (忽略该 peer)
```

## 配置参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `D` | 6 | 目标 mesh 大小 |
| `Dlo` | 4 | mesh 下限（低于触发 GRAFT） |
| `Dhi` | 12 | mesh 上限（超过触发 PRUNE） |
| `Dlazy` | 6 | Gossip 目标数量 |
| `HeartbeatInterval` | 1s | 心跳间隔 |
| `FanoutTTL` | 60s | Fanout 过期时间 |
| `SeenTTL` | 120s | 已见消息缓存时间 |
| `HistoryLength` | 5 | 历史消息窗口 |
| `HistoryGossip` | 3 | Gossip 历史窗口 |
| `PruneBackoff` | 60s | PRUNE 后退避时间 |
| `IWantFollowupTime` | 3s | IWANT 超时时间 |

## 使用示例

```go
// 创建配置
config := gossipsub.DefaultConfig()

// 创建路由器
router := gossipsub.NewRouter(
    config,
    localID,
    identity, // 用于消息签名
    endpoint,
    logger,
)

// 启动
router.Start(ctx)

// 订阅主题
msgCh, cancel, err := router.Subscribe("my-topic")
if err != nil {
    log.Fatal(err)
}
defer cancel()

// 接收消息
go func() {
    for msg := range msgCh {
        fmt.Printf("收到消息: %s\n", msg.Data)
    }
}()

// 发布消息
err = router.Publish(ctx, "my-topic", []byte("Hello, World!"))

// 停止
router.Stop()
```

## 协议消息

使用 Protobuf 定义（`pkg/proto/gossipsub/gossipsub.proto`）：

```protobuf
message RPC {
    repeated SubOpts subscriptions = 1;
    repeated Message publish = 2;
    ControlMessage control = 3;
}

message ControlMessage {
    repeated ControlIHave ihave = 1;  // 通知有消息
    repeated ControlIWant iwant = 2;  // 请求消息
    repeated ControlGraft graft = 3;  // 加入 mesh
    repeated ControlPrune prune = 4;  // 离开 mesh
}
```

## 类型集成

本模块使用 dep2p 统一类型系统：

| gossipsub 类型 | 统一类型 |
|----------------|----------|
| `Message` | `types.Message` |
| `RPC` | `types.GossipRPC` |
| `ControlMessage` | `types.GossipControl` |
| `PeerState` | `types.GossipPeerState` |
| `TopicState` | `types.GossipTopicState` |
| `Config` | 适配 `messaging.GossipSubConfig` |

## 相关文档

- [GossipSub v1.1 规范](https://github.com/libp2p/specs/blob/master/pubsub/gossipsub/gossipsub-v1.1.md)
- [GossipSub 重构报告](../../../../docs/05-iterations/gossipsub-refactor-report.md)
- [GossipSub 实施文档](../../../../docs/05-iterations/components/gossipsub-impl.md)
- [消息服务接口](../../../../pkg/interfaces/messaging/messaging.go)

