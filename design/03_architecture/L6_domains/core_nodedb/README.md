# core_nodedb 模块设计

> **版本**: v1.0.0  
> **更新日期**: 2026-01-18  
> **架构层**: Core Layer  
> **代码位置**: `internal/core/nodedb/`

---

## 概述

NodeDB 模块提供节点数据库实现，用于持久化节点信息以加速启动发现。

## 核心功能

1. **节点持久化** - 存储节点 ID、地址、最后活跃时间
2. **种子查询** - 按活跃时间查询可用节点
3. **Pong 记录** - 跟踪节点健康状态
4. **拨号管理** - 记录拨号尝试和失败次数
5. **自动清理** - 定期清理过期节点

## 核心接口

```go
type NodeDB interface {
    UpdateNode(node *NodeRecord) error
    GetNode(id string) *NodeRecord
    RemoveNode(id string) error
    QuerySeeds(count int, maxAge time.Duration) []*NodeRecord
    LastPongReceived(id string) time.Time
    UpdateLastPong(id string, t time.Time) error
    UpdateDialAttempt(id string, success bool) error
    Size() int
    Close() error
}
```

## 数据结构

```go
type NodeRecord struct {
    ID          string
    IP          net.IP
    UDP         int
    TCP         int
    Addrs       []string
    LastPong    time.Time
    LastSeen    time.Time
    FailedDials int
    LastDial    time.Time
}
```

## 配置

```go
type Config struct {
    MaxNodes        int           // 最大节点数，默认 10000
    NodeExpiry      time.Duration // 过期时间，默认 7 天
    CleanupInterval time.Duration // 清理间隔，默认 1 小时
    MaxFailedDials  int           // 最大失败次数，默认 5
}
```

## 实现

目前提供 `MemoryDB` 内存实现，适用于测试和开发。
生产环境可扩展使用 `Storage` 模块提供的持久化存储。

## 依赖关系

- 无外部依赖
- 可被 Discovery 层和 Peerstore 使用

## 来源

设计参考 go-ethereum `p2p/enode.DB`。
来源于 [20260118-additional-feature-absorption.md](../../../_discussions/20260118-additional-feature-absorption.md) Phase 9.3。
