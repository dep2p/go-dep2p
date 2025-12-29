# internal/config - 配置管理层

配置管理层负责定义、校验和分发 dep2p 配置。

## 职责

1. **配置定义** - 定义详细的内部配置结构
2. **默认值管理** - 提供所有配置项的默认值
3. **配置校验** - 校验配置的有效性
4. **配置分发** - 通过 Provider 将配置分发给各组件

## 文件结构

```
internal/config/
├── config.go       # 配置结构定义
├── provider.go     # 配置提供者和 fx 模块
├── defaults.go     # 默认值常量
├── validator.go    # 配置校验
└── README.md       # 本文件
```

## 配置层次

```
用户配置 (pkg/dep2p.UserConfig)
    ↓
  转换
    ↓
内部配置 (internal/config.Config)
    ↓
  Provider
    ↓
组件配置 (*IdentityConfig, *TransportConfig, ...)
```

## 配置结构

```go
type Config struct {
    ListenAddrs       []string
    Identity          IdentityConfig
    Transport         TransportConfig
    Security          SecurityConfig
    Muxer             MuxerConfig
    NAT               NATConfig
    Discovery         DiscoveryConfig
    Relay             RelayConfig
    ConnectionManager ConnectionManagerConfig
    Protocol          ProtocolConfig
    Messaging         MessagingConfig
}
```

## Provider 使用

```go
// 创建配置
cfg := config.NewConfig()

// 创建 Provider
provider := config.NewProvider(cfg)

// 获取组件配置
identityCfg := provider.GetIdentity()
transportCfg := provider.GetTransport()
```

## fx 集成

```go
// fx 模块自动提供配置
config.Module()

// 组件可以注入具体配置
type ModuleInput struct {
    fx.In
    TransportConfig *config.TransportConfig `name:"transport_config"`
}
```

## 配置校验

```go
cfg := config.NewConfig()

if err := config.Validate(cfg); err != nil {
    // 处理校验错误
    fmt.Println(err)
}
```

