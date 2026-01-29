# 测试隔离 (Isolation)

> 测试环境隔离、测试夹具、资源管理

---

## 目录结构

```
isolation/
├── README.md              # 本文件
└── fixtures/              # 测试夹具
    └── README.md
```

---

## 概述

测试隔离确保测试之间相互独立，不会产生副作用或相互干扰。本目录定义测试隔离策略和测试夹具管理方法。

---

## 隔离原则

| 原则 | 说明 |
|------|------|
| **独立性** | 每个测试独立运行，不依赖其他测试 |
| **可重复性** | 测试结果可重复，不受环境影响 |
| **无副作用** | 测试不留下持久化状态 |
| **并行安全** | 测试可并行执行 |

---

## 隔离策略

### 1. 网络隔离

```go
// 使用随机端口避免冲突
func newTestListener(t *testing.T) net.Listener {
    t.Helper()
    l, err := net.Listen("tcp", "127.0.0.1:0")
    require.NoError(t, err)
    t.Cleanup(func() { l.Close() })
    return l
}
```

### 2. 文件系统隔离

```go
// 使用临时目录
func newTestDir(t *testing.T) string {
    t.Helper()
    dir := t.TempDir()  // 自动清理
    return dir
}
```

### 3. 身份隔离

```go
// 每个测试生成独立身份
func newTestIdentity(t *testing.T) *Identity {
    t.Helper()
    id, err := NewIdentity()
    require.NoError(t, err)
    return id
}
```

### 4. Realm 隔离

```go
// 每个测试使用独立 Realm
func newTestRealm(t *testing.T) *Realm {
    t.Helper()
    name := fmt.Sprintf("test-realm-%s", t.Name())
    realm, err := NewRealm(name)
    require.NoError(t, err)
    return realm
}
```

---

## 资源管理

### 自动清理

```go
func TestExample(t *testing.T) {
    // 创建资源
    node := createTestNode(t)
    
    // 使用 t.Cleanup 确保清理
    t.Cleanup(func() {
        node.Close()
    })
    
    // 测试逻辑
    // ...
}
```

### 超时控制

```go
func TestWithTimeout(t *testing.T) {
    ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
    defer cancel()
    
    // 使用 ctx 执行操作
    // ...
}
```

### 并行测试

```go
func TestParallel(t *testing.T) {
    t.Parallel()  // 标记为可并行
    
    // 确保使用隔离的资源
    node := createTestNode(t)
    defer node.Close()
    
    // 测试逻辑
}
```

---

## 测试辅助函数

### 通用辅助函数

```go
// testutil/helpers.go

// WaitFor 等待条件满足
func WaitFor(t *testing.T, condition func() bool, timeout time.Duration) {
    t.Helper()
    deadline := time.Now().Add(timeout)
    for time.Now().Before(deadline) {
        if condition() {
            return
        }
        time.Sleep(10 * time.Millisecond)
    }
    t.Fatal("condition not met within timeout")
}

// RequireEventually 断言最终成功
func RequireEventually(t *testing.T, check func() error, timeout time.Duration) {
    t.Helper()
    deadline := time.Now().Add(timeout)
    var lastErr error
    for time.Now().Before(deadline) {
        if err := check(); err == nil {
            return
        } else {
            lastErr = err
        }
        time.Sleep(10 * time.Millisecond)
    }
    t.Fatalf("check failed: %v", lastErr)
}
```

### 网络辅助函数

```go
// FreePort 获取空闲端口
func FreePort(t *testing.T) int {
    t.Helper()
    l, err := net.Listen("tcp", "127.0.0.1:0")
    require.NoError(t, err)
    defer l.Close()
    return l.Addr().(*net.TCPAddr).Port
}

// WaitForPort 等待端口可用
func WaitForPort(t *testing.T, port int, timeout time.Duration) {
    t.Helper()
    WaitFor(t, func() bool {
        conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), time.Second)
        if err == nil {
            conn.Close()
            return true
        }
        return false
    }, timeout)
}
```

---

## 常见问题

### 端口冲突

**问题**：多个测试使用相同端口

**解决**：始终使用 `:0` 让系统分配端口

```go
// 错误
listener, _ := net.Listen("tcp", ":8080")

// 正确
listener, _ := net.Listen("tcp", ":0")
port := listener.Addr().(*net.TCPAddr).Port
```

### 资源泄漏

**问题**：测试后资源未释放

**解决**：使用 `t.Cleanup()` 确保清理

```go
func TestExample(t *testing.T) {
    resource := acquireResource()
    t.Cleanup(func() {
        resource.Release()
    })
}
```

### 测试顺序依赖

**问题**：测试依赖特定执行顺序

**解决**：确保每个测试完全独立

```go
// 错误：共享状态
var sharedCounter int

// 正确：每个测试独立
func TestA(t *testing.T) {
    counter := 0
    // ...
}
```

---

## 快速链接

| 文档 | 说明 |
|------|------|
| [fixtures/](fixtures/) | 测试夹具管理 |

---

**最后更新**：2026-01-11
