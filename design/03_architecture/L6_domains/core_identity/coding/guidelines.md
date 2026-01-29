# core_identity 编码指南

> 代码规范与最佳实践

---

## 代码组织

```
internal/core/identity/
├── module.go           # Fx 模块，放在最前
├── service.go          # 主接口实现
├── keygen.go           # 密钥生成（内部）
├── nodeid.go           # NodeID 工具（内部）
├── errors.go           # 错误定义
└── service_test.go     # 测试
```

---

## 命名规范

### 类型命名

| 类型 | 命名规则 | 示例 |
|------|----------|------|
| 接口 | 名词或动词+er | `Service`, `Signer` |
| 实现 | 小写开头 | `service`, `keygen` |
| 错误 | Err 前缀 | `ErrInvalidKey` |

### 函数命名

| 函数类型 | 命名规则 | 示例 |
|----------|----------|------|
| 构造函数 | New 前缀 | `NewService` |
| 获取器 | 直接名词 | `ID()`, `PublicKey()` |
| 派生函数 | From/Derive 前缀 | `FromBytes()`, `DeriveNodeID()` |

---

## 错误处理

### 错误定义

```
// errors.go
package identity

import "errors"

var (
    ErrInvalidKeyLength = errors.New("identity: invalid key length")
    ErrSignatureFailed  = errors.New("identity: signature verification failed")
)
```

### 错误包装

```
func NewServiceFromBytes(raw []byte) (*service, error) {
    if len(raw) != ed25519.PrivateKeySize {
        return nil, fmt.Errorf("%w: expected %d, got %d",
            ErrInvalidKeyLength, ed25519.PrivateKeySize, len(raw))
    }
    // ...
}
```

---

## 并发模式

Identity 模块数据不可变，天然线程安全：

```
// 不需要锁
type service struct {
    privateKey ed25519.PrivateKey  // 不可变
    publicKey  ed25519.PublicKey   // 不可变
    nodeID     types.NodeID        // 不可变
}

// 所有方法都是只读的，可并发调用
func (s *service) ID() types.NodeID { return s.nodeID }
func (s *service) Sign(data []byte) ([]byte, error) { ... }
```

---

## 日志规范

```
import "log/slog"

// 使用结构化日志
slog.Info("identity created",
    "nodeID", s.nodeID.Pretty(),
)

// 不要记录私钥！
// BAD: slog.Info("key", "private", s.privateKey)
```

---

## 测试规范

```
func TestDeriveNodeID(t *testing.T) {
    // Arrange
    _, pub, _ := ed25519.GenerateKey(rand.Reader)
    
    // Act
    nodeID := DeriveNodeID(pub)
    
    // Assert
    assert.Equal(t, 32, len(nodeID))
}
```

---

**最后更新**：2026-01-11
