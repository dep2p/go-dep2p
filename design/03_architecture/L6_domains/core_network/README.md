# Network 模块设计

> Core Layer - 网络接口监控

## 概述

network 模块提供网络接口监控和变化检测功能，支持跨平台的网络事件订阅。

## 核心功能

1. **网络接口监控** - 监控本地网络接口状态
2. **网络变化检测** - 检测网络配置变化（Major/Minor）
3. **平台适配** - Unix/Windows/Stub 实现
4. **事件订阅** - 订阅网络变化事件

## 代码位置

```
internal/core/network/
├── doc.go              # 包文档
├── monitor.go          # 网络监控器
├── monitor_unix.go     # Unix 实现
├── monitor_windows.go  # Windows 实现
├── monitor_stub.go     # Stub 实现
└── handler.go          # 变化处理器
```

## 公共接口

```
pkg/interfaces/network.go
```

## 依赖

- 无外部依赖
- 被 NetMon、Recovery 使用
