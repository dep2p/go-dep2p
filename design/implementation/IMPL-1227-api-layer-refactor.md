# IMPL-1227: API 分层与 Realm 隔离重构实施跟踪

**日期**：2024-12-28  
**来源**：[DISC-1227-api-layer-design](../discussions/DISC-1227-api-layer-design.md) + [DISC-1227-relay-isolation](../discussions/DISC-1227-relay-isolation.md)  
**状态**：✅ 已完成（Phase 1-7 全部完成）  
**性质**：破坏性重构（不向后兼容）

---

## 目标概述

| 核心变更 | 当前状态 | 目标状态 |
|---------|---------|---------|
| API 模型 | `node.Send()` 上帝对象 | `node.JoinRealm() → Realm → Services` |
| 成员认证 | `JoinKey` 无验证闭环 | PSK 成员证明（MAC 验证） |
| RealmID | `H(creatorPubKey \|\| name)` | `H(realmKey)` 不可枚举 |
| 协议命名 | `/dep2p/app/chat/1.0.0` 全局 | `/dep2p/app/<realmID>/chat/1.0.0` |
| 中继验证 | 无 Realm 验证 | PSK + 协议白名单 |

---

## 一、设计文档更新

### 1.1 必须更新

| 文档路径 | 变更类型 | 具体变更 |
|---------|---------|---------|
| `design/architecture/layers.md` | 重写 | 服务对象模型替代扁平 API |
| `design/architecture/overview.md` | 重写 | 协议命名空间隔离架构图 |
| `design/protocols/application/realm.md` | 重写 | PSK 成员证明协议规范 |
| `design/protocols/transport/relay.md` | 重写 | 分层中继规范（System vs Realm） |
| `design/invariants/INV-002-realm-membership.md` | 重写 | PSK 验证不变量 |
| `design/adr/0002-realm-isolation.md` | 补充 | PSK 决策记录 |
| `design/adr/0003-relay-first-connect.md` | 重写 | Realm Relay 验证流程 |

### 1.2 新增文档

| 文档路径 | 内容 |
|---------|------|
| `design/protocols/foundation/protocol-namespace.md` | 协议命名空间规范 |
| `design/adr/0004-psk-membership.md` | PSK 成员认证 ADR |

---

## 二、用户文档更新

### 2.1 API 参考（全部重写）

| 文档 | 变更 |
|------|------|
| `docs/{zh,en}/reference/api/node.md` | `JoinRealm` 返回 `Realm` 对象 |
| `docs/{zh,en}/reference/api/realm.md` | 新增服务 getter API |
| `docs/{zh,en}/reference/api/messaging.md` | 从 Realm 获取，协议自动添加前缀 |
| `docs/{zh,en}/reference/protocol-ids.md` | 协议命名空间规范 |

### 2.2 教程（全部重写）

| 文档 | 变更 |
|------|------|
| `docs/{zh,en}/getting-started/quickstart.md` | 新 API 示例 |
| `docs/{zh,en}/getting-started/first-realm.md` | `realmKey` 入会 |
| `docs/{zh,en}/tutorials/*` | 全部按新 API 重写 |

---

## 三、代码重构详情

### 3.1 类型定义变更

#### `pkg/types/ids.go`

```diff
// 当前 RealmID 定义
type RealmID string

+ // 新增：RealmKey 类型
+ type RealmKey [32]byte
+
+ // GenerateRealmKey 生成高熵 Realm 密钥
+ func GenerateRealmKey() RealmKey {
+     var key RealmKey
+     if _, err := rand.Read(key[:]); err != nil {
+         panic(err)
+     }
+     return key
+ }
```

#### `pkg/types/realm.go`（重写）

```diff
- // GenerateRealmID 根据创建者公钥和名称生成 RealmID
- func GenerateRealmID(creatorPubKey []byte, realmName string) RealmID {
-     h := sha256.New()
-     h.Write(creatorPubKey)
-     h.Write([]byte(realmName))
-     ...
- }

+ // DeriveRealmID 从 realmKey 派生 RealmID
+ // 公式: RealmID = SHA256("dep2p-realm-id-v1" || H(realmKey))
+ // 返回：完整 SHA256 哈希的十六进制字符串（64字符）
+ func DeriveRealmID(realmKey RealmKey) RealmID {
+     keyHash := sha256.Sum256(realmKey[:])
+     h := sha256.New()
+     h.Write([]byte("dep2p-realm-id-v1"))
+     h.Write(keyHash[:])
+     hash := h.Sum(nil)
+     return RealmID(hex.EncodeToString(hash))  // 完整32字节 = 64字符hex
+ }
```

**说明**：RealmID 派生规则彻底变更，所有旧 RealmID 值将被废弃。开发阶段无真实数据，直接丢弃即可。

---

### 3.2 接口层变更

#### `pkg/interfaces/realm/realm.go`（重大重构）

**删除**：
- `RealmManager.JoinRealm(ctx, realmID, opts) error` - 返回值变更

**新增**：

```go
// ============================================================================
// Realm 接口（Layer 2 产物）- 新增
// ============================================================================

type Realm interface {
    // 基本信息
    Name() string
    ID() RealmID
    Key() RealmKey  // 返回 realmKey（用于成员证明）
    
    // 成员管理
    Members() []NodeID
    MemberCount() int
    IsMember(peer NodeID) bool
    
    // Layer 3 服务入口（核心变更！）
    Messaging() Messaging
    PubSub() PubSub
    Discovery() RealmDiscovery
    Streams() StreamManager
    Relay() RealmRelayService
    
    // 生命周期
    Leave() error
    Context() context.Context
}

// ============================================================================
// RealmManager 接口变更
// ============================================================================

type RealmManager interface {
    // 变更：返回 Realm 对象而非 error
    // 方式1：使用 Option（推荐，统一风格）
    JoinRealm(ctx context.Context, name string, opts ...RealmOption) (Realm, error)
    
    // 方式2：显式传 key（便捷方法，内部转换为 Option）
    JoinRealmWithKey(ctx context.Context, name string, realmKey RealmKey, opts ...RealmOption) (Realm, error)
    
    // 保留
    LeaveRealm() error
    CurrentRealm() Realm  // 返回类型变更
    IsMember() bool
    
    // 删除
    // - IsMemberOf(realmID) - 单 Realm 无需
    // - RealmPeers(realmID) - 移至 Realm 接口
    // - RealmMetadata(realmID) - 移至 Realm 接口
}

// RealmOption Realm 加入选项
type RealmOption func(*RealmOptions)

// RealmOptions Realm 加入选项结构
type RealmOptions struct {
    RealmKey RealmKey  // 必须提供（通过 WithRealmKey 设置）
}

// WithRealmKey 设置 Realm 密钥（必须）
func WithRealmKey(key RealmKey) RealmOption {
    return func(opts *RealmOptions) {
        opts.RealmKey = key
    }
}
```

**说明**：`JoinRealm` 签名彻底变更，所有调用方代码需要按新 API 重写。

#### `pkg/interfaces/realm/services.go`（新文件）

```go
// ============================================================================
// Layer 3 服务接口
// ============================================================================

// Messaging 消息服务（从 Realm 获取）
type Messaging interface {
    // Send 发送消息（使用默认协议，自动添加 Realm 前缀）
    Send(ctx context.Context, to NodeID, data []byte) error
    
    // SendWithProtocol 发送消息（指定协议，框架自动添加 Realm 前缀）
    // 用户只需写 "chat/1.0.0"，框架自动转换为 "/dep2p/app/<realmID>/chat/1.0.0"
    SendWithProtocol(ctx context.Context, to NodeID, protocol string, data []byte) error
    
    Request(ctx context.Context, to NodeID, data []byte) ([]byte, error)
    RequestWithProtocol(ctx context.Context, to NodeID, protocol string, data []byte) ([]byte, error)
    OnMessage(handler MessageHandler)
    OnRequest(handler RequestHandler)
    OnProtocol(protocol string, handler ProtocolHandler)
}

// PubSub 发布订阅服务
type PubSub interface {
    Join(ctx context.Context, topic string) (Topic, error)
    Topics() []Topic
}

// Topic 主题对象
type Topic interface {
    Name() string
    Publish(ctx context.Context, data []byte) error
    Subscribe() (Subscription, error)
    Peers() []NodeID
    Leave() error
}

// Subscription 订阅句柄
type Subscription interface {
    Messages() <-chan *PubSubMessage
    Cancel()
}

// RealmDiscovery Realm 内发现
type RealmDiscovery interface {
    FindPeers(ctx context.Context, opts ...FindOption) ([]NodeID, error)
    FindPeersWithService(ctx context.Context, service string) ([]NodeID, error)
    Advertise(ctx context.Context, service string) error
    Watch(ctx context.Context) (<-chan MemberEvent, error)
}

// StreamManager 流管理
type StreamManager interface {
    // Open 打开流（协议自动添加 Realm 前缀）
    // 用户只需写 "file-transfer/1.0.0"，框架自动转换为 "/dep2p/app/<realmID>/file-transfer/1.0.0"
    Open(ctx context.Context, to NodeID, protocol string) (Stream, error)
    
    // SetHandler 注册协议处理器（协议自动添加 Realm 前缀）
    SetHandler(protocol string, handler StreamHandler)
    RemoveHandler(protocol string)
}

// RealmRelayService Realm 中继服务
type RealmRelayService interface {
    Serve(ctx context.Context, opts ...RelayOption) error
    StopServing() error
    IsServing() bool
    FindRelays(ctx context.Context) ([]NodeID, error)
    Reserve(ctx context.Context, relay NodeID) (Reservation, error)
    Stats() RelayStats  // 获取中继统计（设计文档要求）
}
```

#### `pkg/interfaces/realm/psk.go`（新文件）

```go
// ============================================================================
// PSK 成员证明
// ============================================================================

// MembershipProof PSK 成员证明
type MembershipProof struct {
    NodeID    NodeID    // 证明发起者（自己的 NodeID）
    RealmID   RealmID   // 所属 Realm 的 ID
    PeerID    NodeID    // 目标节点（通信对方的 NodeID）—— 绑定证明到特定目标
    Nonce     [16]byte  // 随机数（防重放）
    Timestamp int64     // 时间戳（限制有效期）
    MAC       [32]byte  // HMAC-SHA256 签名
}

// ProofGenerator 成员证明生成器
type ProofGenerator interface {
    Generate(ctx context.Context, peerID NodeID) (*MembershipProof, error)
}

// ProofVerifier 成员证明验证器
type ProofVerifier interface {
    // Verify 验证成员证明
    // expectedPeerID: 预期的目标节点（验证 proof.PeerID 是否匹配）
    // - 中继场景：R 验证时，expectedPeerID = 请求中的 targetNodeID
    // - 直连场景：B 验证时，expectedPeerID = 自己的 NodeID
    Verify(proof *MembershipProof, expectedPeerID NodeID) error
}
```

---

### 3.3 协议命名空间

#### `pkg/protocolids/sys.go`（修改）

```diff
+ // ============================================================================
+ // 协议前缀模板
+ // ============================================================================
+
+ // RealmProtocolPrefix Realm 协议前缀模板
+ const RealmProtocolPrefix = "/dep2p/realm/%s/"
+
+ // AppProtocolPrefix 应用协议前缀模板
+ const AppProtocolPrefix = "/dep2p/app/%s/"

+ // ============================================================================
+ // 运行时协议生成
+ // ============================================================================
+
+ // FullAppProtocol 生成完整应用协议 ID
+ func FullAppProtocol(realmID types.RealmID, userProto string) types.ProtocolID {
+     return types.ProtocolID(fmt.Sprintf("/dep2p/app/%s/%s", realmID, userProto))
+ }
+
+ // FullRealmProtocol 生成完整 Realm 协议 ID
+ func FullRealmProtocol(realmID types.RealmID, subProto string) types.ProtocolID {
+     return types.ProtocolID(fmt.Sprintf("/dep2p/realm/%s/%s", realmID, subProto))
+ }
+
+ // ValidateUserProtocol 验证用户协议（不能以 /dep2p/sys/ 开头，不能包含其他 RealmID）
+ func ValidateUserProtocol(proto string, currentRealmID RealmID) error {
+     // 检查系统协议前缀
+     if strings.HasPrefix(proto, SysPrefix) {
+         return ErrReservedProtocol
+     }
+     // 检查 Realm 协议前缀（用户不能直接指定）
+     if strings.HasPrefix(proto, "/dep2p/realm/") {
+         return ErrReservedProtocol
+     }
+     // 检查应用协议前缀（用户不能直接指定完整路径）
+     if strings.HasPrefix(proto, "/dep2p/app/") {
+         // 进一步检查：如果包含其他 RealmID，拒绝
+         if extractedRealmID, err := ExtractRealmID(types.ProtocolID(proto)); err == nil {
+             if extractedRealmID != currentRealmID {
+                 return ErrCrossRealmProtocol  // 跨 Realm 协议攻击
+             }
+         }
+         return ErrReservedProtocol  // 用户不能直接指定完整路径
+     }
+     return nil
+ }
+
+ // ExtractRealmID 从协议 ID 提取 RealmID
+ func ExtractRealmID(proto types.ProtocolID) (types.RealmID, error) {
+     s := string(proto)
+     if strings.HasPrefix(s, "/dep2p/app/") {
+         parts := strings.SplitN(s[len("/dep2p/app/"):], "/", 2)
+         if len(parts) >= 1 {
+             return types.RealmID(parts[0]), nil
+         }
+     }
+     if strings.HasPrefix(s, "/dep2p/realm/") {
+         parts := strings.SplitN(s[len("/dep2p/realm/"):], "/", 2)
+         if len(parts) >= 1 {
+             return types.RealmID(parts[0]), nil
+         }
+     }
+     return "", ErrNoRealmInProtocol
+ }
```

**说明**：协议 ID 格式彻底变更，所有现有协议注册代码需要按新格式重写。

---

### 3.4 核心实现变更

#### `internal/core/realm/manager.go`（重大重构）

**关键变更点**：

| 方法 | 变更 |
|------|------|
| `JoinRealm` | 返回 `Realm` 对象，接收 `realmKey` |
| `CurrentRealm` | 返回 `Realm` 对象而非 `RealmID` |
| `realmState` | 存储 `realmKey`，用于 PSK 验证 |

```diff
type realmState struct {
    metadata     *types.RealmMetadata
+   realmKey     types.RealmKey  // 新增：存储密钥
    peers        map[types.NodeID]*peerInfo
    lastAnnounce time.Time
}

- func (m *Manager) JoinRealm(ctx context.Context, realmID types.RealmID, opts ...realmif.JoinOption) error {
+ func (m *Manager) JoinRealm(ctx context.Context, name string, opts ...realmif.RealmOption) (realmif.Realm, error) {
+     // 解析选项
+     realmOpts := &realmif.RealmOptions{}
+     for _, opt := range opts {
+         opt(realmOpts)
+     }
+     
+     // 必须提供 realmKey
+     if realmOpts.RealmKey == (types.RealmKey{}) {
+         return nil, ErrRealmKeyRequired
+     }
+     
+     // 从 realmKey 派生 RealmID
+     realmID := types.DeriveRealmID(realmOpts.RealmKey)
      ...
+     // 创建 Realm 对象
+     realm := &realmImpl{
+         manager:  m,
+         name:     name,
+         id:       realmID,
+         key:      realmOpts.RealmKey,
+         state:    state,
+     }
+     return realm, nil
+ }
+
+ // JoinRealmWithKey 便捷方法（内部转换为 Option）
+ func (m *Manager) JoinRealmWithKey(ctx context.Context, name string, realmKey types.RealmKey, opts ...realmif.RealmOption) (realmif.Realm, error) {
+     return m.JoinRealm(ctx, name, append(opts, realmif.WithRealmKey(realmKey))...)
+ }
```

#### `internal/core/realm/realm_impl.go`（新文件）

```go
// realmImpl Realm 接口实现
type realmImpl struct {
    manager  *Manager
    name     string
    id       types.RealmID
    key      types.RealmKey
    state    *realmState
    
    // 服务实例（懒加载）
    messaging   realmif.Messaging
    pubsub      realmif.PubSub
    discovery   realmif.RealmDiscovery
    streams     realmif.StreamManager
    relay       realmif.RealmRelayService
    
    mu sync.RWMutex
}

func (r *realmImpl) Name() string { return r.name }
func (r *realmImpl) ID() types.RealmID { return r.id }
func (r *realmImpl) Key() types.RealmKey { return r.key }

func (r *realmImpl) Messaging() realmif.Messaging {
    r.mu.Lock()
    defer r.mu.Unlock()
    if r.messaging == nil {
        r.messaging = newRealmMessaging(r)
    }
    return r.messaging
}

// ... 其他服务 getter
```

#### `internal/core/realm/psk.go`（新文件）

```go
// PSKVerifier PSK 成员验证器
type PSKVerifier struct {
    realmKey types.RealmKey
    realmID  types.RealmID
}

// GenerateProof 生成成员证明
// nodeID: 自己的 NodeID（证明发起者）
// peerID: 目标节点的 NodeID（通信对方，绑定证明到特定目标）
func (v *PSKVerifier) GenerateProof(nodeID types.NodeID, peerID types.NodeID) (*realmif.MembershipProof, error) {
    proof := &realmif.MembershipProof{
        NodeID:    nodeID,
        RealmID:   v.realmID,
        PeerID:    peerID,  // 目标节点
        Timestamp: time.Now().Unix(),
    }
    
    // 生成随机 nonce
    if _, err := rand.Read(proof.Nonce[:]); err != nil {
        return nil, err
    }
    
    // 计算 MAC
    // MAC = HMAC-SHA256(HKDF(realmKey, "dep2p-realm-membership-v1"), nodeID || realmID || peerID || nonce || timestamp)
    key := v.deriveKey()
    h := hmac.New(sha256.New, key)
    h.Write(nodeID.Bytes())
    h.Write([]byte(v.realmID))
    h.Write(peerID.Bytes())  // peerID 是目标节点，绑定证明
    h.Write(proof.Nonce[:])
    binary.Write(h, binary.BigEndian, proof.Timestamp)
    copy(proof.MAC[:], h.Sum(nil))
    
    return proof, nil
}

// VerifyProof 验证成员证明
// expectedPeerID: 预期的目标节点
// - 中继场景：R 验证时，expectedPeerID = 请求中的 targetNodeID
// - 直连场景：B 验证时，expectedPeerID = 自己的 NodeID
func (v *PSKVerifier) VerifyProof(proof *realmif.MembershipProof, expectedPeerID types.NodeID) error {
    // 1. 检查时间戳（5分钟窗口）
    now := time.Now().Unix()
    if abs(now - proof.Timestamp) > 300 {
        return ErrProofExpired
    }
    
    // 2. 检查 peerID 匹配（证明是否绑定到预期目标）
    if proof.PeerID != expectedPeerID {
        return ErrPeerIDMismatch  // 证明的目标与预期不符
    }
    
    // 3. 重新计算 MAC 并比较
    key := v.deriveKey()
    h := hmac.New(sha256.New, key)
    h.Write(proof.NodeID.Bytes())
    h.Write([]byte(proof.RealmID))
    h.Write(proof.PeerID.Bytes())  // 使用 proof 中的 peerID
    h.Write(proof.Nonce[:])
    binary.Write(h, binary.BigEndian, proof.Timestamp)
    
    expectedMAC := h.Sum(nil)
    if !hmac.Equal(expectedMAC, proof.MAC[:]) {
        return ErrInvalidProof
    }
    
    return nil
}

func (v *PSKVerifier) deriveKey() []byte {
    return hkdf.Extract(sha256.New, v.realmKey[:], []byte("dep2p-realm-membership-v1"))
}
```

---

### 3.5 Messaging 适配

#### `internal/core/messaging/service.go`（重大重构）

```diff
type service struct {
    ...
+   currentRealm realmif.Realm  // 新增：当前 Realm 引用
}

// Send 发送消息（使用默认协议，自动添加 Realm 前缀）
- func (s *service) Send(ctx context.Context, nodeID types.NodeID, protocol types.ProtocolID, data []byte) error {
+ func (s *service) Send(ctx context.Context, nodeID types.NodeID, data []byte) error {
+     if s.currentRealm == nil {
+         return ErrNotInRealm
+     }
+     // 使用默认消息协议，自动添加 Realm 前缀
+     fullProto := protocolids.FullAppProtocol(s.currentRealm.ID(), "messaging/1.0.0")
+     return s.SendWithProtocol(ctx, nodeID, "messaging/1.0.0", data)
+ }
+
+ // SendWithProtocol 发送消息（指定协议，框架自动添加 Realm 前缀）
+ func (s *service) SendWithProtocol(ctx context.Context, nodeID types.NodeID, protocol string, data []byte) error {
+     if s.currentRealm == nil {
+         return ErrNotInRealm
+     }
+     // 验证用户协议（不能是系统协议，不能包含其他 RealmID）
+     if err := protocolids.ValidateUserProtocol(protocol, s.currentRealm.ID()); err != nil {
+         return err
+     }
+     // 自动添加 Realm 前缀
+     fullProto := protocolids.FullAppProtocol(s.currentRealm.ID(), protocol)
      ...
  }

// Publish 发布到 topic（topic 前缀自动添加）
  func (s *service) Publish(ctx context.Context, topic string, data []byte) error {
+     if s.currentRealm == nil {
+         return ErrNotInRealm
+     }
+     // topic 自动添加 Realm 前缀
+     // 用户写 "blocks"，实际 topic: "/dep2p/app/<realmID>/blocks"
+     // 不需要 /pubsub/ 前缀，通过服务类型区分
+     fullTopic := fmt.Sprintf("/dep2p/app/%s/%s", s.currentRealm.ID(), topic)
      ...
  }
```

**说明**：`Send/Publish` 签名彻底变更，所有调用方代码需要按新 API 重写。

---

### 3.6 PubSub 适配

#### `internal/core/messaging/pubsub.go`

```diff
// Subscribe 订阅主题
- func (s *service) Subscribe(ctx context.Context, topic string) (Subscription, error) {
+ func (s *service) Subscribe(ctx context.Context, topic string) (messagingif.Subscription, error) {
+     if s.currentRealm == nil {
+         return nil, ErrNotInRealm
+     }
+     // topic 自动添加 Realm 前缀
+     // 用户写 "blocks"，实际 topic: "/dep2p/app/<realmID>/blocks"
+     // 不需要 /pubsub/ 前缀，通过服务类型区分（PubSub vs Streams）
+     fullTopic := fmt.Sprintf("/dep2p/app/%s/%s", s.currentRealm.ID(), topic)
      ...
  }
```

---

### 3.7 Relay 适配

#### System Relay vs Realm Relay 区分

**重要**：需要明确区分两种中继，实施文档中需要补充 System Relay 的实现说明。

| 中继类型 | 实现位置 | 协议限制 | 验证方式 |
|---------|---------|---------|---------|
| System Relay | `internal/core/relay/server/system_relay.go`（新文件） | 只允许 `/dep2p/sys/*` | 无成员验证 |
| Realm Relay | `internal/core/relay/server/realm_relay.go`（新文件） | 只允许 `/dep2p/app/<realmID>/*` 和 `/dep2p/realm/<realmID>/*` | PSK 成员证明 |

#### `internal/core/relay/server/server.go`（重大重构）

```diff
type Server struct {
    ...
+   // Realm 验证（新增）
+   realmVerifier *realm.PSKVerifier
+   realmID       types.RealmID
}

// handleConnect 处理连接请求
func (s *Server) handleConnect(stream endpoint.Stream) {
    ...
+   // 新增：PSK 成员验证（R 验证 A）
+   if s.realmVerifier != nil {
+       proof, err := readMembershipProof(stream)
+       if err != nil {
+           writeError(stream, ErrInvalidProof)
+           return
+       }
+       // R 验证 A 的 PSK 证明
+       if err := s.realmVerifier.VerifyProof(proof, req.SourceNodeID); err != nil {
+           writeError(stream, ErrNotMember)
+           return
+       }
+   }
+   
+   // 新增：协议白名单验证
+   if !s.isProtocolAllowed(req.Protocol) {
+       writeError(stream, ErrProtocolNotAllowed)
+       return
+   }
+   
+   // 转发连接请求到 B
+   ...
+   
+   // 新增：B 验证 A 的证明（双重验证，防止 Relay 作恶）
+   // B 在收到连接请求时，会验证 A 的 PSK 证明
+   // 这需要在 B 端的连接处理逻辑中实现
+   ...
}

+ // isProtocolAllowed 检查协议是否允许
+ func (s *Server) isProtocolAllowed(proto types.ProtocolID) bool {
+     protoStr := string(proto)
+     
+     // 系统协议：拒绝
+     if strings.HasPrefix(protoStr, protocolids.SysPrefix) {
+         return false
+     }
+     
+     // 本 Realm 应用协议：允许
+     expectedAppPrefix := fmt.Sprintf("/dep2p/app/%s/", s.realmID)
+     if strings.HasPrefix(protoStr, expectedAppPrefix) {
+         return true
+     }
+     
+     // 本 Realm 控制协议：允许
+     expectedRealmPrefix := fmt.Sprintf("/dep2p/realm/%s/", s.realmID)
+     if strings.HasPrefix(protoStr, expectedRealmPrefix) {
+         return true
+     }
+     
+     // 其他：拒绝
+     return false
+ }
```

---

### 3.8 Node Facade 适配

#### `node.go`（重大重构）

```diff
// JoinRealm 加入 Realm（返回 Realm 对象）
- func (n *Node) JoinRealm(ctx context.Context, realmID types.RealmID, opts ...realmif.JoinOption) error {
+ func (n *Node) JoinRealm(ctx context.Context, name string, opts ...realmif.RealmOption) (realmif.Realm, error) {
      if n.Realm() == nil {
-         return fmt.Errorf("Realm 未启用")
+         return nil, fmt.Errorf("Realm 未启用")
      }
-     return n.Realm().JoinRealm(ctx, realmID, opts...)
+     return n.Realm().JoinRealm(ctx, name, opts...)
+ }
+
+ // JoinRealmWithKey 便捷方法
+ func (n *Node) JoinRealmWithKey(ctx context.Context, name string, realmKey types.RealmKey, opts ...realmif.RealmOption) (realmif.Realm, error) {
+     if n.Realm() == nil {
+         return nil, fmt.Errorf("Realm 未启用")
+     }
+     return n.Realm().JoinRealmWithKey(ctx, name, realmKey, opts...)
  }

// Send 发送消息（从当前 Realm 获取 Messaging）
- func (n *Node) Send(ctx context.Context, nodeID types.NodeID, protocol types.ProtocolID, data []byte) error {
+ func (n *Node) Send(ctx context.Context, nodeID types.NodeID, data []byte) error {
-     if !n.IsMember() {
-         return endpoint.ErrNotMember
-     }
-     if n.Messaging() == nil {
-         return fmt.Errorf("Messaging 未启用")
-     }
-     return n.Messaging().Send(ctx, nodeID, protocol, data)
+     realm := n.CurrentRealm()
+     if realm == nil {
+         return ErrNotInRealm
+     }
+     return realm.Messaging().Send(ctx, nodeID, data)
  }

// CurrentRealm 返回当前 Realm 对象
- func (n *Node) CurrentRealm() types.RealmID {
+ func (n *Node) CurrentRealm() realmif.Realm {
      if n.Realm() == nil {
-         return ""
+         return nil
      }
      return n.Realm().CurrentRealm()
  }
```

---

### 3.9 配置变更

#### `internal/config/config.go`

```diff
type RealmConfig struct {
    ...
-   DefaultRealmID string
+   // 删除 DefaultRealmID，必须显式提供 realmKey
}
```

#### `options.go`

```diff
+ // WithRealmKey 设置 Realm 密钥（用于 JoinRealm）
+ // 注意：这是 RealmOption，不是 Node Option
+ func WithRealmKey(key types.RealmKey) realmif.RealmOption {
+     return realmif.WithRealmKey(key)
+ }

- // 删除 WithDefaultRealm
- // 删除所有与旧 RealmID 相关的 Option
```

---

## 四、文件变更清单

### 4.1 新增文件（14 个）

| 文件路径 | 用途 |
|---------|------|
| `pkg/interfaces/realm/services.go` | Layer 3 服务接口 |
| `pkg/interfaces/realm/psk.go` | PSK 成员证明接口 |
| `internal/core/realm/realm_impl.go` | Realm 对象实现 |
| `internal/core/realm/psk.go` | PSK 验证器实现 |
| `internal/core/realm/messaging.go` | Realm Messaging 适配 |
| `internal/core/realm/pubsub.go` | Realm PubSub 适配 |
| `internal/core/realm/discovery.go` | Realm Discovery 适配 |
| `internal/core/realm/streams.go` | Realm StreamManager 适配 |
| `internal/core/realm/relay_service.go` | Realm Relay Service 实现 |
| `internal/core/relay/server/system_relay.go` | System Relay 实现（新文件） |
| `internal/core/relay/server/realm_relay.go` | Realm Relay 实现（新文件，与 System Relay 分离） |
| `design/protocols/foundation/protocol-namespace.md` | 协议命名空间规范 |
| `design/adr/0004-psk-membership.md` | PSK 决策记录 |

### 4.2 重大修改文件（18 个）

| 文件路径 | 变更程度 |
|---------|---------|
| `pkg/types/realm.go` | 🔴 重写 |
| `pkg/types/ids.go` | 🟡 新增类型 |
| `pkg/interfaces/realm/realm.go` | 🔴 重写 |
| `pkg/protocolids/sys.go` | 🟡 新增函数 |
| `internal/core/realm/manager.go` | 🔴 重写 |
| `internal/core/realm/auth.go` | 🔴 重写 |
| `internal/core/messaging/service.go` | 🔴 重写 |
| `internal/core/messaging/pubsub.go` | 🟡 修改 |
| `internal/core/relay/server/server.go` | 🔴 重写（分离 System/Realm） |
| `internal/core/relay/client.go` | 🟡 修改（区分 System/Realm 客户端） |
| `node.go` | 🔴 重写 |
| `options.go` | 🟡 修改 |
| `dep2p.go` | 🟡 修改 |
| `internal/config/config.go` | 🟡 修改 |
| `internal/app/runtime.go` | 🟡 修改 |
| `internal/app/modulesets.go` | 🟡 修改 |

### 4.3 测试文件更新（预计 20+ 个）

所有涉及 `JoinRealm`、`Send`、`Publish`、`Subscribe` 的测试文件都需要更新。

---

## 五、重构影响分析

### 5.1 数据变更

| 变更项 | 影响 | 处理方式 |
|--------|------|---------|
| RealmID 派生规则 | 所有旧 RealmID 值失效 | **直接丢弃**，开发阶段无真实数据 |
| DHT 数据 | 所有旧 Realm 相关 DHT 记录失效 | **直接丢弃**，重新加入 Realm |
| 协议 ID 格式 | 所有旧协议注册失效 | **直接重写**，按新格式重新注册 |
| 配置文件 | 旧配置格式不兼容 | **直接重写**，按新格式配置 |

**重要**：本项目处于开发阶段，无真实用户数据，所有旧数据可直接丢弃，无需任何迁移工具或兼容层。

### 5.2 代码变更

| 变更项 | 影响范围 | 处理方式 |
|--------|---------|---------|
| `JoinRealm` 签名 | 所有调用方 | **全部重写**，编译期发现 |
| `Send/Publish` 签名 | Node Facade + 业务代码 | **全部重写**，编译期发现 |
| Messaging 接口 | 所有消息相关代码 | **全部重写**，编译期发现 |
| Realm 接口 | 所有 Realm 相关代码 | **全部重写**，编译期发现 |

### 5.3 性能与复杂度

| 项 | 影响 | 验证方式 |
|----|------|---------|
| PSK 验证性能 | 每次连接/中继请求 | MAC 计算高效，基准测试验证 |
| 协议路由复杂度 | ProtocolRouter | 单元测试覆盖 |
| Relay 验证延迟 | 中继连接 | 验证在握手阶段完成，一次性 |
| 服务懒加载竞态 | Realm 服务 getter | sync.Once 或 sync.Mutex |

---

## 六、实施顺序

```
Phase 1: 类型基础 [1 周]
├── RealmKey 类型
├── DeriveRealmID 函数
├── 协议命名空间函数
└── PSK 证明类型

Phase 2: 接口定义 [1 周]
├── Realm 接口
├── Layer 3 服务接口
├── PSK 验证接口
└── 删除废弃接口

Phase 3: 核心实现 [2 周]
├── realmImpl 实现
├── PSKVerifier 实现
├── Manager 重构
└── 服务适配层

Phase 4: Messaging/PubSub 适配 [1 周]
├── 协议前缀自动添加
├── Topic 前缀处理
└── 接口对齐

Phase 5: Relay 适配 [1 周]
├── PSK 验证集成
├── 协议白名单
└── System vs Realm 分流

Phase 6: Facade + 配置 [1 周]
├── Node Facade 重构
├── Options 更新
└── 配置清理

Phase 7: 测试 + 文档 [2 周]
├── 单元测试更新
├── 集成测试更新
├── 设计文档更新
└── 用户文档更新

总计: ~9 周
```

---

## 七、验收标准

### 功能验收

- [ ] `node.JoinRealm(name, realmKey)` 返回 `Realm` 对象
- [ ] `realm.Messaging().Send()` 正常工作
- [ ] `realm.PubSub().Join()` 正常工作
- [ ] 不同 Realm 协议完全隔离
- [ ] Realm Relay 验证 PSK 成员证明
- [ ] 用户无法注册 `/dep2p/sys/*` 协议
- [ ] 协议前缀由框架自动处理

### 测试验收

- [ ] 所有现有测试更新并通过
- [ ] 新增 PSK 验证单元测试
- [ ] 新增协议隔离集成测试
- [ ] 新增 Realm Relay 验证测试

### 文档验收

- [ ] 设计文档全部更新
- [ ] 用户文档全部更新
- [ ] quickstart 示例可运行

---

## 八、关联文档

- [DISC-1227-api-layer-design](../discussions/DISC-1227-api-layer-design.md)
- [DISC-1227-relay-isolation](../discussions/DISC-1227-relay-isolation.md)

---

## 九、审查发现的问题与修复

### 9.1 已修复的问题

| 问题 | 状态 | 修复内容 |
|------|------|---------|
| JoinRealm API 不一致 | ✅ 已修复 | 统一为 `JoinRealm(name, opts)` + `JoinRealmWithKey(name, key, opts)` |
| RealmID 格式不一致 | ✅ 已修复 | 改为完整32字节SHA256（64字符hex） |
| RealmRelayService 缺少 Stats() | ✅ 已修复 | 添加 `Stats() RelayStats` 方法 |
| StreamManager.Open 协议自动补全 | ✅ 已修复 | 明确说明协议自动添加 Realm 前缀 |
| Relay 双重验证缺失 | ✅ 已修复 | 添加 B 验证 A 的说明 |
| System Relay 实现缺失 | ✅ 已修复 | 添加 System Relay vs Realm Relay 区分说明 |
| 协议验证缺少 realmID 匹配 | ✅ 已修复 | `ValidateUserProtocol` 增加 realmID 匹配检查 |
| Messaging 缺少 SendWithProtocol | ✅ 已修复 | 添加 `SendWithProtocol` 和 `RequestWithProtocol` |

### 9.2 已确认的设计决策

| 问题 | 决策 | 理由 |
|------|------|------|
| PSK 证明中的 peerID | **peerID = 目标节点**（通信对方） | 1. 绑定目标：证明含义为"我要与 peerID 通信"<br>2. 防中间人：R 无法将 A→B 的证明用于 A→C<br>3. 双重验证：B 收到时验证 peerID == 自己 |
| PubSub topic 格式 | **不需要 `/pubsub/` 前缀** | 1. 简洁性：用户写 `blocks`，实际 topic 是 `/dep2p/app/<realmID>/blocks`<br>2. 区分方式：通过服务类型区分（PubSub vs Streams），不是路径前缀<br>3. 一致性：所有应用层协议/topic 统一格式 `/dep2p/app/<realmID>/<name>` |

---

## 更新日志

| 日期 | 变更 |
|------|------|
| 2024-12-28 | 初始创建 |
| 2024-12-28 | 删除向后兼容，扩展代码实施细节 |
| 2024-12-28 | 删除所有迁移相关内容，明确彻底重构，旧数据直接丢弃 |
| 2024-12-28 | 修复审查发现的8个问题：API一致性、RealmID格式、Relay验证、System Relay区分等 |
| 2024-12-28 | 确认设计决策：peerID=目标节点，PubSub topic 不需要 /pubsub/ 前缀 |
| 2024-12-28 | **Phase 1 完成**: 类型基础 - RealmKey、DeriveRealmID、协议命名空间函数、PSK 证明类型 |
| 2024-12-28 | **Phase 2 完成**: 接口定义 - Realm 接口、Layer 3 服务接口、PSK 验证接口、旧接口废弃标记 |
| 2024-12-28 | **Phase 3 完成**: 核心实现 - PSKAuthenticator、realmImpl、Manager 重构、服务适配层 |
| 2024-12-28 | **Phase 4 完成**: Messaging/PubSub 适配 - Manager 服务注入、服务适配器完善 |
| 2024-12-28 | **Phase 5 完成**: Relay 适配 - Server Realm 字段、PSK 验证集成、协议白名单 |
| 2024-12-28 | **Phase 6 完成**: Facade + 配置 - 彻底移除向后兼容代码，更新所有示例/测试使用新 API |

---

## 十、实施进度跟踪

### Phase 1: 类型基础 ✅ 已完成

| 任务 | 状态 | 文件 |
|------|------|------|
| RealmKey 类型定义 | ✅ | `pkg/types/ids.go` |
| DeriveRealmID 函数 | ✅ | `pkg/types/realm.go` |
| 协议命名空间函数 | ✅ | `pkg/protocolids/sys.go` |
| PSK 证明类型 | ✅ | `pkg/types/psk.go` |

### Phase 2: 接口定义 ✅ 已完成

| 任务 | 状态 | 文件 |
|------|------|------|
| Realm 接口 | ✅ | `pkg/interfaces/realm/realm.go` |
| RealmManager 重构 | ✅ | `pkg/interfaces/realm/realm.go` |
| RealmOption 选项 | ✅ | `pkg/interfaces/realm/realm.go` |
| Layer 3 服务接口 | ✅ | `pkg/interfaces/realm/services.go` |
| PSK 验证接口 | ✅ | `pkg/interfaces/realm/psk.go` |
| 旧接口废弃标记 | ✅ | `pkg/interfaces/realm/realm.go` |

### Phase 3: 核心实现 ✅ 已完成

| 任务 | 状态 | 文件 |
|------|------|------|
| PSKAuthenticator 实现 | ✅ | `internal/core/realm/psk.go` |
| realmImpl 实现 | ✅ | `internal/core/realm/realm_impl.go` |
| Manager 重构 | ✅ | `internal/core/realm/manager.go` |
| 服务适配层 | ✅ | `internal/core/realm/services_adapters.go` |
| 相关文件更新 | ✅ | `auth.go`, `sync.go`, `discovery/service.go`, `node.go` |
| 示例/测试迁移 | ✅ | Phase 6 完成：全部迁移到新 API |

### Phase 4: Messaging/PubSub 适配 ✅ 已完成

| 任务 | 状态 | 文件 |
|------|------|------|
| Manager 服务注入 | ✅ | `internal/core/realm/manager.go` |
| realmImpl 服务依赖 | ✅ | `internal/core/realm/realm_impl.go` |
| Messaging 适配器完善 | ✅ | `internal/core/realm/services_adapters.go` |
| PubSub 适配器完善 | ✅ | `internal/core/realm/services_adapters.go` |
| Module 服务注入 | ✅ | `internal/core/realm/module.go` |

**主要变更**：
- `Manager` 添加 `SetMessaging()` 方法，注入底层消息服务
- `realmImpl` 持有 `messagingSvc` 引用，供服务适配器使用
- `realmMessaging` 适配器调用底层 `Send/Request/SetHandler` 方法
- `realmPubSub` 适配器调用底层 `Subscribe/Publish` 方法
- 协议/Topic 前缀自动添加，用户无感知

### Phase 5: Relay 适配 ✅ 已完成

| 任务 | 状态 | 文件 |
|------|------|------|
| Server Realm 字段 | ✅ | `internal/core/relay/server/server.go` |
| Relay 错误类型 | ✅ | `internal/core/relay/errors.go` |
| PSK 验证集成 | ✅ | `internal/core/relay/server/server.go` |
| 协议白名单 | ✅ | `internal/core/relay/server/server.go` |
| realmRelay 适配器 | ✅ | `internal/core/realm/services_adapters.go` |

**主要变更**：
- `Server` 添加 `realmID` 和 `pskAuth` 字段，支持 Realm Relay 模式
- `SetRealmID()` 和 `SetPSKAuthenticator()` 方法配置 Realm Relay
- `isProtocolAllowed()` 实现协议白名单（System vs Realm 协议过滤）
- `verifyPSKMembership()` 在预留阶段验证 PSK 成员资格
- `realmRelay` 适配器记录 Realm 配置，供底层 Relay Server 使用
- 新增错误码 `ErrCodePermission` 和 `ErrCodeProtocolNotAllowed`

### Phase 6: Facade + 配置 ✅ 已完成

| 任务 | 状态 | 文件 |
|------|------|------|
| Node Facade 重构 | ✅ | `node.go` |
| Manager 向后兼容移除 | ✅ | `internal/core/realm/manager.go` |
| RealmManager 接口清理 | ✅ | `pkg/interfaces/realm/realm.go` |
| 配置清理 | ✅ | `internal/config/config.go` |
| 示例代码更新 | ✅ | `examples/*/main.go` |
| 测试代码更新 | ✅ | `tests/**/*.go`, `internal/core/realm/realm_test.go` |

**主要变更**：
- 移除 `JoinRealmLegacy()`、`CurrentRealmID()` 等向后兼容方法
- `node.Send(ctx, nodeID, data)` 不再需要 protocol 参数（从 Realm.Messaging() 获取）
- `node.Request(ctx, nodeID, data)` 同上
- 删除 `DefaultRealmID` 和 `AutoJoin` 配置字段
- 所有示例和测试使用 `JoinRealmWithKey(ctx, name, realmKey)` 新 API
- `MustJoinRealm(name, realmKey)` 测试工具函数签名变更

### Phase 7: 测试 + 文档 ✅ 已完成

#### 7.1 单元测试更新
- ✅ `internal/core/realm/realm_test.go` - RealmManager 单元测试
- ✅ `internal/core/realm/psk_test.go` - PSKAuthenticator 单元测试
- ✅ `internal/core/realm/services_adapters_test.go` - 服务适配器测试
- ✅ `pkg/protocolids/sys_test.go` - 协议命名空间函数测试
- ✅ `pkg/types/realm_test.go` - RealmKey/RealmID 类型测试

#### 7.2 集成测试更新
- ✅ `tests/e2e/realm_test.go` - Realm 隔离 E2E 测试
- ✅ `tests/e2e/messaging_test.go` - Messaging 服务 E2E 测试
- ✅ `tests/e2e/pubsub_test.go` - PubSub 服务 E2E 测试
- ✅ `tests/e2e/relay_test.go` - Relay PSK 验证 E2E 测试

#### 7.3 设计文档更新
- ✅ `design/architecture/layers.md` - 三层架构更新
- ✅ `design/architecture/overview.md` - 架构总览更新
- ✅ `design/protocols/application/realm.md` - PSK 认证协议
- ✅ `design/protocols/transport/relay.md` - 分层中继设计
- ✅ `design/protocols/foundation/protocol-namespace.md` - 新增协议命名空间
- ✅ `design/invariants/INV-002-realm-membership.md` - PSK 不变量
- ✅ `design/adr/0002-realm-isolation.md` - ADR 更新

#### 7.4 用户文档更新
- ✅ `docs/zh/reference/api/node.md` - Node API 参考
- ✅ `docs/zh/reference/api/realm.md` - Realm API 参考
- ✅ `docs/zh/getting-started/quickstart.md` - 快速入门
- ✅ `docs/zh/getting-started/first-realm.md` - 第一个 Realm

---

## 实施完成总结

### 完成日期
2024-12-28

### 核心变更清单

| 变更项 | 变更前 | 变更后 |
|--------|--------|--------|
| JoinRealm 返回值 | `error` | `(Realm, error)` |
| RealmID 派生 | `H(name)` | `SHA256("dep2p-realm-id-v1" \|\| H(realmKey))` |
| 协议前缀 | 全局 `/dep2p/app/*` | `/dep2p/app/<realmID>/*` 自动添加 |
| 成员认证 | 无 | PSK MembershipProof |
| Layer 3 服务 | Node 方法 | Realm 对象 getter |
| Relay 验证 | 无 | PSK + 协议白名单 |

### 破坏性变更
- `node.JoinRealm(realmID)` → `node.JoinRealmWithKey(name, realmKey)`
- `node.CurrentRealm()` 返回 `Realm` 而非 `RealmID`
- `node.Send/Request/Publish/Subscribe` 移至 `realm.Messaging()/PubSub()`
- `config.RealmConfig.AutoJoin` 已移除
- 协议 ID 不再需要手动添加 Realm 前缀
