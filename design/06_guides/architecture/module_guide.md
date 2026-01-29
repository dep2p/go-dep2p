# 模块开发指南

> 创建和开发新模块的指南

---

## 1. 模块结构

### 1.1 目录结构

```
internal/core/mymodule/
├── mymodule.go           # 主要实现
├── options.go            # 配置选项
├── errors.go             # 错误定义
├── types.go              # 类型定义
├── mymodule_test.go      # 单元测试
└── doc.go                # 包文档

pkg/interfaces/mymodule/
└── mymodule.go           # 公共接口
```

### 1.2 包文档

```go
// Package mymodule provides ...
//
// Example usage:
//
//	m := mymodule.New(opts...)
//	// use m
//
package mymodule
```

---

## 2. 接口设计

### 2.1 定义公共接口

```go
// pkg/interfaces/mymodule/mymodule.go

package mymodule

import "context"

// Service defines the main interface for mymodule.
type Service interface {
    // Start initializes and starts the service.
    Start(ctx context.Context) error
    
    // Stop gracefully stops the service.
    Stop() error
    
    // DoSomething performs the main operation.
    DoSomething(ctx context.Context, input Input) (Output, error)
}

// Input represents the input for DoSomething.
type Input struct {
    // ...
}

// Output represents the result of DoSomething.
type Output struct {
    // ...
}
```

### 2.2 内部接口

```go
// internal/core/mymodule/internal.go

package mymodule

// internalService is used internally.
type internalService interface {
    // internal methods
}
```

---

## 3. 实现模块

### 3.1 主结构体

```go
// internal/core/mymodule/mymodule.go

package mymodule

import (
    "context"
    "sync"
    "sync/atomic"
    
    iface "github.com/dep2p/dep2p/pkg/interfaces/mymodule"
)

// Ensure implementation
var _ iface.Service = (*Module)(nil)

// Module implements the mymodule.Service interface.
type Module struct {
    config  *Config
    
    mu      sync.RWMutex
    started int32
    
    // dependencies
    dep1 Dependency1
    dep2 Dependency2
    
    // internal state
    // ...
}

// New creates a new Module with the given options.
func New(opts ...Option) (*Module, error) {
    cfg := defaultConfig()
    for _, opt := range opts {
        opt(cfg)
    }
    
    if err := cfg.validate(); err != nil {
        return nil, err
    }
    
    return &Module{
        config: cfg,
    }, nil
}
```

### 3.2 生命周期方法

```go
// Start implements Service.Start.
func (m *Module) Start(ctx context.Context) error {
    if !atomic.CompareAndSwapInt32(&m.started, 0, 1) {
        return ErrAlreadyStarted
    }
    
    // 初始化逻辑
    // ...
    
    return nil
}

// Stop implements Service.Stop.
func (m *Module) Stop() error {
    if !atomic.CompareAndSwapInt32(&m.started, 1, 0) {
        return nil // 已停止
    }
    
    // 清理逻辑
    // ...
    
    return nil
}
```

### 3.3 业务方法

```go
// DoSomething implements Service.DoSomething.
func (m *Module) DoSomething(ctx context.Context, input iface.Input) (iface.Output, error) {
    // 检查状态
    if atomic.LoadInt32(&m.started) == 0 {
        return iface.Output{}, ErrNotStarted
    }
    
    // 检查 context
    select {
    case <-ctx.Done():
        return iface.Output{}, ctx.Err()
    default:
    }
    
    // 业务逻辑
    m.mu.RLock()
    defer m.mu.RUnlock()
    
    // ...
    
    return iface.Output{}, nil
}
```

---

## 4. 配置选项

### 4.1 Options 模式

```go
// internal/core/mymodule/options.go

package mymodule

// Config holds the configuration for Module.
type Config struct {
    MaxConnections int
    Timeout        time.Duration
    // ...
}

func defaultConfig() *Config {
    return &Config{
        MaxConnections: 100,
        Timeout:        30 * time.Second,
    }
}

func (c *Config) validate() error {
    if c.MaxConnections <= 0 {
        return errors.New("max connections must be positive")
    }
    return nil
}

// Option configures the Module.
type Option func(*Config)

// WithMaxConnections sets the maximum connections.
func WithMaxConnections(n int) Option {
    return func(c *Config) {
        c.MaxConnections = n
    }
}

// WithTimeout sets the timeout duration.
func WithTimeout(d time.Duration) Option {
    return func(c *Config) {
        c.Timeout = d
    }
}
```

---

## 5. 错误处理

### 5.1 错误定义

```go
// internal/core/mymodule/errors.go

package mymodule

import "errors"

var (
    // ErrNotStarted is returned when the module is not started.
    ErrNotStarted = errors.New("mymodule: not started")
    
    // ErrAlreadyStarted is returned when the module is already started.
    ErrAlreadyStarted = errors.New("mymodule: already started")
    
    // ErrInvalidInput is returned when the input is invalid.
    ErrInvalidInput = errors.New("mymodule: invalid input")
)

// Error represents a mymodule error with context.
type Error struct {
    Op  string // operation
    Err error  // underlying error
}

func (e *Error) Error() string {
    return fmt.Sprintf("mymodule: %s: %v", e.Op, e.Err)
}

func (e *Error) Unwrap() error {
    return e.Err
}
```

---

## 6. 依赖注入

### 6.1 使用 Fx

```go
// internal/core/mymodule/fx.go

package mymodule

import "go.uber.org/fx"

// Module provides mymodule dependencies.
var FxModule = fx.Module("mymodule",
    fx.Provide(New),
    fx.Invoke(registerHooks),
)

func registerHooks(lc fx.Lifecycle, m *Module) {
    lc.Append(fx.Hook{
        OnStart: func(ctx context.Context) error {
            return m.Start(ctx)
        },
        OnStop: func(ctx context.Context) error {
            return m.Stop()
        },
    })
}
```

---

## 7. 测试

### 7.1 单元测试

```go
// internal/core/mymodule/mymodule_test.go

package mymodule

import (
    "context"
    "testing"
    
    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
    m, err := New()
    require.NoError(t, err)
    assert.NotNil(t, m)
}

func TestModule_Start(t *testing.T) {
    m, _ := New()
    
    err := m.Start(context.Background())
    require.NoError(t, err)
    
    // 重复启动应返回错误
    err = m.Start(context.Background())
    assert.ErrorIs(t, err, ErrAlreadyStarted)
    
    require.NoError(t, m.Stop())
}

func TestModule_DoSomething(t *testing.T) {
    tests := []struct {
        name    string
        input   Input
        want    Output
        wantErr bool
    }{
        {"valid", Input{}, Output{}, false},
        {"invalid", Input{}, Output{}, true},
    }
    
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            m, _ := New()
            m.Start(context.Background())
            defer m.Stop()
            
            got, err := m.DoSomething(context.Background(), tt.input)
            if tt.wantErr {
                assert.Error(t, err)
            } else {
                assert.NoError(t, err)
                assert.Equal(t, tt.want, got)
            }
        })
    }
}
```

---

## 8. 文档

### 8.1 设计文档

在 `design/03_architecture/L6_domains/` 创建模块设计文档：

```
design/03_architecture/L6_domains/core_mymodule/
├── README.md
└── design.md
```

使用 `design/templates/arch_template.md` 模板。

---

## 9. 清单

开发新模块时检查以下事项：

- [ ] 定义公共接口 (`pkg/interfaces/`)
- [ ] 实现模块 (`internal/core/`)
- [ ] 配置选项 (Options 模式)
- [ ] 错误定义
- [ ] 单元测试 (≥ 80% 覆盖率)
- [ ] 集成测试
- [ ] 设计文档
- [ ] 代码评审

---

**最后更新**：2026-01-11
