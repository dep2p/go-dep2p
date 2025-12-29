# internal/config/components - 组件配置

本目录包含各组件的配置选项结构和转换逻辑。

## 与 config.Config 的关系

`config.Config` 是完整的内部配置结构，包含所有组件的配置。
`components/` 下的 `*Options` 结构是从 `config.Config` 提取的组件级配置，
便于组件直接使用。

```
config.Config
├── Identity    → components.IdentityOptions
├── Transport   → components.TransportOptions
├── Security    → components.SecurityOptions
├── Muxer       → components.MuxerOptions
├── NAT         → components.NATOptions
├── Discovery   → components.DiscoveryOptions
├── Relay       → components.RelayOptions
├── ConnMgr     → components.ConnMgrOptions
└── Messaging   → components.MessagingOptions
```

## 文件列表

| 文件 | 描述 |
|------|------|
| identity.go | 身份选项 |
| transport.go | 传输选项 |
| security.go | 安全选项 |
| muxer.go | 多路复用选项 |
| nat.go | NAT 穿透选项 |
| discovery.go | 发现服务选项 |
| relay.go | 中继选项 |
| connmgr.go | 连接管理选项 |
| messaging.go | 消息服务选项 |

## 使用方式

```go
// 从完整配置创建组件选项
cfg := config.NewConfig()
transportOpts := components.NewTransportOptions(&cfg.Transport)

// 或使用默认值
transportOpts := components.DefaultTransportOptions()
```

## 设计原则

1. **简化使用**: Options 结构比 Config 更简单，直接暴露组件需要的参数
2. **类型安全**: 每个组件有自己的选项类型，避免混淆
3. **便捷方法**: 提供 `Default*Options()` 和辅助方法

