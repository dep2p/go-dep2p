# 需求注册表 (Registry)

> 所有需求的统一索引，按层级和状态分类

---

## 1. 需求总览

### 1.1 统计

| 层级 | 总数 | Draft | Approved | Implementing | Implemented | Tested |
|------|------|-------|----------|--------------|-------------|--------|
| F1: 身份层 | 2 | 2 | 0 | 0 | 0 | 0 |
| F2: 传输层 | 3 | 0 | 0 | 3 | 0 | 0 |
| F3: 网络层 | 6 | 6 | 0 | 0 | 0 | 0 |
| F4: 安全层 | 2 | 2 | 0 | 0 | 0 | 0 |
| F5: Realm 层 | 3 | 3 | 0 | 0 | 0 | 0 |
| F6: 协议层 | 4 | 4 | 0 | 0 | 0 | 0 |
| F7: API 层 | 3 | 3 | 0 | 0 | 0 | 0 |
| NF: 非功能 | 5 | 5 | 0 | 0 | 0 | 0 |
| **合计** | **28** | **25** | **0** | **3** | **0** | **0** |

### 1.2 优先级分布

| 优先级 | 数量 | 说明 |
|--------|------|------|
| P0 | 10 | 关键路径，必须完成 |
| P1 | 12 | 重要功能，应该完成 |
| P2 | 6 | 增强功能，可以推迟 |

---

## 2. 功能需求索引

### 2.1 F1: 身份层 (Identity)

| ID | 标题 | 类型 | 优先级 | 状态 | 竞品参考 |
|----|------|------|--------|------|----------|
| [REQ-ID-001](functional/F1_identity/REQ-ID-001.md) | NodeID 设计 | generic | P0 | draft | iroh, libp2p |
| [REQ-ID-002](functional/F1_identity/REQ-ID-002.md) | 密钥管理 | generic | P1 | draft | iroh, libp2p |

### 2.2 F2: 传输层 (Transport)

| ID | 标题 | 类型 | 优先级 | 状态 | 竞品参考 |
|----|------|------|--------|------|----------|
| [REQ-TRANS-001](functional/F2_transport/REQ-TRANS-001.md) | QUIC 传输 | generic | P0 | implementing | iroh |
| [REQ-TRANS-002](functional/F2_transport/REQ-TRANS-002.md) | 连接管理 | generic | P0 | implementing | iroh, libp2p |
| [REQ-TRANS-003](functional/F2_transport/REQ-TRANS-003.md) | 流多路复用 | generic | P1 | implementing | iroh |

### 2.3 F3: 网络层 (Network)

| ID | 标题 | 类型 | 优先级 | 状态 | 竞品参考 |
|----|------|------|--------|------|----------|
| [REQ-NET-001](functional/F3_network/REQ-NET-001.md) | 节点发现 | generic | P0 | draft | iroh, libp2p, torrent |
| [REQ-NET-002](functional/F3_network/REQ-NET-002.md) | NAT 穿透 | generic | P1 | draft | iroh, libp2p |
| [REQ-NET-003](functional/F3_network/REQ-NET-003.md) | Relay 中继 | generic | P0 | draft | iroh, libp2p |
| [REQ-NET-004](functional/F3_network/REQ-NET-004.md) | 网络变化处理 | generic | P1 | draft | iroh, go-dep2p-main |
| [REQ-NET-005](functional/F3_network/REQ-NET-005.md) | **网络弹性与恢复** | generic | **P0** | draft | **旧 dep2p** |
| [REQ-NET-006](functional/F3_network/REQ-NET-006.md) | **可达性验证** | generic | **P1** | draft | **旧 dep2p** |

### 2.4 F4: 安全层 (Security)

| ID | 标题 | 类型 | 优先级 | 状态 | 竞品参考 |
|----|------|------|--------|------|----------|
| [REQ-SEC-001](functional/F4_security/REQ-SEC-001.md) | 传输加密 | generic | P0 | draft | iroh (TLS), libp2p (Noise) |
| [REQ-SEC-002](functional/F4_security/REQ-SEC-002.md) | 身份验证 | generic | P1 | draft | iroh, libp2p |

### 2.5 F5: Realm 层 (DeP2P 特有)

| ID | 标题 | 类型 | 优先级 | 状态 | 竞品参考 |
|----|------|------|--------|------|----------|
| [REQ-REALM-001](functional/F5_realm/REQ-REALM-001.md) | Realm 强制隔离 | dep2p-specific | P0 | draft | **独有** |
| [REQ-REALM-002](functional/F5_realm/REQ-REALM-002.md) | Realm PSK 认证 | dep2p-specific | P1 | draft | **独有** |
| [REQ-REALM-003](functional/F5_realm/REQ-REALM-003.md) | Relay | dep2p-specific | P1 | draft | **独有** |

### 2.6 F6: 协议层 (Protocol)

| ID | 标题 | 类型 | 优先级 | 状态 | 竞品参考 |
|----|------|------|--------|------|----------|
| [REQ-PROTO-001](functional/F6_protocol/REQ-PROTO-001.md) | 协议命名空间 | generic | P1 | draft | libp2p |
| [REQ-PROTO-002](functional/F6_protocol/REQ-PROTO-002.md) | 消息格式 | generic | P2 | draft | libp2p |
| [REQ-PROTO-003](functional/F6_protocol/REQ-PROTO-003.md) | 流式通信 | generic | P1 | draft | iroh, libp2p |
| [REQ-PROTO-004](functional/F6_protocol/REQ-PROTO-004.md) | **可靠消息投递** | dep2p-specific | **P0** | draft | **旧 dep2p** |

### 2.7 F7: API 层 (Interface)

| ID | 标题 | 类型 | 优先级 | 状态 | 竞品参考 |
|----|------|------|--------|------|----------|
| [REQ-API-001](functional/F7_api/REQ-API-001.md) | Node API 设计 | dep2p-specific | P0 | draft | iroh Endpoint |
| [REQ-API-002](functional/F7_api/REQ-API-002.md) | 连接语义 | dep2p-specific | P1 | draft | iroh |
| [REQ-API-003](functional/F7_api/REQ-API-003.md) | 事件通知 | generic | P2 | draft | iroh Watcher, libp2p Notifiee |

---

## 3. 非功能需求索引

| ID | 标题 | 类别 | 优先级 | 状态 |
|----|------|------|--------|------|
| [REQ-PERF-001](non_functional/REQ-PERF-001.md) | 性能基准 | 性能 | P1 | draft |
| [REQ-SCALE-001](non_functional/REQ-SCALE-001.md) | 可扩展性 | 扩展性 | P1 | draft |
| [REQ-AVAIL-001](non_functional/REQ-AVAIL-001.md) | 可用性 | 可用性 | P2 | draft |
| [REQ-OPS-001](non_functional/REQ-OPS-001.md) | 可观测性 | 运维 | P1 | draft |
| [REQ-COMPAT-001](non_functional/REQ-COMPAT-001.md) | 兼容性 | 兼容性 | P2 | draft |

---

## 4. 按优先级排序

### P0 - 关键路径

| ID | 标题 | 层级 | 状态 |
|----|------|------|------|
| REQ-ID-001 | NodeID 设计 | F1 | draft |
| REQ-TRANS-001 | QUIC 传输 | F2 | draft |
| REQ-TRANS-002 | 连接管理 | F2 | draft |
| REQ-NET-001 | 节点发现 | F3 | draft |
| REQ-NET-003 | Relay 中继 | F3 | draft |
| **REQ-NET-005** | **网络弹性与恢复** | **F3** | draft |
| REQ-SEC-001 | 传输加密 | F4 | draft |
| REQ-REALM-001 | Realm 强制隔离 | F5 | draft |
| **REQ-PROTO-004** | **可靠消息投递** | **F6** | draft |
| REQ-API-001 | Node API 设计 | F7 | draft |

### P1 - 重要功能

| ID | 标题 | 层级 | 状态 |
|----|------|------|------|
| REQ-ID-002 | 密钥管理 | F1 | draft |
| REQ-TRANS-003 | 流多路复用 | F2 | draft |
| REQ-NET-002 | NAT 穿透 | F3 | draft |
| REQ-NET-004 | 网络变化处理 | F3 | draft |
| **REQ-NET-006** | **可达性验证** | **F3** | draft |
| REQ-SEC-002 | 身份验证 | F4 | draft |
| REQ-REALM-002 | Realm PSK 认证 | F5 | draft |
| REQ-REALM-003 | Relay | F5 | draft |
| REQ-PROTO-001 | 协议命名空间 | F6 | draft |
| REQ-PROTO-003 | 流式通信 | F6 | draft |
| REQ-API-002 | 连接语义 | F7 | draft |
| REQ-PERF-001 | 性能基准 | NF | draft |
| REQ-SCALE-001 | 可扩展性 | NF | draft |
| REQ-OPS-001 | 可观测性 | NF | draft |

---

## 5. 需求与竞品对照

### 5.1 来自 iroh 的启示

| iroh 特性 | 对应需求 | 说明 |
|-----------|----------|------|
| Endpoint | REQ-API-001 | Node API 设计 |
| MagicSock | REQ-TRANS-002 | 连接管理抽象 |
| QUIC 优先 | REQ-TRANS-001 | QUIC 传输 |
| Home Relay | REQ-NET-003 | Relay 中继 |
| Dial by NodeID | REQ-API-002 | 连接语义 |

### 5.2 来自 go-libp2p 的启示

| libp2p 特性 | 对应需求 | 说明 |
|-------------|----------|------|
| 模块化设计 | 整体架构 | 可插拔组件 |
| multistream-select | REQ-PROTO-001 | 协议命名空间 |
| Noise XX | REQ-SEC-001 | 传输加密 |
| Circuit Relay v2 | REQ-NET-003 | Relay 中继 |
| DHT | REQ-NET-001 | 节点发现 |

### 5.3 DeP2P 独有需求

| 需求 | 说明 | 创新点 |
|------|------|--------|
| REQ-REALM-001 | Realm 强制隔离 | 业务域隔离 |
| REQ-REALM-002 | Realm PSK 认证 | 成员验证 |
| REQ-REALM-003 | Relay | 统一 Relay |

---

## 6. 变更历史

| 日期 | 变更 |
|------|------|
| 2026-01-11 | 初始版本，创建需求框架 |
| 2026-01-15 | 添加 REQ-NET-004 网络变化处理 |
| 2026-01-18 | 添加 REQ-NET-005 网络弹性与恢复、REQ-NET-006 可达性验证、REQ-PROTO-004 可靠消息投递（基于旧代码分析） |
| 2026-01-18 | 更新 F2 传输层需求状态为 implementing，添加保活和 Stream 修复说明 |

---

**最后更新**：2026-01-18
