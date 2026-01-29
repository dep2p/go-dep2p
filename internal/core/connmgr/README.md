# Core ConnMgr - 连接管理器

> **版本**: v1.0.0  
> **状态**: ✅ 已完成  
> **覆盖率**: 86.6%

---

## 概述

`connmgr` 模块实现连接管理器，负责连接池水位控制、优先级管理、保护机制和连接门控。

**核心功能**:
- 🌊 水位控制 - 自动回收多余连接
- 🛡️ 连接保护 - 保护关键连接不被回收
- 🏆 优先级管理 - 基于标签的优先级
- 🚪 连接门控 - 多阶段拦截和过滤

---

## 快速开始

### 创建连接管理器

```go
import "github.com/dep2p/go-dep2p/internal/core/connmgr"

cfg := connmgr.Config{
    LowWater:    100,                // 低水位（目标连接数）
    HighWater:   400,                // 高水位（触发回收）
    GracePeriod: 20 * time.Second,   // 新连接保护期
}

mgr, err := connmgr.New(cfg)
if err != nil {
    log.Fatal(err)
}
defer mgr.Close()
```

### 标签管理

```go
// 添加标签
mgr.TagPeer("peer-1", "bootstrap", 50)
mgr.TagPeer("peer-1", "relay", 50)

// 移除标签
mgr.UntagPeer("peer-1", "relay")

// 更新标签
mgr.UpsertTag("peer-1", "score", func(old int) int {
    return old + 10
})

// 获取标签信息
info := mgr.GetTagInfo("peer-1")
fmt.Printf("Total score: %d\n", info.Value)
```

### 连接保护

```go
// 保护重要连接
mgr.Protect("peer-1", "important")

// 取消保护
hasMore := mgr.Unprotect("peer-1", "important")

// 检查保护状态
if mgr.IsProtected("peer-1", "important") {
    // 连接受保护
}
```

### 手动触发回收

```go
ctx := context.Background()
mgr.TrimOpenConns(ctx)
```

### 连接门控

```go
import "github.com/dep2p/go-dep2p/internal/core/connmgr"

gater := connmgr.NewGater()

// 阻止节点
gater.BlockPeer("bad-peer")

// 拨号时会被拦截
if !gater.InterceptPeerDial("bad-peer") {
    // 拨号被拒绝
}

// 解除阻止
gater.UnblockPeer("bad-peer")
```

---

## API 文档

### Manager 接口

```go
type Manager struct { ... }

// New 创建连接管理器
func New(cfg Config) (*Manager, error)

// TagPeer 为节点添加标签
func (m *Manager) TagPeer(peerID string, tag string, weight int)

// UntagPeer 移除节点标签
func (m *Manager) UntagPeer(peerID string, tag string)

// UpsertTag 更新或插入节点标签
func (m *Manager) UpsertTag(peerID string, tag string, upsert func(int) int)

// GetTagInfo 获取节点的标签信息
func (m *Manager) GetTagInfo(peerID string) *TagInfo

// Protect 保护节点连接不被裁剪
func (m *Manager) Protect(peerID string, tag string)

// Unprotect 取消节点保护
func (m *Manager) Unprotect(peerID string, tag string) bool

// IsProtected 检查节点是否受保护
func (m *Manager) IsProtected(peerID string, tag string) bool

// TrimOpenConns 裁剪连接到目标数量
func (m *Manager) TrimOpenConns(ctx context.Context)

// Close 关闭连接管理器
func (m *Manager) Close() error
```

### Gater 接口

```go
type Gater struct { ... }

// NewGater 创建连接门控器
func NewGater() *Gater

// BlockPeer 阻止节点
func (g *Gater) BlockPeer(peer string)

// UnblockPeer 解除节点阻止
func (g *Gater) UnblockPeer(peer string)

// InterceptPeerDial 在拨号前检查是否允许
func (g *Gater) InterceptPeerDial(peerID string) bool

// InterceptAddrDial 在拨号前检查是否允许连接到目标地址
func (g *Gater) InterceptAddrDial(peerID string, addr string) bool

// InterceptAccept 在接受连接前检查是否允许
func (g *Gater) InterceptAccept(conn Connection) bool

// InterceptSecured 在安全握手后检查是否允许
func (g *Gater) InterceptSecured(dir Direction, peerID string, conn Connection) bool

// InterceptUpgraded 在连接升级后检查是否允许
func (g *Gater) InterceptUpgraded(conn Connection) (bool, error)
```

---

## 水位控制机制

```
连接数 ≤ LowWater (100)       → 不回收
LowWater < 连接数 ≤ HighWater → 可能回收
连接数 > HighWater (400)      → 触发 Trim，回收至 LowWater
```

### 回收流程

1. 获取所有连接
2. 过滤受保护的连接
3. 计算每个连接的优先级分数
4. 按分数排序（升序）
5. 关闭低分连接，直到达到低水位

---

## 优先级计算

**评分公式**:
```
Priority = Σ(TagScores)
```

**常用标签及权重**:
- `"bootstrap"`: 50 - 引导节点
- `"relay"`: 50 - 中继节点
- `"realm-member"`: 100 - Realm 成员
- `"dht-routing"`: 30 - DHT 路由表节点

**示例**:
```go
// 设置优先级
mgr.TagPeer("peer-1", "bootstrap", 50)
mgr.TagPeer("peer-1", "relay", 50)
// 总分 = 50 + 50 = 100

mgr.TagPeer("peer-2", "realm-member", 100)
mgr.TagPeer("peer-2", "relay", 50)
// 总分 = 100 + 50 = 150（更高优先级，不易被回收）
```

---

## 测试结果

### 单元测试

✅ **25/25 通过** (10 个跳过)

**Manager 测试**:
- ✅ TestManager_New - 创建管理器
- ✅ TestManager_TagPeer - 标签操作
- ✅ TestManager_UntagPeer - 移除标签
- ✅ TestManager_UpsertTag - 更新标签
- ✅ TestManager_Protect - 保护连接
- ✅ TestManager_Unprotect - 取消保护
- ✅ TestManager_Close - 关闭
- ✅ TestManager_Concurrent - 并发安全

**Gater 测试**:
- ✅ TestGater_InterceptPeerDial - 拦截拨号
- ✅ TestGater_InterceptAddrDial - 拦截地址拨号
- ✅ TestGater_BlockUnblock - 阻止和解除阻止
- ✅ TestGater_MultipleBlocks - 多节点阻止
- ✅ TestGater_Concurrent - 并发安全

**Trim 测试**:
- ✅ TestManager_CalculateScore - 分数计算
- ✅ TestManager_GetConnsToClose - 获取需要关闭的连接
- ✅ TestManager_TrimWithProtection - 保护机制
- ✅ TestManager_TrimBelowLowWater - 低于低水位不回收

**覆盖率**: **86.6%** ✅

---

## 架构

### 组件关系

```
┌─────────────────────────────────────────┐
│          Manager (连接管理器)            │
├─────────────────────────────────────────┤
│  - TagPeer()                            │
│  - Protect()                            │
│  - TrimOpenConns()                      │
└─────────────┬───────────────────────────┘
              │
      ┌───────┴────────┐
      │                │
┌─────▼─────┐    ┌────▼──────┐
│ tagStore  │    │protectStore│
│ (标签存储)│    │ (保护存储) │
└───────────┘    └────────────┘

┌─────────────────────────────────────────┐
│           Gater (连接门控)               │
├─────────────────────────────────────────┤
│  - InterceptPeerDial()                  │
│  - InterceptAccept()                    │
│  - InterceptSecured()                   │
└─────────────────────────────────────────┘
```

### 依赖关系

```
connmgr 依赖：
  ├── peerstore (可选) - 获取节点信息
  └── eventbus (可选) - 发布连接事件

被依赖：
  ├── swarm - 使用 connmgr 管理连接
  └── host - 集成 connmgr
```

---

## 性能

- **标签操作**: O(1) 时间复杂度
- **回收操作**: O(n log n) 时间复杂度（排序）
- **保护检查**: O(1) 时间复杂度
- **并发安全**: 所有方法都是线程安全的

---

## 注意事项

1. ⚠️ **保护优先**: 受保护的连接永远不会被回收
2. ⚠️ **异步回收**: `TrimOpenConns` 应在后台调用
3. ⚠️ **上下文取消**: 回收支持通过 context 取消
4. ⚠️ **配置验证**: 创建时会验证配置（LowWater < HighWater）

---

## 未来扩展

- [ ] 衰减标签 - 标签权重随时间衰减
- [ ] 分段锁 - 减少锁竞争（连接数 > 10000 时）
- [ ] 内存监控 - 低内存时强制回收
- [ ] 后台定时回收 - 定期检查并回收

---

**最后更新**: 2026-01-13
