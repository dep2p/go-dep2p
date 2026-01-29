# Relay API 参考

**最后更新**：2025-12-31  
**版本**：v2.0/v2.1

---

## 1. 配置 API

### 1.1 WithRelayMap

配置 RelayMap（v2.0/v2.1 必选）

```go
func WithRelayMap(relayMap *relayif.RelayMap) Option
```

**参数**：
- `relayMap`: RelayMap 配置对象

**示例**：
```go
relayMap := &relayif.RelayMap{
    Version: "2025.1",
    Entries: []relayif.RelayMapEntry{
        {
            NodeID:   relay1ID,
            Addrs:    []string{"/ip4/1.2.3.4/udp/4001/quic-v1"},
            Region:   "AS",
            AuthMode: relayif.AuthModePublic,
        },
        {
            NodeID:   relay2ID,
            Addrs:    []string{"/ip4/5.6.7.8/udp/4001/quic-v1"},
            Region:   "NA",
            AuthMode: relayif.AuthModePublic,
        },
    },
}

node, err := dep2p.New(
    dep2p.WithRelayMap(relayMap),
)
```

### 1.2 WithGeoIP（v2.1）

配置 GeoIP 模块用于区域感知选路

```go
func WithGeoIP(config *geoipif.Config) Option
```

**参数**：
- `config`: GeoIP 配置

**示例**：
```go
geoipConfig := &geoipif.Config{
    Enabled: true,
    DBPath:  "/path/to/GeoLite2-City.mmdb",
    CacheSize: 1000,
}

node, err := dep2p.New(
    dep2p.WithRelayMap(relayMap),
    dep2p.WithGeoIP(geoipConfig),
)
```

---

## 2. RelayMap 数据结构

### 2.1 RelayMap

```go
type RelayMap struct {
    Entries []RelayMapEntry `json:"entries"`  // 至少 2 个
    Version string          `json:"version,omitempty"`
}
```

**方法**：

- `Validate() error` - 验证配置有效性

### 2.2 RelayMapEntry

```go
type RelayMapEntry struct {
    NodeID   types.NodeID   `json:"node_id"`           // 必选
    Addrs    []string       `json:"addrs"`             // 必选，至少一个
    Region   string         `json:"region,omitempty"`   // v2.1：区域标识
    Weight   int            `json:"weight,omitempty"`   // 负载均衡权重
    RealmID  types.RealmID  `json:"realm_id,omitempty"` // v2.1：安全域
    AuthMode AuthMode       `json:"auth_mode,omitempty"` // v2.1：认证模式
}
```

**字段说明**：

| 字段 | 类型 | 必选 | 说明 |
|------|------|------|------|
| `NodeID` | `types.NodeID` | ✅ | Relay 服务器的节点 ID |
| `Addrs` | `[]string` | ✅ | Relay 服务器的可拨号地址（multiaddr） |
| `Region` | `string` | ❌ | 地理区域标识（如 "AS", "NA", "EU"） |
| `Weight` | `int` | ❌ | 负载均衡权重（默认 100） |
| `RealmID` | `types.RealmID` | ❌ | 关联的 Realm ID（仅 realm_psk 时有效） |
| `AuthMode` | `AuthMode` | ❌ | 认证模式（"public" 或 "realm_psk"） |

**AuthMode 常量**：

```go
const (
    AuthModePublic  AuthMode = "public"    // 公共 Relay
    AuthModeRealmPSK AuthMode = "realm_psk" // 私域 Relay
)
```

---

## 3. RelayManager API

### 3.1 获取 RelayManager

```go
rm := node.RelayManager()
if rm == nil {
    // Relay 客户端未启用
    return
}
```

### 3.2 状态查询

#### HomeRelay

获取当前 Home Relay

```go
func (rm *RelayManager) HomeRelay() *relayif.ActiveRelay
```

**返回**：当前 Home Relay，如果未建立则返回 `nil`

**示例**：
```go
home := rm.HomeRelay()
if home != nil {
    fmt.Printf("Home Relay: %s, Latency: %v\n",
        home.Entry.NodeID.ShortString(),
        home.Latency)
}
```

#### BackupRelay

获取当前 Backup Relay

```go
func (rm *RelayManager) BackupRelay() *relayif.ActiveRelay
```

**返回**：当前 Backup Relay，如果未建立则返回 `nil`

#### Status

获取 RelayManager 完整状态

```go
func (rm *RelayManager) Status() *relayif.RelayManagerStatus
```

**返回**：包含 Home Relay、Backup Relay、健康状态等信息

**示例**：
```go
status := rm.Status()
fmt.Printf("Home: %s, Backup: %s, Health: %s\n",
    status.HomeRelay.Entry.NodeID.ShortString(),
    status.BackupRelay.Entry.NodeID.ShortString(),
    status.HealthStatus)
```

### 3.3 v2.1 增强 API

#### GetRelayForContext

根据连接上下文获取合适的 Relay（v2.1）

```go
func (rm *RelayManager) GetRelayForContext(realmID types.RealmID) *relayif.ActiveRelay
```

**参数**：
- `realmID`: 目标连接的 RealmID（空字符串表示系统/非 Realm 连接）

**返回**：合适的 Relay，没有可用时返回 `nil`

**选路规则**：
- 系统连接（`realmID == ""`）：只返回 `public` Relay
- Realm 连接（`realmID != ""`）：只返回同 RealmID 的 `realm_psk` Relay

**示例**：
```go
// 系统连接
systemRelay := rm.GetRelayForContext("")
if systemRelay == nil {
    log.Error("no public relay available")
}

// Realm 连接
realmRelay := rm.GetRelayForContext("my-realm")
if realmRelay == nil {
    log.Error("no realm relay available for my-realm")
}
```

#### GetCandidates

获取当前候选池（v2.1）

```go
func (rm *RelayManager) GetCandidates() []*namespace.RelayCandidate
```

**返回**：当前候选池中的所有 Relay 候选

**示例**：
```go
candidates := rm.GetCandidates()
for _, c := range candidates {
    fmt.Printf("候选: %s, 来源: %s, 区域: %s, AuthMode: %s\n",
        c.NodeID.ShortString(),
        c.Source,      // "config" 或 "dht"
        c.Region,
        c.AuthMode)
}
```

#### SetAllowRealmFallbackToPublic

设置是否允许 Realm 连接回退到 public Relay（v2.1）

```go
func (rm *RelayManager) SetAllowRealmFallbackToPublic(allow bool)
```

**参数**：
- `allow`: 是否允许回退（默认 `false`，强隔离）

**注意**：启用回退可能影响安全性，请谨慎使用

---

## 4. ActiveRelay 数据结构

```go
type ActiveRelay struct {
    Entry       RelayMapEntry
    Reservation Reservation
    Latency     time.Duration
    LastProbe   time.Time
    FailCount   int
}
```

**字段说明**：

| 字段 | 类型 | 说明 |
|------|------|------|
| `Entry` | `RelayMapEntry` | Relay 配置信息 |
| `Reservation` | `Reservation` | Reservation 对象 |
| `Latency` | `time.Duration` | 当前延迟 |
| `LastProbe` | `time.Time` | 最后探测时间 |
| `FailCount` | `int` | 连续失败次数 |

---

## 5. RelayManagerStatus 数据结构

```go
type RelayManagerStatus struct {
    HomeRelay   *ActiveRelay
    BackupRelay *ActiveRelay
    HealthStatus map[types.NodeID]HealthStatus
    LatencyStats map[types.NodeID]*LatencyStats
}
```

**字段说明**：

| 字段 | 类型 | 说明 |
|------|------|------|
| `HomeRelay` | `*ActiveRelay` | 当前 Home Relay |
| `BackupRelay` | `*ActiveRelay` | 当前 Backup Relay |
| `HealthStatus` | `map[types.NodeID]HealthStatus` | 各 Relay 的健康状态 |
| `LatencyStats` | `map[types.NodeID]*LatencyStats` | 各 Relay 的延迟统计 |

---

## 6. HealthStatus 枚举

```go
type HealthStatus int

const (
    HealthUnknown HealthStatus = iota
    HealthGood                 // RTT < 200ms, 无失败
    HealthDegraded             // RTT 200-500ms 或有少量失败
    HealthBad                  // RTT > 500ms 或连续失败
    HealthUnreachable          // 完全不可达
)
```

---

## 7. RelayCandidate 数据结构（v2.1）

```go
type RelayCandidate struct {
    NodeID       types.NodeID
    Addrs        []string
    Region       string
    AuthMode     string
    RealmID      types.RealmID
    Source       string        // "config" / "dht" / "cache"
    DiscoveredAt time.Time
    ExpiresAt    time.Time
}
```

**方法**：

- `IsExpired() bool` - 检查是否已过期
- `Key() string` - 生成唯一 key（用于去重）

---

## 8. 错误处理

### 8.1 常见错误

| 错误 | 说明 | 处理方式 |
|------|------|----------|
| `ErrRelayMapEmpty` | RelayMap 为空 | 提供至少 2 个 Relay 条目 |
| `ErrAllRelayProbesFailed` | 所有 Relay 探测失败 | 检查网络连接和 Relay 地址 |
| `ErrRelayMapInvalid` | RelayMap 配置无效 | 检查 NodeID 和地址格式 |

### 8.2 错误示例

```go
node, err := dep2p.New(
    dep2p.WithRelayMap(relayMap),
)
if err != nil {
    if errors.Is(err, relayif.ErrRelayMapEmpty) {
        log.Fatal("RelayMap 必须包含至少 2 个 Relay")
    }
    if errors.Is(err, relayif.ErrAllRelayProbesFailed) {
        log.Fatal("所有 Relay 不可达，请检查网络连接")
    }
    log.Fatalf("启动失败: %v", err)
}
```

---

## 9. 完整示例

### 9.1 基础配置

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/dep2p/go-dep2p"
    relayif "github.com/dep2p/go-dep2p/pkg/interfaces/relay"
    "github.com/dep2p/go-dep2p/pkg/types"
)

func main() {
    ctx := context.Background()

    // 配置 RelayMap
    relayMap := &relayif.RelayMap{
        Version: "2025.1",
        Entries: []relayif.RelayMapEntry{
            {
                NodeID:   relay1ID,
                Addrs:    []string{"/ip4/1.2.3.4/udp/4001/quic-v1"},
                Region:   "AS",
                AuthMode: relayif.AuthModePublic,
            },
            {
                NodeID:   relay2ID,
                Addrs:    []string{"/ip4/5.6.7.8/udp/4001/quic-v1"},
                Region:   "NA",
                AuthMode: relayif.AuthModePublic,
            },
        },
    }

    // 创建节点
    node, err := dep2p.New(
        dep2p.WithRelayMap(relayMap),
    )
    if err != nil {
        log.Fatalf("启动失败: %v", err)
    }
    defer node.Close()

    // 访问 RelayManager
    rm := node.RelayManager()
    if rm == nil {
        log.Fatal("RelayManager 未启用")
    }

    // 查询状态
    status := rm.Status()
    fmt.Printf("Home Relay: %s, Latency: %v\n",
        status.HomeRelay.Entry.NodeID.ShortString(),
        status.HomeRelay.Latency)
}
```

### 9.2 v2.1 增强配置

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/dep2p/go-dep2p"
    geoipif "github.com/dep2p/go-dep2p/pkg/interfaces/geoip"
    relayif "github.com/dep2p/go-dep2p/pkg/interfaces/relay"
    "github.com/dep2p/go-dep2p/pkg/types"
)

func main() {
    ctx := context.Background()

    // 配置 RelayMap
    relayMap := &relayif.RelayMap{
        Version: "2025.1",
        Entries: []relayif.RelayMapEntry{
            // 公共 Relay
            {
                NodeID:   publicRelayID,
                Addrs:    []string{"/ip4/1.2.3.4/udp/4001/quic-v1"},
                Region:   "AS",
                AuthMode: relayif.AuthModePublic,
            },
            // 私域 Relay
            {
                NodeID:   realmRelayID,
                Addrs:    []string{"/ip4/10.0.0.1/udp/4001/quic-v1"},
                Region:   "AS",
                AuthMode: relayif.AuthModeRealmPSK,
                RealmID:  "my-realm",
            },
        },
    }

    // 配置 GeoIP
    geoipConfig := &geoipif.Config{
        Enabled: true,
        DBPath:  "/path/to/GeoLite2-City.mmdb",
    }

    // 创建节点
    node, err := dep2p.New(
        dep2p.WithRelayMap(relayMap),
        dep2p.WithGeoIP(geoipConfig),
    )
    if err != nil {
        log.Fatalf("启动失败: %v", err)
    }
    defer node.Close()

    // 访问 RelayManager
    rm := node.RelayManager()

    // 查看候选池
    candidates := rm.GetCandidates()
    fmt.Printf("候选池大小: %d\n", len(candidates))
    for _, c := range candidates {
        fmt.Printf("  - %s (来源: %s, 区域: %s)\n",
            c.NodeID.ShortString(), c.Source, c.Region)
    }

    // 获取连接用的 Relay
    systemRelay := rm.GetRelayForContext("")
    realmRelay := rm.GetRelayForContext("my-realm")

    fmt.Printf("系统连接 Relay: %s\n", systemRelay.Entry.NodeID.ShortString())
    fmt.Printf("Realm 连接 Relay: %s\n", realmRelay.Entry.NodeID.ShortString())
}
```

---

## 10. 参考文档

- [使用指南](../how-to/use-relay.md) - 如何配置和使用 Relay
- [架构设计](../../design/architecture/relay-v2.md) - 详细架构设计
- [概念文档](../concepts/relay-architecture.md) - Relay 架构概念

