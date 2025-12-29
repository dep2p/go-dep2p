# DHT 分布式哈希表实现

## 概述

基于 Kademlia 算法的分布式哈希表实现，用于去中心化的节点发现和数据存储。

## 文件结构

```
dht/
├── README.md              # 本文件
├── dht.go                 # 核心类型定义、配置、构造函数
├── dht_lifecycle.go       # 生命周期管理（Start, Stop, Bootstrap）
├── dht_query.go           # 查询操作（FindPeer, FindPeers, DiscoverPeers）
├── dht_values.go          # 值存储（PutValue, GetValue）
├── dht_providers.go       # Provider 管理（AddProvider, GetProviders, Announce）
├── dht_peerrecord.go      # PeerRecord 发布（PublishPeerRecord, LookupPeerRecord）
├── dht_address.go         # 地址工具类型（stringAddress）
├── dht_routing.go         # 路由表管理（AddNode, RemoveNode, UpdateNode）
├── dht_network.go         # 网络/身份注入（SetNetwork, SetIdentity）
├── routing.go             # Kademlia 路由表实现
├── keys.go                # Key 生成与哈希
├── handler.go             # 请求处理器
├── network_adapter.go     # 网络适配器
├── peer_record.go         # SignedPeerRecord 实现
├── module.go              # Fx 模块定义
└── *_test.go              # 测试文件
```

## 模块职责

### dht.go - 核心定义

- `DHT` 结构体定义
- `Config` 配置结构体
- `Network` 接口定义
- `Identity` / `IdentityWithPubKey` 接口
- `NewDHT` 构造函数
- 接口断言

### dht_lifecycle.go - 生命周期

- `Start(ctx)` - 启动 DHT
- `Stop()` / `Close()` - 停止 DHT
- `Bootstrap(ctx)` - 引导流程
- `refreshLoop()` / `refreshBuckets()` - 桶刷新
- `cleanupLoop()` / `cleanupStore()` - 清理过期数据

### dht_query.go - 查询操作

- `FindPeer(ctx, id)` - 查找特定节点
- `FindPeers(ctx, ids)` - 批量查找节点
- `FindClosestPeers(ctx, key, count)` - 查找最近节点
- `DiscoverPeers(ctx, namespace)` - 发现 namespace 下的节点
- `lookupPeers(ctx, key)` - 迭代 FIND_NODE

### dht_values.go - 值存储

- `PutValue(ctx, key, value)` - 存储值
- `PutValueWithTTL(ctx, key, value, ttl)` - 带 TTL 存储
- `GetValue(ctx, key)` - 获取值
- `iterativeFindValue(ctx, key)` - 迭代 FIND_VALUE
- `mergeCandidates(...)` - 合并候选节点

### dht_providers.go - Provider 管理

- `AddProvider(ctx, namespace)` - 注册为 Provider
- `GetProviders(ctx, namespace)` - 获取 Providers
- `Announce(ctx, namespace)` - 通告（实现 Announcer 接口）
- `StopAnnounce(namespace)` - 停止通告
- `ProviderInfo` / `providerEntry` / `ProviderRecord` 类型

### dht_peerrecord.go - PeerRecord

- `PublishPeerRecord(ctx, addrs)` - 发布签名的 PeerRecord
- `LookupPeerRecord(ctx, nodeID)` - 查询 PeerRecord
- `UpdateLocalAddrs(addrs)` - 更新本地地址
- `republishLocalAddrs()` - 重新发布地址
- `nextMonotonicSeqno()` - 单调递增序列号

### dht_address.go - 地址工具

- `stringAddress` 类型及其方法
- `IsPublic()` / `IsPrivate()` / `IsLoopback()` 等判断

### dht_routing.go - 路由表管理

- `AddNode(id, addrs, realmID)` - 添加节点
- `RemoveNode(id)` - 移除节点
- `UpdateNode(id, addrs, rtt)` - 更新节点
- `RecordFailure(id)` - 记录失败
- `RoutingTable()` - 获取路由表包装器

### dht_network.go - 网络/身份

- `SetNetwork(network)` - 注入网络层
- `GetNetwork()` - 获取网络层
- `SetIdentity(identity)` - 设置身份
- `GetIdentity()` - 获取身份
- `Mode()` / `SetMode(mode)` - 模式管理

## Kademlia 算法

### XOR 距离

```go
func distance(a, b types.NodeID) *big.Int {
    result := new(big.Int)
    for i := 0; i < 32; i++ {
        result.SetBit(result, i*8, uint(a[i]^b[i]))
    }
    return result
}
```

### 查找流程

```
1. 计算目标 NodeID 的 XOR 距离
2. 从路由表选择 α 个最近节点
3. 并行向这些节点发送 FIND_NODE
4. 将返回的更近节点加入候选列表
5. 重复直到没有更近的节点
```

### DHT 模式

```go
const (
    DHTModeAuto   DHTMode = iota  // 自动选择
    DHTModeServer                  // 服务器模式（完全参与）
    DHTModeClient                  // 客户端模式（仅查询）
)
```

## 配置

```go
type Config struct {
    Mode              DHTMode       // 运行模式
    BucketSize        int           // K-桶大小，默认 20
    Alpha             int           // 并行度，默认 3
    QueryTimeout      time.Duration // 查询超时，默认 30s
    RefreshInterval   time.Duration // 刷新间隔，默认 1h
    ReplicationFactor int           // 副本数，默认 3
    EnableValueStore  bool          // 启用值存储，默认 true
    MaxRecordAge      time.Duration // 记录最大存活时间，默认 24h
    BootstrapPeers    []PeerInfo    // 引导节点
}
```

## Key 格式

DHT 使用标准化的 Key 格式：

- `dep2p/v1/sys/peer/<NodeID>` - PeerRecord 地址记录
- `dep2p/v1/sys/<namespace>` - Provider 注册

所有 Key 在存储和距离计算时使用 SHA256 哈希，确保均匀分布。

## 使用示例

```go
// 创建 DHT
config := dht.DefaultConfig()
d := dht.NewDHT(config, nil, "my-realm")

// 启动
d.Start(ctx)
defer d.Stop()

// 存储值
d.PutValue(ctx, "my-key", []byte("my-value"))

// 获取值
value, err := d.GetValue(ctx, "my-key")

// 注册为 Provider
d.AddProvider(ctx, "my-service")

// 发现 Providers
providers, err := d.GetProviders(ctx, "my-service")
```
