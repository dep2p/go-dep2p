# Realm 模块

> **版本**: v1.1.0  
> **更新日期**: 2026-01-23  
> **定位**: Realm 管理与认证（Realm Layer）

---

## 模块概述

realm 是 DeP2P 的核心创新，提供业务隔离域管理，包括加入/离开 Realm 和成员认证。

| 属性 | 值 |
|------|-----|
| **架构层** | Realm Layer |
| **代码位置** | `internal/realm/` |
| **协议前缀** | `/dep2p/app/<realmID>/realm/*` |
| **状态** | ✅ 已实现 |
| **依赖** | core_host, core_identity, discovery |

---

## 核心职责

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                         realm 职责                                           │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  1. Realm 生命周期                                                          │
│     • 加入 Realm (JoinRealmWithKey)                                         │
│     • 离开 Realm (Leave)                                                    │
│     • 切换 Realm                                                            │
│                                                                             │
│  2. 成员认证 (INV-002)                                                      │
│     • PSK 派生密钥 (HKDF)                                                   │
│     • 挑战-响应认证                                                         │
│     • 成员资格验证                                                          │
│                                                                             │
│  3. 成员发现                                                                │
│     • Rendezvous 注册                                                       │
│     • 成员缓存与同步                                                        │
│     • 成员状态管理                                                          │
│     • ★ Relay 地址簿查询（调用 core_relay）                                │
│                                                                             │
│  4. 中继服务使用                                                            │
│     • 调用 core_relay 的统一 Relay 服务                                     │
│     • ⚠️ 地址簿由 core_relay 统一管理，Realm 只调用查询接口                 │
│                                                                             │
│  5. ★ "仅 ID 连接"支持                                                     │
│     • realm.Connect(ctx, targetNodeID) 支持纯 NodeID 连接                  │
│     • 自动地址发现（Peerstore → MemberList → DHT（权威）→ Relay（缓存））  │
│     • 连接优先级：直连 → NAT 打洞 → Relay 保底                             │
│     • 连接边界：仅 Realm 内允许"仅 ID 连接"                                │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## ★ "仅 ID 连接"边界

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    "仅 ID 连接"的严格边界（★ 核心特性）                      │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  Realm 内（✅ 允许"仅 ID 连接"）                                            │
│  ═══════════════════════════════                                            │
│                                                                             │
│  realm.Connect(ctx, targetNodeID)   ← ✅ 允许                               │
│                                                                             │
│  地址发现来源（v2.0 优先级）：                                               │
│  1. Peerstore 本地缓存                                                      │
│  2. MemberList 成员列表                                                     │
│  3. DHT 查询（★ 权威来源）                                                  │
│  4. Relay 地址簿（缓存回退）                                                │
│                                                                             │
│  保底机制：Relay 数据转发（总是可达）                                       │
│                                                                             │
│  ─────────────────────────────────────────────────────────────────────────  │
│                                                                             │
│  跨 Realm / 节点级（❌ 禁止"仅 ID 连接"）                                   │
│  ═══════════════════════════════════════                                    │
│                                                                             │
│  node.Connect(ctx, targetNodeID)    ← ❌ 返回 ErrAddressRequired           │
│  node.Connect(ctx, multiaddr)       ← ✅ 必须提供地址                       │
│                                                                             │
│  这是刻意的设计限制，体现 Realm 作为业务边界的核心理念                      │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 子模块结构

```
internal/realm/
├── module.go              # Fx 模块定义
├── interfaces/            # 内部接口
│   └── realm.go
├── manager/               # Realm 生命周期管理
├── auth/                  # 成员认证
│   ├── psk.go            # PSK 派生
│   └── challenge.go      # 挑战-响应
├── member/                # 成员管理
│   └── cache.go          # 成员缓存
└── connector/             # ★ 连接管理（"仅 ID 连接"支持）
    ├── connector.go      # 连接器实现
    └── resolver.go       # 地址解析（多源优先级）

# ⚠️ 注意：地址簿子模块已移至 core_relay
# Realm 通过 RelayService 接口调用地址簿功能，不再自行维护
```

## ★ 地址簿归属说明（v1.1.0 变更）

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    地址簿归属变更说明                                         │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  变更原因：                                                                  │
│  ─────────                                                                  │
│  • 统一 Relay 概念后，地址簿功能统一由 core_relay 管理                      │
│  • 避免 Realm 和 Relay 两处维护地址簿导致的数据不一致                        │
│  • v2.0 Relay 三大职责的第一层是"缓存加速层"（DHT 是权威目录）              │
│                                                                             │
│  Realm 如何使用地址簿：                                                      │
│  ─────────────────────                                                      │
│                                                                             │
│  // 通过 core_relay 的 RelayService 接口                                    │
│  entry := relayService.QueryAddress(targetNodeID)                           │
│                                                                             │
│  地址发现优先级（Realm 内）：                                                │
│  ───────────────────────────                                                │
│  1. Peerstore 本地缓存                                                      │
│  2. MemberList 成员同步获取                                                 │
│  3. ★ Relay 地址簿查询（调用 core_relay）                                   │
│  4. Relay 数据转发保底                                                      │
│                                                                             │
│  ★ 关键约束：                                                               │
│  • Relay 地址簿存储的是 Reachability 验证过的可达地址                       │
│  • Realm 不自行维护地址簿，只消费 core_relay 提供的服务                     │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

## ★ 打洞协调依赖说明

```
┌─────────────────────────────────────────────────────────────────────────────┐
│                    打洞协调需要信令通道                                       │
├─────────────────────────────────────────────────────────────────────────────┤
│                                                                             │
│  "仅 ID 连接"流程中的打洞阶段：                                              │
│  ═══════════════════════════════                                            │
│                                                                             │
│  1. 直连尝试失败                                                            │
│  2. 检查 NAT 类型是否允许打洞                                               │
│  3. ★ 打洞协调需要信令通道（通常是已建立的 Relay 连接）                     │
│  4. 通过 Relay 交换候选地址                                                 │
│  5. 执行 UDP 打洞                                                           │
│  6. 打洞成功后保留 Relay 连接作为备份                                       │
│                                                                             │
│  Realm 连接器中的打洞调用：                                                  │
│  ─────────────────────────                                                  │
│                                                                             │
│  fn connectWithPriority(target: NodeID):                                    │
│      // 直连尝试                                                            │
│      if direct := dialDirect(target); direct.is_ok() {                     │
│          return direct                                                      │
│      }                                                                      │
│                                                                             │
│      // 打洞尝试（需要 Relay 信令通道）                                     │
│      if relay.IsConnected() && canHolePunch() {                            │
│          // ★ 通过 Relay 协调打洞                                          │
│          if punched := natService.HolePunch(target, relay); punched.is_ok() {│
│              relay.KeepAsBackup()  // 打洞后保留                            │
│              return punched                                                 │
│          }                                                                  │
│      }                                                                      │
│                                                                             │
│      // Relay 转发保底                                                      │
│      return relay.Dial(target)                                             │
│                                                                             │
└─────────────────────────────────────────────────────────────────────────────┘
```

---

## 目录导航

| 文档 | 说明 |
|------|------|
| [requirements/requirements.md](requirements/requirements.md) | 需求追溯 |
| [design/overview.md](design/overview.md) | 整体设计 |
| [design/internals.md](design/internals.md) | 内部设计 |
| [coding/guidelines.md](coding/guidelines.md) | 编码指南 |
| [testing/strategy.md](testing/strategy.md) | 测试策略 |

---

## 相关文档

| 文档 | 说明 |
|------|------|
| [L5: Realm 领域](../../L5_models/realm/) | 领域模型 |
| [L3: Realm 流程](../../L3_behavioral/realm_flow.md) | 行为设计 |
| [L2: 五层架构](../../L2_structural/layer_model.md) | 架构定义 |

---

**最后更新**：2026-01-23
