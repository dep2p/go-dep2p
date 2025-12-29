# Realm 模块

> Realm（领域）管理模块，实现业务网络隔离。

---

## 概述

Realm 是 dep2p 实现业务隔离的核心机制，在共享基础设施的同时实现业务层完全隔离。

### 核心功能

> **更新时间**: 2025-12-23
> **版本**: v1.1（严格单 Realm 模型）

| 功能 | 描述 | 状态 |
|------|------|------|
| Realm 成员管理 | 加入/离开 Realm | ✅ 完成 |
| DHT 分片 | Realm 感知的 DHT Key | ✅ 完成 |
| 节点发现隔离 | 只发现同 Realm 节点 | ✅ 完成 |
| Pub-Sub 隔离 | 主题在 Realm 内隔离 | ✅ 完成 |
| 访问控制 | JoinKey 验证 | ✅ 完成 |
| RealmAuth 协议 | 连接级 Realm 验证 | ✅ 完成 |

---

## 架构位置

```
Layer 4: Realm（领域层）
├── 依赖 Layer 3: Service（Discovery, Messaging）
├── 被依赖 Layer 5: Messaging（Realm 感知的 Pub-Sub）
└── 被依赖 Layer 6: API（Endpoint 的 Realm 操作）
```

---

## 文件结构

```
internal/core/realm/
├── module.go      # fx 模块定义
├── manager.go     # RealmManager 核心实现
├── auth.go        # RealmAuth 协议实现
├── access.go      # 访问控制（JoinKey 验证）
├── filter.go      # Realm 感知的节点过滤
├── messaging.go   # Realm 感知的消息隔离
├── sync.go        # Realm 成员同步协议
├── errors.go      # 错误定义
└── README.md      # 本文档
```

---

## fx 模块

### 提供

| 接口 | 实现 | 标签 |
|------|------|------|
| `realmif.RealmManager` | `*Manager` | `name:"realm_manager"` |

### 依赖

| 依赖 | 说明 |
|------|------|
| `*config.Config` | 配置 |

---

## 接口定义

```go
// v1.1: 严格单 Realm 模型
type RealmManager interface {
    // Realm 成员管理
    CurrentRealm() RealmID
    JoinRealm(ctx context.Context, realmID RealmID, opts ...JoinOption) error
    LeaveRealm() error  // v1.1: 无参数，离开当前 Realm
    IsMember() bool     // v1.1: 无参数，检查是否已加入任何 Realm
    IsMemberOf(realmID RealmID) bool

    // Realm 内节点管理
    RealmPeers(realmID RealmID) []NodeID
    RealmPeerCount(realmID RealmID) int

    // Realm 感知的 DHT
    RealmDHTKey(nodeID NodeID, realmID RealmID) []byte

    // Realm 元数据
    RealmMetadata(realmID RealmID) (*RealmMetadata, error)

    // 生命周期
    Start(ctx context.Context) error
    Stop() error
}
```

---

## 配置

```go
// v1.1: 严格单 Realm 模型
type RealmConfig struct {
    Enable           bool   // 启用 Realm 管理
    DefaultRealmID   string // 默认 Realm ID
    AutoJoin         bool   // 自动加入默认 Realm（v1.1 默认 false）
    IsolateDiscovery bool   // 隔离节点发现
    IsolatePubSub    bool   // 隔离 Pub-Sub
}
```

---

## 设计文档

- [Realm 协议规范](../../../docs/01-design/protocols/application/04-realm.md)
- [架构层次](../../../docs/01-design/architecture/layers.md)

---

## 实现状态

> **更新时间**: 2025-12-23

所有核心功能已实现：

| 文件 | 功能 | 状态 |
|------|------|------|
| `manager.go` | Realm 成员管理、DHT 分片 | ✅ 完成 |
| `auth.go` | RealmAuth 协议、连接级验证 | ✅ 完成 |
| `access.go` | JoinKey 验证、访问控制 | ✅ 完成 |
| `filter.go` | Realm 感知的节点过滤 | ✅ 完成 |
| `messaging.go` | Realm 感知的 Pub-Sub 隔离 | ✅ 完成 |
| `sync.go` | Realm 成员同步协议 | ✅ 完成 |

