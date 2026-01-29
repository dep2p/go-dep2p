# pkg/ 层设计原则

> 定义 DeP2P pkg/ 层各子包的定位和使用方式

---

## 概述

`pkg/` 层是 DeP2P 的**公共包**，对外暴露。但 pkg/ 下的不同子包有不同的定位和使用方式。

---

## pkg/ 层子包分类

### 1. 工具包（Utility Packages）

**直接调用，无需接口抽象**

| 包名 | 功能 | 使用方式 |
|------|------|----------|
| `pkg/types` | 公共类型定义 | 直接使用类型 |
| `pkg/crypto` | 密码学工具 | 直接调用函数 |
| `pkg/multiaddr` | 多地址解析 | 直接调用解析器 |
| `pkg/proto` | Protobuf 定义 | 直接使用生成的代码 |

**特点**：
- ✅ 无状态或轻量状态
- ✅ 纯函数或工厂模式
- ✅ 直接调用，类似标准库
- ✅ 不需要依赖注入
- ❌ 不需要在 `pkg/interfaces/` 中定义接口

**使用示例**：

```go
// pkg/types - 直接使用类型
import "github.com/dep2p/go-dep2p/pkg/types"

peerID := types.PeerID("12D3KooW...")
realmID := types.NewRealmID()

// pkg/crypto - 直接调用函数
import "github.com/dep2p/go-dep2p/pkg/crypto"

priv, pub, err := crypto.GenerateKeyPair(crypto.KeyTypeEd25519)
sig, err := crypto.Sign(priv, data)
```

---

### 2. 接口包（Interface Package）

**抽象系统组件，需要实现和注入**

| 包名 | 功能 | 使用方式 |
|------|------|----------|
| `pkg/interfaces` | 系统模块抽象接口 | 定义契约，internal/ 实现 |

**特点**：
- ✅ 定义系统组件的契约
- ✅ 供 `internal/` 层实现
- ✅ 允许模块替换和测试
- ✅ 使用依赖注入（Fx）

**使用示例**：

```go
// pkg/interfaces - 定义接口
import "github.com/dep2p/go-dep2p/pkg/interfaces"

type Host interface {
    ID() types.PeerID
    Connect(ctx context.Context, pi types.PeerInfo) error
    // ...
}

// internal/core/host - 实现接口
type hostImpl struct {
    id types.PeerID
    // ...
}

func (h *hostImpl) ID() types.PeerID {
    return h.id
}
```

---

## 设计原则对比

| 维度 | pkg/ 工具包 | pkg/interfaces |
|------|-------------|----------------|
| **定位** | 具体实现的工具 | 抽象系统契约 |
| **调用方式** | 直接调用 | 通过接口调用 |
| **依赖注入** | 不需要 | 需要（Fx） |
| **替换性** | 不需要替换 | 允许多实现 |
| **测试** | 直接测试 | Mock 接口 |
| **类比** | 标准库（crypto/ed25519） | 系统接口（io.Reader） |

---

## 实施流程差异

### pkg/ 工具包组件

```
Step 1: 设计审查         ✅ 执行
Step 2: 接口定义         ❌ 跳过（无需接口）
Step 3: 测试先行         ✅ 执行
Step 4: 核心实现         ✅ 执行（无 Fx 模块）
Step 5: 测试通过         ✅ 执行
Step 6: 集成验证         ✅ 执行
Step 7: 设计复盘         ✅ 执行
Step 8: 文档更新         ✅ 执行
```

### internal/ 组件（通过 pkg/interfaces）

```
Step 1: 设计审查         ✅ 执行
Step 2: 接口定义         ✅ 执行（定义 pkg/interfaces/）
Step 3: 测试先行         ✅ 执行
Step 4: 核心实现         ✅ 执行（含 Fx 模块）
Step 5: 测试通过         ✅ 执行
Step 6: 集成验证         ✅ 执行
Step 7: 设计复盘         ✅ 执行
Step 8: 文档更新         ✅ 执行
```

---

## 判断标准

**何时使用工具包模式（直接调用）**：
- ✅ 纯函数或工厂函数
- ✅ 无长期状态
- ✅ 不需要生命周期管理
- ✅ 不需要多实现

**何时使用接口模式（抽象组件）**：
- ✅ 有复杂状态管理
- ✅ 需要生命周期（Start/Stop）
- ✅ 可能有多种实现
- ✅ 需要 Mock 测试

---

## 反例说明

### ❌ 错误做法 1：为工具包定义接口

```go
// 错误：pkg/interfaces/crypto.go
type Crypto interface {
    GenerateKeyPair(KeyType) (PrivateKey, PublicKey, error)
    Sign(PrivateKey, []byte) ([]byte, error)
}

// 问题：
// - crypto 是纯工具函数，不需要抽象
// - 没有状态，不需要依赖注入
// - 类似标准库，直接调用即可
```

**正确做法**：

```go
// pkg/crypto - 直接提供函数
func GenerateKeyPair(kt KeyType) (PrivateKey, PublicKey, error) {
    // ...
}

func Sign(key PrivateKey, data []byte) ([]byte, error) {
    // ...
}
```

### ❌ 错误做法 2：工具包使用 Fx 模块

```go
// 错误：pkg/crypto/module.go
var Module = fx.Module("crypto",
    fx.Provide(NewCryptoService),
)

// 问题：
// - crypto 无需状态管理
// - 无需生命周期
// - 增加不必要的复杂度
```

---

## 相关文档

- [代码规范](code_standards.md)
- [接口设计](../../L4_interfaces/public_interfaces.md)

---

**最后更新**：2026-01-13
