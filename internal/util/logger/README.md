# Logger 日志工具

## 概述

基于标准库 `log/slog` 的统一日志系统，支持按子系统配置日志级别、环境变量配置、结构化日志。

## 目录结构

```
logger/
├── README.md        # 本文件
├── logger.go        # 核心 API：Logger(), GlobalLogger(), SetLevel()
├── config.go        # 环境变量解析：ConfigFromEnv()
└── handler.go       # slog Handler 实现
```

## 快速开始

```go
package discovery

import "github.com/dep2p/go-dep2p/internal/util/logger"

// 获取子系统 Logger（推荐在包级别声明）
var log = logger.Logger("discovery")

func foo() {
    log.Info("peer discovered", "peer", peerID, "count", len(peers))
    log.Debug("connection details", "addr", addr, "latency", latency)
    log.Error("connection failed", "err", err, "peer", peerID)
}
```

## 环境变量配置

### DEP2P_LOG_LEVEL

设置日志级别，支持按子系统配置。

格式: `子系统=级别,子系统=级别,默认级别`

```bash
# 所有模块使用 info 级别
export DEP2P_LOG_LEVEL=info

# discovery 使用 debug，其他使用 info
export DEP2P_LOG_LEVEL=discovery=debug,info

# 多个模块自定义级别
export DEP2P_LOG_LEVEL=discovery=debug,transport=warn,nat=debug,info
```

支持的级别：
- `debug` - 调试信息
- `info` - 一般信息（默认）
- `warn` / `warning` - 警告
- `error` - 错误

### DEP2P_LOG_FORMAT

设置日志输出格式。

```bash
# 文本格式（默认）
export DEP2P_LOG_FORMAT=text

# JSON 格式（适合日志收集系统）
export DEP2P_LOG_FORMAT=json
```

### DEP2P_LOG_ADD_SOURCE

是否在日志中添加源码位置。

```bash
# 启用（默认）
export DEP2P_LOG_ADD_SOURCE=true

# 禁用
export DEP2P_LOG_ADD_SOURCE=false
```

## API 参考

### Logger(subsystem string) *slog.Logger

获取指定子系统的 Logger。同一子系统多次调用返回相同实例。

```go
var log = logger.Logger("discovery")
```

### GlobalLogger() *slog.Logger

返回全局 Logger，用于不属于特定子系统的日志。

```go
log := logger.GlobalLogger()
```

### SetLevel(subsystem string, level slog.Level)

动态设置子系统的日志级别（运行时调整）。

```go
logger.SetLevel("discovery", slog.LevelDebug)
```

### Discard() *slog.Logger

返回丢弃所有日志的 Logger，用于测试。

```go
log := logger.Discard()
```

### With(subsystem string, args ...any) *slog.Logger

创建带有预设属性的 Logger。

```go
log := logger.With("discovery", "peer", peerID)
log.Info("connected")  // 自动包含 peer 属性
```

## 日志输出示例

### 文本格式

```
ts=2025-12-22T10:00:00.000Z level=info subsystem=discovery msg="peer discovered" peer=5Q2STWvB... count=5
ts=2025-12-22T10:00:01.000Z level=debug subsystem=transport msg="connection established" addr=/ip4/192.168.1.1/tcp/4001
ts=2025-12-22T10:00:02.000Z level=error subsystem=nat msg="port mapping failed" err="timeout"
```

### JSON 格式

```json
{"ts":"2025-12-22T10:00:00.000Z","level":"info","subsystem":"discovery","msg":"peer discovered","peer":"5Q2STWvB...","count":5}
```

## 迁移指南（从 zap）

### 迁移前

```go
type Service struct {
    logger *zap.Logger
}

func NewService(logger *zap.Logger) *Service {
    if logger == nil {
        logger = zap.NewNop()  // 问题：日志被静默
    }
    return &Service{logger: logger}
}

func (s *Service) foo() {
    s.logger.Info("message",
        zap.String("key", value),
        zap.Error(err),
    )
}
```

### 迁移后

```go
import "github.com/dep2p/go-dep2p/internal/util/logger"

var log = logger.Logger("service")

type Service struct {
    // 不再需要 logger 字段
}

func NewService() *Service {
    return &Service{}
}

func (s *Service) foo() {
    log.Info("message",
        "key", value,
        "err", err,
    )
}
```

### 主要变化

1. 移除结构体中的 `logger` 字段
2. 使用包级别的 `var log = logger.Logger("subsystem")`
3. 日志调用简化：
   - `zap.String("k", v)` → `"k", v`
   - `zap.Error(err)` → `"err", err`
   - `zap.Int("n", 1)` → `"n", 1`

## 测试中使用

```go
func TestFoo(t *testing.T) {
    // 方式 1：使用 Discard
    log := logger.Discard()
    
    // 方式 2：使用标准库的 DiscardHandler
    log := slog.New(slog.DiscardHandler{})
}
```

## 与 go-libp2p 的兼容性

本日志系统的设计参考了 go-libp2p 的 `gologshim` 包：
- 都使用标准库 `log/slog`
- 环境变量格式类似（`DEP2P_LOG_LEVEL` vs `GOLOG_LOG_LEVEL`）
- 支持按子系统配置日志级别
