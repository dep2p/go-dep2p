# Realm 加入流程 (Realm Flow)

> Realm 认证、加入、成员管理的端到端流程

---

## 文档定位

本文档是 L3_behavioral 的**纵向流程文档**，专注于描述 Realm 加入与管理的完整行为。

### 与横切面的关系

Realm 加入是节点生命周期的 Phase B 阶段，详见 [lifecycle_overview.md](lifecycle_overview.md)：

| 生命周期阶段 | Realm 相关内容 | 本文档章节 |
|-------------|---------------|-----------|
| Phase B: Realm 加入 | PSK 认证与 RealmID 派生 | [PSK 派生流程](#psk-派生) |
| Phase B: Realm 加入 | DHT 发布与成员发现 | [完整加入流程](#完整加入流程) |
| Phase B: Realm 加入 | 成员同步（Gossip） | [成员同步流程](#成员同步流程) |

---

## 流程概述

> 详见 [DHT-Realm 架构重构方案](../../_discussions/20260126-dht-realm-architecture-redesign.md)

Realm 是 DeP2P 的业务隔离域，加入 Realm 需要 PSK 认证。

```mermaid
flowchart LR
    Start["JoinRealm(psk)"] --> Derive["PSK 派生 RealmID"]
    Derive --> Publish["发布 DHT Provider + PeerRecord"]
    Publish --> Return["返回 Realm 对象"]
    Return --> Loop["后台发现循环"]
    Loop --> Auth["成员认证"]
    Auth --> MemberList["更新 MemberList"]
    MemberList --> Loop
```

### 参与组件

| 组件 | 目录 | 职责 |
|------|------|------|
| **RealmManager** | `internal/realm/manager/` | Realm 生命周期管理 |
| **RealmAuth** | `internal/realm/auth/` | 成员认证（PSK 双向认证） |
| **PSK** | `internal/realm/auth/` | 密钥派生 |
| **DHT** | `internal/discovery/dht/` | Provider + PeerRecord 发布与查询 |
| **MemberSyncTopic** | `internal/realm/` | 成员列表 Gossip 同步 |
| **Peerstore** | `internal/core/peerstore/` | 地址与成员信息缓存 |

---

## 完整加入流程

采用"先发布后发现"模式：

```mermaid
sequenceDiagram
    participant App as 应用层
    participant RM as RealmManager
    participant PSK as PSK
    participant DHT as DHT
    participant Peer as 其他成员
    
    App->>RM: JoinRealm(psk)
    
    Note over RM,PSK: Step 1: PSK 派生
    RM->>PSK: DeriveRealmID(psk)
    PSK->>PSK: HKDF 派生
    PSK-->>RM: RealmID, RealmKey
    
    Note over RM,DHT: Step 2: 发布 Provider（声明成员身份）
    RM->>DHT: Provide("/dep2p/v2/realm/<RealmID>/members", selfID)
    DHT-->>RM: Provider 发布确认
    
    Note over RM,DHT: Step 3: 发布 PeerRecord（发布地址）
    RM->>RM: 构建 SignedRealmPeerRecord
    RM->>DHT: Put("/dep2p/v2/realm/<RealmID>/peer/<selfID>", record)
    DHT-->>RM: PeerRecord 发布确认
    
    Note over RM,App: Step 4: 返回 Realm 对象（非阻塞）
    RM->>RM: 初始化 MemberList
    RM-->>App: Realm 实例（可能暂无其他成员）
    
    Note over RM,Peer: Step 5: 后台发现循环（goroutine）
    loop 指数退避（2s → 60s）
        RM->>DHT: FindProviders(realmMembersKey)
        DHT-->>RM: providers[]
        loop 对每个新发现的 provider
            RM->>DHT: Get(provider.PeerRecordKey)
            DHT-->>RM: SignedRealmPeerRecord
            RM->>Peer: Connect(addrs)
            RM->>Peer: PSK 双向认证
            alt 认证成功
                RM->>RM: MemberList.Add(provider)
            else 认证失败
                RM->>RM: 忽略该节点
            end
        end
    end
```

---

## 阶段详解

### 阶段 1: PSK 派生

```
PSK 派生流程：

  输入: PSK (用户提供的预共享密钥)
  输出: RealmID, RealmKey

  步骤:
    1. RealmID = HKDF(
         salt: "dep2p-realm-id-v1",
         ikm: PSK,
         info: SHA256(PSK),
         length: 32
       )
       
    2. RealmKey = HKDF(
         salt: RealmID,
         ikm: PSK,
         info: "dep2p realm key",
         length: 32
       )
```

```mermaid
flowchart TB
    PSK["PSK (预共享密钥)"]
    
    PSK --> HKDF1["HKDF 派生 RealmID"]
    HKDF1 --> RealmID["RealmID (32 bytes)"]
    
    PSK --> HKDF2["HKDF 派生 RealmKey"]
    RealmID --> HKDF2
    HKDF2 --> RealmKey["RealmKey (32 bytes)"]
```

### 阶段 2: DHT 发布

直接发布到 DHT（先发布后发现）：

```mermaid
sequenceDiagram
    participant Node as 本节点
    participant DHT as DHT
    participant Cache as MemberCache
    participant PS as Peerstore

    Note over Node,DHT: 发布 Provider（声明成员身份）
    Node->>DHT: Provide("/dep2p/v2/realm/<RealmID>/members", selfID)
    DHT-->>Node: 发布确认
    
    Note over Node,DHT: 发布 PeerRecord（发布地址）
    Node->>Node: 构建 SignedRealmPeerRecord
    Node->>DHT: Put("/dep2p/v2/realm/<RealmID>/peer/<selfID>", record)
    DHT-->>Node: 发布确认
    
    Node->>Cache: 初始化 MemberList
    Node->>PS: 写入本地地址
```

### 阶段 3: 成员发现（后台循环）

> 发现循环在后台持续运行，使用指数退避策略。

```mermaid
sequenceDiagram
    participant Node as 本节点
    participant DHT as DHT
    participant Cache as MemberCache
    
    loop 指数退避（2s → 60s）
        Node->>DHT: FindProviders("/dep2p/v2/realm/<RealmID>/members")
        DHT-->>Node: providers[]
        
        loop 对每个新 provider
            Node->>DHT: Get("/dep2p/v2/realm/<RealmID>/peer/<providerID>")
            DHT-->>Node: SignedRealmPeerRecord
            Node->>Cache: 缓存 provider 地址
        end
        
        alt 发现新成员
            Node->>Node: 重置退避间隔为 2s
        else 无新成员
            Node->>Node: 间隔翻倍（最大 60s）
        end
    end
```

### 阶段 4: 成员认证（PSK 双向认证）

> **角色协商**：NodeID 字节序大的做发起方（Initiator）

```mermaid
sequenceDiagram
    participant I as Initiator
    participant R as Responder
    
    Note over I,R: 角色协商：NodeID 大的做 Initiator
    
    I->>R: AuthChallenge(nonceI)
    R->>R: 计算 HMAC(psk, nonceI)
    R-->>I: AuthResponse(hmac)
    R->>I: AuthChallenge(nonceR)
    I->>I: 验证 R 的 hmac
    I->>I: 计算 HMAC(psk, nonceR)
    I-->>R: AuthResponse(hmac)
    R->>R: 验证 I 的 hmac
    R-->>I: AuthResult(ok/fail)
    
    alt 双方验证通过
        I->>I: MemberList.Add(R)
        R->>R: MemberList.Add(I)
    else 任一方验证失败
        I->>I: 忽略该节点
        R->>R: 忽略该节点
    end
```

> **注**：连接层加密由底层（libp2p TLS/Noise）处理，此处只做身份确认。

---

## Realm 状态机

```mermaid
stateDiagram-v2
    [*] --> NotJoined
    
    NotJoined --> Deriving: JoinRealm(psk)
    Deriving --> Publishing: PSK 有效
    Deriving --> NotJoined: PSK 无效
    
    Publishing --> Joined: DHT 发布成功
    Publishing --> NotJoined: DHT 发布失败
    
    Joined --> Discovering: 后台发现循环
    Discovering --> Authenticating: 发现新成员
    Authenticating --> Joined: 认证完成/失败
    
    Joined --> Leaving: Leave()
    Joined --> Switching: JoinRealm(其他 PSK)
    
    Switching --> Deriving: 离开当前 Realm
    
    Leaving --> NotJoined: 离开完成
```

### 状态说明

| 状态 | 说明 |
|------|------|
| **NotJoined** | 未加入任何 Realm |
| **Deriving** | 正在派生 RealmID/RealmKey |
| **Publishing** | 正在发布 DHT Provider + PeerRecord |
| **Joined** | 已加入 Realm（可能暂无其他成员） |
| **Discovering** | 后台发现循环运行中 |
| **Authenticating** | 正在对新发现的成员进行 PSK 双向认证 |
| **Leaving** | 正在离开 Realm |
| **Switching** | 正在切换 Realm |

---

## 成员同步流程

加入 Realm 后，需要持续同步成员列表，并同步成员地址（V2 消息包含地址）：

```mermaid
sequenceDiagram
    participant Node as 本节点
    participant RV as Rendezvous
    participant Topic as MemberSyncTopic
    participant Cache as MemberCache
    participant PS as Peerstore
    
    Note over Node,Cache: 后台同步任务
    
    loop 每 30 秒
        Node->>RV: DISCOVER(realmID)
        RV-->>Node: peers[]
        
        Node->>Node: 对比本地成员列表
        
        alt 有新成员
            Node->>Node: 认证新成员
            Node->>Cache: 添加成员
            Node->>Topic: 广播 join2:<json>（含地址）
        end
        
        alt 有离线成员
            Node->>Cache: 标记离线
        end
    end

    Note over Node,Topic: 成员同步 topic 监听（join2/sync2）
    Topic-->>Node: join2 / sync2 消息
    Node->>Cache: 更新成员信息
    Node->>PS: 写入成员地址（缓存）
```

---

## 离开 Realm 流程

```mermaid
sequenceDiagram
    participant App as 应用层
    participant RM as RealmManager
    participant RV as Rendezvous
    participant Peers as 其他成员
    
    App->>RM: Leave()
    
    RM->>RV: UNREGISTER(realmID)
    RV-->>RM: OK
    
    RM->>Peers: LEAVE_NOTIFY(realmID, nodeID)
    
    RM->>RM: 清理 Realm 状态
    RM->>RM: 停止后台任务
    
    RM-->>App: 离开完成
```

---

## 成员离线检测（v1.1 新增）

> 详见 [disconnect_detection.md](disconnect_detection.md) 完整断开检测流程

### 离线检测与成员状态同步

```
核心原则: 连接即成员（INV-003）

  成员在线状态 = 有活跃连接 ∧ 通过认证
  
  检测到断开 → 更新 MemberList → 广播状态变更
```

### 成员离线处理流程

```mermaid
sequenceDiagram
    participant Swarm as Swarm
    participant MM as MemberManager
    participant Witness as WitnessService
    participant Peers as 其他成员
    
    Note over Swarm: 检测到 PeerA 断开
    Swarm->>MM: EvtPeerDisconnected(PeerA)
    
    alt 优雅断开（收到 MemberLeave）
        MM->>MM: Remove(PeerA)
        MM->>Peers: 广播成员状态变更
    else 非优雅断开
        MM->>MM: 启动宽限期定时器 (15s)
        MM->>MM: 标记 PeerA 为 DISCONNECTING
        
        alt 宽限期内重连
            Swarm->>MM: EvtPeerConnected(PeerA)
            MM->>MM: 取消定时器
            MM->>MM: 恢复 PeerA 为 CONNECTED
        else 宽限期超时
            MM->>MM: Remove(PeerA)
            MM->>Witness: 触发见证报告（可选）
            MM->>Peers: 广播成员状态变更
        end
    end
```

### 断开保护期

```
目的: 防止竞态条件导致刚断开的成员被重新添加

机制:
  1. Remove() 时记录 peerID 到 recentlyDisconnected
  2. Add() 时检查保护期（默认 30s）
  3. 保护期内的 Add() 被静默忽略
```

### 震荡处理

```
目的: 避免网络抖动导致的成员状态频繁变更

震荡判定: 60s 内 >= 3 次断开/重连

处理策略:
  - 震荡中的成员不触发见证报告
  - 状态变更延迟广播（5s debounce）
  - 成员列表标记为"不稳定"
```

---

## 错误处理

### 错误类型

| 错误 | 原因 | 处理 |
|------|------|------|
| **ErrInvalidPSK** | PSK 格式无效 | 返回错误 |
| **ErrRendezvousFailed** | Rendezvous 注册失败 | 重试或返回错误 |
| **ErrAuthFailed** | 成员认证失败 | 忽略该节点 |
| **ErrAlreadyJoined** | 已加入该 Realm | 返回当前 Realm |
| **ErrNotMember** | 未加入 Realm 调用业务 API | 返回错误 |

### 重试策略

```
Rendezvous 重试:
  - 指数退避
  - 初始间隔: 1 秒
  - 最大间隔: 30 秒
  - 最大重试: 5 次

成员认证重试:
  - 单个成员失败不影响整体
  - 后台定期重试
```

---

## 代码路径

| 阶段 | 代码路径 |
|------|----------|
| Realm 管理 | `internal/realm/manager/` |
| PSK 派生 | `internal/realm/auth/` |
| 成员认证（PSK 双向） | `internal/realm/auth/` |
| 成员管理 | `internal/realm/member/` |
| 发现循环 | `internal/realm/discovery.go` |
| 成员同步 | `internal/realm/realm.go` |
| DHT 发布 | `internal/discovery/dht/` |

---

## 相关文档

### L3 行为文档

| 文档 | 说明 |
|------|------|
| [lifecycle_overview.md](lifecycle_overview.md) | ★ 节点生命周期横切面（Phase B: Realm 加入） |
| [disconnect_detection.md](disconnect_detection.md) | ★ 断开检测流程（成员离线检测） |
| [discovery_flow.md](discovery_flow.md) | 节点发现流程 |
| [messaging_flow.md](messaging_flow.md) | 消息传递流程 |
| [relay_flow.md](relay_flow.md) | 中继流程 |
| [state_machines.md](state_machines.md) | 状态机定义 |

### 约束与 ADR

| 文档 | 说明 |
|------|------|
| [INV-002](../../01_context/decisions/invariants/INV-002-realm-membership.md) | Realm 成员资格 |
| [INV-003](../../01_context/decisions/invariants/INV-003-connection-membership.md) | 连接即成员不变量 |
| [ADR-0010](../../01_context/decisions/ADR-0010-relay-explicit-config.md) | Relay 明确配置 |
| [ADR-0012](../../01_context/decisions/ADR-0012-disconnect-detection.md) | 断开检测架构 |
| [Realm 协议规范](../../02_constraints/protocol/L4_application/realm.md) | 协议详情 |
| [存活检测协议](../../02_constraints/protocol/L4_application/liveness.md) | MemberLeave/Witness 协议 |

---

**最后更新**：2026-01-28（新增成员离线检测章节）
