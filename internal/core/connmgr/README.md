# 连接管理模块 (ConnMgr)

## 概述

连接管理模块提供 P2P 节点的连接生命周期管理，包括连接数量控制、连接保护、黑名单机制和抖动容错。

## 架构

```
┌─────────────────────────────────────────────────────────────────────┐
│                         ConnMgr 模块                                 │
├─────────────────────────────────────────────────────────────────────┤
│                                                                      │
│  ┌──────────────────────────┐    ┌──────────────────────────┐       │
│  │   ConnectionManager      │    │   ConnectionGater        │       │
│  ├──────────────────────────┤    ├──────────────────────────┤       │
│  │ - 连接数量控制           │    │ - Peer 黑名单            │       │
│  │ - 水位线机制             │    │ - IP 地址黑名单          │       │
│  │ - 连接保护               │    │ - 子网黑名单             │       │
│  │ - 智能裁剪               │    │ - 连接拦截               │       │
│  └──────────────────────────┘    └──────────────────────────┘       │
│                                           │                          │
│  ┌──────────────────────────┐            ▼                          │
│  │   JitterTolerance        │   ┌──────────────────┐               │
│  ├──────────────────────────┤   │   GaterStore     │               │
│  │ - 短暂断连容错           │   │  (持久化存储)    │               │
│  │ - 状态保持时间           │   └──────────────────┘               │
│  │ - 指数退避重连           │                                       │
│  └──────────────────────────┘                                       │
│                                                                      │
└─────────────────────────────────────────────────────────────────────┘
```

## 文件结构

```
internal/core/connmgr/
├── module.go       # fx 模块定义
├── manager.go      # ConnectionManager 实现
├── filter.go       # 连接过滤和门控集成
├── trim.go         # 裁剪逻辑
├── protect.go      # 连接保护
├── gater.go        # ConnectionGater 实现
├── store.go        # 持久化存储实现
├── jitter.go       # 抖动容错实现
├── README.md       # 本文档
├── gater_test.go   # 门控单元测试
└── jitter_test.go  # 抖动容错单元测试
```

## 核心功能

### 1. ConnectionManager - 连接管理器

管理节点连接的生命周期：

| 功能 | 状态 | 说明 |
|------|------|------|
| 水位线控制 | ✅ 已实现 | 使用 Low/High/Emergency 三级水位线 |
| 连接保护 | ✅ 已实现 | 通过 Tag 保护重要连接不被裁剪 |
| 智能裁剪 | ✅ 已实现 | 优先裁剪空闲、非保护的连接 |
| 连接过滤 | ✅ 已实现 | 支持自定义过滤器 |

### 2. ConnectionGater - 黑名单机制

提供主动连接控制能力：

| 功能 | 状态 | 说明 |
|------|------|------|
| Peer 黑名单 | ✅ 已实现 | 阻止特定节点的连接 |
| IP 黑名单 | ✅ 已实现 | 阻止特定 IP 地址的连接 |
| 子网黑名单 | ✅ 已实现 | 阻止整个子网的连接 |
| 持久化 | ✅ 已实现 | 支持文件或内存存储 |

### 3. JitterTolerance - 抖动容错

处理网络短暂断连场景：

| 功能 | 状态 | 说明 |
|------|------|------|
| 状态保持 | ✅ 已实现 | 断连后保持连接状态一段时间 |
| 自动重连 | ✅ 已实现 | 支持指数退避重连策略 |
| 状态回调 | ✅ 已实现 | 断连/重连事件通知 |
| 超时清理 | ✅ 已实现 | 超过保持时间自动清理状态 |

## 使用示例

### 基本使用

```go
import (
    "github.com/dep2p/go-dep2p/internal/core/connmgr"
    connmgrif "github.com/dep2p/go-dep2p/pkg/interfaces/connmgr"
)

// 创建连接管理器
config := connmgrif.DefaultConfig()
manager := connmgr.NewConnectionManager(config, logger)

// 保护重要连接
manager.Protect(nodeID, "bootstrap")

// 检查连接数
count := manager.ConnCount()
```

### 黑名单管理

```go
// 创建连接门控
gaterConfig := connmgrif.DefaultGaterConfig()
gater, _ := connmgr.NewConnectionGater(gaterConfig, logger)

// 阻止恶意节点
gater.BlockPeer(maliciousPeerID)

// 阻止 IP 地址
gater.BlockAddr(net.ParseIP("192.168.1.100"))

// 阻止整个子网
_, subnet, _ := net.ParseCIDR("10.0.0.0/8")
gater.BlockSubnet(subnet)

// 检查节点是否被阻止
if gater.IsBlocked(nodeID) {
    // 拒绝连接
}
```

### 抖动容错

```go
// 创建抖动容错管理器
jitterConfig := connmgr.JitterConfig{
    HoldDuration:    30 * time.Second,  // 状态保持时间
    MaxRetries:      5,                  // 最大重试次数
    InitialBackoff:  100 * time.Millisecond,
    MaxBackoff:      10 * time.Second,
}
jitter := connmgr.NewJitterTolerance(jitterConfig, logger)

// 启动抖动容错
jitter.Start(ctx)

// 通知断连
jitter.NotifyDisconnected(peerID)

// 检查是否应该移除（超过保持时间）
if jitter.ShouldRemove(peerID) {
    // 执行清理
}

// 通知重连成功
jitter.NotifyReconnected(peerID)
```

### 使用持久化存储

```go
// 使用文件存储
store, _ := connmgr.NewFileGaterStore("/path/to/blocklist.json")

gaterConfig := connmgrif.DefaultGaterConfig()
gaterConfig.Store = store

gater, _ := connmgr.NewConnectionGater(gaterConfig, logger)
// 规则会自动持久化到文件
```

### 连接拦截

```go
// 在拨号前检查
if !gater.InterceptPeerDial(targetPeerID) {
    return errors.New("peer is blocked")
}

// 在接受连接时检查
if !gater.InterceptAccept(remoteAddr) {
    return errors.New("address is blocked")
}

// 在安全握手后检查
if !gater.InterceptSecured(direction, remotePeerID) {
    return errors.New("peer is blocked")
}
```

## 配置参数

### ConnectionManager 配置

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `LowWater` | 50 | 低水位线 |
| `HighWater` | 100 | 高水位线 |
| `EmergencyWater` | 150 | 紧急水位线 |
| `GracePeriod` | 1m | 新连接保护期 |
| `IdleTimeout` | 5m | 空闲超时 |
| `TrimInterval` | 1m | 裁剪检查间隔 |

### ConnectionGater 配置

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `Enabled` | true | 是否启用门控 |
| `AutoCloseBlocked` | false | 阻止时是否自动关闭连接 |
| `Store` | nil | 持久化存储 |

### JitterTolerance 配置

| 参数 | 默认值 | 说明 |
|------|--------|------|
| `HoldDuration` | 30s | 断连后状态保持时间 |
| `MaxRetries` | 5 | 最大重连尝试次数 |
| `InitialBackoff` | 100ms | 初始重连退避时间 |
| `MaxBackoff` | 10s | 最大重连退避时间 |
| `BackoffMultiplier` | 2.0 | 退避时间倍增系数 |

## 保护标签

预定义的保护标签：

| 标签 | 说明 |
|------|------|
| `bootstrap` | Bootstrap 节点 |
| `validator` | 验证者节点 |
| `relay` | 中继节点 |
| `dht` | DHT 邻居 |
| `mdns` | mDNS 发现的节点 |
| `active` | 活跃通信节点 |
| `persistent` | 持久连接 |

## 相关文档

- [connmgr 接口定义](../../../pkg/interfaces/connmgr/connmgr.go)
- [gater 接口定义](../../../pkg/interfaces/connmgr/gater.go)
