# internal/app - 应用编排层

应用编排层负责组装和协调所有 dep2p 模块。

## 职责

1. **模块组装** - 按依赖顺序组装 fx 模块
2. **依赖注入** - 通过 fx 框架管理依赖关系
3. **生命周期管理** - 管理应用的启动和停止

## 文件结构

```
internal/app/
├── bootstrap.go    # 模块组装和 fx 应用创建
├── lifecycle.go    # 生命周期管理
├── options.go      # 内部选项
└── README.md       # 本文件
```

## 模块层级

```
Tier 0: Config
    └── config.Config

Tier 1: Foundation
    ├── identity.Module()
    └── address.Module()

Tier 2: Transport
    ├── transport.Module()
    ├── security.Module()
    └── muxer.Module()

Tier 3: Network Services
    ├── nat.Module()       (可选)
    ├── discovery.Module() (可选)
    └── relay.Module()     (可选)

Tier 3+: Application
    ├── protocol.Module()
    ├── connmgr.Module()
    └── messaging.Module()

Tier 4: Aggregation
    └── endpoint.Module()
```

## 使用方式

```go
// 创建配置
cfg := config.NewConfig()

// 创建 Bootstrap
bootstrap := app.NewBootstrap(cfg)

// 构建（不启动）
endpoint, err := bootstrap.Build()

// 或者构建并启动
endpoint, err := bootstrap.Start(ctx)

// 停止
err := bootstrap.Stop(ctx)
```

## 与其他包的关系

```
pkg/dep2p/            用户 API 入口
    ↓
internal/app/         应用编排层 (本包)
    ↓
internal/config/      配置管理层
    ↓
internal/core/        组件实现层
    ↓
pkg/interfaces/       接口契约层
```

