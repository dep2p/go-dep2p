# 术语表 (Glossary)

> 统一 DeP2P 文档的术语定义，避免歧义

---

## 核心概念

| 术语 | 英文 | 定义 |
|------|------|------|
| **DeP2P** | Decentralized P2P | 去中心化 P2P 网络库，以身份优先、Realm 隔离为核心设计 |
| **Node** | Node | 代表本地 P2P 节点的对象，是 DeP2P 的入口 |
| **NodeID** | Node Identifier | 节点唯一标识，由公钥哈希派生：`NodeID = SHA256(PublicKey)` |
| **Realm** | Realm | 业务隔离域，使用 PSK 认证成员 |
| **RealmID** | Realm Identifier | Realm 唯一标识，派生规则：`HKDF(PSK, salt="dep2p-realm-id-v1")` |
| **PSK** | Pre-Shared Key | 预共享密钥，用于 Realm 成员认证和 RealmID 派生 |

---

## 身份与安全

| 术语 | 英文 | 定义 |
|------|------|------|
| **Ed25519** | Ed25519 | DeP2P 使用的签名算法 |
| **公钥** | Public Key | Ed25519 公钥，32 字节 |
| **私钥** | Private Key | Ed25519 私钥，32 字节种子 |
| **TLS 1.3** | TLS 1.3 | QUIC 内置的传输层安全协议 |
| **身份验证** | Identity Verification | 连接时验证对端 NodeID 匹配期望值 |
| **成员认证** | Membership Authentication | 使用 PSK 派生密钥验证 Realm 成员资格 |

---

## 传输层

| 术语 | 英文 | 定义 |
|------|------|------|
| **QUIC** | QUIC | DeP2P 的主要传输协议（RFC 9000） |
| **流** | Stream | QUIC 流，双向数据通道 |
| **连接** | Connection | QUIC 连接，包含多个流 |
| **0-RTT** | Zero Round-Trip Time | QUIC 的零往返握手恢复 |
| **连接迁移** | Connection Migration | QUIC 支持的网络切换不断连 |

---

## 网络层

| 术语 | 英文 | 定义 |
|------|------|------|
| **DHT** | Distributed Hash Table | 分布式哈希表，用于节点发现 |
| **Kademlia** | Kademlia | DHT 算法，基于 XOR 距离 |
| **Bootstrap** | Bootstrap | 引导节点，用于初始发现 |
| **mDNS** | Multicast DNS | 多播 DNS，局域网发现 |
| **Rendezvous** | Rendezvous | 基于命名空间的发现服务 |

---

## NAT 穿透

> ★ 核心区分：外部地址发现、打洞、中继是**三个不同目的**的能力

| 术语 | 英文 | 定义 |
|------|------|------|
| **NAT** | Network Address Translation | 网络地址转换 |
| **★ 外部地址发现** | External Address Discovery | 知道"自己在公网上的地址"，不等于中继 |
| **★ 观察地址** | Observed Address | 对端报告的本节点外部地址，来源不可靠需多源验证 |
| **STUN** | Session Traversal Utilities for NAT | 外部地址发现协议，询问服务器获取公网地址 |
| **★ 打洞** | Hole Punching | UDP 打洞技术，**需要信令通道协调**，建立 NAT 后直连 |
| **★ 打洞地址** | Hole Punch Address | 打洞时交换的地址，仅在 Cone NAT + 协议匹配 + 地址有效时适用 |
| **★ 信令通道** | Signaling Channel | 打洞协调的必要条件，通常由 Relay 连接提供 |
| **★ NAT 类型** | NAT Type | Full Cone/Restricted/Port Restricted/Symmetric，影响打洞决策 |
| **★ Symmetric NAT** | Symmetric NAT | 每连接分配不同端口，打洞困难，双方均为此类型时直接用 Relay |
| **UPnP** | Universal Plug and Play | 自动端口映射协议 |
| **NAT-PMP** | NAT Port Mapping Protocol | 端口映射协议 |

---

## ★ 地址分类（关键概念）

> 候选地址与可达地址是两个不同概念，必须区分

| 术语 | 英文 | 定义 |
|------|------|------|
| **★ 候选地址** | Candidate Address | STUN/Observed 返回的地址，**未经验证**，可能不可达 |
| **★ 可达地址** | Verified/Reachable Address | 经过 Reachability/AutoNAT 验证的地址，可对外发布 |
| **★ 地址状态流转** | Address State Flow | Candidate → Validating → Verified → Published |
| **★ Reachability 验证** | Reachability Verification | 通过 AutoNAT/dialback 验证候选地址是否真正可达 |
| **★ DHT 发布约束** | DHT Publish Constraint | 只能发布可达地址，不可达时发布 Relay 地址 |
| **★ 发布 vs 广告** | Publish vs Advertise | Publish=写入 DHT Peer Record，Advertise=宣告提供某服务 |

---

## 中继（v2.0 更新）

> **v2.0 核心变化**：DHT 是权威目录，Relay 地址簿是缓存加速层

| 术语 | 英文 | 定义 |
|------|------|------|
| **Relay** | Relay | 统一中继服务，具有三大职责 |
| **★ Relay 三大职责 (v2.0)** | Triple Role | **缓存加速层** + 打洞协调信令 + 数据通信保底 |
| **★ 缓存加速层 (v2.0)** | Cache Acceleration Layer | Relay 地址簿作为 DHT 的本地缓存，仅在 DHT 失败时回退使用 |
| **★ 打洞协调信令** | Hole Punch Signaling | Relay 连接作为打洞协调的信令通道（交换候选地址） |
| **★ 数据通信保底** | Data Relay Fallback | 直连/打洞失败时 Relay 作为数据转发通道 |
| **★ 地址簿** | Address Book | Relay 维护的成员地址信息存储（v2.0: 非权威缓存） |
| **★ MemberAddressBook** | Member Address Book | Relay 维护的成员地址簿聚合根 |
| **★ MemberEntry** | Member Entry | 地址簿中的成员条目，包含 NodeID、地址、NAT 类型等 |
| **★ 显式配置** | Explicit Configuration | Relay 地址需要配置，不支持自动发现（ADR-0010） |
| **惰性连接** | Lazy Connection | 配置中继后不立即连接，按需使用 |
| **★ 基础设施融合** | Infrastructure Fusion | Bootstrap + Relay 可融合部署到同一节点 |
| **★ 保留 Relay 备份** | Keep Relay Backup | 打洞成功后保留 Relay 连接作为 fallback（INV-003） |
| **★ 中继透明原则** | Relay Transparency | 用户不需要知道连接是否走中继 |

---

## ★ DHT 权威目录（v2.0 新增）

> v2.0 核心概念：DHT 作为权威目录，存储签名 PeerRecord

| 术语 | 英文 | 定义 |
|------|------|------|
| **★ v2.0 三层架构** | Three-Layer Architecture | Layer 1: DHT 权威 / Layer 2: 缓存加速 / Layer 3: 连接策略 |
| **★ PeerRecord** | Peer Record | DHT 中存储的签名地址记录，包含 NodeID、地址、TTL、seq 等 |
| **★ 签名验证** | Signature Verification | DHT 记录必须由 NodeID 私钥签名，防止地址投毒 |
| **★ 序列号 (seq)** | Sequence Number | PeerRecord 的递增序号，防止重放攻击 |
| **★ TTL** | Time-to-Live | PeerRecord 的存活时间，30min-4h 动态管理 |
| **★ DHT（v2.0）** | DHT | 单一 DHT + 命名空间，Key 格式见下表 |
| **★ DHT Node Key** | DHT Node Key | `/dep2p/v2/node/<NodeID>` — 存储 SignedPeerRecord |
| **★ DHT Realm Members Key** | DHT Realm Members Key | `/dep2p/v2/realm/<H(RealmID)>/members` — Provider Record，成员发现 |
| **★ DHT Realm Peer Key** | DHT Realm Peer Key | `/dep2p/v2/realm/<H(RealmID)>/peer/<NodeID>` — 存储 SignedRealmPeerRecord |
| **★ 地址发现优先级 (v2.0)** | Address Discovery Priority | Peerstore → MemberList → **DHT（★权威）** → Relay 地址簿（缓存回退） |
| **★ AutoNAT** | AutoNAT | 可达性验证协议，验证 direct_addrs 是否真正可达 |
| **★ dialback** | Dialback | 回拨验证，验证地址可达性 |

---

## 连接

| 术语 | 英文 | 定义 |
|------|------|------|
| **★ 仅 ID 连接** | ID-only Connection | 使用纯 NodeID 进行连接，无需预先知道地址 |
| **★ Realm 内连接** | Intra-Realm Connection | 同 Realm 成员间的连接，支持"仅 ID 连接" |
| **★ 跨 Realm 连接** | Cross-Realm Connection | 不同 Realm 成员间的连接，必须提供地址 |
| **★ 连接边界** | Connection Boundary | "仅 ID 连接"严格限制在 Realm 内的设计原则 |
| **★ ErrAddressRequired** | Address Required Error | 跨 Realm 使用纯 NodeID 连接时返回的错误 |

---

## 节点角色

| 术语 | 英文 | 定义 |
|------|------|------|
| **普通节点** | Client Node | 使用网络服务的节点，可能在 NAT 后面 |
| **Bootstrap 节点** | Bootstrap Node | 为新节点提供 DHT 引导的节点，需要公网可达 |
| **Relay 节点** | Relay Node | 提供中继服务的节点（地址发现 + 数据转发），需要公网可达 |
| **基础设施节点** | Infrastructure Node | Bootstrap 或 Relay 节点的统称，可融合部署 |
| **公网可达** | Publicly Reachable | 成为基础设施节点的唯一硬性条件 |
| **存活探测** | Liveness Probe | Bootstrap 节点特有能力，探测已知节点的在线状态 |
| **能力可组合** | Capability Composability | 一个节点可同时启用 Bootstrap + Relay 能力（融合部署） |
| **去中心化精神** | Decentralization Spirit | 资源要求只是建议，任何人都可以参与 |


---

## 应用服务

| 术语 | 英文 | 定义 |
|------|------|------|
| **Messaging** | Messaging | 点对点消息服务 |
| **PubSub** | Publish-Subscribe | 发布订阅服务 |
| **GossipSub** | GossipSub | 基于 gossip 的 PubSub 协议 |
| **Topic** | Topic | 发布订阅主题 |
| **Subscription** | Subscription | 订阅句柄 |

---

## 协议

| 术语 | 英文 | 定义 |
|------|------|------|
| **Protocol ID** | Protocol Identifier | 协议标识符，格式：`/dep2p/{scope}/{name}/{version}` |
| **ALPN** | Application-Layer Protocol Negotiation | 应用层协议协商 |
| **Protobuf** | Protocol Buffers | 主要序列化格式 |
| **CBOR** | Concise Binary Object Representation | 备选序列化格式 |
| **varint** | Variable-length Integer | 变长整数编码 |

---

## 协议前缀

| 前缀 | 层级 | 说明 |
|------|------|------|
| `/dep2p/sys/` | Layer 1 | 系统协议（DHT、Relay、打洞等） |
| `/dep2p/realm/{id}/` | Layer 2 | Realm 控制协议（认证、成员同步） |
| `/dep2p/app/{id}/` | Layer 3 | 应用协议（用户自定义） |

---

## 编码规范

| 术语 | 定义 |
|------|------|
| **大端序** | 多字节整数使用网络字节序（Big-Endian） |
| **Base58** | 用于 NodeID/RealmID 的人类可读编码 |
| **长度前缀** | 消息使用 varint 编码的长度前缀 |

---

## 系统不变量

| 术语 | 定义 | 文档 |
|------|------|------|
| **INV-001** | 身份第一性：每个连接必须绑定预期 NodeID | [INV-001](../../01_context/decisions/invariants/INV-001-identity-first.md) |
| **INV-002** | Realm 成员资格：业务 API 需要 Realm 成员资格 | [INV-002](../../01_context/decisions/invariants/INV-002-realm-membership.md) |
| **INV-003** | 连接优先级与 Relay 保留：直连 → 打洞 → Relay | [INV-003](invariants.md#inv-003-连接优先级与-relay-保留) |

---

## 错误类型

| 错误 | 说明 |
|------|------|
| **ErrNotMember** | 未加入 Realm，无法使用业务 API |
| **ErrIdentityMismatch** | 对端 NodeID 与期望不匹配 |
| **ErrAuthFailed** | PSK 成员认证失败 |
| **ErrRealmNotFound** | Realm 不存在 |
| **ErrTimeout** | 操作超时 |

---

## 架构决策 (ADR)

| 编号 | 标题 | 说明 |
|------|------|------|
| ADR-0001 | 身份优先 | 连接必须绑定 NodeID |
| ADR-0002 | Realm 隔离 | 业务隔离域设计 |
| ADR-0003 | 中继优先连接 | 连接建立策略 |
| ADR-0004 | 连接优先级策略 | 直连 → 打洞 → Relay，保留备份 |
| ADR-0006 | QUIC 传输 | 选择 QUIC 作为主传输 |
| ADR-0007 | 协议命名 | 协议 ID 命名规范 |
| ADR-0008 | 发现策略 | 多机制发现设计 |
| **★ ADR-0010** | **Relay 显式配置** | **Relay 地址需显式配置，不支持自动发现** |

---

## 相关文档

| 文档 | 说明 |
|------|------|
| [core_concepts.md](core_concepts.md) | 核心概念详解 |
| [domain_map.md](domain_map.md) | 领域映射 |
| [../L2_structural/layer_model.md](../L2_structural/layer_model.md) | 五层软件架构 |
| [SPEC_INDEX.md](../../02_constraints/protocol/SPEC_INDEX.md) | 协议规范索引 |

---

**最后更新**：2026-01-24（v2.0 DHT 权威模型对齐）
