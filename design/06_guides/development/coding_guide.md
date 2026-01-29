# 编码指南

> Go 代码风格和最佳实践

---

## 1. 代码风格

### 1.1 格式化

使用 `gofmt` 和 `goimports`：

```bash
# 格式化
go fmt ./...

# 导入排序
goimports -w .
```

### 1.2 命名规范

| 类型 | 规范 | 示例 |
|------|------|------|
| 包名 | 小写单词 | `transport`, `identity` |
| 接口 | 动词或 -er | `Reader`, `Connector` |
| 结构体 | 名词 | `Node`, `Connection` |
| 方法 | 驼峰 | `Connect`, `SendMessage` |
| 常量 | 驼峰或全大写 | `MaxConnections`, `DEFAULT_PORT` |
| 私有 | 小写开头 | `internalMethod` |

### 1.3 导入分组

```go
import (
    // 标准库
    "context"
    "fmt"

    // 第三方库
    "github.com/stretchr/testify/assert"

    // 本项目
    "github.com/dep2p/dep2p/pkg/types"
)
```

---

## 2. 错误处理

### 2.1 错误定义

```go
// 使用 errors.New 或 fmt.Errorf
var ErrConnectionClosed = errors.New("connection closed")

// 使用 %w 包装错误
func doSomething() error {
    if err := step1(); err != nil {
        return fmt.Errorf("step1 failed: %w", err)
    }
    return nil
}
```

### 2.2 错误检查

```go
// 检查特定错误
if errors.Is(err, ErrConnectionClosed) {
    // 处理
}

// 检查错误类型
var connErr *ConnectionError
if errors.As(err, &connErr) {
    // 处理
}
```

### 2.3 错误日志

```go
// 记录错误上下文
if err != nil {
    log.Error("operation failed",
        "operation", "connect",
        "addr", addr,
        "error", err,
    )
    return err
}
```

---

## 3. 并发编程

### 3.1 Goroutine 管理

```go
// 使用 errgroup 管理
g, ctx := errgroup.WithContext(ctx)

g.Go(func() error {
    return task1(ctx)
})

g.Go(func() error {
    return task2(ctx)
})

if err := g.Wait(); err != nil {
    return err
}
```

### 3.2 Channel 使用

```go
// 带缓冲 channel
ch := make(chan Event, 100)

// 使用 select 处理多个 channel
select {
case event := <-eventCh:
    handleEvent(event)
case <-ctx.Done():
    return ctx.Err()
case <-time.After(timeout):
    return ErrTimeout
}
```

### 3.3 锁使用

```go
type SafeMap struct {
    mu   sync.RWMutex
    data map[string]interface{}
}

func (m *SafeMap) Get(key string) (interface{}, bool) {
    m.mu.RLock()
    defer m.mu.RUnlock()
    v, ok := m.data[key]
    return v, ok
}

func (m *SafeMap) Set(key string, value interface{}) {
    m.mu.Lock()
    defer m.mu.Unlock()
    m.data[key] = value
}
```

---

## 4. 接口设计

### 4.1 小接口

```go
// 好：小而专注的接口
type Reader interface {
    Read(p []byte) (n int, err error)
}

type Writer interface {
    Write(p []byte) (n int, err error)
}

// 组合接口
type ReadWriter interface {
    Reader
    Writer
}
```

### 4.2 接口位置

```go
// 接口应定义在使用者的包中
// 而不是实现者的包中

// pkg/interfaces/transport.go
type Transport interface {
    Listen(addr string) (Listener, error)
    Dial(addr string) (Conn, error)
}
```

---

## 5. 资源管理

### 5.1 Close 模式

```go
type Resource struct {
    closed int32
    // ...
}

func (r *Resource) Close() error {
    if !atomic.CompareAndSwapInt32(&r.closed, 0, 1) {
        return nil // 已关闭
    }
    // 清理资源
    return nil
}

func (r *Resource) isClosed() bool {
    return atomic.LoadInt32(&r.closed) == 1
}
```

### 5.2 Context 使用

```go
// 操作应接受 context
func (n *Node) Connect(ctx context.Context, addr string) error {
    // 检查取消
    select {
    case <-ctx.Done():
        return ctx.Err()
    default:
    }
    
    // 传递 context
    conn, err := n.transport.DialContext(ctx, addr)
    // ...
}
```

---

## 6. 日志规范

### 6.1 日志级别

| 级别 | 使用场景 |
|------|----------|
| Debug | 调试信息 |
| Info | 正常操作 |
| Warn | 异常但可恢复 |
| Error | 错误 |

### 6.2 结构化日志

```go
// 使用结构化日志
log.Info("connection established",
    "remote_id", remoteID,
    "addr", addr,
    "latency", latency,
)

// 避免字符串拼接
// 不好
log.Info("connection to " + addr + " established")
```

---

## 7. 注释规范

### 7.1 包注释

```go
// Package transport provides network transport implementations
// for DeP2P, including TCP, QUIC, and WebSocket.
package transport
```

### 7.2 函数注释

```go
// Connect establishes a connection to the specified address.
// It returns an error if the connection cannot be established
// within the context deadline.
//
// Example:
//
//	conn, err := node.Connect(ctx, "/ip4/127.0.0.1/tcp/8080")
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer conn.Close()
func (n *Node) Connect(ctx context.Context, addr string) (*Conn, error)
```

---

## 8. 代码组织

### 8.1 文件结构

```
module/
├── module.go       # 主要类型和接口
├── options.go      # 配置选项
├── errors.go       # 错误定义
├── helpers.go      # 辅助函数
├── module_test.go  # 测试
└── doc.go          # 包文档
```

### 8.2 函数长度

- 函数应保持简短（< 50 行）
- 复杂逻辑拆分为小函数
- 每个函数只做一件事

---

## 9. Lint 配置

项目使用 golangci-lint，配置见 `.golangci.yml`。

常见检查：
- `errcheck`: 错误检查
- `govet`: 静态分析
- `staticcheck`: 高级静态分析
- `gosec`: 安全检查

---

**最后更新**：2026-01-11
