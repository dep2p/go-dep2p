# pkg_interfaces 编码指南

> **版本**: v1.1.0  
> **更新日期**: 2026-01-13

---

## 接口设计规范

### 1. 接口命名

**规范**：
- 使用名词或动词+er
- 描述接口的核心职责
- 避免 Interface 后缀

**示例**：
```go
// ✅ 好：清晰的接口命名
type Host interface { ... }
type Discovery interface { ... }
type Discoverer interface { ... }
type Advertiser interface { ... }

// ❌ 坏：模糊的命名
type HostInterface interface { ... }  // 多余的 Interface 后缀
type Manager interface { ... }        // 过于通用
type Helper interface { ... }         // 不清楚的职责
```

---

### 2. 方法签名

**规范**：
- context.Context 作为第一个参数（如果有）
- 使用命名返回值（对于复杂返回）
- 错误作为最后一个返回值

**示例**：
```go
// ✅ 好：规范的方法签名
type Discovery interface {
    FindPeers(ctx context.Context, ns string, opts ...DiscoveryOption) (<-chan PeerInfo, error)
    Advertise(ctx context.Context, ns string, opts ...DiscoveryOption) (ttl time.Duration, err error)
}

// ❌ 坏：不规范的签名
type Discovery interface {
    FindPeers(ns string) (error, <-chan PeerInfo)  // ❌ 错误不在最后
    Advertise(opts ...DiscoveryOption, ctx context.Context) error  // ❌ ctx 不在第一个
}
```

---

### 3. 选项模式

**规范**：
- 使用函数选项模式
- 选项函数命名 With*
- 选项结构私有或公开视情况而定

**模板**：
```go
// 选项函数类型
type XXXOption func(*XXXOptions)

// 选项结构
type XXXOptions struct {
    Field1 string
    Field2 int
}

// 选项函数
func WithField1(val string) XXXOption {
    return func(o *XXXOptions) {
        o.Field1 = val
    }
}

// 接口使用
type XXX interface {
    Method(ctx context.Context, opts ...XXXOption) error
}
```

---

### 4. 错误定义

**规范**：
- 使用 var 定义错误常量
- 错误名以 Err 开头
- 错误信息小写，无标点

**示例**：
```go
// ✅ 好：规范的错误定义
var (
    ErrNotFound = errors.New("peer not found")
    ErrAlreadyConnected = errors.New("already connected")
    ErrTimeout = errors.New("operation timed out")
)

// ❌ 坏：不规范的错误
var NotFoundError = errors.New("Peer not found.")  // ❌ 命名和格式
const ErrNotFound = "not found"  // ❌ 应该用 var + errors.New
```

---

### 5. 接口组合

**规范**：
- 优先使用接口组合（嵌入）
- 避免重复方法定义

**示例**：
```go
// ✅ 好：接口组合
type Discoverer interface {
    FindPeers(ctx context.Context, ns string, opts ...DiscoveryOption) (<-chan PeerInfo, error)
}

type Advertiser interface {
    Advertise(ctx context.Context, ns string, opts ...DiscoveryOption) (time.Duration, error)
}

type Discovery interface {
    Discoverer
    Advertiser
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
}

// ❌ 坏：重复定义
type Discovery interface {
    FindPeers(ctx context.Context, ns string, opts ...DiscoveryOption) (<-chan PeerInfo, error)
    Advertise(ctx context.Context, ns string, opts ...DiscoveryOption) (time.Duration, error)
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
}
```

---

### 6. 注释规范

**规范**：
- 接口注释描述用途和职责
- 方法注释说明行为和副作用
- 参数和返回值注释详细

**模板**：
```go
// XXX 定义 xxx 功能的接口
//
// XXX 提供 xxx 服务，负责：
// - 职责1
// - 职责2
//
// 使用示例：
//
//	xxx := NewXXX(...)
//	result, err := xxx.Method(ctx, ...)
type XXX interface {
    // Method 方法说明
    //
    // 详细说明方法的行为和副作用。
    //
    // 参数：
    //   - ctx: 上下文
    //   - param1: 参数说明
    //
    // 返回值：
    //   - result: 结果说明
    //   - error: 错误说明（可能的错误：ErrXXX, ErrYYY）
    Method(ctx context.Context, param1 string) (result string, error)
}
```

**示例**：
```go
// Discovery 定义发现服务接口
//
// Discovery 协调多种发现机制（DHT、mDNS、Bootstrap等），
// 提供统一的节点发现和广播接口。
type Discovery interface {
    // FindPeers 发现节点
    //
    // 在指定命名空间（ns）中发现节点。返回的通道会持续输出发现的节点，
    // 直到上下文取消或达到限制。
    //
    // 参数：
    //   - ctx: 上下文（用于取消）
    //   - ns: 命名空间（如 "rendezvous/my-app"）
    //   - opts: 发现选项（可选）
    //
    // 返回值：
    //   - <-chan PeerInfo: 发现的节点通道
    //   - error: 可能的错误（ErrDiscoveryFailed）
    FindPeers(ctx context.Context, ns string, opts ...DiscoveryOption) (<-chan PeerInfo, error)
}
```

---

### 7. 文件组织

**规范**：
- 相关接口定义在同一文件
- 文件名与主要接口名对应
- 每个文件包含接口、选项、错误定义

**示例**：
```
pkg/interfaces/
├── identity.go        # Identity, PublicKey, PrivateKey
├── host.go            # Host, Stream, Connection, StreamHandler
├── discovery.go       # Discovery, DHT, Discoverer, Advertiser
├── messaging.go       # Messaging, Request, Response
├── realm.go           # Realm, RealmManager, RealmConfig
└── ...
```

---

### 8. 依赖管理

**规范**：
- 只依赖 pkg/types
- 不依赖 pkg/proto（wire format）
- 不依赖 internal/（实现层）
- 不依赖第三方库（除标准库）

**检查**：
```bash
# 验证依赖
go list -deps ./pkg/interfaces/... | grep -v "std" | grep -v "github.com/dep2p/go-dep2p/pkg"
# 应该为空或只有 types
```

**示例**：
```go
// ✅ 好：只依赖 pkg/types
package interfaces

import (
    "context"
    "github.com/dep2p/go-dep2p/pkg/types"
)

type Host interface {
    ID() types.PeerID  // 使用 pkg/types
}

// ❌ 坏：依赖实现层
import "github.com/dep2p/go-dep2p/internal/core/host"  // ❌ 禁止

// ❌ 坏：依赖 proto
import "github.com/dep2p/go-dep2p/pkg/proto/identify"  // ❌ 接口不依赖 wire format
```

---

### 9. 接口粒度

**规范**：
- 单一职责（SRP）
- 7±2 个方法（经验法则）
- 可通过组合扩展

**示例**：
```go
// ✅ 好：职责单一，方法适中
type Messaging interface {
    Send(ctx context.Context, peerID string, protocol string, data []byte) ([]byte, error)
    SendAsync(ctx context.Context, peerID string, protocol string, data []byte) (<-chan *Response, error)
    RegisterHandler(protocol string, handler MessageHandler) error
    UnregisterHandler(protocol string) error
    Close() error
}  // 5 个方法，职责清晰

// ❌ 坏：职责过多
type SuperInterface interface {
    // ... 30+ 个方法
}

// ✅ 好：通过组合扩展
type FullDiscovery interface {
    Discovery
    DHT
    Bootstrap
}
```

---

### 10. 生命周期方法

**规范**：
- 有状态的接口需要 Close() 方法
- 服务接口需要 Start/Stop 方法
- 遵循 io.Closer 接口

**示例**：
```go
// ✅ 好：完整的生命周期
type Discovery interface {
    FindPeers(...) (<-chan PeerInfo, error)
    Advertise(...) (time.Duration, error)
    
    Start(ctx context.Context) error
    Stop(ctx context.Context) error
}

// ✅ 好：实现 io.Closer
type PubSub interface {
    Join(...) (Topic, error)
    Close() error  // 符合 io.Closer
}
```

---

## 相关文档

- [../design/overview.md](../design/overview.md) - 设计概述
- [../testing/strategy.md](../testing/strategy.md) - 测试策略

---

**最后更新**：2026-01-13
