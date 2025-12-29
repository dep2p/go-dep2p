# ADR-0002: 严格单 Realm 隔离

## 元数据

| 属性 | 值 |
|------|-----|
| 状态 | ✅ Accepted |
| 决策日期 | 2024-01-15 |
| 决策者 | DeP2P 核心团队 |
| 相关 ADR | [ADR-0001](0001-identity-first.md) |

---

## 上下文

在设计 DeP2P 的业务隔离机制时，我们需要决定节点如何管理多个业务网络（Realm）的成员资格。

### 问题背景

P2P 应用通常需要业务隔离：

- 聊天应用的不同房间
- 游戏的不同服务器
- 文件共享的不同组

如何管理节点在这些"业务网络"中的成员资格？

### 决策驱动因素

- **简单性**：用户 API 应该简单直观
- **安全性**：业务之间应该隔离，防止信息泄露
- **清晰性**：节点的业务上下文应该明确
- **可预测性**：业务 API 的行为应该可预测

---

## 考虑的选项

### 选项 1: 多 Realm 同时加入

节点可以同时加入多个 Realm，业务 API 需要指定目标 Realm。

```mermaid
flowchart LR
    subgraph Node [节点]
        R1[Realm A]
        R2[Realm B]
        R3[Realm C]
    end
    
    API["Send(realm, peer, msg)"]
    API --> R1
    API --> R2
    API --> R3
```

**优点**:
- 灵活性高
- 可以同时参与多个业务

**缺点**:
- API 复杂：每个调用需要指定 Realm
- 容易混淆：消息可能发到错误的 Realm
- 状态管理复杂：需要管理多个 Realm 的状态
- 安全风险：可能意外泄露信息

### 选项 2: 严格单 Realm 模型

节点同一时间只能加入一个 Realm，切换需要显式 Leave 再 Join。

```mermaid
stateDiagram-v2
    [*] --> NotMember: 节点启动
    NotMember --> MemberA: JoinRealm(A)
    MemberA --> NotMember: LeaveRealm()
    NotMember --> MemberB: JoinRealm(B)
    MemberB --> NotMember: LeaveRealm()
    
    note right of NotMember
        业务 API 返回 ErrNotMember
    end note
    
    note right of MemberA
        所有业务 API 在 Realm A 上下文
    end note
```

**优点**:
- API 简单：无需每次指定 Realm
- 上下文明确：`CurrentRealm()` 总是清晰
- 安全性高：无法意外发送到错误 Realm
- 可预测性：业务行为容易理解

**缺点**:
- 灵活性低：不能同时参与多个业务
- 切换成本：需要 Leave 再 Join
- 不适合某些场景：如跨 Realm 代理

---

## 决策结果

选择 **选项 2: 严格单 Realm 模型**。

### 核心决策

> **每个节点同一时间只能加入一个 Realm，Join 前业务层零交互。**

### 决策理由

1. **简单性优先**
   ```go
   // 简单的 API，无需每次指定 Realm
   node.Send(ctx, peer, proto, data)
   
   // vs 复杂的 API
   node.Send(ctx, realm, peer, proto, data)
   ```

2. **安全性保证**
   - 不可能意外发送消息到错误的 Realm
   - 业务隔离是强制的，不是可选的

3. **上下文清晰**
   - `CurrentRealm()` 总是返回明确的值
   - 不存在"当前 Realm 是哪个"的困惑

4. **错误处理简单**
   - 未加入 Realm 时，业务 API 返回 `ErrNotMember`
   - 行为可预测

```mermaid
flowchart TB
    subgraph Option1 [多 Realm]
        API1["Send(realm, peer, msg)"]
        Risk1["风险: 发错 Realm"]
        Complex1["复杂: 状态管理"]
    end
    
    subgraph Option2 [单 Realm]
        API2["Send(peer, msg)"]
        Safe2["安全: 不可能发错"]
        Simple2["简单: 上下文明确"]
    end
    
    Option1 -->|"复杂且有风险"| Bad["不推荐"]
    Option2 -->|"简单且安全"| Good["推荐"]
```

---

## 后果

### 正面后果

1. **API 简洁**
   ```go
   // 业务 API 无需指定 Realm
   node.Send(ctx, peer, "/app/chat", []byte("hello"))
   node.Request(ctx, peer, "/app/query", data)
   node.Publish(ctx, "/app/events", data)
   ```

2. **安全边界清晰**
   - Realm 成员资格在连接级别验证
   - 非成员的业务消息被拒绝

3. **状态管理简单**
   - 只需管理一个 Realm 的状态
   - `CurrentRealm()` 总是明确

4. **错误处理一致**
   - `ErrNotMember`: 未加入 Realm 时调用业务 API
   - `ErrAlreadyJoined`: 已加入时再次 Join

### 负面后果

1. **不支持同时多 Realm**
   - 某些场景可能需要跨 Realm 操作
   - 需要显式切换

2. **切换有开销**
   - Leave + Join 需要网络操作
   - 可能导致短暂的服务中断

3. **代理场景复杂**
   - 跨 Realm 代理需要多个节点
   - 或者需要特殊的代理模式

### 缓解措施

| 负面后果 | 缓解措施 |
|----------|----------|
| 不支持多 Realm | 使用多个节点实例，或快速切换 |
| 切换开销 | 优化 Join/Leave 流程，支持快速切换 |
| 代理场景 | 未来可能添加专门的代理模式（新 ADR） |

---

## 状态图

```mermaid
stateDiagram-v2
    [*] --> NotMember: 节点启动
    
    NotMember --> Member: JoinRealm(name)
    Member --> NotMember: LeaveRealm()
    Member --> Member: 切换 Realm
    
    state NotMember {
        [*] --> Isolated
        Isolated: 业务 API 返回 ErrNotMember
        Isolated: 系统层正常工作
    }
    
    state Member {
        [*] --> Active
        Active: 业务 API 正常工作
        Active: CurrentRealm() != nil
    }
    
    note right of NotMember
        系统层（DHT/Relay/NAT）正常工作
        只是业务层被隔离
    end note
```

---

## 不变量

此决策产生以下系统不变量：

> **INV-002**: 业务 API（Send/Request/Publish/Subscribe）在 `CurrentRealm() == nil` 时 MUST 返回 `ErrNotMember`。

违反此不变量将导致：
- 业务消息泄露到错误的上下文
- 安全边界被破坏
- 系统行为不可预测

---

## 代码示例（IMPL-1227 更新）

### 正确使用

```go
// 创建节点
node, _ := dep2p.StartNode(ctx, dep2p.WithPreset(dep2p.PresetDesktop))
defer node.Close()

// 此时 CurrentRealm() == nil
// 业务 API 将返回 ErrNotMember

// 使用 realmKey 加入 Realm（PSK 认证）
realmKey := types.GenerateRealmKey() // 或从配置读取
realm, err := node.JoinRealmWithKey(ctx, "my-app-realm", realmKey)
if err != nil {
    log.Fatal(err)
}

// 现在 CurrentRealm() != nil
// realm.ID() 返回从 realmKey 派生的 RealmID
// realm.Name() 返回 "my-app-realm"

// 通过 Realm 对象获取 Layer 3 服务
messaging := realm.Messaging()
pubsub := realm.PubSub()

// 发送消息（协议前缀自动添加）
err = messaging.SendWithProtocol(ctx, peerID, "chat/1.0.0", []byte("hello"))
if err != nil {
    log.Error(err)
}

// 切换 Realm
realm.Leave()  // 先离开
newRealm, _ := node.JoinRealmWithKey(ctx, "another-realm", anotherRealmKey)
```

### 错误处理

```go
// 未加入 Realm 时调用业务 API
err := node.Send(ctx, peerID, []byte("hello"))
if errors.Is(err, dep2p.ErrNotMember) {
    // 需要先 JoinRealm
    log.Warn("请先加入 Realm")
}

// 已加入时再次 Join
_, err := node.JoinRealmWithKey(ctx, "another-realm", anotherKey)
if errors.Is(err, dep2p.ErrAlreadyJoined) {
    // 需要先 LeaveRealm
    log.Warn("请先离开当前 Realm")
}

// PSK 验证失败
if errors.Is(err, realm.ErrPSKAuthFailed) {
    log.Error("realmKey 不匹配，无法加入 Realm")
}
```

---

## 与三层架构的关系

```mermaid
flowchart TB
    subgraph Layer3 [Layer 3: 应用协议层]
        Send["Send/Request"]
        PubSub["Publish/Subscribe"]
    end
    
    subgraph Layer2 [Layer 2: Realm 层]
        RealmMgr["RealmManager"]
        Check{"CurrentRealm?"}
    end
    
    subgraph Layer1 [Layer 1: 系统层]
        DHT["DHT"]
        Relay["Relay"]
        NAT["NAT"]
    end
    
    Send --> Check
    PubSub --> Check
    
    Check -->|"!= nil"| OK["正常处理"]
    Check -->|"== nil"| Error["ErrNotMember"]
    
    RealmMgr --> DHT
    RealmMgr --> Relay
    
    Note1["Layer 1 始终工作"]
    Note2["Layer 2 控制业务隔离"]
    Note3["Layer 3 依赖 Layer 2 状态"]
```

---

## 相关文档

- [核心概念：Realm 隔离](../../docs/zh/concepts/core-concepts.md)
- [Realm 协议规范](../protocols/application/realm.md)
- [Realm API 参考](../../docs/zh/reference/api/realm.md)
- [需求 REQ-REALM-001](../requirements/REQ-REALM-001.md)
- [系统不变量 INV-002](../invariants/INV-002-realm-membership.md)

---

## 备注

### 与其他 P2P 系统的对比

| 系统 | 业务隔离方式 |
|------|--------------|
| libp2p | 无内置 Realm，需自行实现 |
| IPFS | 通过 pubsub topic 隔离 |
| DeP2P | 严格单 Realm 模型 |

DeP2P 的严格单 Realm 模型提供了更强的隔离保证和更简单的 API。

### 未来演进

如果需要支持同时加入多个 Realm，应该：

1. 创建新的 ADR 讨论此需求
2. 考虑使用 "Multi-Realm Node" 模式
3. 保持向后兼容性
4. 不影响现有的单 Realm 行为
