# DeP2P Realm 入门示例

## 概述

这是 DeP2P 的 **Realm 入门示例**，用于理解 DeP2P v1.1 的核心特性：**Realm 强制隔离**。

通过这个示例，你将学到：
- 什么是 Realm（业务隔离租户）
- 为什么需要 JoinRealm
- Node Facade vs Endpoint 的区别
- 强制隔离的设计哲学

---

## 快速开始

```bash
cd examples/realm
go run main.go
```

你将看到完整的 Realm 生命周期：
1. 节点创建
2. 未 JoinRealm 时业务 API 被拒绝（`ErrNotMember`）
3. JoinRealm 后业务 API 可用

---

## 什么是 Realm？

**Realm** 是 DeP2P 的**业务隔离租户**，类似于：
- Kubernetes 的 Namespace
- 云厂商的 VPC（Virtual Private Cloud）
- 数据库的 Schema

### 核心原则

1. **每个节点同一时间只能加入一个 Realm**  
   严格单 Realm 模型，避免混乱和安全问题。

2. **业务 API 必须先 JoinRealm**  
   以下 API 必须先加入 Realm 才能使用：
   - `Node.Send` / `Node.Request`
   - `Node.Publish` / `Node.Subscribe`
   - `Node.Query`（DHT 查询）

3. **系统 API 不受限制**  
   以下 API 不需要 JoinRealm：
   - `Node.Connect` / `Node.ConnectToAddr`
   - `Node.ListenAddrs` / `Node.AdvertisedAddrs`
   - `Discovery` / `NAT` / `Relay`

> （高级/运维/受控）`ConnectWithAddrs` 不在示例中作为常规用法展示；如确有需要，请参考 `docs/04-usage/examples/advanced.md`。

### 为什么需要 Realm？

假设你在开发一个多租户的去中心化应用（如多链钱包、多组织协作工具），你需要：

- **隔离不同应用/租户的数据**  
  应用 A 的消息不应该被应用 B 收到。

- **防止跨租户攻击**  
  恶意节点不应该能够伪装成其他租户成员。

- **简化用户编程模型**  
  用户不需要在每次调用时传递 `tenantID`，框架层面保证隔离。

---

## Node Facade vs Endpoint

DeP2P 提供两种 API 入口：

### Endpoint（最小稳定接口）

```go
endpoint, err := dep2p.New(opts...)  // 创建 Endpoint

// Endpoint 暴露所有底层 API，但不强制 Realm 隔离
conn, _ := endpoint.Connect(ctx, peerID)
stream, _ := conn.OpenStream(ctx, protocol)
```

**用途**：
- 构建自定义上层框架
- 需要完全控制底层行为
- 不需要 Realm 隔离的场景

### Node Facade（推荐用户入口）

```go
node, err := dep2p.StartNode(ctx, opts...)  // 创建并启动 Node Facade

// Node 提供友好的高层 API，并强制 Realm 隔离
node.Realm().JoinRealm(ctx, realmID)
node.Send(ctx, peerID, protocol, data)  // 自动检查 Realm 成员
```

**用途**：
- 快速开发 P2P 应用
- 需要 Realm 隔离的多租户场景
- 推荐给所有用户使用

---

## 示例代码解析

### Step 1: 创建 Node

```go
node, err := dep2p.StartNode(ctx,
    dep2p.WithPreset(dep2p.PresetDesktop),
    // v1.1+ 强制内建：Realm 为底层必备能力，用户无需配置启用开关
    // 使用默认随机端口（QUIC 传输）
)
```

**关键点**：
- 使用 `dep2p.StartNode` 而非 `dep2p.Start`（Node Facade）
- Realm 是内建能力，无需显式启用

### Step 2: 验证强制隔离

```go
err := node.Send(ctx, targetID, protocol, []byte("hello"))
// 返回: endpoint.ErrNotMember
```

**关键点**：
- 未 JoinRealm 时，业务 API 直接返回 `ErrNotMember`
- 这是在 Node Facade 层面拦截的，不会有网络请求

### Step 3: 加入 Realm

```go
realmID := types.RealmID("my-app-realm")
err := node.Realm().JoinRealm(ctx, realmID)
```

**关键点**：
- `node.Realm()` 获取 RealmManager
- `JoinRealm` 后，节点成为该 Realm 的成员

### Step 4: 业务 API 可用

```go
err := node.Publish(ctx, "test-topic", []byte("message"))
// 成功（或返回其他非 ErrNotMember 的错误）
```

**关键点**：
- Join 后，业务 API 不再返回 `ErrNotMember`
- 其他错误（如网络问题）仍可能发生

---

## Realm 强制隔离的检查点

根据 `docs/05-iterations/2025-12-22-user-simplicity-gap-analysis.md` 13.8 节，Realm 隔离的验收点包括：

| 检查点 | 说明 |
|--------|------|
| 1. 未 Join 时业务 API 返回 `ErrNotMember` | `Send/Request/Publish/Subscribe/Query` |
| 2. Join 后业务 API 正常工作 | 成功或返回其他非 `ErrNotMember` 的错误 |
| 3. 重复 Join 返回 `ErrAlreadyJoined` | 严格单 Realm 模型 |
| 4. RealmAuth 握手验证 | 连接后 `conn.RealmContext().IsValid()` |
| 5. PubSub 消息隔离 | `realm/{realmID}/topic` 只在成员间传播 |

---

## 常见问题

### Q: 为什么不自动加入一个默认 Realm？

**A**: 这会带来安全问题。想象以下场景：
- 用户以为自己在一个私有 Realm 中
- 实际上所有人都在"默认 Realm"中
- 敏感数据被所有人看到

强制 JoinRealm 确保用户**明确意识到**自己在哪个 Realm 中。

### Q: 我的应用不需要多租户，是否还需要 JoinRealm？

**A**: 是的。即使是单租户应用，JoinRealm 也提供：
- **明确的边界**：你的应用和其他应用隔离
- **安全性**：其他节点无法伪装成你的应用成员
- **一致性**：所有 DeP2P 应用都遵循相同的模型

你可以使用一个固定的 `realmID`，如：
```go
realmID := types.RealmID("my-app-v1")
node.Realm().JoinRealm(ctx, realmID)
```

### Q: Endpoint 和 Node Facade 可以混用吗？

**A**: 技术上可以，但**不推荐**。混用会导致：
- 部分调用绕过 Realm 检查
- 代码可读性下降
- 容易引入安全漏洞

**推荐做法**：
- 应用层只使用 `Node Facade`
- 仅在构建框架/库时使用 `Endpoint`

---

## 下一步

1. **Echo 示例** (`examples/echo/`)  
   学习协议注册、流通信，以及如何在 Realm 中使用自定义协议。

2. **Chat 示例** (`examples/chat/`)  
   学习 mDNS 自动发现，以及如何用 Realm 实现"聊天室隔离"。

3. **Relay 示例** (`examples/relay/`)  
   学习 NAT 穿透和中继服务。

4. **测试代码** (`tests/e2e/realm_test.go`)  
   查看完整的 Realm 隔离验收测试。

---

## 参考

- [设计文档](../../docs/05-iterations/2025-12-22-user-simplicity-gap-analysis.md) - Realm 隔离设计
- [接口定义](../../pkg/interfaces/realm/realm.go) - RealmManager API
- [实现代码](../../internal/core/realm/) - Realm 核心实现

