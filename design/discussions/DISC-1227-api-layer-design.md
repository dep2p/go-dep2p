# DISC-1227-API 分层与对象产出设计

**日期**：2024-12-27  
**参与者**：@qinglong  
**状态**：✅ 已结论

---

## 背景

在审查 DeP2P 的 quickstart 文档时，发现当前 API 设计存在层次产出物不清晰的问题：

```go
// 当前设计
node, err := dep2p.StartNode(ctx, ...)           // ✅ Layer 1 产出 node
err := node.Realm().JoinRealm(ctx, "my-realm")   // ❌ 只返回 err，没有 realm 对象
node.Send(ctx, peerID, proto, data)              // ❌ Layer 3 操作直接挂在 node 上
node.Publish(ctx, "topic", data)                 // ❌ PubSub 也直接挂在 node 上
```

**问题总结**：
1. `JoinRealm` 返回 `err`，用户拿不到"当前所在 Realm"的操作句柄
2. `Send`、`Publish` 等 Layer 3 操作直接挂在 `node` 上，层次混乱
3. `node` 变成了"上帝对象"，承担了所有职责

---

## 问题分析

### 问题 1：JoinRealm 返回值

- `JoinRealm` 返回 `err`，用户拿不到 "当前所在 Realm" 的操作句柄
- 与三层架构设计矛盾：Layer 2 应该产出 Realm 对象
- 用户无法直接操作 Realm，需要通过 `node.Realm()` 间接访问

### 问题 2：Layer 3 操作直接挂在 node 上

- `Send`、`Publish` 等 Layer 3 操作直接挂在 `node` 上
- `node` 变成了"上帝对象"，承担了所有职责
- 层次混乱，不符合用户心智
- 无法区分"系统操作"和"业务操作"

### 问题 3：与 libp2p 设计对比

libp2p 的设计层次清晰：

```go
// libp2p 的 PubSub 设计（Layer 3 示例）
host, _ := libp2p.New()                          // Layer 1: 产出 host

ps, _ := pubsub.NewGossipSub(ctx, host)          // 创建 PubSub 服务
topic, _ := ps.Join("my-topic")                  // 加入 topic，产出 topic 对象
sub, _ := topic.Subscribe()                      // 订阅，产出 subscription 对象

// 使用
topic.Publish(ctx, data)                         // 通过 topic 发布
msg := <-sub.Messages()                          // 通过 sub 接收
```

**libp2p 的层次感**：
- `host` → 基础连接能力
- `pubsub` → PubSub 服务实例
- `topic` → 具体话题对象
- `subscription` → 订阅句柄

每一层都有明确的对象产出，职责清晰。

---

## 讨论内容

### 核心原则

按照三层架构，**每一层都应该产出一个有意义的对象**：

```
┌─────────────────────────────────────────────────────────────────────────┐
│                    DeP2P 三层架构（修正版）                              │
├─────────────────────────────────────────────────────────────────────────┤
│                                                                          │
│  Layer 3: 应用协议层                                                     │
│           产出：Protocol / Topic / Stream 对象                           │
│           操作：Send / Request / Publish / Subscribe                     │
│                                                                          │
│  Layer 2: Realm 层                                                       │
│           产出：Realm 对象                                               │
│           操作：获取 Messaging / PubSub / Discovery 等服务               │
│                                                                          │
│  Layer 1: 系统基础层                                                     │
│           产出：Node 对象                                                │
│           操作：JoinRealm / LeaveRealm / 底层连接管理                    │
│                                                                          │
└─────────────────────────────────────────────────────────────────────────┘
```

### 对象产出表

| 层级 | 操作 | 应该产出 | 说明 |
|------|------|---------|------|
| Layer 1 | `StartNode()` | `Node` | 代表本节点 |
| Layer 2 | `node.JoinRealm()` | `Realm` | 代表所在业务网络 |
| Layer 3 | `realm.JoinTopic()` | `Topic` | 发布订阅主题 |
| Layer 3 | `topic.Subscribe()` | `Subscription` | 订阅句柄 |
| Layer 3 | `realm.OpenStream()` | `Stream` | 双向流 |

**核心原则**：
1. **每一层都产出一个对象**，作为下一层操作的入口
2. **对象代表"我在这个层次的身份/上下文"**
3. **子操作挂在对应层次的对象上**，而不是全部堆到 `node`

---

### 成员认证：PSK 证明（唯一方案）

本设计面向**去中心化网络**，不依赖任何中心化签发机构。经过权衡，**固定使用 PSK（Pre-Shared Key）方案**作为 Realm 成员认证的唯一实现，理由如下：

1. **完全去中心化**：无需可信第三方，持有密钥即为成员
2. **实现简单可靠**：密码学原语成熟，验证逻辑清晰
3. **与 Relay 隔离原则一致**：Relay 必须是成员才能验证，闭环

#### 入会与验证流程

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                     PSK 成员认证模型                                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│   入会材料                                                                   │
│   ─────────                                                                  │
│   • realmKey: 高熵密钥（≥ 32 bytes 随机数）                                  │
│   • 通过带外渠道分发（邀请链接/二维码/私信等）                               │
│                                                                              │
│   成员证明（每次连接/中继请求时）                                            │
│   ────────────────────────────────                                           │
│   proof = MAC(                                                               │
│       key  = HKDF(realmKey, "dep2p-realm-membership-v1"),                    │
│       data = nodeID || realmID || peerID || nonce || timestamp               │
│   )                                                                          │
│                                                                              │
│   字段说明                                                                   │
│   ─────────                                                                  │
│   • nodeID:   证明发起者（自己的 NodeID）                                    │
│   • realmID:  所属 Realm 的 ID                                               │
│   • peerID:   目标节点（通信对方的 NodeID）—— 绑定证明到特定目标              │
│   • nonce:    随机数（防重放）                                               │
│   • timestamp: 时间戳（限制有效期）                                          │
│                                                                              │
│   peerID 设计决策                                                            │
│   ────────────────                                                           │
│   • peerID 是 **目标节点**（通信对方），不是验证者                            │
│   • 理由1：绑定目标 —— 证明含义为"我要与 peerID 通信"                        │
│   • 理由2：防中间人 —— R 无法将 A→B 的证明用于 A→C                           │
│   • 理由3：双重验证 —— B 收到时验证 peerID == 自己                           │
│   • 中继场景：A 向 R 请求中继到 B，证明中 peerID = B                         │
│                                                                              │
│   验证者                                                                     │
│   ──────                                                                     │
│   • 所有 Realm 成员（包括 Realm Relay）持有同一 realmKey                     │
│   • 独立验证，无需第三方                                                     │
│   • 验证时检查：MAC 正确 + peerID 匹配（中继或目标）+ 时间窗口有效           │
│                                                                              │
│   撤销/轮换                                                                  │
│   ─────────                                                                  │
│   • 通过 realmKey 轮换实现                                                   │
│   • 被撤销成员无法获取新 key，自动失去成员资格                               │
│   • 这是去中心化下最简单可靠的撤销机制                                       │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### API 设计

```go
// 加入 Realm 必须提供 realmKey
realm, err := node.JoinRealm(ctx, "my-business", dep2p.WithRealmKey(realmKey))

// realmKey 的生成（创建新 Realm 时）
realmKey := dep2p.GenerateRealmKey() // 返回 32 bytes 随机数
```

---

### RealmID 的去中心化设计（绑定 realmKey，不可枚举）

仅用"名称哈希"派生 `RealmID` 会导致：
- **可枚举/隐私泄露**：name 往往带业务语义，hash 可被字典枚举
- **冲突/混淆**：同名但不同 key 会被混为同一 Realm
- **升级困难**：后续加入新字段会破坏兼容

**设计决策**：将 `RealmID` 与 `Name` 解耦，绑定 `realmKey`。

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                     RealmID 派生规则                                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│   realmID = SHA256("dep2p-realm-id-v1" || H(realmKey))                       │
│                                                                              │
│   其中：                                                                     │
│   • H(realmKey) 是 realmKey 的哈希，确保即使 realmID 泄露也无法反推 key      │
│   • 前缀 "dep2p-realm-id-v1" 用于版本隔离和域分离                            │
│                                                                              │
│   特性：                                                                     │
│   ✅ 同名不同 key → 不同 realmID（不会冲突）                                  │
│   ✅ 不可枚举（无法从 name 推测 realmID）                                     │
│   ✅ 可升级（通过修改前缀版本号）                                             │
│                                                                              │
│   Name vs ID：                                                               │
│   • Name(): 显示名称/本地别名，仅供 UI 展示，不参与安全边界                   │
│   • ID():   由 realmKey 派生的不可枚举标识，用于协议/DHT/隔离                │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

### 协议命名空间隔离（核心安全边界）

协议命名空间面临与成员认证类似的问题：
- 用户自定义协议如果与系统协议同名会怎样？
- 不同 Realm 的业务协议是否会互相干扰？

**设计决策**：从协议路径层面植入隔离，从根本上杜绝风险。

#### 协议分层与命名规则

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                     协议命名空间分层                                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│   Layer 1: 系统协议（全局唯一，硬编码保护）                                   │
│   ─────────────────────────────────────────                                  │
│   前缀：/dep2p/sys/                                                          │
│   示例：                                                                     │
│     • /dep2p/sys/ping/1.0.0                                                  │
│     • /dep2p/sys/identify/1.0.0                                              │
│     • /dep2p/sys/dht/1.0.0                                                   │
│     • /dep2p/sys/relay/1.0.0                                                 │
│     • /dep2p/sys/holepunch/1.0.0                                             │
│                                                                              │
│   特性：                                                                     │
│     • 用户代码无法注册 /dep2p/sys/* 协议（运行时拒绝）                       │
│     • 不绑定任何 Realm，全网通用                                             │
│                                                                              │
│   ─────────────────────────────────────────────────────────────────────────  │
│                                                                              │
│   Layer 2: Realm 协议（植入 RealmID）                                         │
│   ────────────────────────────────────                                       │
│   前缀：/dep2p/realm/<realmID>/                                              │
│   示例：                                                                     │
│     • /dep2p/realm/abc123.../membership/1.0.0   (成员同步)                   │
│     • /dep2p/realm/abc123.../discovery/1.0.0    (Realm DHT)                  │
│     • /dep2p/realm/abc123.../relay/1.0.0        (Realm Relay)                │
│                                                                              │
│   特性：                                                                     │
│     • 不同 Realm 的协议天然隔离（realmID 不同）                              │
│     • 由框架自动生成，用户不直接操作                                         │
│                                                                              │
│   ─────────────────────────────────────────────────────────────────────────  │
│                                                                              │
│   Layer 3: 应用协议（植入 RealmID）                                           │
│   ──────────────────────────────────                                         │
│   前缀：/dep2p/app/<realmID>/                                                │
│   示例：                                                                     │
│     • /dep2p/app/abc123.../chat/1.0.0                                        │
│     • /dep2p/app/abc123.../file-transfer/1.0.0                               │
│     • /dep2p/app/abc123.../blocks/1.0.0                                      │
│                                                                              │
│   特性：                                                                     │
│     • 用户只需指定 "chat/1.0.0"，框架自动补全 /dep2p/app/<realmID>/          │
│     • 不同 Realm 即使用户指定相同名称，实际协议也完全不同                    │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### 用户视角的协议注册

```go
// 用户只需指定相对协议名
streams := realm.Streams()

// 注册时：用户写 "file-transfer/1.0.0"
// 框架自动转换为 "/dep2p/app/<realmID>/file-transfer/1.0.0"
streams.SetHandler("file-transfer/1.0.0", handler)

// 打开流时同理
stream, _ := streams.Open(ctx, peerID, "file-transfer/1.0.0")

// PubSub topic 也遵循相同规则
// 用户写 "blocks"
// 实际 topic: "/dep2p/app/<realmID>/blocks"
// 注意：不需要 /pubsub/ 前缀，通过服务类型区分（PubSub vs Streams）
topic, _ := realm.PubSub().Join(ctx, "blocks")
```

#### 安全边界保证

| 场景 | 风险 | 解决方案 |
|------|------|---------|
| 用户注册 `/dep2p/sys/ping/1.0.0` | 冒充系统协议 | 运行时拒绝，返回 `ErrReservedProtocol` |
| 用户注册 `/dep2p/app/<otherRealmID>/...` | 跨 Realm 攻击 | 运行时拒绝，只允许当前 RealmID |
| Realm-A 和 Realm-B 都用 `chat/1.0.0` | 协议冲突 | 无冲突，实际协议带不同 realmID |
| 恶意节点尝试连接不属于的 Realm 协议 | 穿透隔离 | PSK 验证失败，连接被拒绝 |

#### 协议前缀常量（实现参考）

```go
const (
    // 系统协议前缀（硬编码保护，用户不可注册）
    ProtocolPrefixSystem = "/dep2p/sys/"
    
    // Realm 协议前缀模板
    ProtocolPrefixRealm = "/dep2p/realm/%s/"  // %s = realmID
    
    // 应用协议前缀模板
    ProtocolPrefixApp = "/dep2p/app/%s/"      // %s = realmID
)

// 内部转换函数
func (r *realm) fullProtocolID(userProto string) string {
    return fmt.Sprintf("/dep2p/app/%s/%s", r.ID(), userProto)
}

// 协议验证
func validateProtocol(proto string) error {
    if strings.HasPrefix(proto, ProtocolPrefixSystem) {
        return ErrReservedProtocol
    }
    return nil
}
```

### 方案对比

#### 方案 A：Realm 作为核心操作句柄

```go
// Layer 1: 启动节点，产出 node
node, err := dep2p.StartNode(ctx, dep2p.WithPreset(dep2p.PresetDesktop))
defer node.Close()

// Layer 2: 加入 Realm，产出 realm 对象
realm, err := node.JoinRealm(ctx, "my-business-network")
// realm, err := node.JoinRealm(ctx, "private-net", dep2p.WithRealmKey(key))

// Layer 3: 通过 realm 获取各种服务/协议

// 方式 1：直接消息（简单场景）
err = realm.Send(ctx, peerID, "/app/notify", data)
resp, err := realm.Request(ctx, peerID, "/app/api", reqData)

// 方式 2：PubSub（订阅发布场景）
topic, err := realm.JoinTopic(ctx, "blocks")
err = topic.Publish(ctx, blockData)
sub, err := topic.Subscribe()
for msg := range sub.Messages() {
    // 处理消息
}

// 方式 3：自定义协议（流式场景）
proto, err := realm.RegisterProtocol("/app/file-transfer/1.0.0", handler)
stream, err := realm.OpenStream(ctx, peerID, "/app/file-transfer/1.0.0")
```

**优点**：
- API 简洁，直接通过 `realm` 操作
- 符合"Realm 是业务入口"的直觉

**缺点**：
- `realm` 接口会变得臃肿（所有服务方法都在这里）
- 服务之间没有明确分离
- 扩展性较差（新增服务需要修改 Realm 接口）

#### 方案 B：更细粒度的服务对象（✅ 采用）

**整体架构图**：

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                           DeP2P 三层服务架构                                 │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  Layer 3: 应用服务层（从 Realm 获取）                                │    │
│  │                                                                      │    │
│  │   ┌──────────────┐  ┌──────────────┐  ┌──────────────┐              │    │
│  │   │  Messaging   │  │   PubSub     │  │  Discovery   │              │    │
│  │   │  ──────────  │  │  ──────────  │  │  ──────────  │              │    │
│  │   │  • Send      │  │  • Join      │  │  • FindPeers │              │    │
│  │   │  • Request   │  │  • Topic     │  │  • Advertise │              │    │
│  │   │  • OnMessage │  │  • Subscribe │  │  • Watch     │              │    │
│  │   └──────────────┘  └──────────────┘  └──────────────┘              │    │
│  │                                                                      │    │
│  │   ┌──────────────┐  ┌──────────────┐  ┌──────────────┐              │    │
│  │   │   Streams    │  │    Relay     │  │  Protocols   │   ... 可扩展  │    │
│  │   │  ──────────  │  │  ──────────  │  │  ──────────  │              │    │
│  │   │  • Open      │  │  • Serve     │  │  • Register  │              │    │
│  │   │  • Accept    │  │  • Reserve   │  │  • Handle    │              │    │
│  │   │  • SetHandler│  │  • FindRelays│  │              │              │    │
│  │   └──────────────┘  └──────────────┘  └──────────────┘              │    │
│  │                                                                      │    │
│  │   产出对象: Topic, Subscription, Stream, Reservation                 │    │
│  │   协议前缀: /dep2p/app/*                                            │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                      ▲                                       │
│                                      │ realm.Messaging() / realm.PubSub()   │
│                                      │ realm.Discovery() / realm.Streams()   │
│                                      │ realm.Relay()                         │
│                                      │                                       │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  Layer 2: Realm 层                                                   │    │
│  │                                                                      │    │
│  │   ┌────────────────────────────────────────────────────────────┐    │    │
│  │   │                        Realm                                │    │    │
│  │   │  ────────────────────────────────────────────────────────  │    │    │
│  │   │  基本信息:                                                 │    │    │
│  │   │  • Name() / ID()           • Members()                     │    │    │
│  │   │  • MemberCount()            • IsMember(peerID)               │    │    │
│  │   │                                                             │    │    │
│  │   │  服务入口（核心！）:                                        │    │    │
│  │   │  • Messaging() → Messaging                                  │    │    │
│  │   │  • PubSub()    → PubSub                                     │    │    │
│  │   │  • Discovery() → RealmDiscovery                             │    │    │
│  │   │  • Streams()   → StreamManager                              │    │    │
│  │   │  • Relay()     → RealmRelayService                          │    │    │
│  │   │                                                             │    │    │
│  │   │  生命周期:                                                  │    │    │
│  │   │  • Leave()                 • Context()                      │    │    │
│  │   └────────────────────────────────────────────────────────────┘    │    │
│  │                                                                      │    │
│  │   产出对象: Realm                                                    │    │
│  │   协议前缀: /dep2p/realm/*                                          │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                      ▲                                       │
│                                      │ node.JoinRealm()                      │
│                                      │                                       │
│  ┌─────────────────────────────────────────────────────────────────────┐    │
│  │  Layer 1: 系统基础层                                                 │    │
│  │                                                                      │    │
│  │   ┌────────────────────────────────────────────────────────────┐    │    │
│  │   │                         Node                                │    │    │
│  │   │  ────────────────────────────────────────────────────────  │    │    │
│  │   │  身份与地址:                                               │    │    │
│  │   │  • ID() / ListenAddrs() / AdvertisedAddrs()                │    │    │
│  │   │  • ShareableAddrs()                                        │    │    │
│  │   │                                                             │    │    │
│  │   │  Realm 管理:                                               │    │    │
│  │   │  • JoinRealm(name) → Realm                                  │    │    │
│  │   │  • JoinRealmWithKey(name, key) → Realm                      │    │    │
│  │   │  • CurrentRealm() → Realm | nil                             │    │    │
│  │   │                                                             │    │    │
│  │   │  底层能力:                                                 │    │    │
│  │   │  • Endpoint()                                               │    │    │
│  │   │                                                             │    │    │
│  │   │  生命周期:                                                 │    │    │
│  │   │  • Close()                                                  │    │    │
│  │   └────────────────────────────────────────────────────────────┘    │    │
│  │                                                                      │    │
│  │   ┌────────────────────────────────────────────────────────────┐    │    │
│  │   │  内部组件（用户不可见）                                     │    │    │
│  │   │  Transport / Security / DHT / Relay / NAT / Bootstrap      │    │    │
│  │   └────────────────────────────────────────────────────────────┘    │    │
│  │                                                                      │    │
│  │   产出对象: Node                                                     │    │
│  │   协议前缀: /dep2p/sys/*                                            │    │
│  └─────────────────────────────────────────────────────────────────────┘    │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

**代码示例**：

```go
// Layer 1
node, _ := dep2p.StartNode(ctx, dep2p.WithPreset(dep2p.PresetDesktop))

// Layer 2
realm, _ := node.JoinRealm(ctx, "my-realm")

// Layer 3: 获取不同的服务对象
messaging := realm.Messaging()       // 消息服务
pubsub := realm.PubSub()             // 发布订阅服务
discovery := realm.Discovery()       // Realm 内发现服务
streams := realm.Streams()           // 流管理服务
relay := realm.Relay()               // Realm 中继服务

// 使用消息服务
messaging.Send(ctx, peerID, data)
resp, _ := messaging.Request(ctx, peerID, data)

// 使用 PubSub 服务
topic, _ := pubsub.Join("blocks")
topic.Publish(ctx, data)
sub, _ := topic.Subscribe()

// 发现 Realm 内的节点
peers, _ := discovery.FindPeers(ctx)

// 使用流服务
stream, _ := streams.Open(ctx, peerID, "/app/file-transfer/1.0.0")

// 使用中继服务
relay.Serve(ctx)  // 成为中继
reservation, _ := relay.Reserve(ctx, relayNodeID)  // 预留中继槽位
```

**优点**：
- 层次更清晰：Node → Realm → Services，每层职责明确
- 服务隔离，互不干扰：Messaging、PubSub、Discovery 等服务独立
- 扩展性更强：新增服务只需在 Realm 上增加 getter，不影响现有 API
- 符合单一职责原则：每个服务只负责自己的功能
- 符合用户直觉：`realm.PubSub().Join("topic").Subscribe()` 链式调用自然
- 生命周期清晰：Subscription 依赖 Topic，Topic 依赖 Realm，Realm 依赖 Node

**缺点**：
- API 稍复杂（但更规范，更易维护）

### 详细对比分析

| 维度 | 当前设计 | 方案 A | 方案 B |
|------|---------|--------|--------|
| **JoinRealm 返回值** | `error` | `(Realm, error)` | `(Realm, error)` |
| **Layer 3 入口** | `node.Send()` | `realm.Send()` / `realm.JoinTopic()` | `realm.Messaging()` / `realm.PubSub()` |
| **层次清晰度** | ❌ 混乱 | ✅ 清晰 | ✅✅ 更清晰 |
| **API 复杂度** | 简单（但混乱） | 中等 | 稍复杂（但规范） |
| **扩展性** | 差 | 好 | 非常好 |
| **符合用户心智** | ❌ | ✅ | ✅✅ |
| **服务隔离** | ❌ | ❌ | ✅ |
| **单一职责** | ❌ | ❌ | ✅ |

### 完整对象流（方案 B）

**对象产出流程图**：

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    方案 B：完整对象产出流程                                   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│  用户代码                          DeP2P 内部                                 │
│  ────────────────────────────────────────────────────────────────────────   │
│                                                                              │
│  StartNode()                       → 初始化 Layer 1                          │
│      │                               - Transport (QUIC)                      │
│      │                               - Security (TLS)                        │
│      │                               - DHT / Relay / NAT / Bootstrap         │
│      │                               - Identity / Addressing                 │
│      ▼                                                                       │
│    node ──────────────────────────→ Node 对象（Layer 1 产物）                 │
│      │                               - ID()                                   │
│      │                               - ListenAddrs()                         │
│      │                               - AdvertisedAddrs()                     │
│      │                               - ShareableAddrs()                      │
│      │                               - JoinRealm() / CurrentRealm()           │
│      │                               - Endpoint()                             │
│      │                               - Close()                                │
│      │                                                                       │
│      │ node.JoinRealm("my-realm")  → 初始化 Layer 2                          │
│      │                               - 成员身份验证                           │
│      │                               - Realm DHT 加入                         │
│      │                               - Realm 成员列表同步                     │
│      ▼                                                                       │
│    realm ─────────────────────────→ Realm 对象（Layer 2 产物）                │
│      │                               - Name() / ID()                          │
│      │                               - Members() / MemberCount()              │
│      │                               - IsMember()                             │
│      │                               - Messaging() / PubSub() / ...           │
│      │                               - Leave() / Context()                    │
│      │                                                                       │
│      ├── realm.Messaging()        → 获取 Messaging 服务                       │
│      │                               - 初始化消息路由                         │
│      │                               - 注册消息处理器                         │
│      ▼                                                                       │
│    messaging ─────────────────────→ Messaging 对象（Layer 3 产物）            │
│      │                               - Send() / SendWithProtocol()            │
│      │                               - Request() / RequestWithProtocol()       │
│      │                               - OnMessage() / OnRequest()              │
│      │                               - OnProtocol()                           │
│      │                                                                       │
│      ├── realm.PubSub()           → 获取 PubSub 服务                          │
│      │                               - 初始化 GossipSub                       │
│      ▼                                                                       │
│    pubsub ────────────────────────→ PubSub 对象（Layer 3 产物）               │
│      │                               - Join() / Topics()                      │
│      │                               - Publish() / Subscribe() (快捷)          │
│      │                                                                       │
│      │ pubsub.Join("blocks")      → 初始化 Topic                              │
│      │                               - 加入 GossipSub topic                    │
│      │                               - 同步订阅者列表                         │
│      ▼                                                                       │
│    topic ─────────────────────────→ Topic 对象（Layer 3 产物）                │
│      │                               - Name()                                 │
│      │                               - Publish() / PublishWithMeta()          │
│      │                               - Subscribe()                             │
│      │                               - Peers()                                 │
│      │                               - Leave()                                 │
│      │                                                                       │
│      │ topic.Subscribe()          → 创建订阅                                  │
│      │                               - 注册消息通道                            │
│      ▼                                                                       │
│    subscription ──────────────────→ Subscription 对象                         │
│                                        - Messages() <-chan *PubSubMessage      │
│                                        - Cancel()                              │
│      │                                                                       │
│      ├── realm.Discovery()        → 获取 Discovery 服务                       │
│      │                               - 初始化 Realm DHT                        │
│      ▼                                                                       │
│    discovery ─────────────────────→ RealmDiscovery 对象（Layer 3 产物）       │
│                                        - FindPeers() / FindPeersWithService()  │
│                                        - Advertise() / StopAdvertise()         │
│                                        - Watch() → Events                     │
│      │                                                                       │
│      ├── realm.Streams()          → 获取 StreamManager 服务                   │
│      │                               - 初始化流处理器表                        │
│      ▼                                                                       │
│    streams ───────────────────────→ StreamManager 对象（Layer 3 产物）        │
│                                        - Open()                                │
│                                        - SetHandler() / RemoveHandler()        │
│      │                                                                       │
│      │ streams.Open(peerID, proto) → 打开流                                    │
│      ▼                                                                       │
│    stream ────────────────────────→ Stream 对象                               │
│                                        - io.Reader / io.Writer / io.Closer    │
│                                        - Protocol() / RemotePeer()             │
│                                        - SetDeadline() / CloseWrite()          │
│      │                                                                       │
│      └── realm.Relay()            → 获取 RealmRelayService 服务               │
│                                        - Serve() / StopServing()               │
│                                        - FindRelays()                          │
│                                        - Reserve() → Reservation               │
│                                        - Stats()                                │
│                                                                              │
│   realm.Leave() ◀───────────────────────────────────────────────────────────  │
│   node.Close()  ◀───────────────────────────────────────────────────────────  │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

**关键点**：
1. **每一层都产出一个对象**：Node → Realm → Services → Sub-objects
2. **对象代表上下文**：每个对象都"知道"自己所在的层次和上下文
3. **生命周期依赖**：子对象依赖父对象，父对象关闭时子对象自动清理
4. **服务独立**：Messaging、PubSub、Discovery 等服务互不干扰，可独立使用

---

## 完整接口设计（方案 B）

### Layer 1: Node 接口

```go
// ═══════════════════════════════════════════════════════════════════════════
// Layer 1: Node - 系统基础层
// ═══════════════════════════════════════════════════════════════════════════

package dep2p

// Node 代表一个 DeP2P 节点（Layer 1 产物）
// 负责底层连接、身份、地址管理
type Node interface {
    // ─────────────────────────────────────────────────────────────────────
    // 身份与地址
    // ─────────────────────────────────────────────────────────────────────
    
    // ID 返回节点的唯一标识（公钥派生）
    ID() NodeID
    
    // ListenAddrs 返回本地监听地址
    ListenAddrs() []Multiaddr
    
    // AdvertisedAddrs 返回对外通告地址（可被其他节点连接）
    AdvertisedAddrs() []Multiaddr
    
    // ShareableAddrs 返回可分享的完整地址（含 NodeID，适合 Bootstrap）
    ShareableAddrs() []string
    
    // ─────────────────────────────────────────────────────────────────────
    // Realm 管理（Layer 1 → Layer 2 的桥梁）
    // ─────────────────────────────────────────────────────────────────────
    
    // JoinRealm 加入业务网络，返回 Realm 对象
    //
    // 当前实现约束：严格单 Realm（强隔离优先）。
    // - 如果已加入其他 Realm，返回 ErrAlreadyInRealm
    // - 未来如需多 Realm，将在不破坏隔离边界的前提下扩展（见本文“多 Realm 演进”讨论）
    JoinRealm(ctx context.Context, name string, opts ...RealmOption) (Realm, error)
    
    // JoinRealmWithKey 加入需要密钥的 Realm
    JoinRealmWithKey(ctx context.Context, name string, key []byte, opts ...RealmOption) (Realm, error)
    
    // CurrentRealm 返回当前所在的 Realm，未加入则返回 nil
    //
    // 说明：该方法适配“单 Realm”约束；若未来支持多 Realm，应提供 Realms()/GetRealm(id) 等 API，
    // 并避免把“当前 Realm”作为全局隐式状态影响上层业务逻辑。
    CurrentRealm() Realm
    
    // ─────────────────────────────────────────────────────────────────────
    // 底层能力（高级用户 / 调试）
    // ─────────────────────────────────────────────────────────────────────
    
    // Endpoint 获取底层连接端点（直接操作连接，绑定身份目标）
    Endpoint() Endpoint
    
    // ─────────────────────────────────────────────────────────────────────
    // 生命周期
    // ─────────────────────────────────────────────────────────────────────
    
    // Close 关闭节点，释放所有资源
    Close() error
}
```

### Layer 2: Realm 接口

```go
// ═══════════════════════════════════════════════════════════════════════════
// Layer 2: Realm - 业务隔离层
// ═══════════════════════════════════════════════════════════════════════════

// Realm 代表一个业务网络（Layer 2 产物）
// 是访问所有 Layer 3 服务的入口
type Realm interface {
    // ─────────────────────────────────────────────────────────────────────
    // 基本信息
    // ─────────────────────────────────────────────────────────────────────
    
    // Name 返回 Realm 名称
    Name() string
    
    // ID 返回 Realm 唯一标识（由名称派生的哈希）
    ID() RealmID
    
    // ─────────────────────────────────────────────────────────────────────
    // 成员管理
    // ─────────────────────────────────────────────────────────────────────
    
    // Members 返回已知的 Realm 成员列表
    Members() []NodeID
    
    // MemberCount 返回成员数量
    MemberCount() int
    
    // IsMember 检查指定节点是否为 Realm 成员
    IsMember(peer NodeID) bool
    
    // ─────────────────────────────────────────────────────────────────────
    // Layer 3 服务获取（核心！）
    // ─────────────────────────────────────────────────────────────────────
    
    // Messaging 获取消息服务（点对点通信）
    Messaging() Messaging
    
    // PubSub 获取发布订阅服务（一对多广播）
    PubSub() PubSub
    
    // Discovery 获取 Realm 内节点发现服务
    Discovery() RealmDiscovery
    
    // Streams 获取流管理服务（原始双向流）
    Streams() StreamManager
    
    // Relay 获取 Realm 中继服务
    Relay() RealmRelayService
    
    // ─────────────────────────────────────────────────────────────────────
    // 生命周期
    // ─────────────────────────────────────────────────────────────────────
    
    // Leave 离开 Realm
    Leave() error
    
    // Context 返回 Realm 的生命周期 Context（Leave 时取消）
    Context() context.Context
}
```

### Layer 3: 服务接口

#### Messaging - 消息服务

```go
// Messaging 提供点对点消息通信能力
type Messaging interface {
    // 发送消息
    Send(ctx context.Context, to NodeID, data []byte) error
    SendWithProtocol(ctx context.Context, to NodeID, protocol string, data []byte) error
    
    // 请求响应
    Request(ctx context.Context, to NodeID, data []byte) ([]byte, error)
    RequestWithProtocol(ctx context.Context, to NodeID, protocol string, data []byte) ([]byte, error)
    
    // 接收消息
    OnMessage(handler MessageHandler)
    OnRequest(handler RequestHandler)
    OnProtocol(protocol string, handler ProtocolHandler)
}

type MessageHandler func(msg *Message)
type RequestHandler func(req *Request) *Response

type Message struct {
    From     NodeID
    Data     []byte
    Protocol string
    Time     time.Time
}

type Request struct {
    From     NodeID
    Data     []byte
    Protocol string
}

type Response struct {
    Data  []byte
    Error error
}
```

#### PubSub - 发布订阅服务

```go
// PubSub 提供一对多的发布订阅能力
type PubSub interface {
    // Topic 管理
    Join(ctx context.Context, name string) (Topic, error)
    Topics() []Topic
    
    // 快捷方法
    Publish(ctx context.Context, topic string, data []byte) error
    Subscribe(ctx context.Context, topic string) (Subscription, error)
}

// Topic 代表一个发布订阅主题
type Topic interface {
    Name() string
    Publish(ctx context.Context, data []byte) error
    PublishWithMeta(ctx context.Context, data []byte, meta map[string]string) error
    Subscribe() (Subscription, error)
    Peers() []NodeID
    Leave() error
}

// Subscription 代表一个订阅
type Subscription interface {
    Messages() <-chan *PubSubMessage
    Cancel()
}

type PubSubMessage struct {
    From   NodeID
    Topic  string
    Data   []byte
    Meta   map[string]string
    SeqNo  uint64
    Time   time.Time
}
```

#### Discovery - Realm 内发现服务

```go
// RealmDiscovery 提供 Realm 内的节点发现能力
type RealmDiscovery interface {
    FindPeers(ctx context.Context, opts ...FindOption) ([]NodeID, error)
    FindPeersWithService(ctx context.Context, service string) ([]NodeID, error)
    Advertise(ctx context.Context, service string, opts ...AdvertiseOption) error
    StopAdvertise(service string) error
    Watch(ctx context.Context) (<-chan MemberEvent, error)
}

type MemberEvent struct {
    Type MemberEventType
    Peer NodeID
    Time time.Time
}

type MemberEventType int

const (
    MemberJoined MemberEventType = iota
    MemberLeft
)
```

#### Streams - 原始流管理

```go
// StreamManager 提供原始双向流能力
type StreamManager interface {
    Open(ctx context.Context, to NodeID, protocol string) (Stream, error)
    SetHandler(protocol string, handler StreamHandler)
    RemoveHandler(protocol string)
}

// Stream 双向流
type Stream interface {
    io.Reader
    io.Writer
    io.Closer
    
    Protocol() string
    RemotePeer() NodeID
    
    SetDeadline(t time.Time) error
    SetReadDeadline(t time.Time) error
    SetWriteDeadline(t time.Time) error
    CloseWrite() error
}

type StreamHandler func(stream Stream)
```

---

## 完整使用示例

```go
package main

import (
    "context"
    "fmt"
    "log"
    "time"
    
    "github.com/dep2p/go-dep2p"
)

func main() {
    ctx := context.Background()
    
    // ═══════════════════════════════════════════════════════════════════
    // Layer 1: 启动节点
    // ═══════════════════════════════════════════════════════════════════
    node, err := dep2p.StartNode(ctx, dep2p.WithPreset(dep2p.PresetDesktop))
    if err != nil {
        log.Fatal(err)
    }
    defer node.Close()
    
    fmt.Printf("🚀 节点已启动: %s\n", node.ID())
    
    // ═══════════════════════════════════════════════════════════════════
    // Layer 2: 加入 Realm
    // ═══════════════════════════════════════════════════════════════════
    realm, err := node.JoinRealm(ctx, "blockchain-mainnet")
    if err != nil {
        log.Fatal(err)
    }
    defer realm.Leave()
    
    fmt.Printf("🌐 已加入 Realm: %s (成员: %d)\n", realm.Name(), realm.MemberCount())
    
    // ═══════════════════════════════════════════════════════════════════
    // Layer 3: 使用各种服务
    // ═══════════════════════════════════════════════════════════════════
    
    // ─────────────────────────────────────────────────────────────────
    // 服务 1: Messaging - 点对点通信
    // ─────────────────────────────────────────────────────────────────
    messaging := realm.Messaging()
    
    // 注册消息处理器
    messaging.OnMessage(func(msg *dep2p.Message) {
        fmt.Printf("📨 收到消息: from=%s, data=%s\n", msg.From, msg.Data)
    })
    
    // 注册请求处理器
    messaging.OnRequest(func(req *dep2p.Request) *dep2p.Response {
        fmt.Printf("📥 收到请求: from=%s\n", req.From)
        return &dep2p.Response{Data: []byte("pong")}
    })
    
    // ─────────────────────────────────────────────────────────────────
    // 服务 2: PubSub - 发布订阅
    // ─────────────────────────────────────────────────────────────────
    pubsub := realm.PubSub()
    
    // 方式 A: 显式加入 Topic
    blocksTopic, err := pubsub.Join(ctx, "blocks")
    if err != nil {
        log.Fatal(err)
    }
    
    // 订阅
    sub, err := blocksTopic.Subscribe()
    if err != nil {
        log.Fatal(err)
    }
    
    // 异步接收消息
    go func() {
        for msg := range sub.Messages() {
            fmt.Printf("📦 收到区块: from=%s, seqno=%d\n", msg.From, msg.SeqNo)
        }
    }()
    
    // 发布
    blocksTopic.Publish(ctx, []byte(`{"height": 12345, "hash": "abc..."}`))
    
    // 方式 B: 快捷方法（自动 Join）
    pubsub.Publish(ctx, "transactions", []byte("tx data"))
    
    // ─────────────────────────────────────────────────────────────────
    // 服务 3: Discovery - 节点发现
    // ─────────────────────────────────────────────────────────────────
    discovery := realm.Discovery()
    
    // 通告自己提供的服务
    discovery.Advertise(ctx, "validator")
    
    // 发现提供特定服务的节点
    validators, err := discovery.FindPeersWithService(ctx, "validator")
    if err == nil {
        fmt.Printf("🔍 发现 %d 个验证节点\n", len(validators))
    }
    
    // ─────────────────────────────────────────────────────────────────
    // 服务 4: Streams - 原始流（大文件传输）
    // ─────────────────────────────────────────────────────────────────
    streams := realm.Streams()
    
    // 注册文件传输协议处理器
    streams.SetHandler("/app/file-transfer/1.0.0", func(s dep2p.Stream) {
        defer s.Close()
        // 处理流...
    })
    
    fmt.Println("✅ 所有服务已就绪，按 Ctrl+C 退出")
    select {}
}
```

---

## 服务对象关系总览

### 方案 B 完整服务关系图

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    方案 B：服务对象关系与生命周期                             │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│   StartNode()                                                                │
│       │                                                                      │
│       ▼                                                                      │
│   ┌──────┐                                                                   │
│   │ Node │ ─────────────────────────────────────────────────────────────┐   │
│   │      │                                                             │   │
│   │  Layer 1 产物                                                      │   │
│   │  • 身份与地址管理                                                   │   │
│   │  • 底层连接管理                                                     │   │
│   │  • Realm 生命周期管理                                               │   │
│   └──┬───┘                                                               │   │
│      │                                                                   │   │
│      │ JoinRealm(name)                                                   │   │
│      │   ↓ 验证成员身份                                                  │   │
│      │   ↓ 加入 Realm DHT                                                │   │
│      ▼                                                                   │   │
│   ┌───────┐                                                              │   │
│   │ Realm │ ─────────────────────────────────────────────────────────┐  │   │
│   │       │                                                          │  │   │
│   │  Layer 2 产物                                                     │  │   │
│   │  • 业务隔离                                                        │  │   │
│   │  • 成员管理                                                        │  │   │
│   │  • Layer 3 服务入口                                                │  │   │
│   └───┬───┘                                                          │  │   │
│       │                                                              │  │   │
│       ├── Messaging() ──▶ ┌──────────────┐                          │  │   │
│       │                   │  Messaging  │                          │  │   │
│       │                   │  ────────── │                          │  │   │
│       │                   │  • Send     │                          │  │   │
│       │                   │  • Request │                          │  │   │
│       │                   │  • OnMsg    │                          │  │   │
│       │                   └──────────────┘                          │  │   │
│       │                                                              │  │   │
│       ├── PubSub() ──────▶ ┌──────────────┐                          │  │   │
│       │                    │   PubSub    │                          │  │   │
│       │                    │  ────────── │                          │  │   │
│       │                    │  • Join     │ ──▶ ┌──────────┐         │  │   │
│       │                    │  • Publish │     │  Topic   │         │  │   │
│       │                    │  • Subscribe│    │  ──────── │         │  │   │
│       │                    └──────────────┘    │ • Publish │         │  │   │
│       │                                        │ • Subscribe│ ──▶ ┌───┐│  │   │
│       │                                        └──────────┘     │Sub││  │   │
│       │                                                          │───││  │   │
│       │                                                          │Msg││  │   │
│       │                                                          └───┘│  │   │
│       │                                                              │  │   │
│       ├── Discovery() ──▶ ┌──────────────┐                          │  │   │
│       │                   │  Discovery  │                          │  │   │
│       │                   │  ────────── │                          │  │   │
│       │                   │  • FindPeers│                          │  │   │
│       │                   │  • Advertise│                          │  │   │
│       │                   │  • Watch    │ ──▶ Events              │  │   │
│       │                   └──────────────┘                          │  │   │
│       │                                                              │  │   │
│       ├── Streams() ────▶ ┌──────────────┐                          │  │   │
│       │                    │  StreamMgr  │                          │  │   │
│       │                    │  ────────── │                          │  │   │
│       │                    │  • Open     │ ──▶ Stream               │  │   │
│       │                    │  • SetHandler│                          │  │   │
│       │                    └──────────────┘                          │  │   │
│       │                                                              │  │   │
│       └── Relay() ───────▶ ┌──────────────┐                          │  │   │
│                            │ RealmRelay  │                          │  │   │
│                            │  ────────── │                          │  │   │
│                            │  • Serve    │                          │  │   │
│                            │  • Reserve  │ ──▶ Reservation         │  │   │
│                            │  • FindRelays│                          │  │   │
│                            └──────────────┘                          │  │   │
│                                                                      │  │   │
│   realm.Leave() ◀────────────────────────────────────────────────────┘  │   │
│   node.Close()  ◀───────────────────────────────────────────────────────┘   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 对象产出与生命周期（详细版）

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                          对象产出与生命周期                                   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│   StartNode()                                                                │
│       │                                                                      │
│       ▼                                                                      │
│   ┌──────┐                                                                   │
│   │ Node │ ─────────────────────────────────────────────────────────────┐   │
│   └──┬───┘                                                               │   │
│      │                                                                   │   │
│      │ JoinRealm()                                                       │   │
│      ▼                                                                   │   │
│   ┌───────┐                                                              │   │
│   │ Realm │ ─────────────────────────────────────────────────────────┐  │   │
│   └───┬───┘                                                          │  │   │
│       │                                                              │  │   │
│       ├── Messaging() ──▶ Messaging ──┬── OnMessage()                │  │   │
│       │                               ├── OnRequest()                │  │   │
│       │                               ├── Send()                     │  │   │
│       │                               └── Request()                  │  │   │
│       │                                                              │  │   │
│       ├── PubSub() ─────▶ PubSub ─────┬── Join() ──▶ Topic ──┐       │  │   │
│       │                               │              │       │       │  │   │
│       │                               │              ├── Publish()   │  │   │
│       │                               │              └── Subscribe() │  │   │
│       │                               │                      │       │  │   │
│       │                               │                      ▼       │  │   │
│       │                               │              ┌──────────────┐│  │   │
│       │                               │              │ Subscription ││  │   │
│       │                               │              │  Messages()  ││  │   │
│       │                               │              └──────────────┘│  │   │
│       │                               │                              │  │   │
│       │                               ├── Publish() (快捷)           │  │   │
│       │                               └── Subscribe() (快捷)         │  │   │
│       │                                                              │  │   │
│       ├── Discovery() ──▶ RealmDiscovery ──┬── FindPeers()           │  │   │
│       │                                    ├── Advertise()           │  │   │
│       │                                    └── Watch() ──▶ Events    │  │   │
│       │                                                              │  │   │
│       ├── Streams() ────▶ StreamManager ───┬── Open() ──▶ Stream     │  │   │
│       │                                    └── SetHandler()          │  │   │
│       │                                                              │  │   │
│       └── Relay() ──────▶ RealmRelayService ─┬── Serve()             │  │   │
│                                              ├── FindRelays()        │  │   │
│                                              └── Reserve() ──▶ Reservation  │  │
│                                                                      │  │   │
│   realm.Leave() ◀────────────────────────────────────────────────────┘  │   │
│   node.Close()  ◀───────────────────────────────────────────────────────┘   │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

### 服务依赖关系图

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                        方案 B：服务依赖关系                                   │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│   Node (Layer 1)                                                             │
│   └── Realm (Layer 2)                                                        │
│       ├── Messaging (Layer 3)                                                │
│       │   └── 依赖 Realm 的成员身份验证                                       │
│       │                                                                      │
│       ├── PubSub (Layer 3)                                                   │
│       │   └── Topic                                                          │
│       │       └── Subscription                                               │
│       │           └── 依赖 Realm 的成员身份验证                               │
│       │                                                                      │
│       ├── Discovery (Layer 3)                                                │
│       │   └── 依赖 Realm DHT                                                 │
│       │                                                                      │
│       ├── Streams (Layer 3)                                                  │
│       │   └── Stream                                                         │
│       │       └── 依赖 Realm 的成员身份验证                                   │
│       │                                                                      │
│       └── Relay (Layer 3)                                                    │
│           └── Reservation                                                    │
│               └── 依赖 Realm 的成员身份验证                                   │
│                                                                              │
│   所有 Layer 3 服务都依赖 Realm 的成员身份验证                                │
│   所有服务都依赖 Node 的底层连接能力                                          │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 结论

采用 **方案 B：细粒度服务对象架构**：

1. **Layer 1 产出 Node**：负责底层连接、身份、地址管理
2. **Layer 2 产出 Realm**：负责业务隔离，是访问 Layer 3 服务的入口
3. **Layer 3 产出服务对象**：
   - `realm.Messaging()` → Messaging 服务
   - `realm.PubSub()` → PubSub 服务
   - `realm.Discovery()` → RealmDiscovery 服务
   - `realm.Streams()` → StreamManager 服务
   - `realm.Relay()` → RealmRelayService 服务

### 设计优点总结

| 维度 | 说明 |
|------|------|
| **层次清晰** | Node → Realm → Services，每层职责明确 |
| **对象产出** | 每次操作都返回有意义的对象，可继续操作 |
| **服务隔离** | Messaging、PubSub、Discovery 等服务独立，互不干扰 |
| **扩展性强** | 新增服务只需在 Realm 上增加 getter，不影响现有 API |
| **符合直觉** | `realm.PubSub().Join("topic").Subscribe()` 链式调用自然 |
| **生命周期清晰** | Subscription 依赖 Topic，Topic 依赖 Realm，Realm 依赖 Node |

---

## 多 Realm 演进方案（当前强隔离 + 未来可扩展）

### 当前设计：严格单 Realm

**决策**：当前版本 **一个 Node 同时只能加入一个 Realm**。

```go
// 当前行为
realm1, _ := node.JoinRealm(ctx, "realm-a", dep2p.WithRealmKey(keyA))
realm2, err := node.JoinRealm(ctx, "realm-b", dep2p.WithRealmKey(keyB))
// err = ErrAlreadyInRealm（必须先 Leave 才能加入另一个）
```

**理由**：

| 维度 | 强隔离的好处 |
|------|-------------|
| **实现简单** | 无需处理多 Realm 资源竞争、协议路由、连接复用 |
| **安全边界清晰** | 所有流量/协议都属于单一 Realm，无泄露风险 |
| **心智负担低** | 用户不需要考虑"这个消息发到哪个 Realm" |
| **符合大多数场景** | 客户端/轻节点通常只参与一个业务网络 |

### 未来演进：多 Realm 模式（如果需要）

若未来出现"网关/桥接/多租户"需求，可按以下原则扩展，**不破坏现有单 Realm API**：

#### 演进原则

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                     多 Realm 演进原则                                        │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                              │
│   1. 保持强隔离不变                                                          │
│   ─────────────────                                                          │
│   • 不同 Realm 的协议天然隔离（/dep2p/app/<realmID>/...）                    │
│   • 不同 Realm 的连接/流互不可见                                             │
│   • 不同 Realm 的 PSK 完全独立                                               │
│                                                                              │
│   2. API 扩展（向后兼容）                                                    │
│   ─────────────────────                                                      │
│   • 新增 Realms() 返回多个 Realm（若启用多 Realm 模式）                      │
│   • CurrentRealm() 行为不变（返回"主" Realm 或最近加入的）                   │
│   • JoinRealm() 在多 Realm 模式下不再返回 ErrAlreadyInRealm                  │
│                                                                              │
│   3. 资源配额                                                                │
│   ───────────                                                                │
│   • 每个 Realm 独立的连接池/带宽配额                                         │
│   • 防止一个 Realm 的流量影响另一个                                          │
│                                                                              │
│   4. 显式模式切换                                                            │
│   ─────────────────                                                          │
│   • 多 Realm 作为 Node 启动选项，默认关闭                                    │
│   • dep2p.WithMultiRealmMode(true)                                           │
│                                                                              │
└─────────────────────────────────────────────────────────────────────────────┘
```

#### 未来 API 预览（仅供参考，当前不实现）

```go
// 启用多 Realm 模式
node, _ := dep2p.StartNode(ctx, 
    dep2p.WithPreset(dep2p.PresetGateway),
    dep2p.WithMultiRealmMode(true),
)

// 可加入多个 Realm
realmA, _ := node.JoinRealm(ctx, "chain-a", dep2p.WithRealmKey(keyA))
realmB, _ := node.JoinRealm(ctx, "chain-b", dep2p.WithRealmKey(keyB))

// 获取所有已加入的 Realm
realms := node.Realms() // []Realm{realmA, realmB}

// 每个 Realm 完全独立操作
realmA.Messaging().Send(ctx, peerInA, data) // 只发到 Realm-A
realmB.PubSub().Join(ctx, "blocks")         // 只在 Realm-B

// 跨 Realm 桥接（显式应用逻辑，框架不提供）
go func() {
    for msg := range realmA.PubSub().Subscribe(ctx, "events").Messages() {
        realmB.Messaging().Send(ctx, gatewayPeer, msg.Data) // 应用自己桥接
    }
}()
```

#### 为什么当前不做多 Realm

| 风险 | 说明 |
|------|------|
| **复杂度爆炸** | 连接复用、协议路由、资源隔离都需重新设计 |
| **需求不明确** | 目前没有明确的多 Realm 产品场景 |
| **可后续追加** | 当前设计不阻止未来扩展（协议命名空间已预留 realmID） |

**结论**：当前固定为单 Realm 强隔离，协议命名空间已植入 RealmID 为未来多 Realm 留好了扩展点。

---

## 后续行动

| 行动 | 负责人 | 状态 |
|------|--------|------|
| 更新架构文档 | @qinglong | ✅ |
| 更新 API 接口定义 | @qinglong | 📋 |
| 更新 quickstart 文档 | @qinglong | 📋 |
| 实现 Realm 接口 | @dev | 📋 |
| 实现各服务接口 | @dev | 📋 |

---

## 关联文档

- → [架构总览](../architecture/overview.md)
- → [三层架构详解](../architecture/layers.md)
- → [讨论记录：分层中继设计](./DISC-1227-relay-isolation.md)
