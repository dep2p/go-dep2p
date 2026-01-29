# Core EventBus 模块

> **版本**: v1.1.0  
> **更新日期**: 2026-01-13  
> **定位**: 事件总线（Core Layer）

---

## 模块概述

core_eventbus 提供进程内的事件发布订阅机制，用于组件间的松耦合通信。它是 DeP2P 内部事件驱动架构的基础。

| 属性 | 值 |
|------|-----|
| **架构层** | Core Layer |
| **代码位置** | `internal/core/eventbus/` |
| **Fx 模块** | `fx.Module("eventbus")` |
| **状态** | ✅ 已实现 |
| **依赖** | 无（底层组件） |

---

## 核心职责

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                       core_eventbus 职责                                     │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. 事件发布 (Emit)                                                          │
│     • 类型安全的事件发布                                                    │
│     • 同步/异步发布模式                                                     │
│                                                                             │
│  2. 事件订阅 (Subscribe)                                                     │
│     • 按事件类型订阅                                                        │
│     • 订阅取消                                                              │
│     • 缓冲区管理                                                            │
│                                                                             │
│  3. 事件类型                                                                 │
│     • 连接事件 (EvtPeerConnected, EvtPeerDisconnected)                     │
│     • 发现事件 (EvtPeerDiscovered)                                         │
│     • 协议事件 (EvtProtocolUpdated)                                        │
│     • 地址事件 (EvtLocalAddrsUpdated)                                      │
│     • NAT 事件 (EvtNATTypeChanged, EvtReachabilityChanged)                 │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 事件类型定义

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          DeP2P 事件类型                                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  连接事件：                                                                  │
│  ├─ EvtPeerConnectednessChanged  │ 节点连接状态变化                         │
│  ├─ EvtPeerIdentificationCompleted │ 身份识别完成                           │
│  └─ EvtPeerIdentificationFailed  │ 身份识别失败                             │
│                                                                             │
│  发现事件：                                                                  │
│  ├─ EvtPeerDiscovered           │ 发现新节点                               │
│  └─ EvtPeerProtocolsUpdated     │ 节点协议更新                             │
│                                                                             │
│  网络事件：                                                                  │
│  ├─ EvtLocalAddressesUpdated    │ 本地地址变化                             │
│  ├─ EvtNATDeviceTypeChanged     │ NAT 类型变化                             │
│  └─ EvtLocalReachabilityChanged │ 可达性变化                               │
│                                                                             │
│  DHT 事件：                                                                  │
│  ├─ EvtDHTQueryProgressed       │ DHT 查询进度                             │
│  └─ EvtDHTRoutingTableUpdated   │ 路由表更新                               │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 公共接口

```go
// pkg/interfaces/eventbus.go

// EventBus 事件总线接口
type EventBus interface {
    // Subscribe 订阅事件
    // eventType 是事件类型的指针，用于类型推断
    Subscribe(eventType interface{}, opts ...SubscriptionOpt) (Subscription, error)
    
    // Emitter 获取事件发射器
    Emitter(eventType interface{}, opts ...EmitterOpt) (Emitter, error)
    
    // GetAllEventTypes 获取所有已注册的事件类型
    GetAllEventTypes() []reflect.Type
}

// Subscription 订阅接口
type Subscription interface {
    // Out 返回事件通道
    Out() <-chan interface{}
    
    // Close 关闭订阅
    Close() error
}

// Emitter 发射器接口
type Emitter interface {
    // Emit 发送事件
    Emit(event interface{}) error
    
    // Close 关闭发射器
    Close() error
}
```

---

## 使用示例

```go
// 订阅连接事件
sub, _ := eventbus.Subscribe(new(EvtPeerConnectednessChanged))
go func() {
    for evt := range sub.Out() {
        e := evt.(EvtPeerConnectednessChanged)
        log.Printf("Peer %s: %s", e.Peer, e.Connectedness)
    }
}()

// 发布事件
emitter, _ := eventbus.Emitter(new(EvtPeerConnectednessChanged))
emitter.Emit(EvtPeerConnectednessChanged{
    Peer:          peerID,
    Connectedness: Connected,
})
```

---

## 参考实现

### go-libp2p EventBus

```
github.com/libp2p/go-libp2p/core/event/
├── bus.go            # 事件总线接口
├── addrs.go          # 地址相关事件
├── identify.go       # Identify 相关事件
├── nattype.go        # NAT 类型事件
├── network.go        # 网络事件
├── protocol.go       # 协议事件
└── reachability.go   # 可达性事件

github.com/libp2p/go-eventbus/
└── basic.go          # 默认实现
```

---

## 相关文档

| 文档 | 说明 |
|------|------|
| [core_swarm](../core_swarm/) | 连接群管理 |
| [core_host](../core_host/) | 主机服务 |
| [L2: 五层架构](../../L2_structural/layer_model.md) | 架构定义 |

---

**最后更新**：2026-01-13
