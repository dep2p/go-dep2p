# pkg_interfaces 测试策略

> **版本**: v1.1.0  
> **更新日期**: 2026-01-13

---

## 测试目标

| 目标 | 指标 | 状态 |
|------|------|------|
| 接口契约验证 | 所有接口 | ✅ 完成 |
| Mock 实现 | 主要接口 | ✅ 9 个 |
| 测试通过率 | 100% | ✅ 完成 |
| 编译验证 | 无错误 | ✅ 完成 |

**说明**：pkg/interfaces 是纯接口定义，代码覆盖率为 0% 是正常的。重点是验证接口契约和提供 Mock 实现。

---

## 测试分类

### 1. 接口契约测试

**目标**：验证接口定义存在且可实现

```go
func TestHostInterface(t *testing.T) {
    // 编译时验证：MockHost 实现了 Host 接口
    var _ interfaces.Host = (*MockHost)(nil)
}
```

**覆盖**：
- ✅ 所有主要接口都有契约测试
- ✅ 验证 Mock 实现符合接口

---

### 2. Mock 实现测试

**目标**：验证 Mock 实现可用于测试

```go
// Mock 实现
type MockHost struct {
    id string
}

func NewMockHost(id string) *MockHost {
    return &MockHost{id: id}
}

func (m *MockHost) ID() string {
    return m.id
}

// Mock 使用测试
func TestHost_ID(t *testing.T) {
    host := NewMockHost("test-id")
    
    if host.ID() != "test-id" {
        t.Errorf("ID() = %v, want test-id", host.ID())
    }
}
```

**覆盖**：
- ✅ 9 个主要接口的 Mock 实现
- ✅ Mock 方法行为测试

---

### 3. 接口方法签名测试

**目标**：验证方法签名正确

```go
func TestDiscovery_MethodSignatures(t *testing.T) {
    disco := NewMockDiscovery()
    
    // 验证 FindPeers 返回通道
    peers, err := disco.FindPeers(context.Background(), "test-ns")
    if err != nil {
        t.Errorf("FindPeers() failed: %v", err)
    }
    
    if peers == nil {
        t.Error("FindPeers() returned nil channel")
    }
}
```

---

## Mock 实现规范

### Mock 命名

**规范**：Mock + 接口名

```go
// ✅ 好
type MockHost struct { ... }
type MockDiscovery struct { ... }
type MockRealm struct { ... }

// ❌ 坏
type FakeHost struct { ... }
type TestHost struct { ... }
```

### Mock 构造函数

**规范**：提供 NewMock* 构造函数

```go
func NewMockHost(id string) *MockHost {
    return &MockHost{
        id:    id,
        addrs: []string{"/ip4/127.0.0.1/tcp/4001"},
    }
}
```

### Mock 方法实现

**规范**：
- 实现所有接口方法
- 返回合理的默认值
- 可配置行为（通过字段）

```go
type MockHost struct {
    id      string
    addrs   []string
    streams map[string]Stream
    
    // 可配置的行为
    ConnectFunc func(ctx context.Context, peerID string, addrs []string) error
}

func (m *MockHost) Connect(ctx context.Context, peerID string, addrs []string) error {
    if m.ConnectFunc != nil {
        return m.ConnectFunc(ctx, peerID, addrs)
    }
    return nil  // 默认行为
}
```

---

## 测试用例设计

### 接口存在性测试

```go
// 验证接口可以被实现
func TestXXXInterface(t *testing.T) {
    var _ interfaces.XXX = (*MockXXX)(nil)
}
```

### 方法行为测试

```go
// 验证 Mock 方法行为
func TestXXX_Method(t *testing.T) {
    mock := NewMockXXX()
    
    result, err := mock.Method(ctx, param)
    if err != nil {
        t.Errorf("Method() failed: %v", err)
    }
    
    if result != expected {
        t.Errorf("Method() = %v, want %v", result, expected)
    }
}
```

### 生命周期测试

```go
// 验证启动/停止
func TestXXX_Lifecycle(t *testing.T) {
    mock := NewMockXXX()
    
    err := mock.Start(context.Background())
    if err != nil {
        t.Errorf("Start() failed: %v", err)
    }
    
    err = mock.Stop(context.Background())
    if err != nil {
        t.Errorf("Stop() failed: %v", err)
    }
    
    err = mock.Close()
    if err != nil {
        t.Errorf("Close() failed: %v", err)
    }
}
```

---

## 测试文件组织

### 文件位置

**规范**：
- 测试文件与接口文件同目录
- 文件名：`<interface>_test.go`
- 包名：`interfaces_test`（外部测试）

**示例**：
```
pkg/interfaces/
├── host.go
├── host_test.go          # 测试 host.go
├── discovery.go
├── discovery_test.go     # 测试 discovery.go
└── ...
```

### 测试包名

```go
// ✅ 好：外部测试包
package interfaces_test

import "github.com/dep2p/go-dep2p/pkg/interfaces"

// ❌ 坏：内部测试包
package interfaces  // 不推荐
```

---

## 测试执行

### CI/CD 流程

```bash
# 接口契约验证
go test ./pkg/interfaces/...

# 详细测试输出
go test -v ./pkg/interfaces/...

# 编译验证
go build ./pkg/interfaces/...
```

### 本地测试

```bash
# 快速测试
go test ./pkg/interfaces/...

# 详细输出
go test -v ./pkg/interfaces/... | grep -E "(PASS|FAIL|===)"

# 验证 Mock 实现
go test -run TestMock ./pkg/interfaces/...
```

---

## 当前测试覆盖

### 测试统计

| 类别 | 数量 |
|------|------|
| 测试文件 | 9 |
| 测试用例 | 42 |
| Mock 实现 | 9 |

### Mock 实现列表

| Mock | 接口 | 状态 |
|------|------|------|
| MockIdentity | Identity | ✅ |
| MockPublicKey | PublicKey | ✅ |
| MockPrivateKey | PrivateKey | ✅ |
| MockHost | Host | ✅ |
| MockStream | Stream | ✅ |
| MockNode | Node | ✅ |
| MockDiscovery | Discovery | ✅ |
| MockMessaging | Messaging | ✅ |
| MockPubSub | PubSub | ✅ |
| MockTopic | Topic | ✅ |
| MockRealm | Realm | ✅ |
| MockPeerstore | Peerstore | ✅ |
| MockConnMgr | ConnManager | ✅ |
| MockTransport | Transport | ✅ |

---

## 未来优化

### 可选的测试增强

1. **接口行为规范测试**（Behavioral Tests）
   - 定义接口行为要求
   - 所有实现都应通过相同的行为测试

2. **接口兼容性测试**
   - 验证 Mock 与实际实现的行为一致性

3. **性能基准测试**
   - 对比不同实现的性能

**优先级**：P3（未来优化）

---

## 相关文档

- [../coding/guidelines.md](../coding/guidelines.md) - 编码指南
- [../design/overview.md](../design/overview.md) - 设计概述

---

**最后更新**：2026-01-13
