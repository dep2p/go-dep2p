# Address 地址管理模块

## 概述

**层级**: Tier 1  
**职责**: 管理节点地址，包括地址簿、地址生命周期、地址签名与验证、多源地址合并、地址优先级与健康度维护，并承载地址管理内部协议。

## 设计引用

> **重要**: 实现前请详细阅读以下设计规范

| 设计文档 | 说明 |
|----------|------|
| [地址协议规范](../../../docs/01-design/protocols/foundation/02-address.md) | 地址格式与类型 |
| [地址管理协议](../../../docs/01-design/protocols/network/04-address-management.md) | 地址簿与状态机 |

## 能力清单

### 地址簿能力 (必须实现)

| 能力 | 状态 | 说明 |
|------|------|------|
| 地址存储 | ✅ 已实现 | 存储节点地址映射 |
| 地址查询 | ✅ 已实现 | 按 NodeID 查询地址 |
| TTL 管理 | ✅ 已实现 | 地址过期清理 |
| 地址更新 | ✅ 已实现 | 添加/更新地址 |
| 多源地址合并 | ✅ 已实现 | DHT/mDNS/邻居/直连/手动合并与冲突处理 |
| 地址验证任务 | ✅ 已实现 | 周期性验证可达性与更新统计（成功/失败/RTT） |
| 失效地址清理 | ✅ 已实现 | 连续失败阈值触发降级/移除 |

### 地址格式能力 (必须实现)

| 能力 | 状态 | 说明 |
|------|------|------|
| 简单格式解析 | ✅ 已实现 | `quic://ip:port` |
| Multiaddr 解析 | ✅ 已实现 | `/ip4/x.x.x.x/udp/port/quic-v1` |
| 地址序列化 | ✅ 已实现 | 地址编码/解码 |

### 地址签名能力 (必须实现)

| 能力 | 状态 | 说明 |
|------|------|------|
| 地址记录签名 | ✅ 已实现 | 签名防伪造 |
| 签名验证 | ✅ 已实现 | 验证地址记录 |
| 序列号管理 | ✅ 已实现 | 防重放攻击 |

### 地址优先级能力

| 能力 | 状态 | 说明 |
|------|------|------|
| 地址排序 | ✅ 已实现 | 按优先级排序（公网>局域网>NAT映射>中继） |
| 地址状态机 | ✅ 已实现 | Unknown/Verified/Reachable/Degraded/Unreachable 等 |
| 优先级评分 | ✅ 已实现 | `Priority = Base + SuccessBonus - FailPenalty - RTTFactor` |

### 地址管理协议 (必须实现)

| 能力 | 状态 | 说明 |
|------|------|------|
| 地址刷新通知 | ✅ 已实现 | `/dep2p/addr-mgmt/1.0` AddressRefreshNotify |
| 地址查询请求/响应 | ✅ 已实现 | `/dep2p/addr-mgmt/1.0` AddressQueryRequest/Response |
| 邻居询问恢复 | ✅ 已实现 | 地址缺失时向 K 个邻居并行询问 |
| 签名验证与序列号冲突处理 | ✅ 已实现 | 无效签名拒绝；序列号新旧判定 |

## 依赖关系

### 接口依赖

```
pkg/types/              → NodeID, Address
pkg/interfaces/core/    → Address, AddressBook
```

### 模块依赖

```
identity → 地址签名与验证
discovery → 地址来源之一（DHT/mDNS/Bootstrap）
connmgr → 连接状态辅助判断（是否已连接）
protocol → 地址管理协议的分发（Router）
```

## 目录结构

```
address/
├── README.md            # 本文件
├── module.go            # fx 模块定义
├── book.go              # 地址簿实现
├── manager.go           # 地址管理器
├── parser.go            # 地址格式解析
├── priority.go          # 地址优先级计算
├── record.go            # 签名地址记录
├── validator.go         # 地址验证器
└── addrmgmt/            # 地址管理协议（/dep2p/addr-mgmt/1.0）✅ 已实现
    ├── handler.go       # 协议处理器（refresh/query）
    └── scheduler.go     # 刷新/验证/清理任务调度
```

## 公共接口

实现 `pkg/interfaces/core/` 中的 AddressBook 接口：

```go
// AddressBook 地址簿接口
type AddressBook interface {
    // Addrs 获取节点的所有地址
    Addrs(id types.NodeID) []Address
    
    // AddAddrs 添加节点地址
    AddAddrs(id types.NodeID, addrs []Address, ttl time.Duration)
    
    // SetAddrs 设置节点地址（覆盖）
    SetAddrs(id types.NodeID, addrs []Address, ttl time.Duration)
    
    // UpdateAddrs 更新地址 TTL
    UpdateAddrs(id types.NodeID, oldTTL, newTTL time.Duration)
    
    // ClearAddrs 清除节点地址
    ClearAddrs(id types.NodeID)
    
    // PeersWithAddrs 返回有地址的节点列表
    PeersWithAddrs() []types.NodeID
}
```

## 关键算法

### 地址格式 (来自设计文档)

```
简单格式:
<transport>://<host>:<port>
示例:
├── quic://192.168.1.100:4001
├── quic://[2001:db8::1]:4001
└── tcp://example.com:4001

Multiaddr 格式:
/<proto>/<value>/<proto>/<value>/...
示例:
├── /ip4/192.168.1.100/udp/4001/quic-v1
├── /ip6/2001:db8::1/udp/4001/quic-v1
├── /dns4/example.com/tcp/4001
└── /ip4/192.168.1.100/udp/4001/quic-v1/p2p/5Q2STWvB...
```

### 地址类型与优先级 (来自设计文档)

```
优先级从高到低:
1. 公网地址 (Public)     → 直接可达
2. 局域网地址 (LAN)      → 局域网内可达
3. NAT 映射地址          → 需要 NAT 穿透
4. 中继地址 (Relay)      → 通过中继到达
```

### 地址优先级算法 (来自设计文档)

```
Priority = BasePriority + SuccessBonus - FailPenalty - RTTFactor

BasePriority（基于地址类型）：
├── 公网直连地址:  80
├── 同局域网地址:  70
├── NAT 映射地址:  60
└── 中继地址:      40

SuccessBonus：min(SuccessCount / TotalAttempts * 20, 20)
FailPenalty： min(ConsecutiveFails * 10, 50)
RTTFactor：   RTT <50ms:0, <100ms:5, <200ms:10, >=200ms:20
```

### 签名地址记录

```go
type AddressRecord struct {
    NodeID    types.NodeID  // 节点 ID
    RealmID   types.RealmID // 领域 ID（可选）
    Sequence  uint64        // 序列号（防重放）
    Addresses []Address     // 地址列表
    Timestamp time.Time     // 创建时间
    TTL       time.Duration // 有效期
    Signature []byte        // 签名
}

// 签名数据 = NodeID + RealmID + Sequence + Addresses + Timestamp + TTL
func (r *AddressRecord) Sign(identity Identity) error {
    data := r.signedData()
    sig, err := identity.Sign(data)
    if err != nil {
        return err
    }
    r.Signature = sig
    return nil
}

func (r *AddressRecord) Verify(pubKey crypto.PublicKey) bool {
    data := r.signedData()
    return pubKey.Verify(data, r.Signature)
}
```

### 地址状态机 (来自设计文档)

```
                    验证成功
    ┌──────────┐  ──────────►  ┌───────────┐
    │ Unknown  │               │  Pending  │
    └──────────┘  ◄──────────  └─────┬─────┘
                    超时               │
                                      │ 连接成功
                                      ▼
    ┌──────────┐  ◄──────────  ┌───────────┐
    │ Invalid  │    连接失败    │ Available │
    └──────────┘               └───────────┘
                                      │
                                      │ 多次失败
                                      ▼
                               ┌───────────┐
                               │Unreachable│
                               └───────────┘
```

### 地址 TTL 管理

```go
type addressEntry struct {
    addr      Address
    ttl       time.Duration
    addedAt   time.Time
    expiresAt time.Time
    state     AddressState
}

func (ab *addressBook) cleanup() {
    ab.mu.Lock()
    defer ab.mu.Unlock()
    
    now := time.Now()
    for nodeID, entries := range ab.addrs {
        valid := entries[:0]
        for _, entry := range entries {
            if entry.expiresAt.After(now) {
                valid = append(valid, entry)
            }
        }
        if len(valid) == 0 {
            delete(ab.addrs, nodeID)
        } else {
            ab.addrs[nodeID] = valid
        }
    }
}
```

### TTL 刷新与宽限期 (来自设计文档)

```
AddressTTL      = 1h
RefreshInterval = 30m
GracePeriod     = 10m
MaxStaleTime    = 2h

有效:     now < ts + ttl
宽限期:   ts+ttl <= now < ts+ttl+grace
过期:     ts+ttl+grace <= now < ts+maxStale
失效:     now >= ts+maxStale
```

### 多源地址合并策略 (来自设计文档)

```
来源优先级（冲突解决）：
1. Direct（直接连接获取）
2. DHT（权威存储）
3. Neighbor（邻居通知）
4. mDNS（本地发现）
5. Manual（手动配置）

合并规则：
1) 序列号大者优先（若有签名记录）
2) 序列号相同则时间戳新者优先
3) 地址集合并去重，并保留统计信息
4) 记录每个地址来源用于调试
```

## fx 模块

```go
type ModuleInput struct {
    fx.In
    Config *addressif.Config `optional:"true"`
}

type ModuleOutput struct {
    fx.Out
    AddressBook core.AddressBook `name:"address_book"`
}

func Module() fx.Option {
    return fx.Module("address",
        fx.Provide(ProvideServices),
        fx.Invoke(registerLifecycle),
    )
}
```

## 配置参数

```go
type Config struct {
    // TTL 配置
    DefaultTTL    time.Duration  // 默认 TTL 1h
    MaxTTL        time.Duration  // 最大 TTL 24h
    
    // 清理配置
    CleanupInterval time.Duration  // 清理间隔 10min
    
    // 容量配置
    MaxAddrsPerPeer int            // 每节点最大地址数 20
}
```

## 相关文档

- [地址协议规范](../../../docs/01-design/protocols/foundation/02-address.md)
- [地址管理协议](../../../docs/01-design/protocols/network/04-address-management.md)
- [pkg/interfaces/core](../../../pkg/interfaces/core/)
