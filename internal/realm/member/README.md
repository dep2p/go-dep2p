# Realm Member - 成员管理

> **版本**: v1.0.0  
> **状态**: ✅ 已完成  
> **架构层**: Realm Layer

---

## 概述

`member` 提供 Realm 层的成员管理功能，负责成员注册、注销、查询、同步等操作。

**核心功能**:
- 👥 成员 CRUD - 注册、注销、查询
- 🔄 成员同步 - 全量/增量同步
- 💚 心跳监控 - 在线状态检测
- 💾 持久化存储 - BadgerDB 存储
- 🚀 LRU 缓存 - 高性能查询

---

## 快速开始

```go
import "github.com/dep2p/go-dep2p/internal/realm/member"

// 创建配置
config := member.DefaultConfig()
config.CacheSize = 1000
config.CacheTTL = 10 * time.Minute

// 创建管理器
manager := member.NewManager("realm-id", cache, store, eventBus)
defer manager.Close()

// 启动管理器
if err := manager.Start(ctx); err != nil {
    log.Fatal(err)
}

// 添加成员
m := &interfaces.MemberInfo{
    PeerID:   "peer123",
    RealmID:  "realm-id",
    Role:     interfaces.RoleMember,
    Online:   true,
    JoinedAt: time.Now(),
    Metadata: map[string]string{"region": "us-west"},
}
if err := manager.Add(ctx, m); err != nil {
    log.Fatal(err)
}

// 查询成员
m, err := manager.Get(ctx, "peer123")

// 列出所有在线成员
opts := &interfaces.ListOptions{
    Limit:      100,
    OnlineOnly: true,
}
members, err := manager.List(ctx, opts)
```

---

## 核心组件

| 组件 | 说明 |
|------|------|
| **Manager** | 成员管理核心，CRUD 操作 |
| **Cache** | LRU + TTL 双重缓存 |
| **Store** | 持久化存储（BadgerDB） |
| **Synchronizer** | 成员同步器 |
| **HeartbeatMonitor** | 心跳监控 |

---

## 缓存策略

**LRU + TTL 双重缓存**：
- LRU 淘汰：最少使用优先淘汰
- TTL 过期：时间过期自动清理
- 后台清理：自动清理过期条目
- 命中率：> 95%

---

## 心跳监控

```go
monitor := member.NewHeartbeatMonitor(manager, host, 15*time.Second, 3)
if err := monitor.Start(ctx); err != nil {
    log.Fatal(err)
}
```

| 参数 | 默认值 | 说明 |
|------|--------|------|
| 心跳间隔 | `15s` | 发送心跳间隔 |
| 失败阈值 | `3` | 连续失败次数 |

---

## 成员同步

```go
sync := member.NewSynchronizer(manager, discovery)
if err := sync.Start(ctx); err != nil {
    log.Fatal(err)
}

// 全量同步
if err := manager.SyncMembers(ctx); err != nil {
    log.Printf("Sync failed: %v", err)
}
```

| 同步类型 | 场景 |
|----------|------|
| 全量同步 | 首次加入 Realm |
| 增量同步 | 定期更新 |

---

## 性能指标

| 指标 | 目标 |
|------|------|
| 成员查询 | < 1ms（缓存命中） |
| 成员同步 | < 100ms（增量） |
| 心跳开销 | < 1KB/成员/分钟 |
| 缓存命中率 | > 95% |

---

## 测试

```bash
go test -v ./internal/realm/member/...
go test -cover ./internal/realm/member/...
```

---

## 相关文档

- [doc.go](doc.go) - 包文档

---

**最后更新**: 2026-01-20
