# DISC: Topic.Peers() 精确实现方案讨论

**日期**: 2025-12-28  
**状态**: ✅ 已实施  
**关联**: DISC-1227-api-layer-design.md

---

## 问题描述

~~当前 `Topic.Peers()` 方法返回 **Realm 全体成员**，而非真正订阅该 Topic 的节点。~~

**✅ 已解决**：`Topic.Peers()` 现在调用 `messagingSvc.TopicPeers()` 返回精确的订阅者列表。

```go
// services_adapters.go:279-291 (已更新)
func (t *realmTopic) Peers() []types.NodeID {
    if t.left {
        return nil
    }
    if t.realm.messagingSvc == nil {
        return nil
    }
    return t.realm.messagingSvc.TopicPeers(t.fullName)
}
```

~~**影响范围**:~~
- ~~无法准确判断 topic 活跃订阅者数量~~
- ~~无法实现精确的消息路由决策~~
- ~~影响 topic 健康度监控~~

---

## 根本原因分析

### 架构层次

```
┌─────────────────────────────────────────────────────────────────┐
│  Layer 3: Realm Services (pkg/interfaces/realm/services.go)    │
│  ─────────────────────────────────────────────────────────────  │
│    Topic.Peers() → ❌ 无法获取精确订阅者                         │
└─────────────────────────────────────────────────────────────────┘
                              ↓ 调用
┌─────────────────────────────────────────────────────────────────┐
│  Layer 2: MessagingService (pkg/interfaces/messaging/)         │
│  ─────────────────────────────────────────────────────────────  │
│    ❌ 接口未暴露 TopicPeers() 方法                               │
└─────────────────────────────────────────────────────────────────┘
                              ↓ 内部持有
┌─────────────────────────────────────────────────────────────────┐
│  Layer 1: GossipRouter (internal/core/messaging/gossipsub/)    │
│  ─────────────────────────────────────────────────────────────  │
│    ✅ MeshPeers(topic) - mesh 中的 peers (约 D=6 个)            │
│    ✅ PeersInTopic(topic) - 所有已知订阅者                       │
└─────────────────────────────────────────────────────────────────┘
```

**核心问题**: 底层 GossipSub 已有能力，但中间层 `MessagingService` 接口未暴露。

### 底层能力详情

| 方法 | 位置 | 返回内容 |
|------|------|---------|
| `MeshManager.MeshPeers(topic)` | mesh.go:327 | mesh 网络中的 peers（D=6，用于快速消息传播）|
| `MeshManager.PeersInTopic(topic)` | mesh.go:733 | 所有订阅该 topic 的已知 peers（包括非 mesh）|

---

## 解决方案

### 方案 A：扩展 MessagingService 接口 ⭐ **推荐**

**思路**: 在 MessagingService 接口层添加查询方法，透传 GossipRouter 能力。

#### 步骤 1: 扩展接口定义

```go
// pkg/interfaces/messaging/messaging.go

type MessagingService interface {
    // ... 现有方法 ...
    
    // ==================== Topic 查询 ====================
    
    // TopicPeers 获取订阅指定 topic 的所有已知 peers
    //
    // 返回所有通过 GossipSub 协议发现的订阅者，包括：
    // - mesh peers（全连接网络成员）
    // - gossip peers（通过 IHAVE/IWANT 交换消息的 peers）
    //
    // 注意：这是本节点视角的已知订阅者，可能不包含网络中所有订阅者。
    TopicPeers(topic string) []types.NodeID
    
    // MeshPeers 获取 topic 的 mesh peers
    //
    // 返回与本节点建立 mesh 连接的 peers（通常 D=6）。
    // 这些 peers 用于第一跳消息传播，是最可靠的消息接收者。
    MeshPeers(topic string) []types.NodeID
}
```

#### 步骤 2: 实现

```go
// internal/core/messaging/service.go

// TopicPeers 获取订阅指定 topic 的所有 peers
func (s *MessagingService) TopicPeers(topic string) []types.NodeID {
    if s.gossipRouter == nil {
        return nil
    }
    return s.gossipRouter.PeersInTopic(topic)
}

// MeshPeers 获取 topic 的 mesh peers
func (s *MessagingService) MeshPeers(topic string) []types.NodeID {
    if s.gossipRouter == nil {
        return nil
    }
    return s.gossipRouter.MeshPeers(topic)
}
```

#### 步骤 3: 更新 GossipRouter 接口

```go
// internal/core/messaging/gossipsub/router.go

// 添加 PeersInTopic 方法到 Router
func (r *Router) PeersInTopic(topic string) []types.NodeID {
    r.mu.RLock()
    defer r.mu.RUnlock()
    return r.mesh.PeersInTopic(topic)
}

// MeshPeers 已存在，确保暴露
func (r *Router) MeshPeers(topic string) []types.NodeID {
    r.mu.RLock()
    defer r.mu.RUnlock()
    return r.mesh.MeshPeers(topic)
}
```

#### 步骤 4: 更新 realmTopic 实现

```go
// internal/core/realm/services_adapters.go

// Peers 返回订阅此主题的节点列表
func (t *realmTopic) Peers() []types.NodeID {
    if t.left {
        return nil
    }
    
    // 获取完整 topic 名（含 Realm 前缀）
    fullTopic := t.fullName
    
    // 从 MessagingService 获取精确的订阅者列表
    return t.messagingSvc.TopicPeers(fullTopic)
}
```

#### 优点
- ✅ 最小改动量
- ✅ 符合现有分层架构
- ✅ 向后兼容（新增方法不破坏现有代码）
- ✅ 直接解决问题

#### 缺点
- ⚠️ 需要修改接口定义（但是扩展性添加）
- ⚠️ 返回的是本节点视角的已知订阅者，不是全网精确值

---

### 方案 B：通过 Subscription 接口查询

**思路**: 在订阅句柄上添加 Peers() 方法。

```go
// pkg/interfaces/messaging/messaging.go
type Subscription interface {
    // ... 现有方法 ...
    
    // Peers 返回该 topic 的当前订阅者
    Peers() []types.NodeID
}
```

#### 优点
- ✅ 语义自然（订阅者查询订阅信息）

#### 缺点
- ⚠️ 需要 Subscription 保持对 Router 的引用
- ⚠️ 需要修改多个 Subscription 实现
- ⚠️ 增加 Subscription 的职责

---

### 方案 C：事件驱动订阅者追踪

**思路**: 在 Realm 层监听 GossipSub 事件，本地维护订阅者列表。

```go
// realmTopic 维护订阅者列表
type realmTopic struct {
    // ...
    subscribersMu sync.RWMutex
    subscribers   map[types.NodeID]struct{}
}

// 监听 GossipSub 事件
func (t *realmTopic) handleGossipEvent(event GossipEvent) {
    switch e := event.(type) {
    case *PeerSubscribedEvent:
        if e.Topic == t.fullName {
            t.addSubscriber(e.PeerID)
        }
    case *PeerUnsubscribedEvent:
        if e.Topic == t.fullName {
            t.removeSubscriber(e.PeerID)
        }
    }
}
```

#### 优点
- ✅ 实时更新
- ✅ 可以添加额外的本地状态

#### 缺点
- ⚠️ 复杂度高
- ⚠️ 需要维护状态一致性
- ⚠️ 需要 GossipSub 暴露事件系统
- ⚠️ 可能与底层状态不一致

---

### 方案 D：独立的 TopicInfo 服务

**思路**: 创建专门的 Topic 信息查询服务。

```go
// pkg/interfaces/messaging/topic_info.go
type TopicInfo interface {
    // TopicPeers 获取 topic 的订阅者
    TopicPeers(topic string) []types.NodeID
    
    // MeshPeers 获取 topic 的 mesh peers
    MeshPeers(topic string) []types.NodeID
    
    // IsSubscribed 检查是否订阅
    IsSubscribed(topic string) bool
    
    // TopicStats 获取 topic 统计
    TopicStats(topic string) *TopicStatistics
}
```

#### 优点
- ✅ 单一职责
- ✅ 可以扩展更多统计功能

#### 缺点
- ⚠️ 增加新的抽象层
- ⚠️ 需要注入额外的依赖
- ⚠️ 过度设计

---

## 方案对比

| 维度 | 方案 A | 方案 B | 方案 C | 方案 D |
|------|--------|--------|--------|--------|
| 改动量 | 小 ✅ | 中 | 大 | 中 |
| 复杂度 | 低 ✅ | 中 | 高 | 中 |
| 实时性 | 查询时 | 查询时 | 实时 ✅ | 查询时 |
| 向后兼容 | 是 ✅ | 是 | 是 | 是 |
| 架构一致性 | 高 ✅ | 中 | 中 | 低 |
| 精确度 | 本节点视角 | 本节点视角 | 本节点视角 | 本节点视角 |

---

## 推荐方案

**推荐采用方案 A：扩展 MessagingService 接口**

理由：
1. **最小改动**: 仅需添加 2 个方法到接口，实现透传底层能力
2. **架构一致**: 符合现有分层设计（Realm → MessagingService → GossipRouter）
3. **向后兼容**: 新增方法不影响现有代码
4. **足够精确**: 返回本节点通过 GossipSub 协议发现的所有订阅者

### 精确度说明

需要明确：**任何方案都无法返回"全网精确订阅者列表"**。

原因：
- GossipSub 是去中心化协议，没有全局订阅者注册表
- 每个节点只知道自己通过协议交互发现的 peers
- 这是协议本身的设计，不是实现缺陷

实际可获取的信息：
- `MeshPeers`: 直接消息传播的 peers（高置信度）
- `TopicPeers`: 通过 IHAVE/IWANT 等协议消息发现的所有订阅者

对于大多数应用场景，`TopicPeers` 已经足够准确。

---

## 实施计划

```
Phase 1: 接口扩展
├── 修改 pkg/interfaces/messaging/messaging.go
├── 修改 internal/core/messaging/service.go
└── 修改 internal/core/messaging/gossipsub/router.go

Phase 2: Realm 层适配
├── 修改 internal/core/realm/services_adapters.go
└── 删除注释警告

Phase 3: 测试验证
├── 单元测试：TopicPeers/MeshPeers
├── 集成测试：多节点订阅场景
└── 更新文档
```

预计改动文件：4 个  
预计改动行数：~50 行

---

## 决策记录

| 日期 | 决策 | 原因 |
|------|------|------|
| 2025-12-28 | 提出讨论 | Topic.Peers() 返回近似值，需要彻底解决 |
| 2025-12-28 | 采用方案 A | 最小改动量、符合架构、向后兼容 |
| 2025-12-28 | ✅ 实施完成 | 扩展 MessagingService 接口，暴露 TopicPeers/MeshPeers |

---

## 附录：GossipSub Mesh 概念

```
                 Peer A
                   │
           ┌───────┼───────┐
           │       │       │
         Peer B ──[ME]── Peer C    ← Mesh Peers (D=6)
           │       │       │
           └───────┼───────┘
                   │
                 Peer D


Mesh Peers: 与本节点建立全连接的 peers，消息直接转发
Topic Peers: 所有已知订阅该 topic 的 peers（包括 mesh + gossip）
```

GossipSub 通过维护 mesh 网络实现高效的消息传播：
- 每个节点维护 D=6 个 mesh peers
- 消息首先通过 mesh 传播
- 未在 mesh 中的 peers 通过 gossip (IHAVE/IWANT) 获取消息

