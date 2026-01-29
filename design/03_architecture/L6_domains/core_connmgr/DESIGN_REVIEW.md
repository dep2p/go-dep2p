# Core ConnMgr 设计审查

> **审查日期**: 2026-01-13  
> **审查人**: AI Agent  
> **版本**: v1.0.0

---

## 一、设计文档审查

### 1.1 模块定位

**架构层**: Core Layer  
**职责**: 连接池管理、优先级控制、保护策略、门控拦截

**核心目标**:
- 维持连接数在合理范围（水位控制）
- 保护关键连接不被回收
- 基于标签的优先级管理
- 连接建立的多阶段拦截

### 1.2 功能需求对照

| 需求ID | 描述 | 优先级 | 设计方案 |
|--------|------|--------|---------|
| FR-CM-001 | 水位控制 | P0 | LowWater/HighWater 机制 |
| FR-CM-002 | 连接保护 | P0 | 标签保护存储 |
| FR-CM-003 | 优先级标签 | P1 | 标签权重累加 |
| FR-CM-004 | 连接门控 | P1 | 多阶段拦截接口 |

### 1.3 非功能需求

| 需求ID | 描述 | 设计方案 |
|--------|------|---------|
| NFR-CM-001 | 性能 | 异步回收，不阻塞 |
| NFR-CM-002 | 并发安全 | RWMutex 保护共享状态 |

---

## 二、go-libp2p BasicConnMgr 分析

### 2.1 核心设计

**BasicConnMgr 特点**:
- 分段锁设计（256 个 segment）- 减少锁竞争
- 衰减标签（DecayingTag）- 标签权重随时间衰减
- 优雅期（GracePeriod）- 新连接保护期
- 静默期（SilencePeriod）- 回收间隔限制
- 内存压力监控 - 低内存时强制回收

### 2.2 数据结构

```go
// go-libp2p 实现
type BasicConnMgr struct {
    *decayer                    // 衰减器
    clock    clock.Clock        // 时钟
    cfg      *config            // 配置
    segments segments           // 分段存储（256个）
    protected map[peer.ID]map[string]struct{}  // 保护存储
    trimMutex sync.Mutex        // 回收互斥锁
    connCount atomic.Int32      // 连接计数
    lastTrim  time.Time         // 上次回收时间
}

type segment struct {
    sync.Mutex
    peers map[peer.ID]*peerInfo
}

type peerInfo struct {
    id        peer.ID
    firstSeen time.Time
    tags      map[string]int
    decaying  map[*decayingTag]*DecayingValue
    conns     map[network.Conn]time.Time
    temp      bool
}
```

### 2.3 水位控制算法

```go
// 触发条件
if connCount > highWater {
    TrimOpenConns()
}

// 回收逻辑
target := connCount - lowWater
for _, conn := range sortedByScore {
    if count >= target {
        break
    }
    if !isProtected(conn.Peer) {
        conn.Close()
        count++
    }
}
```

### 2.4 优先级计算

```go
// 评分公式
score := 0

// 1. 标签权重
for _, tagValue := range peer.Tags {
    score += tagValue
}

// 2. 衰减标签
for _, decayValue := range peer.Decaying {
    score += decayValue.Value
}

// 3. 时间因素
age := now.Sub(peer.FirstSeen)
if age < gracePeriod {
    score += 1000  // 新连接保护
}

// 4. 方向因素（隐含在标签中）
// libp2p 没有显式的方向加分，通过标签实现
```

---

## 三、DeP2P 设计方案

### 3.1 简化设计

**相比 go-libp2p 的简化**:
- ❌ 不实现衰减标签（DecayingTag）- 降低复杂度
- ❌ 不实现分段锁（segments）- 简化实现
- ❌ 不实现内存压力监控 - 专注核心功能
- ✅ 保留水位控制
- ✅ 保留标签系统
- ✅ 保留保护机制
- ✅ 保留门控接口

**理由**:
- 衰减标签适用于长期运行场景，DeP2P 初期不需要
- 分段锁优化在连接数 < 10000 时收益不大
- 内存监控可后续添加

### 3.2 核心数据结构

```go
// DeP2P 设计
type Manager struct {
    cfg      Config
    tags     *tagStore          // 标签存储
    protects *protectStore      // 保护存储
    
    mu       sync.RWMutex       // 全局锁（简化设计）
    closed   bool
}

type tagStore struct {
    mu   sync.RWMutex
    tags map[string]map[string]int  // peerID -> tag -> value
    firstSeen map[string]time.Time  // peerID -> time
}

type protectStore struct {
    mu       sync.RWMutex
    protects map[string]map[string]struct{}  // peerID -> tags
}
```

### 3.3 优先级计算（DeP2P）

```go
func (m *Manager) calculateScore(peer string) int {
    score := 0
    
    // 1. 标签权重累加
    score += m.tags.Sum(peer)
    
    // 2. 方向加分（通过 peerstore 获取）
    if m.isOutbound(peer) {
        score += 10
    }
    
    // 3. 流计数加分
    if m.hasActiveStreams(peer) {
        score += 20
    }
    
    return score
}
```

### 3.4 回收算法（DeP2P）

```go
func (m *Manager) TrimOpenConns(ctx context.Context) {
    // 1. 获取连接列表（通过 Host 接口）
    conns := m.getConnections()
    
    if len(conns) <= m.cfg.LowWater {
        return  // 低于低水位，不回收
    }
    
    // 2. 收集可回收候选
    candidates := []peerScore{}
    for _, conn := range conns {
        peer := conn.RemotePeer()
        if m.protects.HasAnyProtection(peer) {
            continue  // 跳过受保护
        }
        candidates = append(candidates, peerScore{
            peer:  peer,
            score: m.calculateScore(peer),
        })
    }
    
    // 3. 按分数排序（升序）
    sort.Slice(candidates, func(i, j int) bool {
        return candidates[i].score < candidates[j].score
    })
    
    // 4. 关闭低分连接
    toClose := len(conns) - m.cfg.LowWater
    for i := 0; i < toClose && i < len(candidates); i++ {
        m.closeConnection(candidates[i].peer)
    }
}
```

---

## 四、门控设计

### 4.1 拦截点

```
连接生命周期拦截点：

1. InterceptPeerDial     - 拨号前（基于 PeerID）
2. InterceptAddrDial     - 拨号前（基于地址）
3. InterceptAccept       - 接受入站前
4. InterceptSecured      - 安全握手后
5. InterceptUpgraded     - 连接升级后
```

### 4.2 Gater 实现

```go
type Gater struct {
    blocked map[string]struct{}  // 黑名单
    mu      sync.RWMutex
}

func (g *Gater) InterceptPeerDial(peerID string) bool {
    g.mu.RLock()
    defer g.mu.RUnlock()
    
    _, blocked := g.blocked[peerID]
    return !blocked  // 返回 true 表示允许
}

func (g *Gater) InterceptAccept(conn Connection) bool {
    // 可基于 IP、端口等过滤
    return true
}
```

---

## 五、配置参数

| 参数 | 默认值 | 说明 |
|------|--------|------|
| LowWater | 100 | 低水位（目标连接数）|
| HighWater | 400 | 高水位（触发回收）|
| GracePeriod | 20s | 新连接保护期 |

**水位配置原则**:
- `LowWater`：期望维持的连接数
- `HighWater`：触发回收的阈值
- 建议比例：`HighWater = 3-4 * LowWater`

---

## 六、与其他模块的交互

### 6.1 依赖关系

```
core_connmgr 依赖：
  ├── core_peerstore - 获取节点信息、连接列表
  └── core_eventbus  - 发布连接事件

被依赖：
  ├── core_swarm     - 使用 connmgr 管理连接
  └── core_host      - 集成 connmgr
```

### 6.2 事件交互

**订阅事件**:
- `PeerConnected` - 节点连接时
- `PeerDisconnected` - 节点断开时

**发布事件**:
- `ConnectionTrimmed` - 连接被回收时

---

## 七、关键设计决策

### 7.1 采用的设计

| 设计点 | 决策 | 理由 |
|--------|------|------|
| 锁策略 | 全局 RWMutex | 简化实现，性能足够 |
| 标签系统 | 静态标签 | 不需要衰减，降低复杂度 |
| 回收触发 | 被动触发 | 不需要后台定时器 |
| 保护机制 | 标签保护 | 灵活且易用 |

### 7.2 未采用的设计

| 设计点 | 未采用原因 |
|--------|-----------|
| 分段锁 | 连接数 < 10000 时收益小 |
| 衰减标签 | 初期不需要，后续可添加 |
| 内存监控 | 专注核心功能 |
| 后台定时回收 | 被动回收更高效 |

---

## 八、实现计划

### 8.1 核心文件

1. `tags.go` - 标签存储（~80 行）
2. `protect.go` - 保护存储（~60 行）
3. `manager.go` - 管理器主逻辑（~150 行）
4. `trim.go` - 回收算法（~100 行）
5. `gater.go` - 门控实现（~80 行）
6. `config.go` - 配置（~40 行）
7. `errors.go` - 错误定义（~20 行）
8. `module.go` - Fx 模块（~50 行）
9. `doc.go` - 包文档（~60 行）

**总代码量**: 约 640 行

### 8.2 测试覆盖

**目标覆盖率**: 70%+

**关键测试**:
- 标签操作测试
- 保护机制测试
- 水位回收测试
- 优先级排序测试
- 门控拦截测试
- 并发安全测试

---

## 九、风险与挑战

### 9.1 技术风险

| 风险 | 影响 | 缓解措施 |
|------|------|---------|
| 回收逻辑错误 | 关键连接被关闭 | 保护机制 + 充分测试 |
| 并发竞争 | 数据不一致 | RWMutex + 单元测试 |
| 性能瓶颈 | 大量连接时慢 | 性能测试 + 优化 |

### 9.2 集成挑战

| 挑战 | 说明 | 解决方案 |
|------|------|---------|
| Host 接口缺失 | 需要获取连接列表 | 定义最小接口 |
| 事件通知时机 | 何时触发回收 | 在 Swarm 层集成 |

---

## 十、总结

### 10.1 设计优点

✅ **简化务实** - 去除不必要的复杂度  
✅ **清晰分层** - 标签、保护、回收分离  
✅ **易于测试** - 每个组件独立可测  
✅ **扩展性好** - 后续可添加衰减、分段锁

### 10.2 与 go-libp2p 对比

| 维度 | go-libp2p | DeP2P | 说明 |
|------|-----------|-------|------|
| 复杂度 | 高 | 中 | 简化设计 |
| 性能 | 优秀 | 良好 | 满足需求 |
| 功能 | 完整 | 核心 | 专注核心 |
| 可维护性 | 中 | 高 | 代码更少 |

### 10.3 下一步

1. ✅ 设计审查完成
2. ⏭️ 接口验证
3. ⏭️ 测试先行
4. ⏭️ 核心实现

---

**审查结论**: ✅ **设计合理，可以开始实施**

**审查完成时间**: 2026-01-13
