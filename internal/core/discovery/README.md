# Discovery 节点发现模块

## 概述

**层级**: Tier 3  
**职责**: 提供多种节点发现机制，包括 DHT、mDNS 和 Bootstrap，支持动态发现间隔和 Realm 隔离。

## 设计引用

> **重要**: 实现前请详细阅读以下设计规范

| 设计文档 | 说明 |
|----------|------|
| [发现协议规范](../../../docs/01-design/protocols/network/01-discovery.md) | DHT、mDNS、Bootstrap 机制 |
| [节点存活协议](../../../docs/01-design/protocols/application/01-node-liveness.md) | 动态发现间隔算法 |
| [Realm 协议](../../../docs/01-design/protocols/application/04-realm.md) | Realm 感知的发现 |

## 能力清单

### DHT 发现能力 (必须实现)

| 能力 | 状态 | 说明 |
|------|------|------|
| Kademlia DHT | ✅ 已实现 | K-Bucket 路由表，K=20 |
| 节点查找 | ✅ 已实现 | O(log N) 查找复杂度 |
| 值存储/获取 | ✅ 已实现 | PutValue/GetValue |
| 引导启动 | ✅ 已实现 | Bootstrap 操作 |
| Realm 感知 Key | ✅ 已实现 | `Key = SHA256(RealmID \|\| NodeID)` |
| 迭代查询 | ✅ 已实现 | 并行迭代查找最近节点 |
| DHT 协议实现 | ✅ 已实现 | `/dep2p/sys/dht/1.0.0`（FIND_NODE/FIND_VALUE/STORE/PING/ADD_PROVIDER/GET_PROVIDERS/REMOVE_PROVIDER） |

### mDNS 发现能力 (必须实现)

| 能力 | 状态 | 说明 |
|------|------|------|
| 服务注册 | ✅ 已实现 | 注册 `_dep2p._udp.local` |
| 服务发现 | ✅ 已实现 | 监听本地网络节点 |
| 地址解析 | ✅ 已实现 | 解析发现的节点地址 |

### Bootstrap 能力 (必须实现)

| 能力 | 状态 | 说明 |
|------|------|------|
| 引导节点列表 | ✅ 已实现 | 预配置引导节点 |
| 动态引导 | ✅ 已实现 | 从 DNS/配置获取 |
| 引导重试 | ✅ 已实现 | 失败时重试逻辑 |

### 动态发现间隔能力 (必须实现)

| 能力 | 状态 | 说明 |
|------|------|------|
| 基于连接比例调整 | ✅ 已实现 | 连接少时加速发现 |
| 紧急加速 | ✅ 已实现 | 连接数骤降时触发 |
| 最大/最小间隔约束 | ✅ 已实现 | 防止过快或过慢 |

### Realm 感知能力 (可选)

| 能力 | 状态 | 说明 |
|------|------|------|
| Realm 隔离 | ✅ 已实现 | 不同 Realm 节点互不可见 |
| Private Realm | ✅ 已实现 | 私有 Realm 不可被外部发现 |
| 多 Realm 支持 | ✅ 已实现 | 节点同时属于多个 Realm |

## 依赖关系

### 接口依赖

```
pkg/types/              → NodeID, PeerInfo, RealmID
pkg/interfaces/core/    → Address 接口
pkg/interfaces/discovery/ → DiscoveryService, Discoverer, Announcer, DHT 接口
```

### 模块依赖

```
address   → 地址管理
transport → 网络连接
protocol  → 协议协商
```

### 第三方依赖

```
github.com/grandcat/zeroconf → mDNS 实现
```

## 目录结构

```
discovery/
├── README.md            # 本文件
├── module.go            # fx 模块定义
├── dht/                 # DHT 实现
│   ├── README.md        # DHT 子模块说明
│   ├── dht.go           # DHT 服务
│   ├── routing.go       # 路由表 (K-Bucket)
│   └── query.go         # 查询逻辑
├── mdns/                # mDNS 实现
│   ├── README.md        # mDNS 子模块说明
│   └── mdns.go          # mDNS 发现
└── bootstrap/           # Bootstrap 实现
    ├── README.md        # Bootstrap 子模块说明
    └── bootstrap.go     # 引导服务
```

## 公共接口

实现 `pkg/interfaces/discovery/` 中的接口：

```go
// DiscoveryService 发现服务接口
type DiscoveryService interface {
    Discoverer
    Announcer
    
    // Start 启动发现服务
    Start(ctx context.Context) error
    
    // Stop 停止发现服务
    Stop() error
    
    // RegisterDiscoverer 注册发现器
    RegisterDiscoverer(name string, discoverer Discoverer)
    
    // RegisterAnnouncer 注册通告器
    RegisterAnnouncer(name string, announcer Announcer)
}

// Discoverer 发现器接口
type Discoverer interface {
    // FindPeer 查找指定节点的地址
    FindPeer(ctx context.Context, id types.NodeID) ([]core.Address, error)
    
    // FindPeers 批量查找
    FindPeers(ctx context.Context, ids []types.NodeID) (map[types.NodeID][]core.Address, error)
    
    // FindClosestPeers 查找最近的节点
    FindClosestPeers(ctx context.Context, key []byte, count int) ([]types.NodeID, error)
    
    // DiscoverPeers 发现新节点
    DiscoverPeers(ctx context.Context, namespace string) (<-chan PeerInfo, error)
}

// DHT 分布式哈希表接口
type DHT interface {
    Discoverer
    
    // PutValue 存储值
    PutValue(ctx context.Context, key string, value []byte) error
    
    // GetValue 获取值
    GetValue(ctx context.Context, key string) ([]byte, error)
    
    // Bootstrap 执行引导
    Bootstrap(ctx context.Context) error
}
```

## 关键算法

### Kademlia 距离计算 (来自设计文档)

```go
// XOR 距离
func distance(a, b types.NodeID) *big.Int {
    result := new(big.Int)
    for i := 0; i < len(a); i++ {
        result.SetBit(result, i*8, uint(a[i]^b[i]))
    }
    return result
}

// K-Bucket 索引计算
func bucketIndex(self, target types.NodeID) int {
    dist := distance(self, target)
    return 256 - dist.BitLen()  // 前导零数量
}
```

### Realm 感知的 DHT Key (来自设计文档)

```go
func realmAwareDHTKey(realmID types.RealmID, nodeID types.NodeID) []byte {
    // 无 Realm: Key = SHA256(NodeID)
    // 有 Realm: Key = SHA256(RealmID || NodeID)
    
    if realmID == types.DefaultRealmID {
        hash := sha256.Sum256(nodeID[:])
        return hash[:]
    }
    
    data := append(realmID[:], nodeID[:]...)
    hash := sha256.Sum256(data)
    return hash[:]
}
```

### 动态发现间隔算法 (来自设计文档)

```go
const (
    BaseInterval   = 30 * time.Second  // 基础间隔
    MinInterval    = 5 * time.Second   // 最小间隔
    MaxInterval    = 5 * time.Minute   // 最大间隔
    TargetPeerCount = 50               // 目标连接数
)

func calculateDiscoveryInterval(currentPeers, targetPeers int) time.Duration {
    if targetPeers == 0 {
        return BaseInterval
    }
    
    ratio := float64(currentPeers) / float64(targetPeers)
    
    switch {
    case ratio < 0.3:
        // 紧急模式：连接数严重不足
        return MinInterval
    case ratio < 0.5:
        // 加速模式
        return BaseInterval / 2
    case ratio > 0.9:
        // 减速模式：已接近目标
        return BaseInterval * 2
    default:
        return BaseInterval
    }
}
```

### 紧急恢复机制 (来自设计文档)

```go
// 触发条件:
// 1. 连接数在 5 分钟内下降超过 50%
// 2. 连续 3 次发现失败
// 3. 检测到网络分区
// 4. 核心邻居数量 < 3

type EmergencyRecovery struct {
    triggered      bool
    triggerTime    time.Time
    originalPeers  int
    recoveryMode   bool
}

func (e *EmergencyRecovery) Check(currentPeers int, history []int) bool {
    // 检测连接数骤降
    if len(history) > 0 {
        oldPeers := history[0]
        if float64(currentPeers)/float64(oldPeers) < 0.5 {
            return true  // 触发紧急恢复
        }
    }
    return false
}

// 恢复动作:
// 1. 发现间隔设为最小值 (5秒)
// 2. 并行连接多个 Bootstrap 节点
// 3. 主动发起 DHT 查找
// 4. 持续直到连接数恢复到 LowWater
```

## fx 模块

```go
type ModuleInput struct {
    fx.In
    Address   addressif.AddressBook    `name:"address_book"`
    Transport transportif.Transport    `name:"transport"`
    Config    *discoveryif.Config      `optional:"true"`
}

type ModuleOutput struct {
    fx.Out
    DiscoveryService discoveryif.DiscoveryService `name:"discovery"`
    DHT              discoveryif.DHT              `name:"dht" optional:"true"`
}

func Module() fx.Option {
    return fx.Module("discovery",
        fx.Provide(ProvideServices),
        fx.Invoke(registerLifecycle),
    )
}
```

## 配置参数

```go
type Config struct {
    // DHT 配置
    EnableDHT       bool
    BucketSize      int           // K=20
    Alpha           int           // 并行度，默认 3
    
    // mDNS 配置
    EnableMDNS      bool
    MDNSServiceName string        // _dep2p._udp.local
    
    // Bootstrap 配置
    EnableBootstrap bool
    BootstrapPeers  []string      // 引导节点列表
    
    // 动态发现配置
    BaseInterval    time.Duration // 30s
    MinInterval     time.Duration // 5s
    MaxInterval     time.Duration // 5min
    TargetPeers     int           // 50
}
```

## 相关文档

- [发现协议规范](../../../docs/01-design/protocols/network/01-discovery.md)
- [节点存活协议](../../../docs/01-design/protocols/application/01-node-liveness.md)
- [Realm 协议](../../../docs/01-design/protocols/application/04-realm.md)
- [pkg/interfaces/discovery](../../../pkg/interfaces/discovery/)
